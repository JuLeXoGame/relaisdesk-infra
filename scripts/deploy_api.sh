#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
cd api
go build -o ../bin/api .
go build -o ../bin/relaisdesk-backup ./cmd/backup
go build -o ../bin/relaisdesk-emailcheck ./cmd/emailcheck
go build -o ../bin/relaisdesk-dbcheck ./cmd/dbcheck
install -D -m 0755 ../bin/api /opt/relaisdesk/api/api
install -D -m 0755 ../bin/relaisdesk-backup /opt/relaisdesk/api/relaisdesk-backup
install -D -m 0755 ../bin/relaisdesk-emailcheck /opt/relaisdesk/api/relaisdesk-emailcheck
install -D -m 0755 ../bin/relaisdesk-dbcheck /opt/relaisdesk/api/relaisdesk-dbcheck
install -D -m 0644 relaisdesk-api.service /etc/systemd/system/relaisdesk-api.service
install -D -m 0644 relaisdesk-backup.service /etc/systemd/system/relaisdesk-backup.service
install -D -m 0644 relaisdesk-backup.timer /etc/systemd/system/relaisdesk-backup.timer
install -D -m 0644 ../scripts/relaisdesk.logrotate /etc/logrotate.d/relaisdesk
cp -n .env.example /etc/relaisdesk/api.env 2>/dev/null || true
cp -n ../scripts/backup.env.example /etc/relaisdesk/backup.env 2>/dev/null || true
chown root:relaisdesk /etc/relaisdesk/api.env
chmod 640 /etc/relaisdesk/api.env
chown root:relaisdesk /etc/relaisdesk/backup.env
chmod 640 /etc/relaisdesk/backup.env
if [ ! -f /etc/relaisdesk/backup.key ]; then
  /opt/relaisdesk/api/relaisdesk-backup keygen -out /etc/relaisdesk/backup.key
fi
chown root:relaisdesk /etc/relaisdesk/backup.key
chmod 640 /etc/relaisdesk/backup.key
systemctl daemon-reload
systemctl enable --now relaisdesk-backup.timer
