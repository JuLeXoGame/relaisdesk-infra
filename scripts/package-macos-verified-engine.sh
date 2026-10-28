#!/usr/bin/env bash
# Keep the signed Flutter app intact inside each Go launcher bundle.
set -Eeuo pipefail
: "${ENGINE_APP:?}" "${ENGINE_SHA256:?}" "${BUILD_ARCH:?}" "${RUNNER_TEMP:?}"
[[ $(uname -s) == Darwin && $ENGINE_SHA256 =~ ^[0-9a-f]{64}$ ]]
case "$BUILD_ARCH" in
    arm64) native_arch=arm64; suffix=''; minimum=12.3 ;;
    amd64) native_arch=x86_64; suffix=_Intel; minimum=12.0 ;;
    *) exit 2 ;;
esac
export GOOS=darwin GOARCH="$BUILD_ARCH" CGO_ENABLED=1 MACOSX_DEPLOYMENT_TARGET="$minimum"
export CC="clang -arch $native_arch" CXX="clang++ -arch $native_arch"
engine_name=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$ENGINE_APP/Contents/Info.plist")
[[ $engine_name == RelaisDesk ]]
engine_relative=Contents/Helpers/RelaisDeskEngine.app
engine_executable="$engine_relative/Contents/MacOS/$engine_name"
[[ $(shasum -a 256 "$ENGINE_APP/Contents/MacOS/$engine_name" | cut -d' ' -f1) == "$ENGINE_SHA256" ]]
codesign --verify --deep --strict "$ENGINE_APP"
flags="-s -w -X main.APP_VERSION=1.0.0 -X main.APIURL=https://api.relaisdesk.fr -X main.RUSTDESK_EXPECTED_SHA256=$ENGINE_SHA256"
for product in viewer configurator; do
    if [[ $product == viewer ]]; then
        name=RelaisDesk; identifier=fr.relaisdesk.viewer; image="RelaisDesk_Mac$suffix.dmg"
    else
        name=RelaisDesk_Technicien; identifier=fr.relaisdesk.technicien; image="RelaisDesk_Technicien_Mac$suffix.dmg"
    fi
    staging="$RUNNER_TEMP/relaisdesk-package-$product"
    [[ ! -e $staging ]]
    bundle="$staging/$name.app"
    mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources" "$bundle/Contents/Helpers"
    ditto "$ENGINE_APP" "$bundle/$engine_relative"
    go -C "installer/$product" build -trimpath -mod=readonly -ldflags "$flags" -o "$bundle/Contents/MacOS/$name" .
    python3 - "$bundle" "$name" "$identifier" "$minimum" "$ENGINE_APP" <<'PY'
import pathlib, plistlib, shutil, sys
bundle, name, identifier, minimum, engine = sys.argv[1:]
bundle, engine = pathlib.Path(bundle), pathlib.Path(engine)
info = dict(CFBundleExecutable=name, CFBundleIdentifier=identifier,
            CFBundleName=name, CFBundleDisplayName=name,
            CFBundleInfoDictionaryVersion='6.0', CFBundlePackageType='APPL',
            CFBundleShortVersionString='1.0.0', CFBundleVersion='1.0.0',
            LSMinimumSystemVersion=minimum, NSHighResolutionCapable=True,
            NSPrincipalClass='NSApplication')
icons = sorted((engine/'Contents/Resources').glob('*.icns'))
if icons:
    shutil.copy2(icons[0], bundle/'Contents/Resources'/icons[0].name)
    info['CFBundleIconFile'] = icons[0].name
with (bundle/'Contents/Info.plist').open('wb') as output:
    plistlib.dump(info, output)
PY
    lipo "$bundle/Contents/MacOS/$name" -verify_arch "$native_arch"
    codesign --force --sign - "$bundle"
    codesign --verify --deep --strict "$bundle"
    [[ $(shasum -a 256 "$bundle/$engine_executable" | cut -d' ' -f1) == "$ENGINE_SHA256" ]]
    test -x "$bundle/$engine_relative/Contents/MacOS/service"
    ln -s /Applications "$staging/Applications"
    hdiutil create -volname "$name" -srcfolder "$staging" -format UDZO "$image"
    hdiutil verify "$image"
    check_mount="$RUNNER_TEMP/relaisdesk-mount-$product"
    mkdir -p "$check_mount"
    hdiutil attach -readonly -nobrowse -mountpoint "$check_mount" "$image"
    trap 'hdiutil detach "$check_mount" || true' EXIT
    mounted_bundle="$check_mount/$name.app"
    test -d "$mounted_bundle/$engine_relative/Contents/Frameworks"
    test -d "$mounted_bundle/$engine_relative/Contents/Resources"
    test -x "$mounted_bundle/$engine_relative/Contents/MacOS/service"
    test "$(readlink "$check_mount/Applications")" = /Applications
    codesign --verify --deep --strict "$mounted_bundle"
    [[ $(shasum -a 256 "$mounted_bundle/$engine_executable" | cut -d' ' -f1) == "$ENGINE_SHA256" ]]
    [[ $(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$mounted_bundle/Contents/Info.plist") == 1.0.0 ]]
    hdiutil detach "$check_mount"
    trap - EXIT
    echo "MACOS_PACKAGE_VERIFIED $image engine=$ENGINE_SHA256"
done
shasum -a 256 "RelaisDesk_Mac$suffix.dmg" "RelaisDesk_Technicien_Mac$suffix.dmg" > "SHA256SUMS-macos-$BUILD_ARCH.txt"
[[ $(shasum -a 256 "RelaisDesk_Mac$suffix.dmg" | cut -d' ' -f1) != $(shasum -a 256 "RelaisDesk_Technicien_Mac$suffix.dmg" | cut -d' ' -f1) ]]
