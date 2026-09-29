#!/usr/bin/env python3
"""Back up the RelaisDesk SQLite database and invoice files, encrypted, to Google Drive.

An optional supervision heartbeat is pinged after each fully verified backup.
No package updates, VPN operations, API restarts or remote deletions. OAuth is
configured separately by the operator. Python's standard library is sufficient.
"""
import argparse
import configparser
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


ARCHIVE = re.compile(r"relaisdesk-\d{8}T\d{6}Z\.(db|invoices)\.aesgcm\Z")
DB_STAMP = re.compile(r"relaisdesk-(\d{8}T\d{6}Z)\.db\.aesgcm\Z")
DESTINATION = "relaisdesk-drive:RelaisDesk-Backups/julexogame"


def regular_file(path):
    return stat.S_ISREG(path.lstat().st_mode)


def check_config(path):
    if not path.exists():
        raise ValueError("Connexion Google absente : configurer le fichier rclone.conf avant activation.")
    if not regular_file(path):
        raise ValueError("rclone.conf doit être un fichier régulier, pas un lien.")
    if os.name == "posix":
        if path.stat().st_mode & 0o077 or path.parent.stat().st_mode & 0o077:
            raise ValueError("Permissions requises : dossier rclone 0700, fichier rclone.conf 0600.")
    cfg = configparser.ConfigParser(interpolation=None)
    try:
        cfg.read_string(path.read_text(encoding="utf-8"))
        remote = cfg["relaisdesk-drive"]
        if remote.get("type") != "drive" or remote.get("scope") != "drive.file":
            raise ValueError("Utiliser le remote relaisdesk-drive, type drive, avec le scope limité drive.file.")
        if not remote.get("client_id", "").strip() or not remote.get("client_secret", "").strip():
            raise ValueError("Un client OAuth Google propre à cette sauvegarde est requis.")
        token = json.loads(remote.get("token", "{}"))
        if not isinstance(token, dict) or not token.get("refresh_token"):
            raise ValueError("Autorisation Google incomplète : jeton de renouvellement absent.")
    except (configparser.Error, KeyError, json.JSONDecodeError, UnicodeError):
        # Never include parser diagnostics: they may quote OAuth credentials.
        raise ValueError("Configuration Google invalide ; ne pas publier son contenu.") from None


def archives(directory):
    result = []
    for path in sorted(directory.iterdir()):
        if not ARCHIVE.fullmatch(path.name):
            continue
        if not regular_file(path):
            raise ValueError("Archive non régulière refusée : " + path.name)
        with path.open("rb") as stream:
            if stream.read(8) != b"RDBKAES2":
                raise ValueError("Archive non chiffrée ou ancien format refusé : " + path.name)
        result.append(path)
    return result


def digest(path):
    hasher = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            hasher.update(chunk)
    return hasher.digest()


def check_heartbeat_url(url):
    value = (url or "").strip()
    if not value:
        return ""
    try:
        parsed = urllib.parse.urlparse(value)
    except ValueError:
        raise ValueError("Heartbeat : URL https complète requise.") from None
    if parsed.scheme != "https" or not parsed.hostname:
        raise ValueError("Heartbeat : URL https complète requise.")
    return value


def ping_heartbeat(url):
    # The URL is a bearer secret: never include it in messages or output.
    request = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            if not 200 <= response.status < 300:
                raise RuntimeError("Heartbeat : réponse inattendue du service de supervision.")
    except (urllib.error.URLError, OSError, ValueError) as error:
        raise RuntimeError("Heartbeat : ping impossible, vérifier la connectivité.") from None


def run_checked(command, stage):
    # Ignore inherited rclone overrides, including debug dumps and other remotes.
    environment = {k: v for k, v in os.environ.items()
                   if not k.startswith(("RCLONE_", "BACKUP_"))}
    try:
        result = subprocess.run([str(v) for v in command], env=environment,
                                stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=1800, check=False)
    except subprocess.TimeoutExpired:
        raise RuntimeError(stage + " : délai dépassé, aucune réussite enregistrée.") from None
    if result.returncode:
        raise RuntimeError(f"{stage} : échec (code {result.returncode}). "
                           "Vérifier les accès, l'espace libre et l'autorisation Google.")


