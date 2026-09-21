#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID -eq 0 && $# -eq 4 ]] || { echo 'Usage: STAGING_DIRECTORY SOURCE_SHA HBBS_SHA HBBR_SHA' >&2; exit 1; }
stage=$(realpath -e -- "$1")
source_sha=$2
hbbs_sha=$3
hbbr_sha=$4
[[ $stage == /home/ubuntu/relaisdesk-security-20260919-* ]]
for digest in "$source_sha" "$hbbs_sha" "$hbbr_sha"; do [[ $digest =~ ^[a-f0-9]{64}$ ]]; done
exec 9>/run/lock/relaisdesk-server-deploy.lock
flock -n 9
old_image=relaisdesk/rustdesk-server:d998a32-security-amd64
old_id=sha256:396e3556158404b4ae9fbe087e2b2de00a4af6e73524a5bffc406741d165e14d
new_image=relaisdesk/rustdesk-server:security-20260919-amd64
source_name=rustdesk-server-security-20260919.tar.gz
source_url=https://api.relaisdesk.fr/sources/$source_name
compose=/opt/rustdesk/docker-compose.yml
env_file=/opt/rustdesk/.env
nginx=/etc/nginx/sites-available/relaisdesk-api
[[ $(docker image inspect --format '{{.Id}}' "$old_image") == "$old_id" ]]
for container in rustdesk-hbbs rustdesk-hbbr; do
    [[ $(docker inspect --format '{{.Image}}' "$container") == "$old_id" ]]
    [[ $(docker inspect --format '{{.State.Running}}' "$container") == true ]]
done
for file in "$compose" "$env_file" "$nginx"; do [[ -f $file && ! -L $file ]]; done
[[ $(sha256sum "$stage/rustdesk-server-source.tar.gz" | awk '{print $1}') == "$source_sha" ]]
[[ $(sha256sum "$stage/hbbs" | awk '{print $1}') == "$hbbs_sha" ]]
[[ $(sha256sum "$stage/hbbr" | awk '{print $1}') == "$hbbr_sha" ]]
if docker image inspect "$new_image" >/dev/null 2>&1; then
    [[ $(docker run --rm --network=none --entrypoint sha256sum "$new_image" /usr/local/bin/hbbs | awk '{print $1}') == "$hbbs_sha" ]]
    [[ $(docker run --rm --network=none --entrypoint sha256sum "$new_image" /usr/local/bin/hbbr | awk '{print $1}') == "$hbbr_sha" ]]
fi
release=$(mktemp -d /opt/relaisdesk/deployments/server-security-20260919-XXXXXXXX)
cp -p -- "$compose" "$release/docker-compose.previous.yml"
cp -p -- "$env_file" "$release/env.previous"
cp -p -- "$nginx" "$release/nginx.previous"
install -d -m 0700 "$release/build"
install -m 0755 "$stage/hbbs" "$stage/hbbr" "$release/build/"
tar -xOf "$stage/rustdesk-server-source.tar.gz" ./Dockerfile.security-20260919 > "$release/build/Dockerfile"
docker build --pull=false --network=none -t "$new_image" "$release/build"
docker run --rm --network=none --entrypoint hbbs "$new_image" --help >/dev/null
docker run --rm --network=none --entrypoint hbbr "$new_image" --help >/dev/null
new_id=$(docker image inspect --format '{{.Id}}' "$new_image")
unchanged=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf /opt/relaisdesk/api/api)
while IFS= read -r -d '' file; do unchanged+=("$file"); done < <(find /opt/relaisdesk/downloads -maxdepth 1 -type f -print0 | sort -z)
before_files=$(sha256sum "${unchanged[@]}")
before_vpn=$(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value)
api_pid=$(systemctl show relaisdesk-api.service -p MainPID --value)
auth_before=$(docker inspect rustdesk-hbbs rustdesk-hbbr | python3 -c 'import json,sys,hashlib; a=json.load(sys.stdin); print(hashlib.sha256(repr([sorted(v for v in c["Config"]["Env"] if v.startswith(("RELAISDESK_AUTH_REQUIRED=", "RELAISDESK_AUTH_PUBLIC_KEYS="))) for c in a]).encode()).hexdigest())')
source_touched=0
server_touched=0
rollback() {
    result=$?
    failed_line=${BASH_LINENO[0]}
    trap - ERR
    set +e
    if [[ $server_touched == 1 ]]; then
        cp -p -- "$release/docker-compose.previous.yml" "$compose"
        cp -p -- "$release/env.previous" "$env_file"
        (cd /opt/rustdesk && docker compose up -d --no-build --pull never hbbr hbbs)
    fi
    if [[ $source_touched == 1 ]]; then
        cp -p -- "$release/nginx.previous" "$nginx"
        nginx -t && systemctl reload nginx
    fi
    echo "SERVER_DEPLOY_FAILED line=$failed_line rollback=$release; data and downloads not rewound" >&2
    exit "$result"
}
trap rollback ERR
install -d -o root -g root -m 0755 /opt/relaisdesk/sources
if [[ -e /opt/relaisdesk/sources/$source_name ]]; then
    [[ ! -L /opt/relaisdesk/sources/$source_name ]]
    [[ $(sha256sum "/opt/relaisdesk/sources/$source_name" | awk '{print $1}') == "$source_sha" ]]
