#!/usr/bin/env bash
set -Eeuo pipefail
export PATH=/root/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
work=/root/relaisdesk-followup-20261023
source=$work/aom-3.14.1
if [[ ! -d $source ]]; then
    git clone --branch v3.14.1 --depth 1 https://aomedia.googlesource.com/aom "$source"
fi
[[ $(git -C "$source" rev-parse HEAD) == 03087864cf4bea6abb0d28f95cf7843511413d8f ]]
grep -q 'spatial_layer_id < 0' "$source/av1/av1_cx_iface.c"
cmake -S "$source" -B "$work/aom-build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$work/native" -DCMAKE_INSTALL_LIBDIR=lib -DCONFIG_PIC=1 -DBUILD_SHARED_LIBS=OFF -DENABLE_DOCS=OFF -DENABLE_EXAMPLES=OFF -DENABLE_TESTDATA=OFF -DENABLE_TESTS=OFF -DENABLE_TOOLS=OFF
cmake --build "$work/aom-build" --parallel 2
cmake --install "$work/aom-build"
echo AOM_NATIVE_BUILD_OK
