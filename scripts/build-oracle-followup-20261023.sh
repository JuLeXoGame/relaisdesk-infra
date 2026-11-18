#!/usr/bin/env bash
set -Eeuo pipefail
export PATH=/root/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
project=/mnt/c/Users/Administrator/Documents/Projets/projet
work=/root/relaisdesk-followup-20261023
out=$project/.cache/deploy-20261023
mkdir -p "$out"
component=${1:?api|server|client}
case "$component" in
api)
    export PATH=/home/julien/sdk/go/bin:$PATH
    export GOTOOLCHAIN=go1.26.11 GOCACHE=/home/julien/.cache/go-build
    export CGO_ENABLED=0 GOOS=linux GOARCH=amd64
    cd "$project/api"
    go version
    go build -trimpath -mod=readonly -ldflags='-s -w' -o "$out/api" .
    for name in backup dbcheck emailcheck; do
        go build -trimpath -mod=readonly -ldflags='-s -w' -o "$out/relaisdesk-$name" "./cmd/$name"
    done
    ;;
server)
    mkdir -p "$work"
    [[ ! -e $work/server ]]
    mkdir "$work/server"
    tar -C "$project/rustdesk-server" --exclude=.git --exclude=target --exclude='.env*' --exclude='*.db*' --exclude='id_ed25519*' -cf - . | tar -C "$work/server" -xf -
    cd "$work/server"
    export CARGO_TARGET_DIR=/root/relaisdesk-security-20260919/server-target CARGO_BUILD_JOBS=2
    cargo test --locked --lib
    cargo build --locked --release --bins
    install -m 0755 "$CARGO_TARGET_DIR/release/hbbs" "$CARGO_TARGET_DIR/release/hbbr" "$out/"
    ;;
client|client-resume)
    mkdir -p "$work"
    if [[ $component == client ]]; then
        [[ ! -e $work/client ]]
        mkdir "$work/client"
        tar -C "$project/rustdesk" --exclude=.git --exclude=target --exclude=.dart_tool --exclude=build --exclude=ephemeral --exclude='.env*' -cf - . | tar -C "$work/client" -xf -
    fi
    cd "$work/client"
    # Reuse the bridge from the existing native bundle only for identical FFI input.
    bridge_source=/root/relaisdesk-security-20260919/client
    cmp src/flutter_ffi.rs "$bridge_source/src/flutter_ffi.rs"
    cp "$bridge_source/src/bridge_generated.rs" src/bridge_generated.rs
    for generated in "$bridge_source"/src/bridge_generated.*.rs; do
        [[ -f $generated ]]
        cp "$generated" src/
    done
    cp -a "$project/rustdesk/res/vcpkg/aom/." res/vcpkg/aom/
    export CARGO_TARGET_DIR=/root/rustdesk/target CARGO_BUILD_JOBS=2 LIBCLANG_PATH=/usr/lib/llvm-14/lib
    export PKG_CONFIG_PATH="$work/native/lib/pkgconfig"
    export AOM_STATIC=1 VPX_STATIC=1 LIBYUV_STATIC=1
    # pkg-config avoids static archives in system directories; use a private prefix.
    cp /usr/lib/x86_64-linux-gnu/libvpx.a /usr/lib/x86_64-linux-gnu/libyuv.a "$work/native/lib/"
    cp -a /usr/include/vpx /usr/include/libyuv "$work/native/include/"
    cp /usr/include/libyuv.h "$work/native/include/"
    python3 - "$work/native" <<'PY'
import pathlib,sys,re
prefix=pathlib.Path(sys.argv[1])
for name in ('vpx','libyuv'):
    original=pathlib.Path('/usr/lib/x86_64-linux-gnu/pkgconfig')/(name+'.pc')
    s=original.read_text()
    s=re.sub(r'^prefix=.*$', 'prefix='+str(prefix),s,flags=re.M)
    s=re.sub(r'^libdir=.*$', 'libdir=${prefix}/lib',s,flags=re.M)
    s=re.sub(r'^includedir=.*$', 'includedir=${prefix}/include',s,flags=re.M)
    s=s.replace('-L/usr/lib/x86_64-linux-gnu','-L${libdir}').replace('-I/usr/include','-I${includedir}')
    (prefix/'lib/pkgconfig'/(name+'.pc')).write_text(s)
PY
    touch libs/scrap/build.rs
    [[ $(pkg-config --modversion aom) == 3.14.1 ]]
    cargo build --locked --lib --release --features flutter,unix-file-copy-paste,linux-pkg-config
    bundle=$(mktemp -d "$work/native-package.XXXXXXXX")
    dpkg-deb --raw-extract "$project/installer/viewer/embedded/rustdesk.deb" "$bundle"
    install -m 0755 "$CARGO_TARGET_DIR/release/liblibrustdesk.so" "$bundle/usr/share/rustdesk/lib/librustdesk.so"
    patchelf --set-rpath '$ORIGIN' "$bundle/usr/share/rustdesk/lib/librustdesk.so"
    sed -i 's/^Version:.*/Version: 1.4.9-1relaisdesk20261023/' "$bundle/DEBIAN/control"
    python3 "$project/scripts/set-linux-native-dependencies.py" "$bundle/DEBIAN/control"
    (cd "$bundle" && find usr etc -type f -print0 | sort -z | xargs -0 md5sum > DEBIAN/md5sums)
    dpkg-deb --build --root-owner-group --uniform-compression -Zgzip "$bundle" "$out/rustdesk-linux-amd64.deb"
    cp "$bundle/usr/share/rustdesk/rustdesk" "$out/rustdesk-linux-runner"
    cp "$bundle/usr/share/rustdesk/lib/librustdesk.so" "$out/librustdesk.so"
    ;;
*) exit 2 ;;
esac
echo "BUILD_OK $component"
