#!/usr/bin/env bash
# =============================================================================
# RelaisDesk - Miroir Oracle : RelaisDesk_Portable.exe + sommes + manifeste
# (portage de mirror-portable-oracle.ps1 pour Linux/WSL)
# =============================================================================
# A executer depuis le poste d'exploitation, dans sa propre fenetre :
#
#   bash scripts/mirror-portable-oracle.sh --vps-target "ubuntu@79.72.27.213"
#   bash scripts/mirror-portable-oracle.sh --vps-target "ubuntu@79.72.27.213" --full --stamp "release-1.0.0-20260924"
#   bash scripts/mirror-portable-oracle.sh --vps-target "ubuntu@79.72.27.213" --files "RelaisDesk_Viewer_Linux RelaisDesk_viewer.deb SHA256SUMS.txt release-manifest.json" --stamp "..."
#
# La cle par defaut est ~/.ssh/oracle (format OpenSSH). Convertir une fois
# le .ppk PuTTY via PuTTYgen (Windows) : Conversions -> Export OpenSSH key,
# puis : install -m 0600 cle-openssh ~/.ssh/oracle
#
# Par defaut, le script transfere 3 fichiers (RelaisDesk_Portable.exe et
# metadonnees). Avec --full, il transfere la release complete (12 binaires
# dont 4 DMG macOS + SHA256SUMS.txt + release-manifest.json). --files restreint
# au sous-ensemble nomme (noms valides contre la liste connue, SHA256SUMS.txt
# obligatoire). Dans tous les cas, il les
# installe dans /opt/relaisdesk/downloads (avec rollback), verifie les
# sommes cote serveur et controle que le manifeste public correspond au local.
# Les sommes entrantes sont controlees AVANT toute ecriture : un echec ne
# laisse jamais la production dans un etat mixte. --dry-run valide toute
# la chaine (fichiers, SSH, sudo distant, manifeste) sans rien transferer.
# =============================================================================
set -euo pipefail

VPS_TARGET=""
SSH_KEY="$HOME/.ssh/oracle"
STAMP="viewer-reenroll-20260921"
FULL=0
DRYRUN=0
FILES_OVERRIDE=""

while [ $# -gt 0 ]; do
    case "$1" in
        --vps-target) VPS_TARGET="$2"; shift 2 ;;
        --ssh-key) SSH_KEY="$2"; shift 2 ;;
        --stamp) STAMP="$2"; shift 2 ;;
        --full) FULL=1; shift ;;
        --files) FILES_OVERRIDE="$2"; shift 2 ;;
        --dry-run) DRYRUN=1; shift ;;
        -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if ! printf '%s' "$STAMP" | grep -Eq '^[A-Za-z0-9._-]+$'; then echo "Stamp invalide." >&2; exit 1; fi
if [ -z "$VPS_TARGET" ]; then echo "Parametre --vps-target requis (ex : ubuntu@1.2.3.4)" >&2; exit 1; fi
if [ ! -f "$SSH_KEY" ]; then echo "Cle introuvable : $SSH_KEY (convertir le .ppk PuTTY en cle OpenSSH, voir l'en-tete)" >&2; exit 1; fi

ROOT="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/.." && pwd)"
DOWNLOADS_DIR="$ROOT/relaisdesk/downloads"
SSH_OPTS=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_KEY")
FILES="RelaisDesk_Portable.exe SHA256SUMS.txt release-manifest.json"
if [ "$FULL" = "1" ]; then
    FILES="RelaisDesk_Portable.exe RelaisDesk_Setup.exe RelaisDesk_Technicien.deb RelaisDesk_Technicien_Linux RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux RelaisDesk_Mac.dmg RelaisDesk_Technicien_Mac.dmg RelaisDesk_Mac_Intel.dmg RelaisDesk_Technicien_Mac_Intel.dmg SHA256SUMS.txt release-manifest.json"
