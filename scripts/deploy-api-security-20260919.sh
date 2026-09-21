#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID -eq 0 && $# -eq 5 ]] || { echo 'Usage: prepare|deploy DIRECTORY API_SHA DBCHECK_SHA PREVIOUS_API_SHA' >&2; exit 1; }
mode=$1
location=$(realpath -e -- "$2")
api_sha=$3
dbcheck_sha=$4
previous_sha=$5
for digest in "$api_sha" "$dbcheck_sha" "$previous_sha"; do [[ $digest =~ ^[a-f0-9]{64}$ ]]; done
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9
target=/opt/relaisdesk/api/api
db=/data/relaisdesk/licences.db
service=relaisdesk-api.service
[[ -f $target && ! -L $target && -f $db && ! -L $db ]]
systemctl is-active --quiet "$service"
pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "$target" | awk '{print $1}') == "$previous_sha" ]]
[[ $(sha256sum "/proc/$pid/exe" | awk '{print $1}') == "$previous_sha" ]]
local_get() { curl -fsS --max-time 5 --resolve api.relaisdesk.fr:8443:127.0.0.1 "https://api.relaisdesk.fr:8443$1"; }
counts() { sqlite3 -readonly "$1" "SELECT 'licences',COUNT(*) FROM licences UNION ALL SELECT 'devices',COUNT(*) FROM devices UNION ALL SELECT 'orders',COUNT(*) FROM orders UNION ALL SELECT 'accounts',COUNT(*) FROM customer_accounts;"; }
check_migration() {
    [[ $(sqlite3 -readonly "$1" "SELECT COUNT(*) FROM security_migrations WHERE name='20260919-google-mfa-v1';") == 1 ]]
    [[ $(sqlite3 -readonly "$1" "SELECT COUNT(*) FROM pragma_table_info('customer_2fa_challenges') WHERE name='email_code_allowed';") == 1 ]]
    [[ $(sqlite3 -readonly "$1" 'PRAGMA integrity_check;') == ok ]]
    [[ -z $(sqlite3 -readonly "$1" 'PRAGMA foreign_key_check;') ]]
}
if [[ $mode == prepare ]]; then
    [[ $location == /home/ubuntu/relaisdesk-security-20260919-* && -d $location ]]
    [[ $(sha256sum "$location/api" | awk '{print $1}') == "$api_sha" ]]
    [[ $(sha256sum "$location/relaisdesk-dbcheck" | awk '{print $1}') == "$dbcheck_sha" ]]
    install -d -o root -g relaisdesk -m 0750 /opt/relaisdesk/deployments
    release=$(mktemp -d /opt/relaisdesk/deployments/security-20260919-XXXXXXXX)
    chown root:relaisdesk "$release"
    chmod 0750 "$release"
    install -o root -g root -m 0700 "$target" "$release/api.previous"
    install -o root -g relaisdesk -m 0750 "$location/api" "$release/api.candidate"
    install -o root -g relaisdesk -m 0750 "$location/relaisdesk-dbcheck" "$release/relaisdesk-dbcheck"
    install -d -o relaisdesk -g relaisdesk -m 0700 "$release/preflight"
    sqlite3 -readonly "$db" ".backup '$release/preflight/licences.db'"
    chown relaisdesk:relaisdesk "$release/preflight/licences.db"
    chmod 0600 "$release/preflight/licences.db"
    before=$(counts "$release/preflight/licences.db")
    runuser -u relaisdesk -- "$release/relaisdesk-dbcheck" -db "$release/preflight/licences.db"
    check_migration "$release/preflight/licences.db"
    [[ $(counts "$release/preflight/licences.db") == "$before" ]]
    [[ $(sqlite3 -readonly "$release/preflight/licences.db" "SELECT COUNT(*) FROM customer_sessions WHERE email IN (SELECT email FROM customer_accounts WHERE totp_enabled=1);") == 0 ]]
    echo "PREFLIGHT_OK release=$release"
    exit 0
fi
[[ $mode == deploy && $location == /opt/relaisdesk/deployments/security-20260919-* && -d $location ]]
release=$location
[[ $(stat -c '%U:%G:%a' "$release") == root:relaisdesk:750 ]]
[[ $(sha256sum "$release/api.candidate" | awk '{print $1}') == "$api_sha" ]]
[[ $(sha256sum "$release/relaisdesk-dbcheck" | awk '{print $1}') == "$dbcheck_sha" ]]
[[ $(sha256sum "$release/api.previous" | awk '{print $1}') == "$previous_sha" ]]
check_migration "$release/preflight/licences.db"
unchanged=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf /etc/nginx/sites-available/relaisdesk-api)
while IFS= read -r -d '' file; do unchanged+=("$file"); done < <(find /opt/relaisdesk/downloads -maxdepth 1 -type f -print0 | sort -z)
before_files=$(sha256sum "${unchanged[@]}")
before_vpn=$(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value)
before_containers=$(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr)
pricing=$(local_get /api/v1/public/pricing)
trials=$(local_get /api/v1/public/trials)
touched=0
rollback() {
    result=$?
    trap - ERR
    set +e
    if [[ $touched == 1 ]]; then
        systemctl stop "$service"
        recovery=$(mktemp /opt/relaisdesk/api/.security-rollback-XXXXXXXX)
        if install -o root -g root -m 0755 "$release/api.previous" "$recovery" && mv -f -- "$recovery" "$target"; then systemctl start "$service"; fi
    fi
    echo "DEPLOY_FAILED rollback=$release; database not rewound" >&2
    exit "$result"
}
trap rollback ERR
touched=1
systemctl stop "$service"
sqlite3 -readonly "$db" ".backup '$release/licences.before.db'"
chmod 0600 "$release/licences.before.db"
[[ $(sqlite3 -readonly "$release/licences.before.db" 'PRAGMA integrity_check;') == ok ]]
business_before=$(counts "$db")
next=$(mktemp /opt/relaisdesk/api/.security-next-XXXXXXXX)
install -o root -g root -m 0755 "$release/api.candidate" "$next"
mv -f -- "$next" "$target"
systemctl start "$service"
healthy=0
for attempt in $(seq 1 25); do
    if local_get /api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected"' 2>/dev/null; then healthy=1; break; fi
    sleep 1
done
[[ $healthy == 1 ]]
check_migration "$db"
pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "/proc/$pid/exe" | awk '{print $1}') == "$api_sha" ]]
python3 -c 'import json,sys; assert json.loads(sys.argv[1])==json.loads(sys.argv[2]); assert json.loads(sys.argv[3])==json.loads(sys.argv[4]); print("PRICES_QUOTAS_TRIALS_UNCHANGED")' "$pricing" "$(local_get /api/v1/public/pricing)" "$trials" "$(local_get /api/v1/public/trials)"
[[ $(sha256sum "${unchanged[@]}") == "$before_files" ]]
[[ $(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value) == "$before_vpn" ]]
[[ $(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr) == "$before_containers" ]]
install -o root -g root -m 0755 "$release/relaisdesk-dbcheck" /opt/relaisdesk/api/relaisdesk-dbcheck
printf 'BUSINESS_BEFORE\n%s\nBUSINESS_AFTER\n' "$business_before"
counts "$db"
systemctl show "$service" -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
trap - ERR
echo "DEPLOY_OK api_sha256=$api_sha rollback=$release"