def backup(args):
    if args.retention_days < 1 or args.retention_days > 36500:
        raise ValueError("Rétention locale invalide.")
    check_config(args.config)
    for path in (args.database, args.key, args.backup_binary, args.rclone, args.tar):
        if not regular_file(path):
            raise ValueError("Fichier nécessaire absent ou non régulier : " + str(path))
    with args.database.open("rb") as stream:
        if stream.read(16) != b"SQLite format 3\x00":
            raise ValueError("La source doit être une base SQLite existante, non vide.")
    if args.invoices_dir.is_symlink() or not args.invoices_dir.is_dir():
        raise ValueError("Le dossier des factures doit être un dossier existant, pas un lien.")
    args.heartbeat_url = check_heartbeat_url(args.heartbeat_url)
    if args.directory.is_symlink():
        raise ValueError("Le dossier local de sauvegarde ne doit pas être un lien.")
    args.directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    if os.name == "posix" and args.directory.stat().st_mode & 0o077:
        raise ValueError("Le dossier local de sauvegarde doit être privé (0700).")

    # flock coordinates direct invocations as well as the single systemd unit.
    import fcntl
    with (args.directory / ".google-drive.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("Une sauvegarde Google Drive est déjà en cours.") from None
        perform_backup(args)


def perform_backup(args):
    common = [args.rclone, "--config", args.config, "--log-level", "ERROR",
              "--contimeout", "15s", "--timeout", "2m", "--retries", "3",
              "--low-level-retries", "5", "--transfers", "1", "--checkers", "2",
              "--buffer-size", "4M", "--drive-skip-shortcuts"]
    with tempfile.TemporaryDirectory(prefix=".drive-", dir=args.directory) as temp:
        stage = Path(temp)
        snapshot = stage / "snapshot"
        run_checked([args.backup_binary, "create", "-db", args.database,
                     "-key-file", args.key, "-out-dir", snapshot,
                     "-mirror-dir", "", "-retention-days", "30"], "Création chiffrée")
        created = archives(snapshot)
        if len(created) != 1:
            raise RuntimeError("La création doit produire exactement une archive chiffrée.")
        latest = args.directory / created[0].name
        # Same filesystem, atomic publication, no overwrite even on name collision.
        os.link(created[0], latest)
        os.chmod(latest, 0o600)
        stamped = DB_STAMP.fullmatch(created[0].name)
        if stamped is None:
            raise RuntimeError("La création doit produire une archive chiffrée de base.")
        invoices_name = "relaisdesk-" + stamped.group(1) + ".invoices.aesgcm"
        invoices_tar = stage / "invoices.tar.gz"
        run_checked([args.tar, "--sort=name", "-czf", invoices_tar,
                     "-C", args.invoices_dir.parent, args.invoices_dir.name],
                    "Archive des factures")
        run_checked([args.backup_binary, "seal", "-in", invoices_tar,
                     "-out", stage / invoices_name,
                     "-key-file", args.key], "Chiffrement des factures")
        invoices_latest = args.directory / invoices_name
        os.link(stage / invoices_name, invoices_latest)
        os.chmod(invoices_latest, 0o600)
        pending = archives(args.directory)
        selected = stage / "files.txt"
        selected.write_text("".join(p.name + "\n" for p in pending), encoding="utf-8")
        # Explicit allowlist: no .db, .env, key, plaintext temporary file or symlink.
        # copy (never sync) preserves remote history; immutable rejects replacement.
        run_checked(common + ["copy", args.directory, DESTINATION, "--files-from-raw",
                              selected, "--checksum", "--immutable"], "Envoi Google Drive")
        downloaded = stage / "downloaded.db.aesgcm"
        run_checked(common + ["copyto", DESTINATION + "/" + latest.name,
                              downloaded], "Relecture Google Drive")
        if digest(latest) != digest(downloaded):
            raise RuntimeError("La copie relue depuis Google Drive diffère de l'archive locale.")
        run_checked([args.backup_binary, "verify", "-file", downloaded,
                     "-key-file", args.key], "Déchiffrement et intégrité SQLite de la copie distante")
        downloaded_invoices = stage / "downloaded.invoices.aesgcm"
        run_checked(common + ["copyto", DESTINATION + "/" + invoices_latest.name,
                              downloaded_invoices], "Relecture des factures")
        if digest(invoices_latest) != digest(downloaded_invoices):
            raise RuntimeError("La copie des factures relue depuis Google Drive diffère.")
        opened_tar = stage / "invoices-check.tar.gz"
        run_checked([args.backup_binary, "open", "-file", downloaded_invoices,
                     "-out", opened_tar, "-key-file", args.key],
                    "Déchiffrement des factures distantes")
        run_checked([args.tar, "-tzf", opened_tar], "Contrôle de l'archive des factures")
        # Local retention only AFTER all pending files were uploaded and the
        # latest remote copies were downloaded and verified. Never delete on Drive.
        cutoff = time.time() - args.retention_days * 86400
        for path in pending:
            if path != latest and path != invoices_latest and regular_file(path) \
                    and path.stat().st_mtime < cutoff:
                path.unlink()
        if args.heartbeat_url:
            ping_heartbeat(args.heartbeat_url)
        status = {"verified_at_utc": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                  "archive": latest.name, "destination": DESTINATION,
                  "sha256": digest(latest).hex(),
                  "invoices_archive": invoices_latest.name,
                  "invoices_sha256": digest(invoices_latest).hex()}
        marker = stage / "last-success.json"
        marker.write_text(json.dumps(status, indent=2) + "\n", encoding="utf-8")
        os.replace(marker, args.directory / "google-drive-last-success.json")
        print("Sauvegarde Google Drive relue, déchiffrée et vérifiée : " + latest.name)
        print("Factures chiffrées relues et vérifiées : " + invoices_latest.name)
        if args.heartbeat_url:
            print("Heartbeat de supervision envoyé.")
        else:
            print("Heartbeat désactivé : aucune alerte automatique en cas d'échec silencieux.")


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check-config", action="store_true",
                        help="Vérifier uniquement la configuration, sans sauvegarde ni accès Google")
    parser.add_argument("--config", type=Path, default=Path("/etc/relaisdesk/rclone/rclone.conf"))
    parser.add_argument("--database", type=Path, default=Path("/data/relaisdesk/licences.db"))
    parser.add_argument("--key", type=Path, default=Path("/etc/relaisdesk/backup.key"))
    parser.add_argument("--directory", type=Path, default=Path("/var/backups/relaisdesk"))
    parser.add_argument("--backup-binary", type=Path, default=Path("/opt/relaisdesk/api/relaisdesk-backup"))
    parser.add_argument("--rclone", type=Path, default=Path("/opt/relaisdesk/backup-tools/rclone"))
    parser.add_argument("--tar", type=Path, default=Path("/usr/bin/tar"))
    parser.add_argument("--invoices-dir", type=Path, default=Path("/data/relaisdesk/invoices"))
    parser.add_argument("--heartbeat-url", default=os.environ.get("BACKUP_HEARTBEAT_URL", ""),
                        help="URL https de heartbeat (Better Stack) pingée après succès vérifié")
    parser.add_argument("--retention-days", type=int, default=30)
    args = parser.parse_args()
    try:
        if args.check_config:
            check_config(args.config)
            print("Configuration locale cohérente ; l'accès Google reste à tester.")
        else:
            backup(args)
    except (OSError, ValueError, RuntimeError) as error:
        print("Erreur sauvegarde : " + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
