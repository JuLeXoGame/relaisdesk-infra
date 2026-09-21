#!/usr/bin/env bash
set -euo pipefail
uname -m
free -h
df -h / /opt
sudo systemctl show relaisdesk-api.service -p ActiveState -p SubState -p MainPID -p ExecMainStartTimestamp -p NRestarts --no-pager
pid=$(sudo systemctl show relaisdesk-api.service -p MainPID --value)
sudo sha256sum /opt/relaisdesk/api/api "/proc/$pid/exe" /etc/nginx/sites-available/relaisdesk-api
sudo docker ps --format '{{.Names}} {{.Image}} {{.Status}}'
sudo docker inspect --format '{{.Name}} image={{.Image}} started={{.State.StartedAt}} workdir={{.Config.WorkingDir}}{{range .Mounts}} mount={{.Source}}:{{.Destination}}{{end}}' rustdesk-hbbs rustdesk-hbbr
sudo docker compose -f /opt/rustdesk/docker-compose.yml config --images
sudo sed -n '1,180p' /opt/rustdesk/docker-compose.yml
sudo grep -hE '^(API_BIND|API_PORT|DB_PATH|SERVER_SOURCE_URL|CLIENT_SOURCE_URL|RELEASE_MANIFEST_PATH|RELEASE_PUBLIC_KEY)=' /etc/relaisdesk/api.env
sudo find /opt/relaisdesk/downloads -maxdepth 1 -type f -printf '%f %s\n' | sort
sudo find /etc/relaisdesk /opt/relaisdesk -maxdepth 2 -type f -iname '*release*' -printf '%p\n'
sudo sqlite3 -readonly /data/relaisdesk/licences.db "SELECT 'licences',COUNT(*) FROM licences UNION ALL SELECT 'devices',COUNT(*) FROM devices UNION ALL SELECT 'orders',COUNT(*) FROM orders UNION ALL SELECT 'accounts',COUNT(*) FROM customer_accounts UNION ALL SELECT 'mfa_accounts',COUNT(*) FROM customer_accounts WHERE totp_enabled=1;"
sudo sqlite3 -readonly /data/relaisdesk/licences.db "SELECT name FROM security_migrations ORDER BY name; PRAGMA quick_check;"
sudo systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --no-pager
sudo ss -lntp '( sport = :8443 )'
curl --fail --silent --show-error --max-time 10 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health
printf '\n'
