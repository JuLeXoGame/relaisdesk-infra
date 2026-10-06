#!/bin/bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

# Build natif Linux du technicien (GUI Fyne : CGO + X11, pas de cross-compile
# depuis Windows). Exécuté dans WSL (Ubuntu-22.04).
#
# Entrées : embedded/rustdesk.deb (fork, dans l'arbre ; RUSTDESK_FORK_LINUX_DEB
#   permet d'en fournir un autre). Les 3 pins (ELF, DEB, SO) sont calculés
#   depuis ce paquet et injectés au build ; le test TestLinuxPinsMatchEmbedded
#   en pré-vol refuse toute dérive entre paquet embarqué et pins Go.
# Sorties : RelaisDesk_Technicien_Linux (ELF) + RelaisDesk_Technicien.deb dans
#   DL_DIR (../../relaisdesk/downloads par défaut, côté WSL), rapatriés
#   ensuite vers Windows par installer/build_linux_technicien.ps1.
#
# Variables : APIURL, VERSION, RELEASE_PUBLIC_KEY, RUSTDESK_FORK_LINUX_DEB, DL_DIR.

APIURL="${APIURL:-https://api.relaisdesk.fr}"
VERSION="${VERSION:-1.0.0}"
RELEASE_PUBLIC_KEY="${RELEASE_PUBLIC_KEY:-K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo}"
FORK_DEB="${RUSTDESK_FORK_LINUX_DEB:-embedded/rustdesk.deb}"
DL_DIR="${DL_DIR:-../../relaisdesk/downloads}"

if ! command -v go >/dev/null 2>&1; then
    if [ -x "$HOME/sdk/go/bin/go" ]; then
        export PATH="$HOME/sdk/go/bin:$PATH"
    else
        echo "go introuvable : lancez scripts/setup-wsl-go.sh (ou exportez PATH)." >&2
        exit 1
    fi
fi
if [ -z "${CGO_LDFLAGS:-}" ] && [ -e "$HOME/sdk/sysroot/lib/libXxf86vm.so" ]; then
    export CGO_LDFLAGS="-L$HOME/sdk/sysroot/lib"
fi
for tool in dpkg-deb sha256sum; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "outil requis manquant : $tool" >&2
        exit 1
    fi
done

test -f "$FORK_DEB"
if [ "$FORK_DEB" -ef embedded/rustdesk.deb ]; then
    :
else
    cp -- "$FORK_DEB" embedded/rustdesk.deb
fi
RUSTDESK_PACKAGE_SHA256="$(sha256sum embedded/rustdesk.deb | awk '{print $1}')"
TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT
dpkg-deb -x embedded/rustdesk.deb "$TMPD"
RUSTDESK_BINARY_SHA256="$(sha256sum "$TMPD/usr/share/rustdesk/rustdesk" | awk '{print $1}')"
RUSTDESK_SO_SHA256="$(sha256sum "$TMPD/usr/share/rustdesk/lib/librustdesk.so" | awk '{print $1}')"
echo "Pins fork : deb=$RUSTDESK_PACKAGE_SHA256 elf=$RUSTDESK_BINARY_SHA256 so=$RUSTDESK_SO_SHA256"

echo "Pré-vol : cohérence pins Go / paquet embarqué..."
go test -count=1 -run 'TestLinuxPinsMatchEmbedded' .

echo "Compilation native (CGO) : APIURL=$APIURL VERSION=$VERSION"
CGO_ENABLED=1 go build -mod=readonly -trimpath \
    -ldflags "-s -w -X main.APIURL=$APIURL -X main.APP_VERSION=$VERSION -X main.RELEASE_PUBLIC_KEY=$RELEASE_PUBLIC_KEY -X main.RUSTDESK_EXPECTED_SHA256=$RUSTDESK_BINARY_SHA256 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$RUSTDESK_PACKAGE_SHA256 -X main.RUSTDESK_SO_EXPECTED_SHA256=$RUSTDESK_SO_SHA256" \
    -o relaisdesk-configurator .

echo "Suite de tests (gate Linux)..."
go test -count=1 ./...

echo "Empaquetage .deb..."
go -C ../debpack run -mod=readonly . -bin ../configurator/relaisdesk-configurator \
    -pkg relaisdesk-configurator -name relaisdesk-configurator -version "$VERSION" \
    -desc "RelaisDesk Technicien - Support a distance" -out ../configurator/relaisdesk-configurator.deb

mkdir -p "$DL_DIR"
cp -- ./relaisdesk-configurator "$DL_DIR/RelaisDesk_Technicien_Linux"
cp -- ./relaisdesk-configurator.deb "$DL_DIR/RelaisDesk_Technicien.deb"
echo "OK :"
sha256sum "$DL_DIR/RelaisDesk_Technicien_Linux" "$DL_DIR/RelaisDesk_Technicien.deb"
