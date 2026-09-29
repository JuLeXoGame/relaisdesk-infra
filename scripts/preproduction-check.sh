#!/usr/bin/env bash
# RelaisDesk - Controle de preproduction (portage de preproduction-check.ps1).
# Usage :
#   bash scripts/preproduction-check.sh [--live-api-url URL] [--rustdesk-host H]
#       [--rendezvous-port P] [--relay-port P] [--email-domain D] [--dkim-selector S]
set -euo pipefail

LIVE_API_URL=""
RUSTDESK_HOST="api.relaisdesk.fr"
RENDEZVOUS_PORT=21116
RELAY_PORT=21117
EMAIL_DOMAIN="relaisdesk.fr"
DKIM_SELECTOR=""

while [ $# -gt 0 ]; do
    case "$1" in
        --live-api-url) LIVE_API_URL="$2"; shift 2 ;;
        --rustdesk-host) RUSTDESK_HOST="$2"; shift 2 ;;
        --rendezvous-port) RENDEZVOUS_PORT="$2"; shift 2 ;;
        --relay-port) RELAY_PORT="$2"; shift 2 ;;
        --email-domain) EMAIL_DOMAIN="$2"; shift 2 ;;
        --dkim-selector) DKIM_SELECTOR="$2"; shift 2 ;;
        -h|--help)
            sed -n '2,6p' "$0"
            exit 0
            ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_MODULES="database api tests keygen installer/configurator installer/viewer installer/debpack dashboard_admin"

for module in $GO_MODULES; do
    echo "[TEST] $module"
    ( cd "$PROJECT_ROOT/$module" && go test ./... ) || { echo "Échec des tests Go dans $module" >&2; exit 1; }
done

if [ -n "$DKIM_SELECTOR" ]; then
    echo "[EMAIL] Vérification ponctuelle SPF/DKIM/DMARC"
    ( cd "$PROJECT_ROOT/api" && go run ./cmd/emailcheck -domain "$EMAIL_DOMAIN" -selector "$DKIM_SELECTOR" -strict=true ) \
        || { echo "Échec de l'authentification DNS des courriels" >&2; exit 1; }
fi

echo "[TEST] rustdesk-server (autorisation et quota simultané)"
( cd "$PROJECT_ROOT/rustdesk-server" && cargo test --locked --lib relaisdesk_auth ) \
    || { echo "Échec des tests du serveur RustDesk" >&2; exit 1; }

if [ -n "$LIVE_API_URL" ]; then
    HEALTH_URL="${LIVE_API_URL%/}/api/v1/health"
    echo "[LIVE] $HEALTH_URL"
    STATUS=$(curl -fsS -m 10 "$HEALTH_URL" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status',''))")
    if [ "$STATUS" != "healthy" ]; then echo "API live dégradée" >&2; exit 1; fi

    for port in "$RENDEZVOUS_PORT" "$RELAY_PORT"; do
        if ! timeout 10 bash -c "cat < /dev/null > /dev/tcp/$RUSTDESK_HOST/$port" 2>/dev/null; then
            echo "Port TCP $RUSTDESK_HOST:$port inaccessible" >&2; exit 1
        fi
    done
fi

echo "Préproduction validée : achat, activation, autorisations signées et quota simultané."
