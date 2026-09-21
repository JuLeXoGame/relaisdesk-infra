#!/usr/bin/env bash
# September 2026 auth fix: API + one Nginx header. No clients, VPN, OS or billing changes.
# All passwords, private configuration and database backups remain on Oracle.
set -Eeuo pipefail
umask 077
[[ $EUID -eq 0 && $# -eq 7 ]] || { echo 'Usage (root): SCRIPT prepare|deploy PATH API_SHA DBCHECK_SHA NGINX_SHA PREVIOUS_API_SHA PREVIOUS_NGINX_SHA' >&2; exit 1; }
mode=$1
location=$(realpath -e -- "$2")
api_sha=$3
dbcheck_sha=$4
nginx_sha=$5
previous_api_sha=$6
previous_nginx_sha=$7
for digest in "$api_sha" "$dbcheck_sha" "$nginx_sha" "$previous_api_sha" "$previous_nginx_sha"; do
    [[ $digest =~ ^[a-f0-9]{64}$ ]] || exit 1
done
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9 || { echo 'Another deployment is in progress' >&2; exit 1; }
target=/opt/relaisdesk/api/api
db=/data/relaisdesk/licences.db
nginx_config=/etc/nginx/sites-available/relaisdesk-api
service=relaisdesk-api.service
[[ -f $target && -f $db && -f $nginx_config && ! -L $target && ! -L $db && ! -L $nginx_config ]]
systemctl is-active --quiet "$service"
pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "$target" | awk '{print $1}') == "$previous_api_sha" ]]
[[ $(sha256sum "/proc/$pid/exe" | awk '{print $1}') == "$previous_api_sha" ]]
[[ $(sha256sum "$nginx_config" | awk '{print $1}') == "$previous_nginx_sha" ]]

local_get() {
    curl --fail --silent --show-error --max-time 4 --resolve api.relaisdesk.fr:8443:127.0.0.1 "https://api.relaisdesk.fr:8443$1"
}
business_counts() {
    sqlite3 -readonly "$1" "SELECT 'licences',COUNT(*) FROM licences UNION ALL SELECT 'devices',COUNT(*) FROM devices UNION ALL SELECT 'orders',COUNT(*) FROM orders UNION ALL SELECT 'accounts',COUNT(*) FROM customer_accounts;"
}
check_migration() {
    [[ $(sqlite3 -readonly "$1" "SELECT COUNT(*) FROM security_migrations WHERE name='20260912-auth-v1';") == 1 ]]
    [[ $(sqlite3 -readonly "$1" "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('totp_enrollments','used_totp_codes');") == 2 ]]
    [[ $(sqlite3 -readonly "$1" 'PRAGMA integrity_check;') == ok ]]
    [[ -z $(sqlite3 -readonly "$1" 'PRAGMA foreign_key_check;') ]]
}

if [[ $mode == prepare ]]; then
    [[ $location == /home/ubuntu/relaisdesk-api-security-20260912-* && -d $location ]]
    [[ $(sha256sum "$location/api" | awk '{print $1}') == "$api_sha" ]]
    [[ $(sha256sum "$location/relaisdesk-dbcheck" | awk '{print $1}') == "$dbcheck_sha" ]]
    [[ $(sha256sum "$location/nginx-api.candidate" | awk '{print $1}') == "$nginx_sha" ]]
    # Enforce a one-line Nginx change, preserving all current listeners/settings.
    diff -u "$nginx_config" <(sed '/^[[:space:]]*proxy_set_header CF-Connecting-IP "";$/d' "$location/nginx-api.candidate")
    install -d -o root -g relaisdesk -m 0750 /opt/relaisdesk/deployments
    release=$(mktemp -d /opt/relaisdesk/deployments/security-20260912-XXXXXXXX)
    chown root:relaisdesk "$release"
    chmod 0750 "$release"
    install -o root -g root -m 0700 "$target" "$release/api.previous"
    install -o root -g root -m 0600 "$nginx_config" "$release/nginx-api.previous"
    install -o root -g relaisdesk -m 0750 "$location/api" "$release/api.candidate"
    install -o root -g relaisdesk -m 0750 "$location/relaisdesk-dbcheck" "$release/relaisdesk-dbcheck"
    install -o root -g root -m 0600 "$location/nginx-api.candidate" "$release/nginx-api.candidate"
    [[ $(sha256sum "$release/api.candidate" | awk '{print $1}') == "$api_sha" ]]
    [[ $(sha256sum "$release/relaisdesk-dbcheck" | awk '{print $1}') == "$dbcheck_sha" ]]
    [[ $(sha256sum "$release/nginx-api.candidate" | awk '{print $1}') == "$nginx_sha" ]]
    install -d -o relaisdesk -g relaisdesk -m 0700 "$release/preflight"
    sqlite3 -readonly "$db" ".backup '$release/preflight/licences.db'"
    chown relaisdesk:relaisdesk "$release/preflight/licences.db"
    chmod 0600 "$release/preflight/licences.db"
    counts=$(business_counts "$release/preflight/licences.db")
    runuser -u relaisdesk -- "$release/relaisdesk-dbcheck" -db "$release/preflight/licences.db"
    check_migration "$release/preflight/licences.db"
    [[ $(business_counts "$release/preflight/licences.db") == "$counts" ]]
    [[ $(sqlite3 -readonly "$release/preflight/licences.db" 'SELECT (SELECT COUNT(*) FROM admin_sessions)+(SELECT COUNT(*) FROM customer_sessions)+(SELECT COUNT(*) FROM technician_sessions)+(SELECT COUNT(*) FROM customer_login_tokens)+(SELECT COUNT(*) FROM customer_2fa_challenges)+(SELECT COUNT(*) FROM technician_2fa_challenges);') == 0 ]]
    nginx -t
    echo "PREFLIGHT_OK release=$release"
    exit 0
