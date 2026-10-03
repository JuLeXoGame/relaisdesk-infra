#!/usr/bin/env bash
# =============================================================================
# RelaisDesk - Miroir Oracle : RelaisDesk_Portable.exe + sommes + manifeste
# (portage de mirror-portable-oracle.ps1 pour Linux/WSL)
# =============================================================================
# A executer depuis le poste d'exploitation, dans sa propre fenetre :
#
#   bash scripts/mirror-portable-oracle.sh --vps-target "ubuntu@79.72.27.213"
#   bash scripts/mirror-portable-oracle.sh --vps-target "ubuntu@79.72.27.213" --full --stamp "release-1.0.0-20260924"
#
# La cle par defaut est ~/.ssh/oracle (format OpenSSH). Convertir une fois
# le .ppk PuTTY via PuTTYgen (Windows) : Conversions -> Export OpenSSH key,
# puis : install -m 0600 cle-openssh ~/.ssh/oracle
#
# Par defaut, le script transfere 3 fichiers (RelaisDesk_Portable.exe et
# metadonnees). Avec --full, il transfere la release complete (12 binaires
# dont 4 DMG macOS + SHA256SUMS.txt + release-manifest.json). Dans les deux cas, il les
# installe dans /opt/relaisdesk/downloads (avec rollback), verifie les
# sommes cote serveur et controle que le manifeste public correspond au local.
# =============================================================================
set -euo pipefail

VPS_TARGET=""
SSH_KEY="$HOME/.ssh/oracle"
STAMP="viewer-reenroll-20260921"
FULL=0

while [ $# -gt 0 ]; do
    case "$1" in
        --vps-target) VPS_TARGET="$2"; shift 2 ;;
        --ssh-key) SSH_KEY="$2"; shift 2 ;;
        --stamp) STAMP="$2"; shift 2 ;;
        --full) FULL=1; shift ;;
        -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if ! printf '%s' "$STAMP" | grep -Eq '^[A-Za-z0-9._-]+$'; then echo "Stamp invalide." >&2; exit 1; fi
if [ -z "$VPS_TARGET" ]; then echo "Parametre --vps-target requis (ex : ubuntu@1.2.3.4)" >&2; exit 1; fi
if [ ! -f "$SSH_KEY" ]; then echo "Cle introuvable : $SSH_KEY (convertir le .ppk PuTTY en cle OpenSSH, voir l'en-tete)" >&2; exit 1; fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOWNLOADS_DIR="$ROOT/relaisdesk/downloads"
SSH_OPTS=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_KEY")
FILES="RelaisDesk_Portable.exe SHA256SUMS.txt release-manifest.json"
if [ "$FULL" = "1" ]; then
    FILES="RelaisDesk_Portable.exe RelaisDesk_Setup.exe RelaisDesk_Technicien.deb RelaisDesk_Technicien_Linux RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux RelaisDesk_Mac.dmg RelaisDesk_Technicien_Mac.dmg RelaisDesk_Mac_Intel.dmg RelaisDesk_Technicien_Mac_Intel.dmg SHA256SUMS.txt release-manifest.json"
fi

for f in $FILES; do
    if [ ! -f "$DOWNLOADS_DIR/$f" ]; then echo "Fichier manquant : $DOWNLOADS_DIR/$f" >&2; exit 1; fi
done

echo "===> 1/3 Transfert vers Oracle ($VPS_TARGET)"
ssh "${SSH_OPTS[@]}" "$VPS_TARGET" "mkdir -p /tmp/relaisdesk-$STAMP-downloads" \
    || { echo "ssh mkdir a echoue" >&2; exit 1; }
for f in $FILES; do
    echo "  Envoi $f ..."
    scp "${SSH_OPTS[@]}" "$DOWNLOADS_DIR/$f" "$VPS_TARGET:/tmp/relaisdesk-$STAMP-downloads/" \
        || { echo "Transfert echoue : $f" >&2; exit 1; }
done
echo "  [OK] Transfert termine"

echo "===> 2/3 Installation dans /opt/relaisdesk/downloads"
REMOTE_TMP=$(mktemp /tmp/relaisdesk-dl-XXXXXXXX.sh)
trap 'rm -f "$REMOTE_TMP"' EXIT
cat > "$REMOTE_TMP" <<EOF
set -euo pipefail
STAMP="$STAMP"
INCOMING="/tmp/relaisdesk-\${STAMP}-downloads"
TARGET="/opt/relaisdesk/downloads"
DEPLOY="/opt/relaisdesk/deployments/\${STAMP}/rollback-downloads"
FILES="$FILES"
sudo install -d -m 0700 -o root -g root "\$DEPLOY"
for f in \$FILES; do [ -f "\$INCOMING/\$f" ] || { echo "manquant: \$f"; exit 1; }; if [ -f "\$TARGET/\$f" ] && [ ! -f "\$DEPLOY/\$f" ]; then sudo cp -p "\$TARGET/\$f" "\$DEPLOY/\$f"; fi; sudo install -m 0644 --owner=\$(stat -c "%U" "\$TARGET") --group=\$(stat -c "%G" "\$TARGET") "\$INCOMING/\$f" "\$TARGET/\$f.new"; sudo mv -f "\$TARGET/\$f.new" "\$TARGET/\$f"; done
(cd "\$TARGET" && sudo -u "\$(stat -c "%U" .)" sha256sum --check SHA256SUMS.txt)
rm -rf "\$INCOMING"
echo "DOWNLOADS OK"
EOF
ssh "${SSH_OPTS[@]}" "$VPS_TARGET" 'bash -s' < "$REMOTE_TMP" \
    || { echo "Installation cote serveur echouee" >&2; exit 1; }
echo "  [OK] Miroir installe et sommes verifiees"

echo "===> 3/3 Verification du manifeste public"
LOCAL_SIG=$(python3 -c "import json; print(json.load(open('$DOWNLOADS_DIR/release-manifest.json'))['signature'])")
REMOTE_SIG=$(curl -fsS -m 30 "https://api.relaisdesk.fr/api/v1/downloads/release-manifest.json" | python3 -c "import json,sys; print(json.load(sys.stdin)['signature'])")
if [ "$REMOTE_SIG" != "$LOCAL_SIG" ]; then echo "Manifeste public different du manifeste signe local" >&2; exit 1; fi
echo "  [OK] Miroir Oracle a jour et verifie"
