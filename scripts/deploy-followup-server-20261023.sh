#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
[[ $EUID == 0 && $# == 2 ]]
stage=$(realpath -e -- "$1")
source_url=$2
[[ $stage == /home/ubuntu/relaisdesk-followup-20261023 ]]
[[ $source_url =~ ^https://github.com/JuLeXoGame/rustdesk-server/tree/[a-f0-9]{40}$ ]]
exec 9>/run/lock/relaisdesk-server-deploy.lock
flock -n 9
old_image=relaisdesk/rustdesk-server:security-20260919-amd64
old_id=sha256:a20d77c6743f47625397e7baa8ba1009f3f4308dc738dc587d0172d807a97103
new_image=relaisdesk/rustdesk-server:followup-20261023-amd64
compose=/opt/rustdesk/docker-compose.yml
env_file=/opt/rustdesk/.env
(cd "$stage" && sha256sum --check SHA256SUMS-server.txt)
hbbs_sha=$(sha256sum "$stage/hbbs" | cut -d ' ' -f1)
hbbr_sha=$(sha256sum "$stage/hbbr" | cut -d ' ' -f1)
for container in rustdesk-hbbs rustdesk-hbbr; do
    [[ $(docker inspect --format '{{.Image}} {{.State.Running}}' "$container") == "$old_id true" ]]
done
for file in "$compose" "$env_file"; do [[ -f $file && ! -L $file ]]; done
release=$(mktemp -d /opt/relaisdesk/deployments/followup-server-20261023-XXXXXXXX)
cp -p "$compose" "$release/docker-compose.previous.yml"
cp -p "$env_file" "$release/env.previous"
install -d -m 0700 "$release/build"
install -m 0755 "$stage/hbbs" "$stage/hbbr" "$release/build/"
cat > "$release/build/Dockerfile" <<EOF
FROM $old_image
LABEL org.opencontainers.image.title="RelaisDesk Server security 2026-10-23" org.opencontainers.image.licenses="AGPL-3.0-only" org.opencontainers.image.source="$source_url"
COPY --chmod=0755 hbbs /usr/local/bin/hbbs
COPY --chmod=0755 hbbr /usr/local/bin/hbbr
EOF
docker build --pull=false --network=none -t "$new_image" "$release/build"
docker run --rm --network=none --entrypoint hbbs "$new_image" --help >/dev/null
docker run --rm --network=none --entrypoint hbbr "$new_image" --help >/dev/null
new_id=$(docker image inspect --format '{{.Id}}' "$new_image")
unchanged=(/etc/relaisdesk/api.env /etc/systemd/system/relaisdesk-api.service /etc/wireguard/wg0.conf /etc/nginx/sites-available/relaisdesk-api /opt/relaisdesk/api/api)
while IFS= read -r -d '' file; do unchanged+=("$file"); done < <(find /opt/relaisdesk/downloads -maxdepth 1 -type f -print0 | sort -z)
before_files=$(sha256sum "${unchanged[@]}")
before_vpn=$(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value)
api_pid=$(systemctl show relaisdesk-api.service -p MainPID --value)
auth_hash() { docker inspect rustdesk-hbbs rustdesk-hbbr | python3 -c 'import json,sys,hashlib; a=json.load(sys.stdin); print(hashlib.sha256(repr([sorted(v for v in c["Config"]["Env"] if v.startswith(("RELAISDESK_AUTH_REQUIRED=", "RELAISDESK_AUTH_PUBLIC_KEYS="))) for c in a]).encode()).hexdigest())'; }
auth_before=$(auth_hash)
python3 - "$compose" "$env_file" "$old_image" "$new_image" "$source_url" <<'PY'
import pathlib,sys,re
c,e=map(pathlib.Path,sys.argv[1:3]); old,new,url=sys.argv[3:]
s=c.read_text(); assert s.count(old)==2
t=e.read_text(); t,n=re.subn(r'^SERVER_SOURCE_URL=.*$',f'SERVER_SOURCE_URL={url}',t,flags=re.M); assert n==1
c.with_suffix('.followup-candidate.yml').write_text(s.replace(old,new))
e.with_name('.env.followup-candidate').write_text(t)
PY
chmod 0600 /opt/rustdesk/.env.followup-candidate /opt/rustdesk/docker-compose.followup-candidate.yml
(cd /opt/rustdesk && docker compose --env-file .env.followup-candidate -f docker-compose.followup-candidate.yml config --quiet)
touched=0
rollback() {
    result=$?
    trap - ERR
    set +e
    if [[ $touched == 1 ]]; then
        cp -p "$release/docker-compose.previous.yml" "$compose"
        cp -p "$release/env.previous" "$env_file"
        (cd /opt/rustdesk && docker compose up -d --no-build --pull never hbbr hbbs)
    fi
    echo "SERVER_DEPLOY_FAILED rollback=$release; data not rewound" >&2
    exit "$result"
}
trap rollback ERR
touched=1
(cd /opt/rustdesk && docker compose stop hbbs hbbr)
tar -C /opt/rustdesk -czf "$release/data.before.tar.gz" data
chmod 0600 "$release/data.before.tar.gz"
key_before=$(find /opt/rustdesk/data -maxdepth 1 -type f -name 'id_ed25519*' -exec sha256sum {} +)
[[ -n $key_before ]]
mv -f /opt/rustdesk/docker-compose.followup-candidate.yml "$compose"
mv -f /opt/rustdesk/.env.followup-candidate "$env_file"
(cd /opt/rustdesk && docker compose up -d --no-build --pull never hbbr hbbs)
sleep 12
for container in rustdesk-hbbs rustdesk-hbbr; do
    [[ $(docker inspect --format '{{.Image}} {{.State.Running}} {{.RestartCount}}' "$container") == "$new_id true 0" ]]
    [[ $(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container") == true ]]
done
[[ $(docker exec rustdesk-hbbs sha256sum /usr/local/bin/hbbs | cut -d ' ' -f1) == "$hbbs_sha" ]]
[[ $(docker exec rustdesk-hbbr sha256sum /usr/local/bin/hbbr | cut -d ' ' -f1) == "$hbbr_sha" ]]
[[ $(auth_hash) == "$auth_before" ]]
[[ $(find /opt/rustdesk/data -maxdepth 1 -type f -name 'id_ed25519*' -exec sha256sum {} +) == "$key_before" ]]
[[ $(sha256sum "${unchanged[@]}") == "$before_files" ]]
[[ $(systemctl show wg-quick@wg0.service -p ActiveState -p ExecMainStartTimestamp --value) == "$before_vpn" ]]
[[ $(systemctl show relaisdesk-api.service -p MainPID --value) == "$api_pid" ]]
curl -fsS --max-time 10 --resolve api.relaisdesk.fr:443:127.0.0.1 https://api.relaisdesk.fr/api/v1/health
trap - ERR
echo "SERVER_DEPLOY_OK image=$new_id rollback=$release source=$source_url"
