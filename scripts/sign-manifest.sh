#!/usr/bin/env bash
# RelaisDesk - Signature du manifeste de telechargement (portage de installer/sign_manifest.ps1).
set -euo pipefail

VERSION="1.0.0"
DOWNLOADS_DIR=""
API_DIR=""
SIGNING_KEY=""
PUBLIC_KEY="K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
KEY_ID="release-1"
API_URL="https://api.relaisdesk.fr"

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --downloads-dir) DOWNLOADS_DIR="$2"; shift 2 ;;
        --api-dir) API_DIR="$2"; shift 2 ;;
        --signing-key) SIGNING_KEY="$2"; shift 2 ;;
        --public-key) PUBLIC_KEY="$2"; shift 2 ;;
        --key-id) KEY_ID="$2"; shift 2 ;;
        --base-url) API_URL="$2"; shift 2 ;;
        -h|--help) echo "Usage : $0 --signing-key K [--version V] [--downloads-dir D] [--api-dir A] [--public-key P] [--key-id I] [--base-url U]"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if [ -z "$SIGNING_KEY" ]; then echo "Parametre --signing-key requis." >&2; exit 1; fi
if [ ! -f "$SIGNING_KEY" ]; then echo "Cle de signature introuvable : $SIGNING_KEY" >&2; exit 1; fi
PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -z "$DOWNLOADS_DIR" ]; then DOWNLOADS_DIR="$PROJECT/relaisdesk/downloads"; fi
if [ -z "$API_DIR" ]; then API_DIR="$PROJECT/api"; fi
if ! printf '%s' "$PUBLIC_KEY" | grep -Eq '^[A-Za-z0-9_-]{43}$'; then echo "Cle publique invalide." >&2; exit 1; fi

ARTIFACTS="RelaisDesk_Portable.exe RelaisDesk_Setup.exe RelaisDesk_Technicien.deb RelaisDesk_Technicien_Linux RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux"

# 1. Update SHA256SUMS.txt
for name in $ARTIFACTS; do
    if [ ! -f "$DOWNLOADS_DIR/$name" ]; then echo "Artefact manquant : $DOWNLOADS_DIR/$name" >&2; exit 1; fi
done
{
    # shellcheck disable=SC2086
    for name in $ARTIFACTS; do
        printf '%s  %s\n' "$(sha256sum "$DOWNLOADS_DIR/$name" | awk '{print $1}')" "$name"
    done
} | LC_ALL=C sort -k2 > "$DOWNLOADS_DIR/SHA256SUMS.txt"
echo "Updated SHA256SUMS.txt"

# 2. Run release-manifest tool
INCLUDE_LIST=""
for name in $ARTIFACTS; do INCLUDE_LIST="$INCLUDE_LIST,$name"; done
INCLUDE_LIST="${INCLUDE_LIST#,},SHA256SUMS.txt"
( cd "$API_DIR" && go run -buildvcs=false ./cmd/release-manifest \
    -downloads "$DOWNLOADS_DIR" \
    -version "$VERSION" \
    -base-url "$API_URL/api/v1/downloads" \
    -key-id "$KEY_ID" \
    -private-key "$SIGNING_KEY" \
    -public-key "$PUBLIC_KEY" \
    -include "$INCLUDE_LIST" ) || { echo "Manifest signing failed" >&2; exit 1; }
echo "Manifest signed and verified successfully."
