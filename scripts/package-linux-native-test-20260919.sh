#!/usr/bin/env bash
set -euo pipefail
# Run only in the local Ubuntu build environment, never on the production VPS.
work=/root/relaisdesk-security-20260919
bundle=$work/native-package
output=/mnt/c/Users/Administrator/Documents/Projets/projet/.cache/oracle-security-20260919
library=/root/rustdesk/target/release/liblibrustdesk.so
[[ -d $work/client && -f $library && ! -e $bundle ]]
[[ $library -nt $work/client/Cargo.toml ]]
dpkg-deb --raw-extract /mnt/c/Users/Administrator/Documents/Projets/projet/installer/viewer/embedded/rustdesk.deb "$bundle"
install -m 0755 "$library" "$bundle/usr/share/rustdesk/lib/librustdesk.so"
patchelf --set-rpath '$ORIGIN' "$bundle/usr/share/rustdesk/lib/librustdesk.so"
[[ $(patchelf --print-rpath "$bundle/usr/share/rustdesk/lib/librustdesk.so") == '$ORIGIN' ]]
python3 - "$bundle/DEBIAN/control" <<'PY'
import pathlib,sys,re
p=pathlib.Path(sys.argv[1]); s=p.read_text()
s,n=re.subn(r'^Version:.*$', 'Version: 1.4.9-1relaisdesk20260919',s,flags=re.M); assert n==1
p.write_text(s)
PY
(cd "$bundle" && find usr etc -type f -print0 | sort -z | xargs -0 md5sum > DEBIAN/md5sums)
dpkg-deb --build --root-owner-group --uniform-compression -Zgzip "$bundle" "$output/rustdesk-security-linux-amd64.deb"
cp -- "$bundle/usr/share/rustdesk/rustdesk" "$output/rustdesk-linux-runner"
cp -- "$bundle/usr/share/rustdesk/lib/librustdesk.so" "$output/librustdesk.so"
sha256sum "$output/rustdesk-security-linux-amd64.deb" "$output/rustdesk-linux-runner" "$output/librustdesk.so"
echo 'Native test package only; nothing installed or published.'
