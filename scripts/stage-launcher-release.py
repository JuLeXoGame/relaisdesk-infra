"""Stage both launcher modules and their cross-module integrity fixtures.

Only staged copies receive the release pins; the development payloads stay intact.
"""
import argparse
import hashlib
import re
import shutil
from pathlib import Path


def replace_pin(path, name, value, powershell=False):
    text = path.read_text(encoding="utf-8-sig")
    prefix = r"\$" if powershell else r"\b"
    pattern = prefix + re.escape(name) + r'(\s*=\s*)"[^"]*"'
    text, count = re.subn(pattern, lambda m: ("$" if powershell else "") + name + m[1] + '"' + value + '"', text)
    if count != 1:
        raise ValueError(f"Expected one {name} in {path}, found {count}")
    path.write_text(text, encoding="utf-8", newline="\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", type=Path, required=True)
    parser.add_argument("--destination", type=Path, required=True)
    parser.add_argument("--platform", choices=("windows", "linux"), required=True)
    parser.add_argument("--payload", type=Path, required=True)
    parser.add_argument("--payload-sha256", required=True)
    parser.add_argument("--runner-sha256")
    parser.add_argument("--library-sha256")
    parser.add_argument("--service-sha256")
    args = parser.parse_args()
    values = [args.payload_sha256]
    if args.platform == "linux":
        values += [args.runner_sha256, args.library_sha256]
    else:
        values += [args.service_sha256]
    if any(not v or not re.fullmatch(r"[a-fA-F0-9]{64}", v) for v in values):
        parser.error("All payload pins must be SHA-256 digests")
    digest = hashlib.sha256()
    with args.payload.open("rb") as payload:
        for block in iter(lambda: payload.read(1024 * 1024), b""):
            digest.update(block)
    if digest.hexdigest() != args.payload_sha256.lower():
        parser.error("Payload digest mismatch")
    args.destination.mkdir(parents=True, exist_ok=False)
    for app in ("viewer", "configurator"):
        target = args.destination / app
        (target / "embedded").mkdir(parents=True)
        for source in (args.project / "installer" / app).iterdir():
            if source.is_file() and (source.suffix in (".go", ".syso", ".png", ".ico", ".manifest") or source.name in ("go.mod", "go.sum")):
                shutil.copy2(source, target / source.name)
        extension = "exe" if args.platform == "windows" else "deb"
        shutil.copy2(args.payload, target / "embedded" / f"rustdesk.{extension}")
        config = target / f"config_{args.platform}.go"
        if args.platform == "windows":
            replace_pin(config, "RUSTDESK_EXPECTED_SHA256", args.payload_sha256.lower())
            if app == "viewer":
                replace_pin(config, "RUSTDESK_SERVICE_EXPECTED_SHA256", args.service_sha256.lower())
        else:
            for key, value in (("RUSTDESK_EXPECTED_SHA256", args.runner_sha256), ("RUSTDESK_PACKAGE_EXPECTED_SHA256", args.payload_sha256), ("RUSTDESK_SO_EXPECTED_SHA256", args.library_sha256)):
                replace_pin(config, key, value.lower())
    scripts = ("build_technicien_portable.ps1", "build_viewer_windows.ps1", "build_linux_technicien.ps1", "build_linux_viewer.ps1")
    for name in scripts:
        shutil.copy2(args.project / "installer" / name, args.destination / name)
    if args.platform == "windows":
        replace_pin(args.destination / scripts[0], "windowsHash", args.payload_sha256.lower(), True)
        replace_pin(args.destination / scripts[1], "forkWindowsSha256", args.payload_sha256.lower(), True)
        replace_pin(args.destination / scripts[1], "forkWindowsServiceSha256", args.service_sha256.lower(), True)
    else:
        for name in scripts[2:]:
            for key, value in (("rustdeskElfSha", args.runner_sha256), ("rustdeskDebSha", args.payload_sha256), ("rustdeskSoSha", args.library_sha256)):
                replace_pin(args.destination / name, key, value.lower(), True)


if __name__ == "__main__":
    main()