fi

[[ $mode == deploy && $location == /opt/relaisdesk/deployments/security-20260912-* && -d $location ]]
release=$location
[[ $(stat -c '%U:%G:%a' "$release") == root:relaisdesk:750 ]]
[[ $(sha256sum "$release/api.candidate" | awk '{print $1}') == "$api_sha" ]]
[[ $(sha256sum "$release/relaisdesk-dbcheck" | awk '{print $1}') == "$dbcheck_sha" ]]
[[ $(sha256sum "$release/nginx-api.candidate" | awk '{print $1}') == "$nginx_sha" ]]
[[ $(sha256sum "$release/api.previous" | awk '{print $1}') == "$previous_api_sha" ]]
[[ $(sha256sum "$release/nginx-api.previous" | awk '{print $1}') == "$previous_nginx_sha" ]]
check_migration "$release/preflight/licences.db"
diff -u "$nginx_config" <(sed '/^[[:space:]]*proxy_set_header CF-Connecting-IP "";$/d' "$release/nginx-api.candidate")

# These digests stay in memory and are compared, never printed.
unchanged_files=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf)
while IFS= read -r -d '' file; do unchanged_files+=("$file"); done < <(find /opt/relaisdesk/downloads -maxdepth 1 -type f -print0 | sort -z)
config_before=$(sha256sum "${unchanged_files[@]}")
containers_before=$(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr)
vpn_before=$(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value)
pricing_before=$(local_get /api/v1/public/pricing)
trials_before=$(local_get /api/v1/public/trials)
service_touched=0
nginx_touched=0
rollback() {
    code=$?
    trap - ERR
    set +e
    echo "DEPLOY_FAILED: rollback files are in $release" >&2
    if [[ $nginx_touched -eq 1 ]]; then
        install -o root -g root -m 0644 "$release/nginx-api.previous" "$nginx_config"
        nginx -t && systemctl reload nginx
    fi
    if [[ $service_touched -eq 1 ]]; then
        systemctl stop "$service"
        recovery=$(mktemp /opt/relaisdesk/api/.security-rollback-XXXXXXXX)
        if install -o root -g root -m 0755 "$release/api.previous" "$recovery" && mv -f -- "$recovery" "$target"; then
            systemctl start "$service"
            systemctl is-active "$service"
        else
            echo 'ROLLBACK_FAILED: manual recovery needed' >&2
        fi
        echo 'Database retained without rewinding business writes; old sessions remain invalidated.' >&2
    fi
    exit "$code"
}
trap rollback ERR

service_touched=1
systemctl stop "$service"
sqlite3 -readonly "$db" ".backup '$release/licences.before.db'"
chmod 0600 "$release/licences.before.db"
[[ $(sqlite3 -readonly "$release/licences.before.db" 'PRAGMA integrity_check;') == ok ]]
counts_before=$(business_counts "$db")
candidate=$(mktemp /opt/relaisdesk/api/.security-next-XXXXXXXX)
install -o root -g root -m 0755 "$release/api.candidate" "$candidate"
mv -f -- "$candidate" "$target"
nginx_touched=1
install -o root -g root -m 0644 "$release/nginx-api.candidate" "$nginx_config"
nginx -t
systemctl reload nginx
systemctl start "$service"

healthy=0
for attempt in $(seq 1 20); do
    if local_get /api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected"' 2>/dev/null; then
        healthy=1
        break
    fi
    sleep 1
done
[[ $healthy -eq 1 ]]
new_pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "/proc/$new_pid/exe" | awk '{print $1}') == "$api_sha" ]]
pricing_after=$(local_get /api/v1/public/pricing)
trials_after=$(local_get /api/v1/public/trials)
python3 -c 'import json,sys; assert json.loads(sys.argv[1])==json.loads(sys.argv[2]); assert json.loads(sys.argv[3])==json.loads(sys.argv[4]); print("PRICES_QUOTAS_TRIALS_AND_LEGAL_UNCHANGED")' "$pricing_before" "$pricing_after" "$trials_before" "$trials_after"
check_migration "$db"
# New legitimate writes could occur after restart: never restore the DB for a count change.
echo 'BUSINESS_COUNTS_BEFORE'
printf '%s\n' "$counts_before"
echo 'BUSINESS_COUNTS_AFTER'
business_counts "$db"
[[ $(sha256sum "${unchanged_files[@]}") == "$config_before" ]]
[[ $(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr) == "$containers_before" ]]
[[ $(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value) == "$vpn_before" ]]
systemctl is-active --quiet wg-quick@wg0.service
systemctl show "$service" -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
ss -lntp '( sport = :8443 )'
trap - ERR
echo "DEPLOY_OK api_sha256=$api_sha rollback=$release"
