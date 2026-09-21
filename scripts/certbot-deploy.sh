#!/usr/bin/env bash
# Hook de déploiement pour certbot
# À copier dans /etc/letsencrypt/renewal-hooks/deploy/relaisdesk.sh
# Ce script est exécuté automatiquement après chaque renouvellement réussi.
set -euo pipefail

CERT_DIR="/opt/relaisdesk/certs"
LE_LIVE="/etc/letsencrypt/live/api.relaisdesk.fr"

if [[ ! -r "$LE_LIVE/fullchain.pem" || ! -r "$LE_LIVE/privkey.pem" ]]; then
    echo "Certificat api.relaisdesk.fr introuvable" >&2
    exit 1
fi

mkdir -p "$CERT_DIR"
chown root:relaisdesk "$CERT_DIR"
chmod 750 "$CERT_DIR"

cp "$LE_LIVE/fullchain.pem" "$CERT_DIR/fullchain.pem"
cp "$LE_LIVE/privkey.pem"   "$CERT_DIR/privkey.pem"

chown root:relaisdesk "$CERT_DIR/fullchain.pem" "$CERT_DIR/privkey.pem"
chmod 644 "$CERT_DIR/fullchain.pem"
chmod 640 "$CERT_DIR/privkey.pem"

# Redémarrer l'API si elle est déjà en service. Lors de la toute première
# émission, le hook doit pouvoir préparer les fichiers avant son démarrage.
api_action="non redémarrée"
if [[ "${RELAISDESK_SKIP_SERVICE_RESTART:-0}" != "1" ]] && systemctl is-active --quiet relaisdesk-api; then
    systemctl restart relaisdesk-api
    api_action="redémarrée"
fi
if systemctl is-active --quiet nginx; then
    if nginx -t >/dev/null 2>&1; then
        systemctl reload nginx
    else
        echo "[$(date)] ERREUR: nginx -t a échoué, rechargement Nginx annulé." >&2
    fi
fi

mkdir -p /var/log/relaisdesk
echo "[$(date)] Certificats déployés ; API ${api_action}, Nginx rechargé s'il est actif." >> /var/log/relaisdesk/certbot.log