fi
if [ -n "$FILES_OVERRIDE" ]; then
    if [ "$FULL" = "1" ]; then echo "--files et --full sont exclusifs" >&2; exit 1; fi
    KNOWN="RelaisDesk_Portable.exe RelaisDesk_Setup.exe RelaisDesk_Technicien.deb RelaisDesk_Technicien_Linux RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux RelaisDesk_Mac.dmg RelaisDesk_Technicien_Mac.dmg RelaisDesk_Mac_Intel.dmg RelaisDesk_Technicien_Mac_Intel.dmg SHA256SUMS.txt release-manifest.json"
    for tok in $FILES_OVERRIDE; do
        case " $KNOWN " in *" $tok "*) ;; *) echo "Fichier inconnu : $tok" >&2; exit 1;; esac
    done
    case " $FILES_OVERRIDE " in *" SHA256SUMS.txt "*) ;; *) echo "SHA256SUMS.txt obligatoire avec --files" >&2; exit 1;; esac
    FILES="$FILES_OVERRIDE"
fi

for f in $FILES; do
    if [ ! -f "$DOWNLOADS_DIR/$f" ]; then echo "Fichier manquant : $DOWNLOADS_DIR/$f" >&2; exit 1; fi
done

if [ "$DRYRUN" = "1" ]; then
    echo "DRY-RUN : aucune écriture, vérifications seules."
    echo "-- fichiers locaux --"
    (cd "$DOWNLOADS_DIR" && sha256sum $FILES)
    echo "-- SSH + sudo distant --"
    ssh "${SSH_OPTS[@]}" "$VPS_TARGET" 'echo SSH_OK; sudo -n true && echo SUDO_NOPASSWD_OK; stat -c "%U:%G %a %n" /opt/relaisdesk/downloads'
    echo "-- manifeste public vs local --"
    LOCAL_SIG=$(python3 -c "import json; print(json.load(open('$DOWNLOADS_DIR/release-manifest.json'))['signature'])")
    REMOTE_SIG=$(curl -fsS -m 30 "https://api.relaisdesk.fr/api/v1/downloads/release-manifest.json" | python3 -c "import json,sys; print(json.load(sys.stdin)['signature'])")
    if [ "$REMOTE_SIG" = "$LOCAL_SIG" ]; then echo "manifeste: IDENTIQUE (miroir à jour)"; else echo "manifeste: DIVERGENT (miroir à pousser)"; fi
    echo "DRY-RUN OK"
    exit 0
fi

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
cleanup() { rm -rf "\$INCOMING"; }
trap cleanup EXIT
for f in \$FILES; do [ -f "\$INCOMING/\$f" ] || { echo "manquant: \$f"; exit 1; }; done
case " \$FILES " in *" SHA256SUMS.txt "*) ;; *) echo "SHA256SUMS.txt non transféré"; exit 1;; esac
# Pré-vérification AVANT toute écriture : chaque binaire entrant doit
# correspondre à la somme entrée. Un échec ici ne touche pas la production.
for f in \$FILES; do
  case "\$f" in SHA256SUMS.txt|release-manifest.json) continue;; esac
  want=\$(awk -v f="\$f" '\$2==f{print \$1; exit}' "\$INCOMING/SHA256SUMS.txt")
  [ -n "\$want" ] || { echo "pas d'entrée SHA pour: \$f"; exit 1; }
  got=\$(sha256sum "\$INCOMING/\$f" | awk '{print \$1}')
  [ "\$got" = "\$want" ] || { echo "somme entrante invalide: \$f"; exit 1; }
done
sudo install -d -m 0700 -o root -g root "\$DEPLOY"
for f in \$FILES; do if [ -f "\$TARGET/\$f" ] && [ ! -f "\$DEPLOY/\$f" ]; then sudo cp -p "\$TARGET/\$f" "\$DEPLOY/\$f"; fi; sudo install -m 0644 --owner=\$(stat -c "%U" "\$TARGET") --group=\$(stat -c "%G" "\$TARGET") "\$INCOMING/\$f" "\$TARGET/\$f.new"; sudo mv -f "\$TARGET/\$f.new" "\$TARGET/\$f"; done
# Vérification post-install : uniquement les fichiers transférés (l'arbre
# complet n'est garanti cohérent qu'en mode --full).
for f in \$FILES; do
  case "\$f" in SHA256SUMS.txt|release-manifest.json) continue;; esac
  want=\$(awk -v f="\$f" '\$2==f{print \$1; exit}' "\$TARGET/SHA256SUMS.txt")
  got=\$(sha256sum "\$TARGET/\$f" | awk '{print \$1}')
  [ "\$got" = "\$want" ] || { echo "somme installée invalide: \$f"; exit 1; }
done
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
