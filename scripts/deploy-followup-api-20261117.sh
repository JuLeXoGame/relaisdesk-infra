#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 && $# == 2 ]]
stage=$(realpath -e -- "$1")
previous_sha=$2
[[ $stage == /home/ubuntu/relaisdesk-followup-20261117 && $previous_sha =~ ^[a-f0-9]{64}$ ]]
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9
target=/opt/relaisdesk/api
db=/data/relaisdesk/licences.db
service=relaisdesk-api.service
names=(api relaisdesk-backup relaisdesk-dbcheck relaisdesk-emailcheck)
[[ -f $target/api && ! -L $target/api && -f $db && ! -L $db ]]
(cd "$stage" && sha256sum --check SHA256SUMS-api.txt)
api_sha=$(sha256sum "$stage/api" | cut -d ' ' -f1)
systemctl is-active --quiet "$service"
pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "$target/api" | cut -d ' ' -f1) == "$previous_sha" ]]
[[ $(sha256sum "/proc/$pid/exe" | cut -d ' ' -f1) == "$previous_sha" ]]
local_get() { curl -fsS --max-time 10 --resolve api.relaisdesk.fr:8443:127.0.0.1 "https://api.relaisdesk.fr:8443$1"; }
counts() { sqlite3 -readonly "$1" "SELECT 'licences',COUNT(*) FROM licences UNION ALL SELECT 'devices',COUNT(*) FROM devices UNION ALL SELECT 'orders',COUNT(*) FROM orders UNION ALL SELECT 'accounts',COUNT(*) FROM customer_accounts;"; }
integrity() {
    [[ $(sqlite3 -readonly "$1" 'PRAGMA integrity_check;') == ok ]]
    [[ -z $(sqlite3 -readonly "$1" 'PRAGMA foreign_key_check;') ]]
}
release=$(mktemp -d /opt/relaisdesk/deployments/followup-api-20261117-XXXXXXXX)
chown root:relaisdesk "$release"
chmod 0750 "$release"
install -d -o relaisdesk -g relaisdesk -m 0700 "$release/preflight"
for name in "${names[@]}"; do
    [[ -f $target/$name && ! -L $target/$name ]]
    install -m 0700 "$target/$name" "$release/$name.previous"
done
install -o root -g relaisdesk -m 0750 "$stage/relaisdesk-dbcheck" "$release/relaisdesk-dbcheck.candidate"
sqlite3 -readonly "$db" ".backup '$release/preflight/licences.db'"
chown relaisdesk:relaisdesk "$release/preflight/licences.db"
chmod 0600 "$release/preflight/licences.db"
before=$(counts "$release/preflight/licences.db")
runuser -u relaisdesk -- "$release/relaisdesk-dbcheck.candidate" -db "$release/preflight/licences.db" > "$release/preflight/migration.log" 2>&1
integrity "$release/preflight/licences.db"
[[ $(counts "$release/preflight/licences.db") == "$before" ]]
echo PREFLIGHT_OK
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
        for name in "${names[@]}"; do
            recovery=$(mktemp "$target/.followup-rollback-XXXXXXXX")
            install -o root -g root -m 0755 "$release/$name.previous" "$recovery" && mv -f -- "$recovery" "$target/$name"
        done
        systemctl start "$service"
    fi
    echo "DEPLOY_FAILED line=${BASH_LINENO[0]} rollback=$release; database not rewound" >&2
    exit "$result"
}
trap rollback ERR
touched=1
systemctl stop "$service"
sqlite3 -readonly "$db" ".backup '$release/licences.before.db'"
chmod 0600 "$release/licences.before.db"
integrity "$release/licences.before.db"
business_before=$(counts "$db")
for name in "${names[@]}"; do
    next=$(mktemp "$target/.followup-next-XXXXXXXX")
    install -o root -g root -m 0755 "$stage/$name" "$next"
    mv -f -- "$next" "$target/$name"
done
systemctl start "$service"
healthy=0
for attempt in $(seq 1 25); do
    if local_get /api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected" and d["version"]=="1.0.0"' 2>/dev/null; then healthy=1; break; fi
    sleep 1
done
[[ $healthy == 1 ]]
integrity "$db"
pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "/proc/$pid/exe" | cut -d ' ' -f1) == "$api_sha" ]]
for name in "${names[@]}"; do
    [[ $(sha256sum "$target/$name" | cut -d ' ' -f1) == $(sha256sum "$stage/$name" | cut -d ' ' -f1) ]]
done
python3 -c 'import json,sys; assert json.loads(sys.argv[1])==json.loads(sys.argv[2]); assert json.loads(sys.argv[3])==json.loads(sys.argv[4])' "$pricing" "$(local_get /api/v1/public/pricing)" "$trials" "$(local_get /api/v1/public/trials)"
[[ $(sha256sum "${unchanged[@]}") == "$before_files" ]]
[[ $(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value) == "$before_vpn" ]]
[[ $(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr) == "$before_containers" ]]
printf 'BUSINESS_BEFORE\n%s\nBUSINESS_AFTER\n' "$business_before"
counts "$db"
systemctl show "$service" -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
trap - ERR
echo "DEPLOY_OK api_sha256=$api_sha rollback=$release"
