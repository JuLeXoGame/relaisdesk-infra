#!/usr/bin/env bash
# RelaisDesk - Preparation du paquet natif fleet (portage de prepare-fleet-payload.ps1).
# Usage : bash scripts/prepare-fleet-payload.sh <rustdesk.exe natif> <destination>
# Affiche le SHA-256 (minuscules) de rustdesk.exe copie.
set -euo pipefail

if [ $# -ne 2 ]; then echo "Usage : $0 <rustdesk.exe natif> <destination>" >&2; exit 1; fi
NATIVE="$1"
DEST="$2"

if [ ! -f "$NATIVE" ] || [ -L "$NATIVE" ] || [ "$(basename "$NATIVE")" != "rustdesk.exe" ]; then
    echo "Un rustdesk.exe natif, issu de la compilation du fork, est requis." >&2; exit 1
fi
SRC_DIR="$(dirname "$NATIVE")"
# Tous les intrants requis sont verifies avant toute copie ; jamais d'execution
# du wrapper portable pour obtenir les composants depuis un cache.
for name in rustdesk.exe sciter.dll dylib_virtual_display.dll; do
    src="$SRC_DIR/$name"
    if [ ! -f "$src" ] || [ -L "$src" ] || [ "$(stat -c %s "$src")" -lt 2 ]; then
        echo "Composant natif invalide : $name" >&2; exit 1
    fi
    if [ "$(head -c 2 "$src" | od -An -tx1 | tr -d ' \n')" != "4d5a" ]; then
        echo "Composant PE invalide : $name" >&2; exit 1
    fi
done
mkdir -p "$DEST"
if [ ! -d "$DEST" ] || [ -L "$DEST" ]; then echo "Le dossier du paquet natif ne doit pas être un lien." >&2; exit 1; fi
for entry in "$DEST"/*; do
    [ -e "$entry" ] || continue
    base="$(basename "$entry")"
    case "$base" in
        rustdesk.exe|sciter.dll|dylib_virtual_display.dll|README.txt) ;;
        *) echo "Fichier inattendu dans le paquet natif : $base" >&2; exit 1 ;;
    esac
    if [ -d "$entry" ] || [ -L "$entry" ]; then echo "Composant de destination non sûr : $base" >&2; exit 1; fi
done
for name in rustdesk.exe sciter.dll dylib_virtual_display.dll; do
    cp -f "$SRC_DIR/$name" "$DEST/$name"
done
sha256sum "$DEST/rustdesk.exe" | awk '{print $1}'
