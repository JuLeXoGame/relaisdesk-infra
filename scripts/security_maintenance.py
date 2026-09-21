"""On-demand dependency maintenance in update/, never an in-place deployment."""
from __future__ import annotations

import argparse
import datetime as dt
import difflib
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import sys
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request
import uuid

SOURCE_DIRS = ("api", "database", "keygen", "dashboard_admin", "installer", "rustdesk",
               "rustdesk-server", "relaisdesk", "scripts", "tests")
SKIP_DIRS = {".git", ".svn", "node_modules", "target", ".dart_tool", ".flutter-plugins",
             "__pycache__", ".venv", "venv", ".gradle", ".idea", ".vs", "vcpkg_installed"}
SKIP_PREFIXES = ("relaisdesk/downloads", "api/data", "api/invoices",
                 "rustdesk/flutter/build", "rustdesk-server/ui/html/dist")
SECRET_NAMES = {"licenseadmin.txt", "dns token(important).txt", "id_ed25519", "id_rsa",
                "network-auth-ed25519", "release-signing-ed25519", ".npmrc", ".netrc", ".pypirc"}
SECRET_EXTENSIONS = {".ppk", ".pem", ".key", ".pfx", ".p12", ".kdbx", ".db", ".sqlite", ".sqlite3"}
ARTIFACT_EXTENSIONS = {".exe", ".dll", ".deb", ".dmg", ".msi", ".a", ".lib", ".so", ".o", ".obj", ".log"}
EMBEDDED_INPUTS = {f"installer/{role}/embedded/rustdesk.{extension}"
                   for role in ("configurator", "viewer") for extension in ("exe", "deb")}
EMBEDDED_INPUTS |= {f"installer/viewer/embedded/fleet/{name}"
                    for name in ("rustdesk.exe", "sciter.dll", "dylib_virtual_display.dll")}
GO_MODULES = ("database", "api", "tests", "keygen", "installer/configurator", "installer/viewer",
              "installer/debpack", "dashboard_admin")
RUST_MODULES = ("rustdesk", "rustdesk-server", "rustdesk-server/ui")
EDITABLE_PATHS = ({f"{module}/{name}" for module in GO_MODULES for name in ("go.mod", "go.sum")}
                  | {f"{module}/Cargo.lock" for module in RUST_MODULES}
                  | {"rustdesk-server/ui/html/package-lock.json", "rustdesk/flutter/pubspec.lock",
                     "rustdesk/libs/portable/requirements.txt"})
PRIVATE_MARKER = re.compile(rb"(?:-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|PuTTY-User-Key-File-)")
MANUAL_LIMITS = [
    "Code propre au projet (authentification, autorisations, concurrence, metier) : revue et tests humains necessaires ; aucun scanner de dependances ne prouve son absence de faille.",
    "GTK3, sodiumoxide et interface Sciter historique : dette de maintenance connue ; non-maintenance ne signifie pas automatiquement vulnerabilite exploitable. Migration manuelle si aucun correctif compatible n'existe.",
    "Correctifs locaux atty/GLib/PHF et dependances Git du fork : conserves. Une alerte sans version compatible exige une analyse et parfois un portage ; aucun changement de commit Git automatique.",
    "Codecs natifs, vcpkg, Sciter DLL, pilotes et SDK : non corriges automatiquement. Baselines et ABI demandent une compilation multi-plateforme et une verification specifique.",
    "Ubuntu, Docker, Nginx, pare-feu, TLS, secrets et serveur Oracle : hors du perimetre de ce script local ; aucune connexion SSH ni operation de deploiement.",
    "Executables deja construits/installeurs : NON corriges par une mise a jour des sources. Recompiler, tester, signer le manifeste et publier les sources correspondantes avant une livraison autorisee.",
    "Les entrees Go de niveau module peuvent concerner des paquets non importes. Elles sont conservees dans le rapport, sans affirmer qu'elles sont exploitables.",
    "Dart Git/path/SDK, CocoaPods, Gradle, les outils de compilation et les dependances Python autres que Brotli ne sont pas couverts par les corrections automatiques. Les bases publiques peuvent aussi ignorer une faille recente ou inconnue.",
]


def linked(path: Path) -> bool:
    info = path.lstat()
    return stat.S_ISLNK(info.st_mode) or bool(getattr(info, "st_file_attributes", 0) & 0x400)


def contained(path: Path, root: Path) -> Path:
    resolved = path.resolve()
    if not resolved.is_relative_to(root.resolve()):
        raise ValueError(f"Chemin hors du dossier de travail : {path.name}")
    return resolved


def digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def json_stream(text: str) -> list[dict]:
    decoder, position, values = json.JSONDecoder(), 0, []
    while position < len(text):
        while position < len(text) and text[position].isspace():
            position += 1
        if position == len(text):
            break
        value, position = decoder.raw_decode(text, position)
        if not isinstance(value, dict):
            raise ValueError("Objet JSON attendu")
        values.append(value)
    if not values:
        raise ValueError("Rapport JSON vide")
    return values


