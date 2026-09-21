"""Apply/undo one audited dependency batch. No network, binaries or secret rotation."""
from __future__ import annotations

import argparse
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import uuid

from security_maintenance import EDITABLE_PATHS, GO_MODULES, linked


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def safe_path(root: Path, relative: str) -> Path:
    """Reject junctions as well as symlinks, traversal and Windows alternate streams."""
    if not isinstance(relative, str) or not relative or "\\" in relative or ":" in relative:
        raise ValueError("Chemin relatif invalide")
    parts = relative.split("/")
    if any(p in {"", ".", ".."} or p.endswith((" ", ".")) for p in parts):
        raise ValueError("Chemin relatif invalide")
    current = root
    if linked(root):
        raise ValueError("Racine liee interdite")
    for part in parts:
        current = current / part
        if os.path.lexists(current) and linked(current):
            raise ValueError("Lien/jonction interdit : " + relative)
    if not current.resolve().is_relative_to(root.resolve()):
        raise ValueError("Chemin hors racine")
    return current


def batch_path(root: Path, run_id: str) -> Path:
    if not re.fullmatch(r"\d{8}-\d{6}-[a-f0-9]{8,32}", run_id):
        raise ValueError("Identifiant de lot invalide")
    run = safe_path(root, "update/" + run_id)
    if not run.is_dir():
        raise ValueError("Lot inexistant")
    return run


def read_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8-sig"))


