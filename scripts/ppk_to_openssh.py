"""Convert unencrypted PuTTY PPK (v2/v3) ssh-rsa key to OpenSSH PEM format."""
import base64
import os
import struct
import sys

SRC = sys.argv[1]
DST = sys.argv[2] if len(sys.argv) > 2 else os.path.expanduser("~/.ssh/oracle")


def read_sections(path):
    meta, sections = {}, {}
    with open(path, "r", encoding="utf-8") as f:
        lines = f.read().splitlines()
    assert lines[0].startswith("PuTTY-User-Key-File-"), lines[0]
    i = 1
    while i < len(lines):
        line = lines[i]
        if line.startswith("Public-Lines:"):
            n = int(line.split(":", 1)[1])
            sections["public"] = base64.b64decode("".join(lines[i + 1:i + 1 + n]))
            i += 1 + n
            continue
        if line.startswith("Private-Lines:"):
            n = int(line.split(":", 1)[1])
            sections["private"] = base64.b64decode("".join(lines[i + 1:i + 1 + n]))
            i += 1 + n
            continue
        if ":" in line:
            k, v = line.split(":", 1)
            meta[k.strip()] = v.strip()
        i += 1
    return meta, sections


def get_string(buf, off):
    (ln,) = struct.unpack(">I", buf[off:off + 4])
    return buf[off + 4:off + 4 + ln], off + 4 + ln


def get_mpint(buf, off):
    raw, off = get_string(buf, off)
    return int.from_bytes(raw, "big"), off


meta, sections = read_sections(SRC)
if meta.get("Encryption") != "none":
    raise SystemExit(f"encrypted PPK not supported: {meta.get('Encryption')}")
pub, priv = sections["public"], sections["private"]
alg, o = get_string(pub, 0)
assert alg == b"ssh-rsa", alg
e, o = get_mpint(pub, o)
n, o = get_mpint(pub, o)
d, o = get_mpint(priv, 0)
p, o = get_mpint(priv, o)
q, o = get_mpint(priv, o)
iqmp, o = get_mpint(priv, o)
assert p * q == n, "key consistency check failed"

from cryptography.hazmat.primitives.asymmetric.rsa import (
    RSAPrivateNumbers,
    RSAPublicNumbers,
)
from cryptography.hazmat.primitives.serialization import (
    Encoding,
    NoEncryption,
    PrivateFormat,
)

numbers = RSAPrivateNumbers(
    p=p, q=q, d=d,
    dmp1=d % (p - 1), dmq1=d % (q - 1), iqmp=iqmp,
    public_numbers=RSAPublicNumbers(e=e, n=n),
)
pem = numbers.private_key().private_bytes(
    Encoding.PEM, PrivateFormat.TraditionalOpenSSL, NoEncryption()
)
os.makedirs(os.path.dirname(DST), mode=0o700, exist_ok=True)
with open(DST, "wb") as f:
    f.write(pem)
os.chmod(DST, 0o600)
print(f"KEY_OK {DST}")
