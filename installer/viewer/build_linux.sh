#!/bin/bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

# -----------------------------------------------------------------------
# APIURL par défaut : domaine professionnel en HTTPS vérifié
# Ancien domaine DuckDNS : https://relaisdesk.duckdns.org:8443
# -----------------------------------------------------------------------
APIURL="${APIURL:-https://api.relaisdesk.fr}"
RUSTDESK_FORK_LINUX_DEB="${RUSTDESK_FORK_LINUX_DEB:?chemin du paquet .deb du fork requis}"
RUSTDESK_FORK_LINUX_BINARY="${RUSTDESK_FORK_LINUX_BINARY:?chemin du binaire ELF du fork requis}"

test -f "$RUSTDESK_FORK_LINUX_DEB"
test -f "$RUSTDESK_FORK_LINUX_BINARY"
cp -- "$RUSTDESK_FORK_LINUX_DEB" embedded/rustdesk.deb
RUSTDESK_PACKAGE_SHA256="$(sha256sum "$RUSTDESK_FORK_LINUX_DEB" | awk '{print $1}')"
RUSTDESK_BINARY_SHA256="$(sha256sum "$RUSTDESK_FORK_LINUX_BINARY" | awk '{print $1}')"
RUSTDESK_SO_SHA256=""
if [[ -n "${RUSTDESK_FORK_LINUX_SO:-}" ]]; then
    test -f "$RUSTDESK_FORK_LINUX_SO"
    RUSTDESK_SO_SHA256="$(sha256sum "$RUSTDESK_FORK_LINUX_SO" | awk '{print $1}')"
fi

echo "🔍 Vérification des pré-requis..."
if ! command -v go &> /dev/null; then
    echo "❌ Erreur: 'go' n'est pas installé."
    exit 1
fi
echo "🏗️ Compilation avec APIURL=$APIURL"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=readonly -trimpath \
    -ldflags "-X main.APIURL=$APIURL -X main.RUSTDESK_EXPECTED_SHA256=$RUSTDESK_BINARY_SHA256 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$RUSTDESK_PACKAGE_SHA256 -X main.RUSTDESK_SO_EXPECTED_SHA256=$RUSTDESK_SO_SHA256" \
    -o relaisdesk-viewer .
go -C ../debpack run -mod=readonly . -bin ../viewer/relaisdesk-viewer \
    -pkg relaisdesk-viewer -name relaisdesk-viewer -version "${VERSION:-1.0.1}" \
    -desc "RelaisDesk Viewer" -out ../viewer/relaisdesk-viewer.deb

echo "✅ Succès ! Le paquet .deb a été généré avec succès dans le dossier actuel."
echo "Pour l'installer : sudo dpkg -i relaisdesk-viewer.deb"