def atomic_write(path: Path, data: bytes):
    temporary = path.with_name("." + path.name + ".maintenance-" + uuid.uuid4().hex)
    try:
        with temporary.open("xb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        if path.exists():
            os.chmod(temporary, path.stat().st_mode & 0o777)
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


@contextmanager
def project_lock(root: Path):
    path = safe_path(root, "update/.apply.lock")
    # Deliberately do not remove a pre-existing lock: it may be a live process
    # or an interrupted transaction that requires inspection of its journal.
    try:
        stream = path.open("x", encoding="ascii")
    except FileExistsError:
        raise ValueError("Une application/restauration est deja active ou interrompue. Examiner update/.apply.lock.")
    try:
        with stream:
            stream.write(str(os.getpid()))
        yield
    finally:
        path.unlink()


def current_bytes(path: Path) -> bytes | None:
    return path.read_bytes() if path.exists() else None


def checked_changes(changes):
    if not isinstance(changes, list):
        raise ValueError("Liste de correctifs invalide")
    seen = set()
    for change in changes:
        name = change["path"]
        if name not in EDITABLE_PATHS or name in seen:
            raise ValueError("Fichier non autorise ou duplique : " + str(name))
        seen.add(name)
        if not re.fullmatch(r"[a-f0-9]{64}", change["after_sha256"]):
            raise ValueError("Empreinte de candidat invalide")
        before = change["before_sha256"]
        if before is None:
            if name not in {m + "/go.sum" for m in GO_MODULES}:
                raise ValueError("Creation de fichier non autorisee")
        elif not re.fullmatch(r"[a-f0-9]{64}", before):
            raise ValueError("Empreinte originale invalide")
    return changes


def validate_report(root: Path, run: Path):
    report = read_json(safe_path(run, "rapport.json"))
    if (report.get("schema_version") != 1 or report.get("mode") != "update"
            or report.get("status") != "CONTROLES_AUTOMATIQUES_TERMINES_RECETTE_REQUISE"
            or report.get("do_not_apply_batch") is not False
            or report.get("run_build_tests") is not True
            or report.get("errors") != []
            or Path(report.get("source", "")).resolve() != root.resolve()
            or Path(report.get("output", "")).resolve() != run.resolve()):
        raise ValueError("Lot non applicable : audit, tests, origine ou etat du rapport invalide")
    components = report.get("components")
    if not isinstance(components, list) or not components:
        raise ValueError("Audit vide")
    for component in components:
        if (component.get("after", {}).get("ok") is not True
                or component.get("after", {}).get("findings") != []
                or "ECHEC" in component.get("validation", "")
                or "ECHEC" in component.get("update", "")):
            raise ValueError("Audit incomplet ou en echec")
    return checked_changes(report["changed_files"])


def execute(root: Path, run_id: str, rollback=False, dry_run=False):
    root = root.absolute()
    run = batch_path(root, run_id)
    if dry_run:
        return transact(root, run, rollback, True)
    with project_lock(root):
        return transact(root, run, rollback, False)


def transact(root: Path, run: Path, rollback: bool, dry_run: bool):
    receipt_path = safe_path(run, "application/receipt.json")
    receipt = None
    if rollback:
        receipt = read_json(receipt_path)
        if receipt.get("state") != "applied" or receipt.get("source") != str(root.resolve()):
            raise ValueError("Ce lot n'est pas une application terminee de ce projet")
        changes = checked_changes(receipt["changes"])
    else:
        changes = validate_report(root, run)
        if os.path.lexists(safe_path(run, "application")):
            raise ValueError("Lot deja applique/tente : consulter application/receipt.json")
    operations = []
    for change in changes:
        relative = change["path"]
        destination = safe_path(root, relative)
        before = current_bytes(destination)
        expected = change["after_sha256"] if rollback else change["before_sha256"]
        if (None if before is None else sha(before)) != expected:
            raise ValueError("Source modifiee depuis le lot : " + relative)
        if rollback and change["before_sha256"] is None:
            after = None
        else:
            candidate = safe_path(run, ("application/before/" if rollback else "fichiers-corriges/") + relative)
            after = candidate.read_bytes()
            wanted = change["before_sha256"] if rollback else change["after_sha256"]
            if sha(after) != wanted:
                raise ValueError("Candidat/sauvegarde altere : " + relative)
        if not destination.parent.is_dir():
            raise ValueError("Dossier source absent : " + relative)
        operations.append((relative, destination, before, after))
    if dry_run or not operations:
        print(("SIMULATION : " if dry_run else "") + str(len(operations)) + " fichier(s), aucun remplacement effectue.")
        return
    if not rollback:
        folder = safe_path(run, "application")
        folder.mkdir()
        for relative, _, before, _ in operations:
            if before is not None:
                backup = safe_path(run, "application/before/" + relative)
                backup.parent.mkdir(parents=True, exist_ok=True)
                atomic_write(backup, before)
                if backup.read_bytes() != before:
                    raise ValueError("Sauvegarde non verifiee")
        receipt = {"source": str(root.resolve()), "changes": changes}
    receipt["state"] = "rollback_in_progress" if rollback else "applying"
    atomic_write(receipt_path, (json.dumps(receipt, indent=2) + "\n").encode())
    completed = []
    try:
        for relative, destination, before, after in operations:
            destination = safe_path(root, relative)
            if current_bytes(destination) != before:
                raise ValueError("Modification concurrente : " + relative)
            if after is None:
                destination.unlink()
            else:
                atomic_write(destination, after)
            completed.append((relative, destination, before, after))
    except BaseException:
        conflicts = []
        for relative, destination, before, after in reversed(completed):
            try:
                destination = safe_path(root, relative)
                if current_bytes(destination) != after:
                    raise ValueError("Modification concurrente pendant le retour arriere")
                if before is None:
                    destination.unlink()
                else:
                    atomic_write(destination, before)
            except Exception:
                conflicts.append(relative)
        receipt["state"] = "recovery_required" if conflicts else "applied" if rollback else "failed_restored"
        receipt["conflicts"] = conflicts
        atomic_write(receipt_path, (json.dumps(receipt, indent=2) + "\n").encode())
        raise
    receipt["state"] = "rolled_back" if rollback else "applied"
    atomic_write(receipt_path, (json.dumps(receipt, indent=2) + "\n").encode())
    print(receipt["state"] + " : " + str(len(operations)) + " fichier(s). Journal : " + str(receipt_path))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", type=Path, required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--rollback", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()
    execute(args.project_root, args.run_id, args.rollback, args.dry_run)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, TypeError) as error:
        print("Operation refusee : " + str(error), file=sys.stderr)
        raise SystemExit(1)
