#!/usr/bin/env bash
# Narrow legal-contract rollout. Run only after the matching OVH files are verified.
# No downloads, Nginx, environment, systemd unit, RustDesk, VPN or OS changes.
set -Eeuo pipefail
umask 077

[[ ${EUID} -eq 0 && $# -eq 4 ]] || { echo 'Usage (root): deploy_api_legal_update.sh STAGING API_SHA256 DBCHECK_SHA256 PREVIOUS_API_SHA256' >&2; exit 1; }
staging=$(realpath -e -- "$1")
expected_api=$2
expected_dbcheck=$3
expected_previous=$4
[[ $staging == /home/ubuntu/relaisdesk-api-legal-20260921-* && -d $staging ]] || exit 1
for digest in "$expected_api" "$expected_dbcheck" "$expected_previous"; do
    [[ $digest =~ ^[a-f0-9]{64}$ ]] || exit 1
done
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9 || { echo 'Another API deployment is in progress' >&2; exit 1; }

target=/opt/relaisdesk/api/api
db=/data/relaisdesk/licences.db
service=relaisdesk-api.service
[[ -f $target && -f $db && ! -L $target && ! -L $db ]] || exit 1
systemctl is-active --quiet "$service"
previous_pid=$(systemctl show "$service" -p MainPID --value)
[[ $(sha256sum "$target" | awk '{print $1}') == "$expected_previous" ]] || { echo 'Unexpected installed API; refusing overwrite' >&2; exit 1; }
[[ $(sha256sum "/proc/$previous_pid/exe" | awk '{print $1}') == "$expected_previous" ]] || exit 1
[[ $(sha256sum "$staging/api" | awk '{print $1}') == "$expected_api" ]] || exit 1
[[ $(sha256sum "$staging/relaisdesk-dbcheck" | awk '{print $1}') == "$expected_dbcheck" ]] || exit 1

local_get() {
    curl --fail --silent --show-error --max-time 4 \
        --resolve api.relaisdesk.fr:8443:127.0.0.1 \
        "https://api.relaisdesk.fr:8443$1"
}
pricing_before=$(local_get /api/v1/public/pricing)
trials_before=$(local_get /api/v1/public/trials)
python3 -c 'import json,sys; d=json.loads(sys.argv[1]); assert d["terms_version"]=="2026-09-11" and d["trial_terms_version"]=="2026-09-11-fleet-v2"' "$trials_before"

install -d -o root -g relaisdesk -m 0750 /opt/relaisdesk/deployments
release=$(mktemp -d /opt/relaisdesk/deployments/legal-20260921-XXXXXXXX)
chown root:relaisdesk "$release"
chmod 0750 "$release"
echo "ROLLBACK_DIRECTORY=$release"
install -o root -g root -m 0700 "$target" "$release/api.previous"
install -o root -g relaisdesk -m 0750 "$staging/api" "$release/api.candidate"
install -o root -g relaisdesk -m 0750 "$staging/relaisdesk-dbcheck" "$release/relaisdesk-dbcheck"
[[ $(sha256sum "$release/api.candidate" | awk '{print $1}') == "$expected_api" ]]
[[ $(sha256sum "$release/relaisdesk-dbcheck" | awk '{print $1}') == "$expected_dbcheck" ]]

# Fingerprints remain private; never print environment or VPN contents.
unchanged_files=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf)
for download in /opt/relaisdesk/downloads/release-manifest.json /opt/relaisdesk/downloads/SHA256SUMS.txt; do
    if [[ -f $download ]]; then unchanged_files+=("$download"); fi
done
config_before=$(sha256sum "${unchanged_files[@]}")
containers_before=$(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr)

install -d -o relaisdesk -g relaisdesk -m 0700 "$release/preflight"
sqlite3 -readonly "$db" ".backup '$release/preflight/licences.db'"
chown relaisdesk:relaisdesk "$release/preflight/licences.db"
chmod 0600 "$release/preflight/licences.db"
runuser -u relaisdesk -- "$release/relaisdesk-dbcheck" -db "$release/preflight/licences.db"
echo 'PREFLIGHT_OK'

service_touched=0
rollback() {
    code=$?
    trap - ERR
    set +e
    if [[ $service_touched -eq 1 ]]; then
        echo 'DEPLOY_FAILED: restoring the previous API; no database rewind' >&2
        systemctl stop "$service"
        recovery=$(mktemp /opt/relaisdesk/api/.legal-rollback-XXXXXXXX)
        if install -o root -g root -m 0755 "$release/api.previous" "$recovery" && mv -f -- "$recovery" "$target"; then
            systemctl start "$service"
            systemctl is-active "$service"
        else
            echo "ROLLBACK_FAILED: manual recovery required from $release/api.previous" >&2
        fi
        echo "Database snapshot retained: $release/licences.before.db" >&2
    fi
    exit "$code"
}
trap rollback ERR

[[ $(sha256sum "$target" | awk '{print $1}') == "$expected_previous" ]]
service_touched=1
systemctl stop "$service"
sqlite3 -readonly "$db" ".backup '$release/licences.before.db'"
chmod 0600 "$release/licences.before.db"
[[ $(sqlite3 -readonly "$release/licences.before.db" 'PRAGMA integrity_check;') == ok ]]
candidate=$(mktemp /opt/relaisdesk/api/.legal-next-XXXXXXXX)
install -o root -g root -m 0755 "$release/api.candidate" "$candidate"
mv -f -- "$candidate" "$target"
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
pricing_after=$(local_get /api/v1/public/pricing)
trials_after=$(local_get /api/v1/public/trials)
python3 -c 'import json,sys; assert json.loads(sys.argv[1])==json.loads(sys.argv[2]); print("PRICES_AND_QUOTAS_UNCHANGED")' "$pricing_before" "$pricing_after"
python3 -c 'import json,sys; before=json.loads(sys.argv[1]); after=json.loads(sys.argv[2]); assert after.pop("terms_version")=="2026-09-21"; assert after.pop("trial_terms_version")=="2026-09-21-fleet-v2"; before.pop("terms_version"); before.pop("trial_terms_version"); assert before==after; print("LEGAL_VERSIONS_OK; TRIAL_AND_B2C_SETTINGS_UNCHANGED")' "$trials_before" "$trials_after"
[[ $(sqlite3 -readonly "$db" 'PRAGMA integrity_check;') == ok ]]
[[ -z $(sqlite3 -readonly "$db" 'PRAGMA foreign_key_check;') ]]
[[ $(sha256sum "${unchanged_files[@]}") == "$config_before" ]]
[[ $(docker inspect --format '{{.Id}} {{.State.StartedAt}}' rustdesk-hbbs rustdesk-hbbr) == "$containers_before" ]]
systemctl is-active --quiet wg-quick@wg0.service
systemctl show "$service" -p ActiveState -p SubState -p MainPID -p NRestarts --no-pager
ss -lntp '( sport = :8443 )'
trap - ERR
echo "DEPLOY_OK api_sha256=$expected_api rollback=$release"
