#!/usr/bin/env bash
# RelaisDesk - Construction du programme de test Linux (portage de build-linux-test-release.ps1).
# Sur Linux, debpack est compile natif (pas de .exe) puis execute directement.
set -euo pipefail

NATIVE_PACKAGE=""; PACKAGE_SHA256=""; RUNNER_SHA256=""; LIBRARY_SHA256=""
WORKDIR=""; OUTDIR=""; RELEASE_PUBLIC_KEY=""; GOCACHE_DIR=""
GO_EXE="go"; VERSION="1.0.4"

while [ $# -gt 0 ]; do
    case "$1" in
        --native-package) NATIVE_PACKAGE="$2"; shift 2 ;;
        --package-sha256) PACKAGE_SHA256="$2"; shift 2 ;;
        --runner-sha256) RUNNER_SHA256="$2"; shift 2 ;;
        --library-sha256) LIBRARY_SHA256="$2"; shift 2 ;;
        --working-directory) WORKDIR="$2"; shift 2 ;;
        --output-directory) OUTDIR="$2"; shift 2 ;;
        --release-public-key) RELEASE_PUBLIC_KEY="$2"; shift 2 ;;
        --go-cache-directory) GOCACHE_DIR="$2"; shift 2 ;;
        --go-executable) GO_EXE="$2"; shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        -h|--help) echo "Usage : $0 --native-package P --package-sha256 H --runner-sha256 H --library-sha256 H --working-directory W --output-directory O --release-public-key K --go-cache-directory G [--go-executable go] [--version 1.0.4]"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

for v in NATIVE_PACKAGE PACKAGE_SHA256 RUNNER_SHA256 LIBRARY_SHA256 WORKDIR OUTDIR RELEASE_PUBLIC_KEY GOCACHE_DIR; do
    if [ -z "${!v}" ]; then echo "Parametre manquant : $v" >&2; exit 1; fi
done
case "$PACKAGE_SHA256$RUNNER_SHA256$LIBRARY_SHA256" in
    *[!a-fA-F0-9]*|"") echo "Empreinte SHA-256 invalide." >&2; exit 1 ;;
esac
if [ "${#PACKAGE_SHA256}" -ne 64 ] || [ "${#RUNNER_SHA256}" -ne 64 ] || [ "${#LIBRARY_SHA256}" -ne 64 ]; then
    echo "Empreinte SHA-256 invalide." >&2; exit 1
fi
if ! printf '%s' "$RELEASE_PUBLIC_KEY" | grep -Eq '^[A-Za-z0-9_-]{43}$'; then echo "Cle publique invalide." >&2; exit 1; fi
if ! printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then echo "Version invalide." >&2; exit 1; fi

PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(realpath -m "$WORKDIR")"
OUTPUT="$(realpath -m "$OUTDIR")"
WEB="$(realpath -m "$PROJECT/relaisdesk")"
for d in "$WORK" "$OUTPUT"; do
    if [ -e "$d" ]; then echo "Use new test directories; no existing files may be overwritten." >&2; exit 1; fi
    if [ "$d" = "$WEB" ] || [ "${d#$WEB/}" != "$d" ]; then echo "Test programs must stay outside the website." >&2; exit 1; fi
done
ACTUAL_PKG="$(sha256sum "$NATIVE_PACKAGE" | awk '{print $1}')"
if [ "${ACTUAL_PKG,,}" != "${PACKAGE_SHA256,,}" ]; then echo "Native package digest mismatch." >&2; exit 1; fi
mkdir -p "$WORK" "$OUTPUT"

(
    export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
    if [ -z "${GOTOOLCHAIN:-}" ] || [ "$GOTOOLCHAIN" = "local" ]; then export GOTOOLCHAIN=auto; fi
    export GOCACHE
    GOCACHE="$(realpath -m "$GOCACHE_DIR")"
    DEBPACK="$WORK/debpack"
    ( cd "$PROJECT/installer/debpack" && "$GO_EXE" build -trimpath -mod=readonly -o "$DEBPACK" . ) \
        || { echo "Go failed in installer/debpack" >&2; exit 1; }
    for app in viewer configurator; do
        STAGED="$WORK/sources/$app"
        mkdir -p "$STAGED/embedded"
        find "$PROJECT/installer/$app" -maxdepth 1 -type f \( -name '*.go' -o -name '*.png' -o -name '*.ico' -o -name 'go.mod' -o -name 'go.sum' \) -exec cp -t "$STAGED" {} +
        cp "$NATIVE_PACKAGE" "$STAGED/embedded/rustdesk.deb"
        FLAGS="-s -w -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$VERSION -X main.RUSTDESK_EXPECTED_SHA256=${RUNNER_SHA256,,} -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=${PACKAGE_SHA256,,} -X main.RUSTDESK_SO_EXPECTED_SHA256=${LIBRARY_SHA256,,}"
        if [ "$app" = "configurator" ]; then FLAGS="$FLAGS -X main.RELEASE_PUBLIC_KEY=$RELEASE_PUBLIC_KEY"; fi
        if [ "$app" = "viewer" ]; then NAME="RelaisDesk_Viewer_Linux"; else NAME="RelaisDesk_Technicien_Linux"; fi
        TESTFLAGS="${FLAGS//-X main./-X $app.}"
        # shellcheck disable=SC2086
        ( cd "$STAGED" && "$GO_EXE" vet -trimpath -mod=readonly -tags ci ./... ) || { echo "Go failed in $STAGED" >&2; exit 1; }
        # shellcheck disable=SC2086
        ( cd "$STAGED" && "$GO_EXE" test -c -trimpath -mod=readonly -tags ci -ldflags "$TESTFLAGS" -o "$WORK/$app.test" . ) || { echo "Go failed in $STAGED" >&2; exit 1; }
        # shellcheck disable=SC2086
        ( cd "$STAGED" && "$GO_EXE" build -trimpath -mod=readonly -ldflags "$FLAGS" -o "$OUTPUT/$NAME" . ) || { echo "Go failed in $STAGED" >&2; exit 1; }
        if [ "$app" = "viewer" ]; then PKG="RelaisDesk_viewer.deb"; else PKG="RelaisDesk_Technicien.deb"; fi
        "$DEBPACK" -bin "$OUTPUT/$NAME" -pkg "relaisdesk-$app" -name "relaisdesk-$app" -version "$VERSION" -desc "RelaisDesk $app - security test" -out "$OUTPUT/$PKG" \
            || { echo "Debian package creation failed." >&2; exit 1; }
    done
    ( cd "$OUTPUT" && sha256sum -- * | LC_ALL=C sort -k2 > SHA256SUMS.txt )
    echo "TEST_BUILD_READY $OUTPUT"
    echo "Linux tests are cross-compiled, not executed by this script. No publication or installation was performed."
)
