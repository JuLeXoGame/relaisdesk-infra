#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 ]]
backup=/opt/relaisdesk/deployments/server-security-20260919-tS9Dk2Hf/api.env.before-source-url
[[ -d ${backup%/*} && ! -e $backup ]]
target=/etc/relaisdesk/api.env
cp -p "$target" "$backup"
rollback() {
    result=$?
    trap - ERR
    cp -p "$backup" "$target"
    systemctl restart relaisdesk-api.service
    exit "$result"
}
trap rollback ERR
python3 - "$target" <<'PY'
import pathlib,re,sys
p=pathlib.Path(sys.argv[1]); before=p.read_text()
after,n=re.subn(r'^SERVER_SOURCE_URL=.*$', 'SERVER_SOURCE_URL=https://api.relaisdesk.fr/sources/rustdesk-server-security-20260919.tar.gz', before, flags=re.M)
assert n == 1
assert [s for s in before.splitlines() if not s.startswith('SERVER_SOURCE_URL=')] == [s for s in after.splitlines() if not s.startswith('SERVER_SOURCE_URL=')]
p.write_text(after)
PY
systemctl restart relaisdesk-api.service
ready=0
for attempt in $(seq 1 20); do
    if curl -fsS --max-time 5 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected"' 2>/dev/null; then ready=1; break; fi
    sleep 1
done
[[ $ready == 1 ]]
trap - ERR
echo 'SOURCE_METADATA_ALIGNED only SERVER_SOURCE_URL changed; API healthy'
