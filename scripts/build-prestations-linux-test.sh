#!/usr/bin/env bash
# Local WSL build only. Does not connect to Oracle, install or publish anything.
set -euo pipefail
project=/mnt/c/Users/Administrator/Documents/Projets/projet
output="$project/bin/tests-prestations-20260919/linux-engine"
[[ -d /root/relaisdesk-security-20260919/client && ! -e "$output" ]]
work=$(mktemp -d /tmp/relaisdesk-prestations-build.XXXXXXXX)
cp -a /root/relaisdesk-security-20260919/client "$work/client"
# Replace the staged source with the current source, not a previous security build.
for item in Cargo.toml Cargo.lock build.rs src libs res .cargo; do
  [[ -e "$project/rustdesk/$item" ]] || continue
  tar -C "$project/rustdesk" --exclude=.git --exclude=target -cf - "$item" | tar -C "$work/client" -xf -
done
export CARGO_TARGET_DIR=/root/rustdesk/target
export CARGO_NET_OFFLINE=true
export CARGO_BUILD_JOBS=2
export LIBCLANG_PATH=/usr/lib/llvm-14/lib
export PATH=/root/.cargo/bin:$PATH
cd "$work/client"
cargo build --offline --locked --lib --release --features flutter,unix-file-copy-paste,linux-pkg-config
library="$CARGO_TARGET_DIR/release/liblibrustdesk.so"
[[ -f "$library" && "$library" -nt "$work/client/src/relaisdesk_meter.rs" ]]
strings "$library" | grep 'billing-meter-v1' >/dev/null || { echo 'Missing new meter in built library'; exit 1; }
bundle="$work/package"
dpkg-deb --raw-extract "$project/installer/viewer/embedded/rustdesk.deb" "$bundle"
install -m 0755 "$library" "$bundle/usr/share/rustdesk/lib/librustdesk.so"
patchelf --set-rpath '$ORIGIN' "$bundle/usr/share/rustdesk/lib/librustdesk.so"
sed -i 's/^Version:.*/Version: 1.4.9-1relaisdesk20260919prestations/' "$bundle/DEBIAN/control"
(cd "$bundle" && find usr etc -type f -print0 | sort -z | xargs -0 md5sum > DEBIAN/md5sums)
mkdir "$output"
dpkg-deb --build --root-owner-group --uniform-compression -Zgzip "$bundle" "$output/rustdesk-prestations-linux-amd64.deb"
cp -- "$bundle/usr/share/rustdesk/rustdesk" "$output/rustdesk-linux-runner"
cp -- "$bundle/usr/share/rustdesk/lib/librustdesk.so" "$output/librustdesk.so"
(cd "$output" && sha256sum rustdesk-prestations-linux-amd64.deb rustdesk-linux-runner librustdesk.so > SHA256SUMS.txt)
printf 'Linux candidate ready: %s\nStaged source: %s\n' "$output" "$work/client"