def snapshot(root: Path, destination: Path) -> tuple[dict, list]:
    inventory, excluded = {}, []
    for directory in SOURCE_DIRS:
        origin = root / directory
        if not origin.is_dir() or linked(origin):
            excluded.append({"path": directory, "reason": "absent ou lien"})
            continue
        for parent, dirs, files in os.walk(origin, followlinks=False):
            parent = Path(parent)
            kept = []
            for name in dirs:
                child = parent / name
                relative = child.relative_to(root).as_posix()
                if (name in SKIP_DIRS or relative in SKIP_PREFIXES or linked(child)
                    or relative.startswith("installer/build/")):
                    continue
                kept.append(name)
            dirs[:] = kept
            for name in files:
                source = parent / name
                relative = source.relative_to(root).as_posix()
                lower = name.lower()
                if relative.startswith("installer/build/") and relative != "installer/build/license.txt":
                    continue
                if (linked(source) or lower in SECRET_NAMES or lower == ".git"
                    or lower == ".env" or lower.startswith(".env.") or lower.endswith(".env")
                    or source.suffix.lower() in SECRET_EXTENSIONS
                    or lower.endswith(("-wal", "-shm"))
                    or re.match(r"(?:credentials|secrets?)[.-]", lower)):
                    excluded.append({"path": relative, "reason": "secret, donnee privee ou lien"})
                    continue
                if source.suffix.lower() in ARTIFACT_EXTENSIONS and relative not in EMBEDDED_INPUTS:
                    continue
                if not source.is_file():
                    continue
                with source.open("rb") as stream:
                    magic = stream.read(8)
                if magic.startswith((b"MZ", b"\x7fELF", b"!<arch>")) and relative not in EMBEDDED_INPUTS:
                    continue
                # No matching secret content is ever copied into a log.
                if source.stat().st_size <= 2 * 1024 * 1024 and PRIVATE_MARKER.search(source.read_bytes()):
                    excluded.append({"path": relative, "reason": "marqueur de cle privee"})
                    continue
                target = destination / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                before = digest(source)
                shutil.copy2(source, target)
                if digest(target) != before or digest(source) != before:
                    raise RuntimeError(f"Fichier modifie pendant la copie : {relative}")
                inventory[relative] = before
    return inventory, excluded


def find_tool(name: str, root: Path) -> str | None:
    explicit = {
        "go": [Path(os.environ.get("ProgramFiles", "C:/Program Files")) / "Go/bin/go.exe"],
        "node": [Path(os.environ.get("ProgramFiles", "C:/Program Files")) / "nodejs/node.exe"],
        "cargo": [Path.home() / ".cargo/bin/cargo.exe"],
        "govulncheck": [root / ".tools/govulncheck.exe", Path.home() / "go/bin/govulncheck.exe"],
        "cargo-audit": [root / ".tools/cargo/bin/cargo-audit.exe", Path.home() / ".cargo/bin/cargo-audit.exe"],
    }
    detected = shutil.which(name)
    if detected:
        return detected
    return next((str(p) for p in explicit.get(name, []) if p.is_file()), None)


def command_prefix(name: str, root: Path) -> list[str] | None:
    # Invoke npm through Node: never interpolate arguments into cmd.exe.
    if name == "npm":
        node = find_tool("node", root)
        if node:
            cli = Path(node).parent / "node_modules/npm/bin/npm-cli.js"
            if cli.is_file():
                return [node, str(cli)]
        npm = shutil.which("npm")
        if npm and Path(npm).suffix.lower() not in {".cmd", ".bat"}:
            return [npm]
        return None
    if name == "dart":
        dart = shutil.which("dart")
        flutter = shutil.which("flutter")
        if dart and Path(dart).suffix.lower() not in {".cmd", ".bat"}:
            return [dart]
        if flutter:
            binary = Path(flutter).parent / "cache/dart-sdk/bin" / ("dart.exe" if os.name == "nt" else "dart")
            if binary.is_file():
                return [str(binary)]
        return None
    binary = find_tool(name, root)
    return [binary] if binary else None


def finding(identifier, package, version, fixed=(), kind="vulnerability", url="", title="") -> dict:
    return {"id": identifier, "package": package, "version": version,
            "fixed_versions": list(fixed), "kind": kind, "url": url, "title": title or ""}


def rust_findings(report: dict) -> list[dict]:
    if not isinstance(report.get("vulnerabilities", {}).get("list"), list):
        raise ValueError("Schema cargo-audit inconnu")
    entries = []
    groups = [("vulnerability", report["vulnerabilities"]["list"])]
    groups += list(report.get("warnings", {}).items())
    for kind, items in groups:
        for item in items:
            advisory, package = item.get("advisory") or {}, item.get("package") or {}
            identifier = advisory.get("id") or f"{kind.upper()}-{package.get('name', '?')}-{package.get('version', '?')}"
            url = (advisory.get("url") or ("https://rustsec.org/advisories/" + identifier + ".html"
                   if identifier.startswith("RUSTSEC-") else "https://crates.io/crates/" + package.get("name", "")))
            entries.append(finding(identifier, package.get("name", "?"), package.get("version", "?"),
                                   (item.get("versions") or {}).get("patched", []), kind, url, advisory.get("title")))
    return entries


