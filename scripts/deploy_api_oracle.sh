#!/usr/bin/env bash
# =============================================================================
# RelaisDesk - Deploiement API prebuilt sur Oracle (transfert + restart + verifs)
# =============================================================================
# A executer depuis un shell avec acces SSH direct (WSL interactif) :
#
#   bash scripts/deploy_api_oracle.sh --vps-target "ubuntu@79.72.27.213"
#
# Cle SSH par defaut : ~/.ssh/oracle (format OpenSSH). La convertir une fois
# depuis .secrets si besoin, puis :
#   install -m 0600 .secrets/oracle_openssh ~/.ssh/oracle
#
# Le script transfere prebuilt/api (5 fichiers), les installe avec rollback,
# redemarre relaisdesk-api et verifie sante + empreinte du binaire actif.
# Procedure miroir de la phase 2 de scripts/deploy-interventions-20260920.sh.
# =============================================================================
set -euo pipefail

VPS_TARGET=""
SSH_KEY="$HOME/.ssh/oracle"
STAMP="api-$(date +%Y%m%d)"

while [ $# -gt 0 ]; do
    case "$1" in
        --vps-target) VPS_TARGET="$2"; shift 2 ;;
        --ssh-key) SSH_KEY="$2"; shift 2 ;;
        --stamp) STAMP="$2"; shift 2 ;;
        -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if ! printf '%s' "$STAMP" | grep -Eq '^[A-Za-z0-9._-]+$'; then echo "Stamp invalide." >&2; exit 1; fi
if [ -z "$VPS_TARGET" ]; then echo "Parametre --vps-target requis (ex : ubuntu@79.72.27.213)" >&2; exit 1; fi
if [ ! -f "$SSH_KEY" ]; then echo "Cle introuvable : $SSH_KEY" >&2; exit 1; fi
command -v ssh >/dev/null || { echo "Commande introuvable : ssh" >&2; exit 1; }
command -v scp >/dev/null || { echo "Commande introuvable : scp" >&2; exit 1; }

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREBUILT_DIR="$ROOT/prebuilt/api"
SSH_OPTS=(-o BatchMode=yes -i "$SSH_KEY")

echo ""
echo "===> 1/3 Transfert API vers Oracle ($VPS_TARGET)"
for b in api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck SHA256SUMS.txt; do
    if [ ! -f "$PREBUILT_DIR/$b" ]; then echo "Fichier manquant : $PREBUILT_DIR/$b" >&2; exit 1; fi
done
ssh "${SSH_OPTS[@]}" "$VPS_TARGET" "mkdir -p /tmp/relaisdesk-$STAMP" \
    || { echo "Impossible de preparer /tmp sur Oracle" >&2; exit 1; }
scp "${SSH_OPTS[@]}" "$PREBUILT_DIR/api" "$PREBUILT_DIR/relaisdesk-backup" "$PREBUILT_DIR/relaisdesk-emailcheck" "$PREBUILT_DIR/relaisdesk-dbcheck" "$PREBUILT_DIR/SHA256SUMS.txt" "$VPS_TARGET:/tmp/relaisdesk-$STAMP/" \
    || { echo "Transfert scp de l'API echoue" >&2; exit 1; }
echo "  [OK] Transfert termine"

REMOTE_API=$(mktemp /tmp/relaisdesk-api-XXXXXXXX.sh)
trap 'rm -f "$REMOTE_API"' EXIT
sed "s/__STAMP__/$STAMP/g" > "$REMOTE_API" <<'REMOTE_EOF'
set -euo pipefail
STAMP="__STAMP__"
INCOMING="/tmp/relaisdesk-${STAMP}"
DEPLOY="/opt/relaisdesk/deployments/${STAMP}/rollback-api"
DB="/data/relaisdesk/licences.db"
cd "$INCOMING"
sha256sum --check SHA256SUMS.txt
sudo install -d -m 0700 -o root -g root "$DEPLOY"
for b in api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck; do
  if [ -f "/opt/relaisdesk/api/$b" ] && [ ! -f "$DEPLOY/$b" ]; then sudo cp -p "/opt/relaisdesk/api/$b" "$DEPLOY/$b"; fi
  sudo install -m 0755 "$INCOMING/$b" "/opt/relaisdesk/api/$b.new"
  sudo mv -f "/opt/relaisdesk/api/$b.new" "/opt/relaisdesk/api/$b"
done
if [ ! -f "$DEPLOY/licences-before.db" ]; then sudo sqlite3 "$DB" ".backup '$DEPLOY/licences-before.db'"; fi
sudo chmod 0600 "$DEPLOY/licences-before.db"
CHECK=$(sudo sqlite3 "$DEPLOY/licences-before.db" "PRAGMA integrity_check;")
[ "$CHECK" = "ok" ] || { echo "integrity_check: $CHECK"; exit 1; }
sudo systemctl restart relaisdesk-api
sleep 5
[ "$(systemctl is-active relaisdesk-api)" = "active" ] || { sudo systemctl status relaisdesk-api --no-pager; exit 1; }
curl --fail --silent --show-error --max-time 10 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health
echo ""
PID=$(systemctl show relaisdesk-api -p MainPID --value)
test "$PID" -gt 0
EXE_SHA=$(sudo sha256sum "/proc/$PID/exe" | awk '{print $1}')
DISK_SHA=$(sudo sha256sum /opt/relaisdesk/api/api | awk '{print $1}')
[ "$EXE_SHA" = "$DISK_SHA" ] || { echo "MISMATCH exe=$EXE_SHA disk=$DISK_SHA"; exit 1; }
echo "API ACTIVE exe_sha=$EXE_SHA"
REMOTE_EOF

echo ""
echo "===> 2/3 Installation et redemarrage API"
ssh "${SSH_OPTS[@]}" "$VPS_TARGET" 'bash -s' < "$REMOTE_API" \
    || { echo "Deploiement API echoue (rollback: /opt/relaisdesk/deployments/$STAMP/rollback-api/)" >&2; exit 1; }

echo ""
echo "===> 3/3 Sante publique"
curl -fsS -m 30 "https://api.relaisdesk.fr/api/v1/health" | head -c 300
echo ""
echo "  [OK] API deployee et saine sur Oracle"
