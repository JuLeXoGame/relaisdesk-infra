#!/usr/bin/env bash
# Historical quota rollout, expecting the 2026-09-10 contracts.
# Not suitable unchanged for the 2026-09-11 legal update; coordinate API and OVH.
# Narrow API-only rollout: no OS, firewall, VPN, Docker, environment or unit changes.
set -Eeuo pipefail
umask 077

[[ ${EUID} -eq 0 ]] || { echo 'Root required' >&2; exit 1; }
[[ $# -eq 3 ]] || { echo 'Usage: deploy_api_quota_update.sh STAGING API_SHA256 DBCHECK_SHA256' >&2; exit 1; }
staging=$(realpath -e -- "$1")
expected_api=$2
expected_dbcheck=$3
[[ $staging == /home/ubuntu/relaisdesk-api-quotas-20260911-* ]] || { echo 'Unexpected staging directory' >&2; exit 1; }
[[ $expected_api =~ ^[a-f0-9]{64}$ && $expected_dbcheck =~ ^[a-f0-9]{64}$ ]] || exit 1
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9 || { echo 'Another API deployment is in progress' >&2; exit 1; }

target=/opt/relaisdesk/api/api
db=/data/relaisdesk/licences.db
service=relaisdesk-api.service
[[ -f $target && -f $db && ! -L $target && ! -L $db ]] || exit 1
systemctl is-active --quiet "$service"
previous_sha=$(sha256sum "$target" | awk '{print $1}')
previous_pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "/proc/$previous_pid/exe" | awk '{print $1}') == "$previous_sha" ]] || { echo 'Running API differs from installed binary' >&2; exit 1; }
[[ $(sha256sum "$staging/api" | awk '{print $1}') == "$expected_api" ]] || exit 1
[[ $(sha256sum "$staging/relaisdesk-dbcheck" | awk '{print $1}') == "$expected_dbcheck" ]] || exit 1

install -d -o root -g relaisdesk -m 0750 /opt/relaisdesk/deployments
release=$(mktemp -d /opt/relaisdesk/deployments/quotas-20260911-XXXXXXXX)
chown root:relaisdesk "$release"
chmod 0750 "$release"
echo "ROLLBACK_DIRECTORY=$release"
install -o root -g root -m 0700 "$target" "$release/api.previous"
install -o root -g relaisdesk -m 0750 "$staging/api" "$release/api.candidate"
install -o root -g relaisdesk -m 0750 "$staging/relaisdesk-dbcheck" "$release/relaisdesk-dbcheck"
[[ $(sha256sum "$release/api.candidate" | awk '{print $1}') == "$expected_api" ]] || exit 1
[[ $(sha256sum "$release/relaisdesk-dbcheck" | awk '{print $1}') == "$expected_dbcheck" ]] || exit 1

# These fingerprints stay in memory; no configuration content is displayed.
unchanged_files=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf)
config_before=$(sha256sum "${unchanged_files[@]}")
containers_before=$(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr)

install -d -o relaisdesk -g relaisdesk -m 0700 "$release/preflight"
sqlite3 -readonly "$db" ".backup '$release/preflight/licences.db'"
chown relaisdesk:relaisdesk "$release/preflight/licences.db"
chmod 0600 "$release/preflight/licences.db"
runuser -u relaisdesk -- "$release/relaisdesk-dbcheck" -db "$release/preflight/licences.db"
sqlite3 -readonly "$release/preflight/licences.db" "SELECT 'PREFLIGHT_FLEET_DEVICES',COUNT(*) FROM devices;"
echo 'PREFLIGHT_OK'

service_touched=0
local_get() {
    curl --fail --silent --show-error --max-time 3 \
        --resolve api.relaisdesk.fr:8443:127.0.0.1 \
        "https://api.relaisdesk.fr:8443$1"
}
rollback() {
    code=$?
    trap - ERR
    set +e
    if [[ $service_touched -eq 1 ]]; then
        echo 'DEPLOY_FAILED: restoring previous API binary; database is preserved' >&2
        systemctl stop "$service"
        install -o root -g root -m 0755 "$release/api.previous" "$target.quota-rollback"
        mv -f -- "$target.quota-rollback" "$target"
        systemctl start "$service"
        systemctl is-active "$service"
        echo "Database safety snapshot: $release/licences.before.db (no automatic data rewind)" >&2
    fi
    exit "$code"
}
trap rollback ERR

# Refuse to overwrite another deployment that occurred during preflight.
[[ $(sha256sum "$target" | awk '{print $1}') == "$previous_sha" ]]
service_touched=1
systemctl stop "$service"
# Fresh SQLite backup after stopping writers, including all committed WAL data.
sqlite3 -readonly "$db" ".backup '$release/licences.before.db'"
chmod 0600 "$release/licences.before.db"
[[ $(sqlite3 -readonly "$release/licences.before.db" 'PRAGMA integrity_check;') == ok ]]
install -o root -g root -m 0755 "$release/api.candidate" "$target.quota-next"
mv -f -- "$target.quota-next" "$target"
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
[[ $(sha256sum "/proc/$new_pid/exe" | awk '{print $1}') == "$expected_api" ]]
local_get /api/v1/public/pricing | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["starter"]["managed_devices"]==500; assert d["pro"]["managed_devices"]==1000; assert d["ultra"]["managed_devices"]==2000; assert d["ultra"]["managed_devices_per_extra_technician"]==5; assert d["ultra"]["max_managed_devices"]==4500; assert d["ultra"]["managed_devices_max_tier_bonus"]==50; assert d["starter"]["price_monthly"]==24.9 and d["pro"]["price_monthly"]==110 and d["ultra"]["base_monthly"]==199; print("PRODUCTION_QUOTAS_OK")'
local_get /api/v1/public/trials | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["enabled"] and d["days"]==30; assert d["terms_version"]=="2026-09-10" and d["trial_terms_version"]=="2026-09-10-fleet-v1"; print("PRODUCTION_TRIAL_CONTRACT_OK")'
[[ $(sqlite3 -readonly "$db" 'PRAGMA integrity_check;') == ok ]]
[[ -z $(sqlite3 -readonly "$db" 'PRAGMA foreign_key_check;') ]]
[[ $(sha256sum "${unchanged_files[@]}") == "$config_before" ]]
[[ $(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr) == "$containers_before" ]]
systemctl is-active --quiet wg-quick@wg0.service
systemctl show "$service" -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
ss -lntp '( sport = :8443 )'
trap - ERR
echo "DEPLOY_OK api_sha256=$expected_api rollback=$release"