def go_findings(messages: list[dict]) -> list[dict]:
    if not any("config" in item for item in messages):
        raise ValueError("Configuration govulncheck absente")
    if any("error" in item for item in messages):
        raise ValueError("Erreur govulncheck dans le flux JSON")
    entries = {}
    advisories = {item["osv"]["id"]: item["osv"] for item in messages if isinstance(item.get("osv"), dict)}
    for item in messages:
        value = item.get("finding")
        if not value:
            continue
        trace = value.get("trace", [])
        frame = trace[0] if trace else {}
        identifier = value["osv"]
        kind = "go-symbol" if frame.get("function") else "go-package" if frame.get("package") else "go-module"
        entry = finding(identifier, frame.get("module", "?"), frame.get("version", "?"),
                        [value["fixed_version"]] if value.get("fixed_version") else [],
                        kind, "https://pkg.go.dev/vuln/" + identifier, advisories.get(identifier, {}).get("summary"))
        key = (entry["id"], entry["package"], entry["version"])
        level = {"go-module": 0, "go-package": 1, "go-symbol": 2}
        if key not in entries or level[kind] > level[entries[key]["kind"]]:
            entries[key] = entry
    return list(entries.values())


def npm_findings(report: dict) -> list[dict]:
    if "error" in report or not isinstance(report.get("vulnerabilities"), dict):
        raise ValueError("Rapport npm audit absent ou en erreur")
    entries = []
    for package, item in report["vulnerabilities"].items():
        for advisory in item.get("via", []):
            if not isinstance(advisory, dict):
                continue
            fix = item.get("fixAvailable")
            fixed = [fix["version"]] if isinstance(fix, dict) and fix.get("version") else []
            entries.append(finding(str(advisory.get("source", advisory.get("url", package))), package,
                                   item.get("range", "?"), fixed, advisory.get("severity", "vulnerability"), advisory.get("url", ""), advisory.get("title")))
    # A nonempty graph without advisory details must not become a clean result.
    if report["vulnerabilities"] and not entries:
        raise ValueError("npm signale des vulnerabilites sans details exploitables")
    return entries


def pub_packages(path: Path) -> tuple[list[tuple[str, str]], dict]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("#") and not text.startswith("packages:"):
        raise ValueError("Format pubspec.lock non reconnu")
    hosted, git = [], {}
    blocks = re.findall(r"^  ([a-zA-Z0-9_]+):\s*\n(.*?)(?=^  [a-zA-Z0-9_]+:\s*$|^sdks:|\Z)", text, re.M | re.S)
    if not blocks:
        raise ValueError("Aucun package Dart lu")
    for name, block in blocks:
        source = re.search(r"^    source: (\w+)\s*$", block, re.M)
        version = re.search(r'^    version: ["\']?([^\s"\']+)["\']?\s*$', block, re.M)
        if not source or not version:
            raise ValueError("Format Dart non pris en charge : " + name)
        if source[1] == "hosted":
            hosted.append((name, version[1]))
        elif source[1] == "git":
            git[name] = block
        elif source[1] not in {"path", "sdk"}:
            raise ValueError("Source Dart inconnue : " + name)
    return hosted, git