else
    install -o root -g root -m 0644 "$stage/rustdesk-server-source.tar.gz" "/opt/relaisdesk/sources/$source_name"
fi
source_touched=1
python3 - "$nginx" "$source_name" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); name=sys.argv[2]; s=p.read_text()
anchor='location / {\n        proxy_pass https://127.0.0.1:8443;'
assert s.count(anchor) == 1 and '/sources/' not in s
block=f'''location = /sources/{name} {{
        alias /opt/relaisdesk/sources/{name};
        default_type application/gzip;
        add_header Content-Disposition 'attachment; filename="{name}"';
        limit_except GET {{ deny all; }}
    }}

    '''
p.write_text(s.replace(anchor,block+anchor,1))
PY
nginx -t
systemctl reload nginx
source_ready=0
for attempt in $(seq 1 10); do
    if downloaded_sha=$(curl -fsS --max-time 30 --resolve api.relaisdesk.fr:443:127.0.0.1 "$source_url" | sha256sum | awk '{print $1}'); then
        if [[ $downloaded_sha == "$source_sha" ]]; then source_ready=1; break; fi
    fi
    sleep 1
done
[[ $source_ready == 1 ]]
python3 - "$compose" "$env_file" "$old_image" "$new_image" "$source_url" <<'PY'
import pathlib,sys,re
c,e=map(pathlib.Path,sys.argv[1:3]); old,new,url=sys.argv[3:]
s=c.read_text(); assert s.count(old)==2
t=e.read_text(); t,n=re.subn(r'^SERVER_SOURCE_URL=.*$',f'SERVER_SOURCE_URL={url}',t,flags=re.M); assert n==1
c.with_suffix('.candidate.yml').write_text(s.replace(old,new))
# Root-only candidate; existing credentials never go to stdout.
e.with_name('.env.security-candidate').write_text(t)
PY
chmod 0600 /opt/rustdesk/.env.security-candidate /opt/rustdesk/docker-compose.candidate.yml
(cd /opt/rustdesk && docker compose --env-file .env.security-candidate -f docker-compose.candidate.yml config --quiet)
server_touched=1
(cd /opt/rustdesk && docker compose stop hbbs hbbr)
tar -C /opt/rustdesk -czf "$release/data.before.tar.gz" data
chmod 0600 "$release/data.before.tar.gz"
key_before=$(find /opt/rustdesk/data -maxdepth 1 -type f -name 'id_ed25519*' -exec sha256sum {} +)
[[ -n $key_before ]]
mv -f -- /opt/rustdesk/docker-compose.candidate.yml "$compose"
mv -f -- /opt/rustdesk/.env.security-candidate "$env_file"
(cd /opt/rustdesk && docker compose up -d --no-build --pull never hbbr hbbs)
sleep 12
for container in rustdesk-hbbs rustdesk-hbbr; do
    [[ $(docker inspect --format '{{.Image}}' "$container") == "$new_id" ]]
    [[ $(docker inspect --format '{{.State.Running}} {{.RestartCount}}' "$container") == 'true 0' ]]
done
[[ $(docker exec rustdesk-hbbs sha256sum /usr/local/bin/hbbs | awk '{print $1}') == "$hbbs_sha" ]]
[[ $(docker exec rustdesk-hbbr sha256sum /usr/local/bin/hbbr | awk '{print $1}') == "$hbbr_sha" ]]
auth_after=$(docker inspect rustdesk-hbbs rustdesk-hbbr | python3 -c 'import json,sys,hashlib; a=json.load(sys.stdin); print(hashlib.sha256(repr([sorted(v for v in c["Config"]["Env"] if v.startswith(("RELAISDESK_AUTH_REQUIRED=", "RELAISDESK_AUTH_PUBLIC_KEYS="))) for c in a]).encode()).hexdigest())')
[[ $auth_before == "$auth_after" ]]
[[ $(find /opt/rustdesk/data -maxdepth 1 -type f -name 'id_ed25519*' -exec sha256sum {} +) == "$key_before" ]]
[[ $(sha256sum "${unchanged[@]}") == "$before_files" ]]
[[ $(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value) == "$before_vpn" ]]
[[ $(systemctl show relaisdesk-api.service -p MainPID --value) == "$api_pid" ]]
curl -fsS --max-time 10 --resolve api.relaisdesk.fr:443:127.0.0.1 https://api.relaisdesk.fr/api/v1/health
trap - ERR
echo
echo "SERVER_DEPLOY_OK image=$new_id rollback=$release source=$source_url"
