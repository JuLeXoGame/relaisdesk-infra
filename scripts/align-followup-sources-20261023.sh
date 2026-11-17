#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 && $# == 2 ]]
server_url=$1
client_url=$2
[[ $server_url =~ ^https://github.com/JuLeXoGame/rustdesk-server/tree/[a-f0-9]{40}$ ]]
[[ $client_url == - || $client_url =~ ^https://github.com/JuLeXoGame/relaisdesk/tree/[a-f0-9]{40}$ ]]
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9
target=/etc/relaisdesk/api.env
[[ -f $target && ! -L $target ]]
release=$(mktemp -d /opt/relaisdesk/deployments/followup-sources-20261023-XXXXXXXX)
cp -p "$target" "$release/api.env.previous"
rollback() {
    code=$?
    trap - ERR
    cp -p "$release/api.env.previous" "$target"
    systemctl restart relaisdesk-api.service
    echo "SOURCE_ALIGNMENT_FAILED rollback=$release" >&2
    exit "$code"
}
trap rollback ERR
python3 - "$target" "$server_url" "$client_url" <<'PY'
import pathlib,re,sys
p=pathlib.Path(sys.argv[1]); before=p.read_text()
updates={'SERVER_SOURCE_URL':sys.argv[2]}
if sys.argv[3] != '-': updates['CLIENT_SOURCE_URL']=sys.argv[3]
after=before
for key,value in updates.items():
    after,n=re.subn(r'^'+key+r'=.*$',key+'='+value,after,flags=re.M)
    assert n==1
def kept(s): return [line for line in s.splitlines() if line.split('=',1)[0] not in updates]
assert kept(before)==kept(after)
p.write_text(after)
PY
systemctl restart relaisdesk-api.service
ready=0
for attempt in $(seq 1 25); do
    if curl -fsS --max-time 5 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected" and d["version"]=="1.0.0"' 2>/dev/null; then ready=1; break; fi
    sleep 1
done
[[ $ready == 1 ]]
trap - ERR
echo "SOURCE_ALIGNMENT_OK rollback=$release"
