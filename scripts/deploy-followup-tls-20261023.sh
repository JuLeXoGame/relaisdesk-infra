#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]]
exec 9>/run/lock/relaisdesk-nginx-deploy.lock
flock -n 9
config=/etc/nginx/sites-available/relaisdesk-api
[[ -f $config && ! -L $config ]]
curl -fsS --max-time 10 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health >/dev/null
release=$(mktemp -d /opt/relaisdesk/deployments/followup-tls-20261023-XXXXXXXX)
cp -p "$config" "$release/nginx.previous"
rollback() {
    code=$?
    trap - ERR
    cp -p "$release/nginx.previous" "$config"
    nginx -t && systemctl reload nginx
    echo "TLS_DEPLOY_FAILED rollback=$release" >&2
    exit "$code"
}
trap rollback ERR
python3 - "$config" <<'PY'
import pathlib, sys
p=pathlib.Path(sys.argv[1]); s=p.read_text()
assert s.count('proxy_ssl_verify off;') == 2
assert s.count('proxy_ssl_trusted_certificate /etc/ssl/certs/ca-certificates.crt;') == 2
assert s.count('proxy_ssl_name api.relaisdesk.fr;') == 2
p.write_text(s.replace('proxy_ssl_verify off;', 'proxy_ssl_verify on;'))
PY
nginx -t
systemctl reload nginx
sleep 3
curl -fsS --max-time 10 --resolve api.relaisdesk.fr:443:127.0.0.1 https://api.relaisdesk.fr/api/v1/health
curl -fsS --max-time 10 --resolve api.relaisdesk.fr:443:127.0.0.1 https://api.relaisdesk.fr/api/v1/downloads/SHA256SUMS.txt >/dev/null
trap - ERR
echo "TLS_DEPLOY_OK rollback=$release"
