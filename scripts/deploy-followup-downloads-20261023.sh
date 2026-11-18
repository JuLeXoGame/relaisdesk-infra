#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 && $# == 1 ]]
stage=$(realpath -e -- "$1")
[[ $stage == /home/ubuntu/relaisdesk-followup-20261023 ]]
exec 9>/run/lock/relaisdesk-api-deploy.lock
flock -n 9
downloads=/opt/relaisdesk/downloads
env_file=/etc/relaisdesk/api.env
[[ -d $downloads && ! -L $downloads && -f $env_file && ! -L $env_file ]]
systemctl is-active --quiet relaisdesk-api.service
python3 "$stage/verify-followup-downloads-20261023.py" "$stage/downloads"
before_manifest=$(sha256sum "$downloads/release-manifest.json")
release=$(mktemp -d /opt/relaisdesk/deployments/followup-downloads-20261023-XXXXXXXX)
cp -p "$env_file" "$release/api.env.previous"
cp -al "$downloads" "$release/downloads.candidate"
python3 - "$stage/downloads" "$release/downloads.candidate" <<'PY'
import json,pathlib,sys,shutil,os
source,destination=map(pathlib.Path,sys.argv[1:])
m=json.loads((source/'release-manifest.json').read_text())
for name in [a['name'] for a in m['artifacts']]+['release-manifest.json']:
    temp=destination/('.new-'+name)
    with (source/name).open('rb') as src, temp.open('xb') as dst:
        shutil.copyfileobj(src,dst,1024*1024)
        dst.flush(); os.fsync(dst.fileno())
    temp.chmod(0o644)
    temp.replace(destination/name)
PY
python3 "$stage/verify-followup-downloads-20261023.py" "$release/downloads.candidate"
[[ $(sha256sum "$downloads/release-manifest.json") == "$before_manifest" ]]
old_moved=0
new_moved=0
touched=0
rollback() {
    result=$?
    trap - ERR
    set +e
    if [[ $touched == 1 ]]; then
        systemctl stop relaisdesk-api.service
        if [[ $new_moved == 1 ]]; then mv "$downloads" "$release/downloads.failed"; fi
        if [[ $old_moved == 1 ]]; then mv "$release/downloads.previous" "$downloads"; fi
        cp -p "$release/api.env.previous" "$env_file"
        systemctl start relaisdesk-api.service
    fi
    echo "DOWNLOAD_DEPLOY_FAILED rollback=$release; database not rewound" >&2
    exit "$result"
}
trap rollback ERR
touched=1
systemctl stop relaisdesk-api.service
mv "$downloads" "$release/downloads.previous"
old_moved=1
mv "$release/downloads.candidate" "$downloads"
new_moved=1
python3 - "$env_file" <<'PY'
import pathlib,re,sys
p=pathlib.Path(sys.argv[1]); before=p.read_text()
url='https://github.com/JuLeXoGame/relaisdesk/tree/1054d0f09e4e1d4aaea1309af714289e6055ba4b'
after,n=re.subn(r'^CLIENT_SOURCE_URL=.*$','CLIENT_SOURCE_URL='+url,before,flags=re.M)
assert n==1
assert [l for l in before.splitlines() if not l.startswith('CLIENT_SOURCE_URL=')]==[l for l in after.splitlines() if not l.startswith('CLIENT_SOURCE_URL=')]
p.write_text(after)
PY
systemctl start relaisdesk-api.service
local_get() { curl -fsS --max-time 20 --resolve api.relaisdesk.fr:443:127.0.0.1 "https://api.relaisdesk.fr$1"; }
ready=0
for attempt in $(seq 1 25); do
    if local_get /api/v1/health 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="healthy" and d["database"]=="connected" and d["version"]=="1.0.0"' 2>/dev/null; then ready=1; break; fi
    sleep 1
done
[[ $ready == 1 ]]
local_get /api/v1/public/releases/latest > "$release/manifest.served.json"
python3 - "$downloads/release-manifest.json" "$release/manifest.served.json" <<'PY'
import json,sys
assert json.load(open(sys.argv[1]))==json.load(open(sys.argv[2]))
PY
while IFS= read -r name; do
    curl -fsS --max-time 120 --range 0-0 --resolve api.relaisdesk.fr:443:127.0.0.1 "https://api.relaisdesk.fr/api/v1/downloads/$name" -o /dev/null
done < <(python3 - "$downloads/release-manifest.json" <<'PY'
import json,sys
for a in json.load(open(sys.argv[1]))['artifacts']:
    if not a['name'].endswith('.dmg'): print(a['name'])
PY
)
trap - ERR
echo "DOWNLOAD_DEPLOY_OK rollback=$release"
sha256sum "$downloads/release-manifest.json"
