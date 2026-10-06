#!/bin/bash
# Installe la toolchain Go native dans WSL (persistante, sans sudo) pour les
# builds Linux natifs : la GUI Fyne (CGO + X11) ne peut pas être cross-compilée
# depuis Windows, le binaire technicien Linux est donc construit ici.
#
# Idempotent : ne re-télécharge pas si la version demandée est déjà installée.
# Emplacement : ~/sdk/go (GOPATH/GOCACHE restent aux défauts ~/go et
# ~/.cache/go-build, persistants eux aussi — ne rien mettre sous /tmp, vidé
# à chaque arrêt de la distribution).
#
# Usage :  bash scripts/setup-wsl-go.sh   (depuis /home/julien/projet)
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.26.6}"
SDK_DIR="${SDK_DIR:-$HOME/sdk}"
GO_DIR="$SDK_DIR/go"

if [ -x "$GO_DIR/bin/go" ] && [ "$("$GO_DIR/bin/go" version | awk '{print $3}')" = "go$GO_VERSION" ]; then
    echo "Go $GO_VERSION déjà installé dans $GO_DIR"
else
    echo "Téléchargement de Go $GO_VERSION..."
    mkdir -p "$SDK_DIR"
    TARBALL="/tmp/go$GO_VERSION.linux-amd64.tar.gz"
    rm -f "$TARBALL"
    if ! curl -fsSL -o "$TARBALL" "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz"; then
        curl -fsSL -o "$TARBALL" "https://dl.google.com/go/go$GO_VERSION.linux-amd64.tar.gz"
    fi
    rm -rf "$GO_DIR"
    tar -C "$SDK_DIR" -xzf "$TARBALL"
    rm -f "$TARBALL"
    echo "Go installé dans $GO_DIR"
fi

# Liaison Xxf86vm : seul le runtime est présent (libxxf86vm1), pas le lien
# libXxf86vm.so du paquet -dev (pas de sudo). On recrée le lien dans un
# sysroot persistant sous $HOME et on l'ajoute aux flags CGO.
if ldconfig -p 2>/dev/null | grep -q libXxf86vm; then
    SYSROOT_LIB="$SDK_DIR/sysroot/lib"
    mkdir -p "$SYSROOT_LIB"
    TARGET="$(ldconfig -p | awk '/libXxf86vm\.so/{print $NF; exit}')"
    if [ -n "$TARGET" ]; then
        ln -sf "$TARGET" "$SYSROOT_LIB/libXxf86vm.so"
        echo "Lien Xxf86vm : $SYSROOT_LIB/libXxf86vm.so -> $TARGET"
    fi
else
    echo "AVERTISSEMENT : libXxf86vm introuvable (ldconfig), la liaison CGO peut échouer." >&2
fi

"$GO_DIR/bin/go" version
echo ""
echo "Pour utiliser cette toolchain :"
echo "  export PATH=\"$GO_DIR/bin:\$PATH\""
if [ -e "$SDK_DIR/sysroot/lib/libXxf86vm.so" ]; then
    echo "  export CGO_LDFLAGS=\"-L$SDK_DIR/sysroot/lib\""
fi
