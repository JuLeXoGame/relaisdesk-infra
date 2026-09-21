#!/usr/bin/env bash
# Installation et activation de Fail2ban pour RelaisDesk sur Oracle Linux / Ubuntu VPS
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
   echo "Ce script doit être exécuté en root (sudo)" >&2
   exit 1
fi

echo "=== Installation et configuration de Fail2ban pour RelaisDesk ==="

if ! command -v fail2ban-client >/dev/null 2>&1; then
    echo "Installation de Fail2ban..."
    apt-get update
    apt-get install -y fail2ban
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

mkdir -p /etc/fail2ban/filter.d /etc/fail2ban/jail.d /var/log/relaisdesk

# Installer le filtre et la prison
cp "${SCRIPT_DIR}/fail2ban/filter.d/relaisdesk-api.conf" /etc/fail2ban/filter.d/relaisdesk-api.conf
cp "${SCRIPT_DIR}/fail2ban/jail.d/relaisdesk-api.local" /etc/fail2ban/jail.d/relaisdesk-api.local

chmod 644 /etc/fail2ban/filter.d/relaisdesk-api.conf /etc/fail2ban/jail.d/relaisdesk-api.local

# S'assurer que le fichier de log existe pour que fail2ban puisse le surveiller
touch /var/log/relaisdesk/api.log
chown relaisdesk:relaisdesk /var/log/relaisdesk/api.log 2>/dev/null || true
chmod 640 /var/log/relaisdesk/api.log

# Activer et redémarrer fail2ban
systemctl enable fail2ban
systemctl restart fail2ban

echo "Attente de l'initialisation de Fail2ban..."
sleep 2

echo "=== Statut de la prison relaisdesk-api ==="
fail2ban-client status relaisdesk-api

echo "Configuration Fail2ban terminée avec succès !"
