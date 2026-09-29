#!/usr/bin/env bash
# RelaisDesk - Extraction du paquet Sciter examine (portage de extract-reviewed-sciter-payload.ps1).
# Decode le binaire comme des donnees, sans jamais l'executer.
# Requiert python3 + module brotli (Debian/Ubuntu : paquet python3-brotli,
# ou un venv projet avec brotli).
set -euo pipefail

PORTABLE=""; EXPECTED_SHA256=""; DEST=""

while [ $# -gt 0 ]; do
    case "$1" in
        --portable) PORTABLE="$2"; shift 2 ;;
        --expected-sha256) EXPECTED_SHA256="$2"; shift 2 ;;
        --destination) DEST="$2"; shift 2 ;;
        -h|--help) echo "Usage : $0 --portable F --expected-sha256 H --destination D"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if [ -z "$PORTABLE" ] || [ -z "$EXPECTED_SHA256" ] || [ -z "$DEST" ]; then echo "Parametres manquants." >&2; exit 1; fi
if ! printf '%s' "$EXPECTED_SHA256" | grep -Eq '^[a-fA-F0-9]{64}$'; then echo "Empreinte SHA-256 invalide." >&2; exit 1; fi
if [ ! -f "$PORTABLE" ] || [ "$(stat -c %s "$PORTABLE")" -gt 67108864 ]; then echo "Invalid portable input." >&2; exit 1; fi
ACTUAL="$(sha256sum "$PORTABLE" | awk '{print $1}')"
if [ "${ACTUAL,,}" != "${EXPECTED_SHA256,,}" ]; then echo "Portable SHA-256 does not match the reviewed release." >&2; exit 1; fi
if [ -e "$DEST" ]; then echo "Use a new extraction directory." >&2; exit 1; fi
command -v python3 >/dev/null || { echo "python3 requis." >&2; exit 1; }
python3 -c "import brotli" 2>/dev/null || { echo "Module python3 brotli requis (ex : paquet python3-brotli)." >&2; exit 1; }

PORTABLE="$PORTABLE" DEST="$DEST" python3 - <<'PYEOF'
import hashlib, os, struct, sys
import brotli

class InvalidData(Exception):
    pass

MARKER = b"rustdesk"
NAMES = ("./rustdesk.exe", "./sciter.dll", "./dylib_virtual_display.dll")
MAX_TOTAL = 67108864

def match(data, at, value):
    if at < 0 or at + len(value) > len(data):
        return False
    return data[at:at + len(value)] == value

def length(data, at, maximum):
    if at < 0 or at + 4 > len(data):
        raise InvalidData()
    (n,) = struct.unpack_from(">I", data, at)
    at += 4
    if n == 0 or n > maximum or at + n > len(data):
        raise InvalidData()
    return n, at

def read(data, offset):
    at = offset + 8
    files = {}
    while not match(data, at, MARKER):
        if len(files) >= 3:
            raise InvalidData()
        n, at = length(data, at, 128)
        path = data[at:at + n].decode("utf-8").replace("\\", "/")
        at += n
        if path not in NAMES:
            raise InvalidData()
        count, at = length(data, at, MAX_TOTAL)
        raw = brotli.decompress(data[at:at + count])
        if len(raw) > MAX_TOTAL:
            raise InvalidData()
        at += count
        if at + 32 > len(data):
            raise InvalidData()
        md5 = data[at:at + 32].decode("ascii")
        at += 32
        if hashlib.md5(raw).hexdigest() != md5.lower():
            raise InvalidData()
        if len(raw) < 64 or raw[0] != 0x4D or raw[1] != 0x5A:
            raise InvalidData()
        (pe,) = struct.unpack_from("<i", raw, 0x3C)
        if pe < 0 or pe + 6 > len(raw):
            raise InvalidData()
        if struct.unpack_from("<I", raw, pe)[0] != 0x4550:
            raise InvalidData()
        if struct.unpack_from("<H", raw, pe + 4)[0] != 0x8664:
            raise InvalidData()
        name = path[2:]
        if name in files:
            raise InvalidData()
        files[name] = raw
    if len(files) != 3 or (not match(data, at + 8, b".\\rustdesk.exe")
                           and not match(data, at + 8, b"./rustdesk.exe")):
        raise InvalidData()
    return files

def extract(data):
    found = None
    for at in range(len(data) - 12):
        if not match(data, at, MARKER):
            continue
        if data[at + 8] != 0 or data[at + 9] != 0 or data[at + 10] != 0:
            continue
        try:
            current = read(data, at)
        except InvalidData:
            continue
        if found is not None:
            raise InvalidData("Ambiguous native package")
        found = current
    if found is None:
        raise InvalidData("Complete native Sciter payload not found")
    return found

data = open(os.environ["PORTABLE"], "rb").read()
try:
    files = extract(data)
except InvalidData as exc:
    print(f"InvalidDataException: {exc}", file=sys.stderr)
    sys.exit(1)
os.makedirs(os.environ["DEST"], exist_ok=False)
for name, raw in files.items():
    path = os.path.join(os.environ["DEST"], name)
    open(path, "wb").write(raw)
    print(f"{name} {len(raw)} {hashlib.sha256(raw).hexdigest()}")
PYEOF
