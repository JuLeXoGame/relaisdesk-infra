#!/usr/bin/env python3
"""Verify the exact signed 1.0.0 download set before a production switch."""
import base64
import hashlib
import json
import pathlib
import sys

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

if not __debug__:
    raise RuntimeError("Optimized Python must not disable release checks")

PUBLIC_KEY = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
NAMES = {
    "RelaisDesk_Portable.exe", "RelaisDesk_Setup.exe",
    "RelaisDesk_Technicien_Portable.exe", "RelaisDesk_Technicien_Setup_1.0.0.exe",
    "RelaisDesk_Technicien.deb", "RelaisDesk_viewer.deb",
    "RelaisDesk_Technicien_Linux", "RelaisDesk_Viewer_Linux",
    "RelaisDesk_Mac.dmg", "RelaisDesk_Mac_Intel.dmg",
    "RelaisDesk_Technicien_Mac.dmg", "RelaisDesk_Technicien_Mac_Intel.dmg",
    "SHA256SUMS.txt",
}


def verify(directory):
    directory = pathlib.Path(directory)
    path = directory / "release-manifest.json"
    assert not path.is_symlink() and path.stat().st_size <= 1024 * 1024
    manifest = json.loads(path.read_text())
    assert set(manifest) == {"version", "published_at", "key_id", "artifacts", "signature"}
    assert manifest["version"] == "1.0.0" and manifest["key_id"] == "release-1"
    artifacts = manifest["artifacts"]
    assert len(artifacts) == len(NAMES) and {a["name"] for a in artifacts} == NAMES
    payload = {key: manifest[key] for key in ("version", "published_at", "key_id", "artifacts")}
    payload["artifacts"] = [{key: a[key] for key in ("name", "url", "sha256", "size")} for a in artifacts]
    decode = lambda value: base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
    Ed25519PublicKey.from_public_bytes(decode(PUBLIC_KEY)).verify(
        decode(manifest["signature"]), json.dumps(payload, separators=(",", ":"), ensure_ascii=False).encode()
    )
    sums = []
    for artifact in artifacts:
        assert set(artifact) == {"name", "url", "sha256", "size"}
        name = artifact["name"]
        assert artifact["url"] == "https://api.relaisdesk.fr/api/v1/downloads/" + name
        item = directory / name
        assert not item.is_symlink() and item.is_file() and item.stat().st_size == artifact["size"] > 0
        digest = hashlib.sha256()
        with item.open("rb") as stream:
            while chunk := stream.read(1024 * 1024):
                digest.update(chunk)
        assert digest.hexdigest() == artifact["sha256"], name
        if name != "SHA256SUMS.txt":
            sums.append(artifact["sha256"] + "  " + name)
    assert sorted((directory / "SHA256SUMS.txt").read_text().splitlines()) == sorted(sums)
    print("DOWNLOADS_VERIFIED version=1.0.0 artifacts=13 signature=Ed25519")


if __name__ == "__main__":
    verify(sys.argv[1])