class Maintenance:
    def __init__(self, args):
        self.args = args
        self.root = Path(args.project_root).resolve()
        if not (self.root / "database/go.mod").is_file() or not (self.root / "rustdesk/Cargo.toml").is_file():
            raise ValueError("Racine RelaisDesk incorrecte")
        output = self.root / "update"
        if os.path.lexists(output) and (linked(output) or not output.is_dir()):
            raise ValueError("update doit etre un vrai dossier, jamais un lien")
        output.mkdir(exist_ok=True)
        run_id = getattr(args, "run_id", None) or (dt.datetime.now().strftime("%Y%m%d-%H%M%S-") + uuid.uuid4().hex[:8])
        if not re.fullmatch(r"\d{8}-\d{6}-[a-f0-9]{8,32}", run_id):
            raise ValueError("Identifiant d'execution invalide")
        self.run = output / run_id
        self.run.mkdir()
        self.project = self.run / "projet"
        self.project.mkdir()
        self.logs = self.run / "logs"
        self.logs.mkdir()
        self.tools = {name: command_prefix(name, self.root) for name in ("go", "cargo", "cargo-audit", "govulncheck", "node", "npm", "dart")}
        self.report = {"started_at": dt.datetime.now(dt.timezone.utc).isoformat(), "mode": "prepare" if args.prepare_only else "audit" if args.audit_only else "update",
                       "source": str(self.root), "output": str(self.run), "components": [], "commands": [], "errors": [], "limitations": MANUAL_LIMITS,
                       "automatic_deployment": False, "binary_rebuild": False,
                       "run_build_tests": bool(args.run_build_tests), "schema_version": 1}
        self.inventory = {}
        self.snapshot_complete = False
        self.environment = os.environ.copy()
        for name in list(self.environment):
            if re.search(r"TOKEN|PASSWORD|SECRET|API_KEY|PRIVATE_KEY|^SMTP_|^STRIPE_|^AWS_|^OCI_|^AZURE_|^GOFLAGS$|^GOWORK$|^RUSTC_WRAPPER$|^RUSTC_WORKSPACE_WRAPPER$|^NODE_OPTIONS$|^PYTHONPATH$|^PYTHONHOME$|^LD_PRELOAD$|^DYLD_|^GIT_CONFIG|^GIT_SSH|^RUSTFLAGS$|^CARGO_ENCODED_RUSTFLAGS$", name, re.I):
                self.environment.pop(name)
        self.environment.update({"GOWORK": "off", "GOTOOLCHAIN": "auto", "GIT_TERMINAL_PROMPT": "0", "GCM_INTERACTIVE": "Never",
                                 "CARGO_TERM_COLOR": "never", "NO_COLOR": "1", "npm_config_ignore_scripts": "true", "PYTHONDONTWRITEBYTECODE": "1"})
        # govulncheck invokes go itself; explicitly discovered tools must also be on PATH.
        tool_dirs = dict.fromkeys(str(Path(prefix[0]).parent) for prefix in self.tools.values() if prefix)
        self.environment["PATH"] = os.pathsep.join(tool_dirs) + os.pathsep + self.environment.get("PATH", "")

    def save_json(self, path: Path, value):
        contained(path, self.run)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

    def call(self, label, command, cwd, extra_env=None):
        contained(Path(cwd), self.run)
        number = len(self.report["commands"]) + 1
        basename = f"{number:03d}-" + re.sub(r"[^a-zA-Z0-9_-]", "-", label)
        stdout, stderr = self.logs / (basename + ".out"), self.logs / (basename + ".err")
        entry = {"label": label, "argv": command, "cwd": str(Path(cwd).relative_to(self.run)),
                 "stdout": str(stdout.relative_to(self.run)), "stderr": str(stderr.relative_to(self.run))}
        self.report["commands"].append(entry)
        print(f"[{number}] {label}", flush=True)
        environment = self.environment | (extra_env or {})
        start = time.monotonic()
        with stdout.open("wb") as out, stderr.open("wb") as err:
            options = {"creationflags": subprocess.CREATE_NO_WINDOW} if os.name == "nt" else {"start_new_session": True}
            try:
                process = subprocess.Popen(command, cwd=cwd, env=environment, stdout=out, stderr=err, stdin=subprocess.DEVNULL, shell=False, **options)
                while process.poll() is None:
                    try:
                        process.wait(timeout=min(25, self.args.timeout))
                    except subprocess.TimeoutExpired:
                        elapsed = time.monotonic() - start
                        if elapsed >= self.args.timeout:
                            self.stop_process(process)
                            entry["timeout"] = True
                            break
                        print(f"    en cours ({int(elapsed)} s), journal {stdout.name}", flush=True)
                    except KeyboardInterrupt:
                        self.stop_process(process)
                        raise
                entry["exit_code"] = process.wait(timeout=10)
                if entry.get("timeout"):
                    entry["exit_code"] = -124
            except OSError as error:
                entry["exit_code"] = -1
                err.write(str(error).encode("utf-8", errors="replace"))
        entry["duration_seconds"] = round(time.monotonic() - start, 2)
        return entry["exit_code"], stdout.read_text(encoding="utf-8", errors="replace")

    @staticmethod
    def stop_process(process):
        try:
            if os.name == "nt":
                taskkill = Path(os.environ.get("SystemRoot", "C:/Windows")) / "System32/taskkill.exe"
                subprocess.run([str(taskkill), "/PID", str(process.pid), "/T", "/F"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=False, timeout=10)
            else:
                os.killpg(process.pid, signal.SIGKILL)
        finally:
            if process.poll() is None:
                process.kill()

    def http_json(self, url, payload=None):
        data = json.dumps(payload).encode() if payload is not None else None
        request = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json", "User-Agent": "RelaisDesk-local-security-maintenance/1"})
        with urllib.request.urlopen(request, timeout=min(self.args.timeout, 45)) as response:
            raw = response.read(20 * 1024 * 1024 + 1)
            if len(raw) > 20 * 1024 * 1024:
                raise ValueError("Reponse de registre trop volumineuse")
            return json.loads(raw)

    def osv(self, ecosystem, packages):
        findings = []
        for start in range(0, len(packages), 50):
            batch = packages[start:start + 50]
            response = self.http_json("https://api.osv.dev/v1/querybatch", {"queries": [
                {"package": {"name": name, "ecosystem": ecosystem}, "version": version} for name, version in batch]})
            results = response.get("results")
            if not isinstance(results, list) or len(results) != len(batch):
                raise ValueError("Reponse OSV incomplete")
            for (name, version), item in zip(batch, results):
                # Never silently treat the first page as the complete audit.
                if item.get("next_page_token"):
                    raise ValueError("OSV : resultat pagine incomplet pour " + name + "; audit manuel requis")
                if not isinstance(item.get("vulns", []), list):
                    raise ValueError("Schema OSV inconnu")
                for advisory in item.get("vulns", []):
                    identifier = advisory["id"]
                    detail = self.http_json("https://api.osv.dev/v1/vulns/" + urllib.parse.quote(identifier, safe=""))
                    fixes = []
                    for affected in detail.get("affected", []):
                        if affected.get("package", {}).get("name") == name:
                            fixes += [event["fixed"] for span in affected.get("ranges", []) for event in span.get("events", []) if "fixed" in event]
                    findings.append(finding(identifier, name, version, fixes, "module", "https://osv.dev/vulnerability/" + identifier, detail.get("summary")))
        return findings

    def audit(self, name, phase, operation):
        result = {"ok": False, "findings": []}
        try:
            result["findings"] = operation()
            result["ok"] = True
        except Exception as error:
            result["error"] = str(error)
        self.save_json(self.run / "audits" / (name.replace("/", "-") + "-" + phase + ".json"), result)
        return result

    def component(self, name, scanner, updater, validator=None):
        result = {"name": name, "update": "non demandee", "validation": "non executee",
                  "before": {"ok": False, "findings": [], "error": "audit non termine"},
                  "after": {"ok": False, "findings": [], "error": "audit non termine"}}
        self.report["components"].append(result)
        result["before"] = self.audit(name, "before", scanner)
        if not self.args.audit_only:
            try:
                result["update"] = updater(result["before"])
            except Exception as error:
                result["update"] = "ECHEC : " + str(error)
        result["after"] = self.audit(name, "after", scanner) if not self.args.audit_only else result["before"]
        if validator:
            try:
                result["validation"] = validator()
            except Exception as error:
                result["validation"] = "ECHEC : " + str(error)
        if result["before"]["ok"] and result["after"]["ok"]:
            old = {(f["id"], f["package"]) for f in result["before"]["findings"]}
            new = {(f["id"], f["package"]) for f in result["after"]["findings"]}
            result["no_longer_reported"] = sorted(old - new)
        self.save_json(self.run / "rapport.json", self.report)

    def required(self, name):
        if not self.tools[name]:
            raise RuntimeError(f"Outil absent : {name}. Voir le guide / -InstallAuditTools.")
        return self.tools[name]

    def checked(self, label, command, cwd, extra_env=None):
        code, output = self.call(label, command, cwd, extra_env)
        if code:
            raise RuntimeError(f"{label} : code {code}, voir logs")
        return output

    def transactional(self, label, paths, operation):
        for path in paths:
            contained(path, self.project)
            if os.path.lexists(path) and linked(path):
                raise ValueError("Un manifeste ne peut pas etre un lien")
        previous = {p: p.read_bytes() if p.exists() else None for p in paths}
        try:
            return operation()
        except BaseException:
            rejected = self.run / "tentatives-rejetees" / (re.sub(r"[^a-zA-Z0-9_-]", "-", label) + "-" + uuid.uuid4().hex[:6])
            for path, content in previous.items():
                contained(path, self.project)
                if os.path.lexists(path) and linked(path):
                    raise ValueError("Manifeste remplace par un lien : restauration automatique refusee")
                if path.exists() and path.read_bytes() != content:
                    rejected.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(path, rejected / path.name)
                if content is None:
                    if path.exists():
                        # Only a new manifest created by this failed operation, inside this run.
                        path.unlink()
                else:
                    path.write_bytes(content)
            raise

    def go_module(self, relative):
        cwd = self.project / relative
        def describe():
            module = json.loads(self.checked(relative + "-go-mod", self.required("go") + ["mod", "edit", "-json"], cwd))
            for item in module.get("Replace") or []:
                target = item["New"]
                if not target.get("Version"):
                    contained(cwd / target["Path"], self.project)
            return module
        def scanner():
            describe()
            code, output = self.call(relative + "-govulncheck", self.required("govulncheck") + ["-format=json", "-scan=module"], cwd)
            values = go_findings(json_stream(output))
            if code:
                raise RuntimeError("govulncheck JSON en echec ; resultat potentiellement incomplet")
            return values
        def update(_):
            go = self.required("go")
            def work():
                module = describe()
                replacements = module.get("Replace") or []
                for item in replacements:
                    target = item["New"]
                    if not target.get("Version"):
                        contained(cwd / target["Path"], self.project)
                replaced = {item["Old"]["Path"] for item in replacements}
                packages = [item["Path"] + "@upgrade" for item in module.get("Require", []) if item["Path"] not in replaced]
                if packages:
                    self.checked(relative + "-go-upgrade", go + ["get"] + packages, cwd)
                self.checked(relative + "-go-toolchain-patch", go + ["get", "go@patch"], cwd)
                after = json.loads(self.checked(relative + "-go-mod-after", go + ["mod", "edit", "-json"], cwd))
                if (after.get("Replace") or []) != replacements:
                    raise RuntimeError("Directives replace modifiees")
                self.checked(relative + "-go-verify", go + ["mod", "verify"], cwd)
                return "mises a jour compatibles preparees"
            return self.transactional(relative, [cwd / "go.mod", cwd / "go.sum"], work)
        def validate():
            if not self.args.run_build_tests:
                return "a compiler/tester (-RunBuildTests)"
            self.checked(relative + "-tests", self.required("go") + ["test", "-trimpath", "-mod=readonly", "./..."], cwd)
            code, output = self.call(relative + "-symbols", self.required("govulncheck") + ["-format=json", "./..."], cwd)
            symbols = go_findings(json_stream(output))
            self.save_json(self.run / "audits" / (relative.replace("/", "-") + "-symbols.json"), symbols)
            if code or any(entry["kind"] == "go-symbol" for entry in symbols):
                raise RuntimeError("Audit des symboles Go non valide")
            return "tests et audit des symboles OK sur la plateforme courante"
        self.component(relative, scanner, update, validate)

    def rust_module(self, relative):
        cwd = self.project / relative
        lock = cwd / "Cargo.lock"
        def scanner():
            code, output = self.call(relative + "-cargo-audit", self.required("cargo-audit") + ["audit", "--json", "--deny", "unsound", "--file", str(lock)], cwd)
            entries = rust_findings(json.loads(output))
            if code not in (0, 1) or (code and not entries):
                raise RuntimeError("cargo-audit en echec sans resultat exploitable")
            return entries
        def guard():
            base = self.project / ("rustdesk" if relative == "rustdesk" else "rustdesk-server")
            self.checked(relative + "-backports", [sys.executable, str(base / "tools/check_security_dependencies.py")], base)
        def update(before):
            if not before["ok"]:
                raise RuntimeError("Audit Rust initial indisponible ; pas de mise a jour a l'aveugle")
            targets = {(entry["package"], entry["version"]) for entry in before["findings"] if entry["kind"] != "unmaintained"}
            if not targets:
                return "aucun correctif Rust compatible a tenter"
            def work():
                manifest = (cwd / "Cargo.toml").read_bytes()
                initial = tomllib.loads(lock.read_text(encoding="utf-8"))
                git = sorted(p["source"] for p in initial["package"] if p.get("source", "").startswith("git+"))
                updated = 0
                for name, version in sorted(targets):
                    current = tomllib.loads(lock.read_text(encoding="utf-8"))["package"]
                    if not re.fullmatch(r"[A-Za-z0-9_-]+", name):
                        raise ValueError("Nom de package inattendu")
                    if any(p["name"] == name and p["version"] == version and p.get("source", "").startswith("registry+") for p in current):
                        self.checked(relative + "-cargo-update-" + name, self.required("cargo") + ["update", name + "@" + version], cwd)
                        updated += 1
                after = tomllib.loads(lock.read_text(encoding="utf-8"))
                if (cwd / "Cargo.toml").read_bytes() != manifest:
                    raise RuntimeError("Manifest Rust modifie de maniere inattendue")
                if sorted(p["source"] for p in after["package"] if p.get("source", "").startswith("git+")) != git:
                    raise RuntimeError("Une revision Git a change ; mise a jour refusee")
                guard()
                return f"{updated} mise(s) a jour Rust ciblee(s) tentee(s)"
            return self.transactional(relative, [lock, cwd / "Cargo.toml"], work)
        def validate():
            guard()
            if self.args.run_build_tests:
                cargo = self.required("cargo")
                if relative == "rustdesk-server/ui":
                    self.checked(relative + "-check", cargo + ["check", "--locked"], cwd)
                else:
                    self.checked(relative + "-atty-tests", cargo + ["test", "--locked", "-p", "atty", "--lib"], cwd)
                    if relative == "rustdesk-server":
                        self.checked(relative + "-auth-tests", cargo + ["test", "--locked", "relaisdesk_auth"], cwd)
                return "correctifs locaux et tests cibles OK ; compilation complete reste a faire"
            return "empreintes des correctifs locaux OK ; compilation non executee"
        self.component(relative, scanner, update, validate)

    def npm_module(self):
        cwd = self.project / "rustdesk-server/ui/html"
        def scanner():
            code, output = self.call("npm-audit", self.required("npm") + ["audit", "--json", "--package-lock-only", "--ignore-scripts"], cwd)
            entries = npm_findings(json.loads(output))
            if code not in (0, 1) or (code and not entries):
                raise RuntimeError("npm audit incomplet")
            return entries
        def update(_):
            def work():
                manifest = (cwd / "package.json").read_bytes()
                # npm audit fix may return nonzero when some vulnerabilities remain.
                code, output = self.call("npm-compatible-fix", self.required("npm") + ["audit", "fix", "--package-lock-only", "--ignore-scripts", "--json"], cwd)
                result = json.loads(output)
                if "error" in result or (code not in (0, 1)):
                    raise RuntimeError("npm audit fix a echoue")
                if (cwd / "package.json").read_bytes() != manifest:
                    raise RuntimeError("package.json modifie : correction sans changement de contraintes attendue")
                return "npm audit fix sans force ni scripts d'installation"
            return self.transactional("npm", [cwd / "package-lock.json", cwd / "package.json"], work)
        def validate():
            if not self.args.run_build_tests:
                return "construction Vite non executee"
            self.checked("npm-ci", self.required("npm") + ["ci", "--ignore-scripts"], cwd)
            self.checked("npm-vite-build", self.required("npm") + ["run", "build"], cwd)
            return "construction Vite OK"
        self.component("npm-ui", scanner, update, validate)

    def python_package(self):
        path = self.project / "rustdesk/libs/portable/requirements.txt"
        def version():
            match = re.fullmatch(r"\s*brotli(?:==([0-9]+\.[0-9]+\.[0-9]+))?\s*", path.read_text(encoding="utf-8"), re.I)
            if not match:
                raise ValueError("Requirements Python modifie : revue du resolveur necessaire")
            return match[1]
        def scanner():
            current = version()
            if not current:
                raise ValueError("Brotli non epingle : version historiquement utilisee indeterminable")
            return self.osv("PyPI", [("brotli", current)])
        def update(_):
            current = version()
            latest = self.http_json("https://pypi.org/pypi/brotli/json")["info"]["version"]
            if not re.fullmatch(r"\d+\.\d+\.\d+", latest):
                raise ValueError("Version Brotli stable non reconnue")
            if current and tuple(map(int, latest.split("."))) < tuple(map(int, current.split("."))):
                raise ValueError("Une retrogradation Brotli a ete refusee")
            if self.osv("PyPI", [("brotli", latest)]):
                raise ValueError("La derniere version de Brotli est encore signalee")
            path.write_text("brotli==" + latest + "\n", encoding="utf-8")
            return "Brotli epingle dans la copie ; environnement Python non modifie"
        self.component("python-portable", scanner, update)

    def dart_module(self):
        cwd = self.project / "rustdesk/flutter"
        lock = cwd / "pubspec.lock"
        def scanner():
            packages, _ = pub_packages(lock)
            return self.osv("Pub", packages)
        def update(before):
            if not before["ok"]:
                raise ValueError("Inventaire Dart/OSV incomplet")
            names = sorted({entry["package"] for entry in before["findings"]})
            if not names:
                return "aucune vulnerabilite Pub signalee"
            def work():
                manifest = (cwd / "pubspec.yaml").read_bytes()
                _, git = pub_packages(lock)
                dart = self.required("dart")
                # The SDK must be a Flutter-bundled Dart SDK for this Flutter project.
                flutter_root = Path(dart[0]).resolve().parents[4]
                if not (flutter_root / "packages/flutter/pubspec.yaml").is_file():
                    raise RuntimeError("SDK Flutter requis pour resoudre les packages du client")
                self.checked("dart-targeted-upgrade", dart + ["pub", "upgrade"] + names, cwd, {"FLUTTER_ROOT": str(flutter_root)})
                if pub_packages(lock)[1] != git:
                    raise ValueError("Revision Git Dart modifiee : correction refusee")
                if (cwd / "pubspec.yaml").read_bytes() != manifest:
                    raise ValueError("Contraintes Dart modifiees : correction refusee")
                return "packages Pub concernes mis a jour ; recette Flutter necessaire"
            return self.transactional("dart", [lock, cwd / "pubspec.yaml"], work)
        self.component("dart-flutter", scanner, update)

    def install_tools(self):
        target = self.run / "tools"
        target.mkdir()
        if self.tools["go"]:
            try:
                self.checked("install-govulncheck", self.tools["go"] + ["install", "golang.org/x/vuln/cmd/govulncheck@latest"], target, {"GOBIN": str(target)})
                self.tools["govulncheck"] = [str(target / ("govulncheck.exe" if os.name == "nt" else "govulncheck"))]
            except Exception as error:
                self.report["errors"].append(str(error))
        if self.tools["cargo"]:
            try:
                self.checked("install-cargo-audit", self.tools["cargo"] + ["install", "cargo-audit", "--locked", "--root", str(target / "cargo")], target)
                self.tools["cargo-audit"] = [str(target / "cargo/bin" / ("cargo-audit.exe" if os.name == "nt" else "cargo-audit"))]
            except Exception as error:
                self.report["errors"].append(str(error))

    def export(self):
        changed, unexpected, source_changed, diff = [], [], [], []
        target = self.run / "fichiers-corriges"
        target.mkdir(exist_ok=True)
        for relative, original_hash in self.inventory.items():
            original, candidate = self.root / relative, self.project / relative
            try:
                contained(original, self.root)
                contained(candidate, self.project)
            except ValueError:
                unexpected.append(relative)
                continue
            if not original.is_file() or linked(original) or digest(original) != original_hash:
                source_changed.append(relative)
                continue
            if not candidate.is_file() or linked(candidate):
                unexpected.append(relative)
                continue
            current_hash = digest(candidate)
            if current_hash == original_hash:
                continue
            if relative not in EDITABLE_PATHS:
                unexpected.append(relative)
                continue
            destination = target / relative
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(candidate, destination)
            changed.append({"path": relative, "before_sha256": original_hash, "after_sha256": current_hash})
            diff.extend(difflib.unified_diff(original.read_text(encoding="utf-8").splitlines(True), candidate.read_text(encoding="utf-8").splitlines(True), "a/" + relative, "b/" + relative))
        # go.sum can legitimately be created for a formerly dependency-free module.
        for relative in GO_MODULES:
            candidate = self.project / relative / "go.sum"
            name = candidate.relative_to(self.project).as_posix()
            try:
                contained(candidate, self.project)
            except ValueError:
                unexpected.append(name)
                continue
            if name not in self.inventory and candidate.is_file() and not linked(candidate):
                if os.path.lexists(self.root / name):
                    source_changed.append(name)
                    continue
                destination = target / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(candidate, destination)
                changed.append({"path": name, "before_sha256": None, "after_sha256": digest(candidate)})
                diff.extend(difflib.unified_diff([], candidate.read_text(encoding="utf-8").splitlines(True), "/dev/null", "b/" + name))
        self.report.update({"changed_files": changed, "unexpected_source_edits": unexpected, "original_source_changed_during_run": source_changed})
        if unexpected or source_changed:
            self.report["errors"].append("Des sources ont change de maniere inattendue : NE PAS appliquer le lot.")
        (self.run / "modifications.patch").write_text("".join(diff), encoding="utf-8")
        (target / "LIRE_AVANT_APPLICATION.txt").write_text("Candidats de dependances, PAS un deploiement valide. Consulter ../rapport.md et ../rapport.json.\nAucun executable n'a ete corrige. Ne pas copier aveuglement sur Oracle.\n", encoding="utf-8")

    def finish(self):
        self.report["changed_files"] = []
        if self.snapshot_complete:
            try:
                self.export()
            except Exception as error:
                self.report["errors"].append("Export incomplet : " + str(error))
        self.report["finished_at"] = dt.datetime.now(dt.timezone.utc).isoformat()
        attention = bool(self.report["errors"]) or any(
            not c["after"]["ok"] or c["after"]["findings"] or "ECHEC" in c["update"] or "ECHEC" in c["validation"]
            for c in self.report["components"])
        self.report["status"] = "A_VERIFIER" if attention else "PREPARATION_SANS_AUDIT" if self.args.prepare_only else "CONTROLES_AUTOMATIQUES_TERMINES_RECETTE_REQUISE"
        self.report["do_not_apply_batch"] = attention or self.args.prepare_only or self.args.audit_only or not self.args.run_build_tests
        lines = ["# Maintenance securite RelaisDesk", "", f"Etat : **{self.report['status']}**", "",
                 "Aucune modification du projet original, aucun deploiement prevu. Un changement externe detecte est indique ci-dessous.",
                 "Les fichiers-corriges sont des candidats, pas une garantie de securite ni des executables mis a jour.", "",
                 "## Composants", "", "| Composant | Audit final | Mise a jour | Validation |", "| --- | --- | --- | --- |"]
        for item in self.report["components"]:
            audit = item["after"]
            state = str(len(audit["findings"])) + " signalement(s)" if audit["ok"] else "INCONNU / ERREUR"
            lines.append("| " + " | ".join(str(value).replace("|", "/").replace("\n", " ") for value in (item["name"], state, item["update"], item["validation"])) + " |")
        lines += ["", "## Signalements restants et controles incomplets", ""]
        for item in self.report["components"]:
            audit = item["after"]
            if not audit["ok"]:
                lines.append(f"- {item['name']} : {audit.get('error', 'controle incomplet')}")
            for entry in audit["findings"]:
                fix = ", ".join(entry["fixed_versions"]) or "aucune version corrigee indiquee par la source ; revue manuelle"
                lines.append(f"- **{entry['id']}** — {item['name']} / {entry['package']} {entry['version']} ({entry['kind']}). Correctif indique : {fix}. {entry['url']}")
                if entry.get("title"):
                    lines.append("  " + entry["title"].replace("\n", " "))
        lines += ["", "## Corrections non automatiques / limites", ""] + ["- " + item for item in MANUAL_LIMITS]
        lines += ["", "## Fichiers modifies", ""] + ["- " + item["path"] for item in self.report["changed_files"]]
        if not self.report["changed_files"]:
            lines.append("Aucun fichier de dependances modifie pendant ce passage.")
        if self.report["errors"]:
            lines += ["", "## Erreurs", ""] + ["- " + error for error in self.report["errors"]]
        if attention:
            lines += ["", "**NE PAS appliquer ce lot en l'etat.** Examiner les signalements, echecs et modifications avant toute integration."]
        lines += ["", "Les moteurs integres et les trois fichiers du service Windows fleet sont copies comme entrees de tests INCHANGEES. Ils ne sont jamais exportes dans fichiers-corriges.",
                  "Les logs, audits JSON, empreintes avant/apres et modifications.patch sont conserves dans ce dossier. Aucun effacement automatique des anciens passages."]
        (self.run / "rapport.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
        self.save_json(self.run / "rapport.json", self.report)
        print(f"\nRapport : {self.run / 'rapport.md'}\nCandidats : {self.run / 'fichiers-corriges'}", flush=True)
        return 2 if attention else 0

    def execute(self):
        try:
            print("Copie des sources avec exclusions de secrets connus dans " + str(self.project), flush=True)
            self.inventory, excluded = snapshot(self.root, self.project)
            self.snapshot_complete = True
            self.save_json(self.run / "inventaire-initial.json", self.inventory)
            self.save_json(self.run / "exclusions.json", excluded)
            if not self.args.prepare_only:
                if self.args.install_audit_tools:
                    self.install_tools()
                for relative in GO_MODULES:
                    self.go_module(relative)
                for relative in RUST_MODULES:
                    self.rust_module(relative)
                self.npm_module()
                self.python_package()
                self.dart_module()
                for script in ("pricing-checks.mjs", "legal-checks.mjs"):
                    if (self.project / "scripts" / script).is_file():
                        try:
                            self.checked(script, self.required("node") + [str(self.project / "scripts" / script)], self.project)
                        except Exception as error:
                            self.report["errors"].append(str(error))
        except KeyboardInterrupt:
            self.report["errors"].append("Interruption par l'utilisateur : resultat partiel, ne pas appliquer automatiquement.")
        except Exception as error:
            self.report["errors"].append(str(error))
        return self.finish()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project-root", default=str(Path(__file__).resolve().parents[1]))
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--audit-only", action="store_true")
    modes.add_argument("--prepare-only", action="store_true")
    parser.add_argument("--run-build-tests", action="store_true")
    parser.add_argument("--install-audit-tools", action="store_true")
    parser.add_argument("--timeout", type=int, default=900)
    parser.add_argument("--run-id", help="Identifiant unique impose par le lanceur ; un dossier existant est refuse")
    args = parser.parse_args()
    if not 30 <= args.timeout <= 7200:
        parser.error("timeout doit etre compris entre 30 et 7200 secondes")
    return Maintenance(args).execute()


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as error:
        print("Preparation impossible : " + str(error), file=sys.stderr)
        raise SystemExit(1)
