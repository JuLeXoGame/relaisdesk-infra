#!/usr/bin/env bash
set -euo pipefail

useradd -r -s /sbin/nologin relaisdesk 2>/dev/null || true
mkdir -p /opt/relaisdesk/api /opt/relaisdesk/downloads /data/relaisdesk /var/log/relaisdesk /var/backups/relaisdesk /etc/relaisdesk
chown -R root:root /opt/relaisdesk
chmod 755 /opt/relaisdesk /opt/relaisdesk/api
chown root:relaisdesk /opt/relaisdesk/downloads
chmod 750 /opt/relaisdesk/downloads
chown -R relaisdesk:relaisdesk /data/relaisdesk /var/log/relaisdesk /var/backups/relaisdesk
chown root:relaisdesk /etc/relaisdesk
chmod 750 /data/relaisdesk /var/log/relaisdesk /var/backups/relaisdesk /etc/relaisdesk
