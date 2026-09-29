#!/usr/bin/env bash
# =============================================================================
# RelaisDesk - Mise en production du suivi d'historique des connexions
# (portage de deploy-interventions-20260920.ps1 pour Linux/WSL)
# =============================================================================
# A executer depuis le poste d'exploitation :
#
#   bash scripts/deploy-interventions-20260920.sh --vps-target "ubuntu@<IP_ORACLE>"
#
# Options : --skip-tests --skip-build --skip-api-deploy --skip-downloads --skip-ovh
# Exemple re-verification OVH seule :
#   bash scripts/deploy-interventions-20260920.sh --skip-tests --skip-build --skip-api-deploy --skip-downloads
#
# Cle SSH par defaut : ~/.ssh/oracle (format OpenSSH ; convertir le .ppk
# PuTTY une fois via PuTTYgen : Conversions -> Export OpenSSH key).
#
# NOTE : la reconstruction des binaires Windows + NSIS (phase 1b d'origine)
# reste sur Windows (script .ps1). Ici, la phase 1 compile nativement
# l'API Linux + le configurateur Linux, signe le manifeste, et VERIFIE
# les artefacts Windows pre-construits (marqueur /connect) au lieu de
# les recompiler. Utilisez --skip-build si tout est deja en place.
# =============================================================================
set -euo pipefail

VPS_TARGET=""
SSH_KEY="$HOME/.ssh/oracle"
SIGNING_KEY=""
STAMP="interventions-20260920"
SKIP_TESTS=0; SKIP_BUILD=0; SKIP_API_DEPLOY=0; SKIP_DOWNLOADS=0; SKIP_OVH=0

while [ $# -gt 0 ]; do
    case "$1" in
        --vps-target) VPS_TARGET="$2"; shift 2 ;;
        --ssh-key) SSH_KEY="$2"; shift 2 ;;
        --signing-key) SIGNING_KEY="$2"; shift 2 ;;
        --stamp) STAMP="$2"; shift 2 ;;
        --skip-tests) SKIP_TESTS=1; shift ;;
        --skip-build) SKIP_BUILD=1; shift ;;
        --skip-api-deploy) SKIP_API_DEPLOY=1; shift ;;
        --skip-downloads) SKIP_DOWNLOADS=1; shift ;;
        --skip-ovh) SKIP_OVH=1; shift ;;
        -h|--help) sed -n '2,22p' "$0"; exit 0 ;;
        *) echo "Option inconnue : $1" >&2; exit 1 ;;
    esac
done

if ! printf '%s' "$STAMP" | grep -Eq '^[A-Za-z0-9._-]+$'; then echo "Stamp invalide." >&2; exit 1; fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT_DIR="$ROOT/scripts"
PREBUILT_DIR="$ROOT/prebuilt/api-$STAMP"
DOWNLOADS_DIR="$ROOT/relaisdesk/downloads"
SSH_OPTS=(-o BatchMode=yes -i "$SSH_KEY")

step() { echo ""; echo "===> $1"; }
ok() { echo "  [OK] $1"; }

assert_command() {
    if ! command -v "$1" >/dev/null 2>&1; then echo "Commande introuvable : $1 (ajoutez-la au PATH puis relancez)" >&2; exit 1; fi
}

# ---------------------------------------------------------------------------
# Phase 0 - Prerequis + tests
# ---------------------------------------------------------------------------
step "Phase 0 - Prerequis"
assert_command "go"
if [ "$SKIP_API_DEPLOY" -eq 0 ] || [ "$SKIP_DOWNLOADS" -eq 0 ]; then
    assert_command "ssh"
    assert_command "scp"
    if [ -z "$VPS_TARGET" ]; then echo "Parametre --vps-target requis (ex : ubuntu@1.2.3.4)" >&2; exit 1; fi
    if [ ! -f "$SSH_KEY" ]; then echo "Cle introuvable : $SSH_KEY" >&2; exit 1; fi
fi
if [ "$SKIP_BUILD" -eq 0 ]; then
    if [ -z "$SIGNING_KEY" ]; then echo "Parametre --signing-key requis pour la phase de compilation." >&2; exit 1; fi
    if [ ! -f "$SIGNING_KEY" ]; then echo "Cle de signature introuvable : $SIGNING_KEY" >&2; exit 1; fi
fi

if [ "$SKIP_TESTS" -eq 0 ]; then
    step "Phase 0 - Tests Go"
    for mod in database keygen api tests installer/configurator; do
        ( cd "$ROOT/$mod" && go test -count=1 ./... ) || { echo "Tests en echec : $mod" >&2; exit 1; }
        ok "Tests verts : $mod"
    done
    step "Phase 0 - Syntaxe des fichiers macOS (configurateur)"
    DARWIN_FILES=$(cd "$ROOT/installer/configurator" && ls *darwin*.go 2>/dev/null || true)
    if [ -z "$DARWIN_FILES" ]; then echo "Aucun fichier darwin trouve" >&2; exit 1; fi
    # shellcheck disable=SC2086
    DARWIN_ERR=$(cd "$ROOT/installer/configurator" && gofmt -e $DARWIN_FILES 2>&1 >/dev/null || true)
    if [ -n "$DARWIN_ERR" ]; then echo "Erreur de syntaxe darwin : $DARWIN_ERR" >&2; exit 1; fi
    ok "Fichiers darwin syntaxiquement valides"
    for mod in database api installer/configurator; do
        UNFMT=$(cd "$ROOT/$mod" && gofmt -l . 2>/dev/null || true)
        if [ -n "$UNFMT" ]; then echo "  [AVERTISSEMENT] gofmt signale : $(echo "$UNFMT" | tr '\n' ' ')" >&2; fi
    done
else
    echo "  [INFO] Tests ignores (--skip-tests)"
fi

# ---------------------------------------------------------------------------
# Phase 1 - Compilation
# ---------------------------------------------------------------------------
if [ "$SKIP_BUILD" -eq 0 ]; then
    step "Phase 1a - Compilation API Linux"
    mkdir -p "$PREBUILT_DIR"
    (
        cd "$ROOT/api"
        export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
        go build -trimpath -o "$PREBUILT_DIR/api" . || { echo "Build api echoue" >&2; exit 1; }
        for tool in "backup:relaisdesk-backup" "emailcheck:relaisdesk-emailcheck" "dbcheck:relaisdesk-dbcheck"; do
            src="${tool%%:*}"; out="${tool##*:}"
            go build -trimpath -o "$PREBUILT_DIR/$out" "./cmd/$src" || { echo "Build $out echoue" >&2; exit 1; }
        done
    )
    ( cd "$PREBUILT_DIR" && sha256sum api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck > SHA256SUMS.txt )
    ok "API Linux compilee : $PREBUILT_DIR"

    step "Phase 1b - Artefacts Windows pre-construits (verification)"
    for f in RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Setup_1.0.0.exe; do
        if [ ! -f "$DOWNLOADS_DIR/$f" ]; then
            echo "Artefact Windows manquant : $DOWNLOADS_DIR/$f (construire d'abord cote Windows, ou --skip-build)" >&2; exit 1
        fi
    done
    if ! grep -q -a "/connect" "$DOWNLOADS_DIR/RelaisDesk_Technicien_Portable.exe"; then
        echo "Le binaire reconstruit ne contient pas le suivi d'intervention (/connect absent)" >&2; exit 1
    fi
    ok "Configurateur Windows verifie avec le suivi d'intervention"

    step "Phase 1b - Compilation configurateur Linux"
    LINUX_BUILD="$PREBUILT_DIR/linux-build"
    mkdir -p "$LINUX_BUILD" "$ROOT/installer/build"
    ( cd "$ROOT/installer/debpack" && go build -trimpath -mod=readonly -o "$LINUX_BUILD/debpack" . ) \
        || { echo "Build debpack echoue" >&2; exit 1; }
    (
        cd "$ROOT/installer/configurator"
        export GOOS=linux GOARCH=amd64 CGO_ENABLED=0
        go build -ldflags "-s -w -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=1.0.0 -X main.RELEASE_PUBLIC_KEY=K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo -X main.RUSTDESK_EXPECTED_SHA256=58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=190b938b370284e091242e915ecebefdacd23eeba142f227d1decb11bc629b30" \
            -o "$LINUX_BUILD/relaisdesk-configurator" . || { echo "Linux configurator build failed" >&2; exit 1; }
    )
    cp -f "$LINUX_BUILD/relaisdesk-configurator" "$DOWNLOADS_DIR/RelaisDesk_Technicien_Linux"
    "$LINUX_BUILD/debpack" -bin "$LINUX_BUILD/relaisdesk-configurator" -pkg "relaisdesk-configurator" -name "relaisdesk-configurator" -version "1.0.0" -desc "RelaisDesk Technicien - Support a distance" -out "$DOWNLOADS_DIR/RelaisDesk_Technicien.deb" \
        || { echo "Debpack failed" >&2; exit 1; }
    cp -f "$DOWNLOADS_DIR/RelaisDesk_Technicien.deb" "$ROOT/installer/build/RelaisDesk_Technicien.deb"
    ok "Configurateur Linux compile et paquet DEB genere"

    step "Phase 1c - Signature du manifeste"
    bash "$SCRIPT_DIR/sign-manifest.sh" --version "1.0.0" --signing-key "$SIGNING_KEY" \
        || { echo "Signature du manifeste echouee" >&2; exit 1; }
    ok "SHA256SUMS.txt et release-manifest.json regeneres et signes"
else
    echo "  [INFO] Compilation ignoree (--skip-build)"
fi

# ---------------------------------------------------------------------------
# Phase 2 - Deploiement API sur Oracle
# ---------------------------------------------------------------------------
if [ "$SKIP_API_DEPLOY" -eq 0 ]; then
    step "Phase 2 - Transfert API vers Oracle"
    for b in api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck SHA256SUMS.txt; do
        if [ ! -f "$PREBUILT_DIR/$b" ]; then echo "Binaire pre-construit manquant : $PREBUILT_DIR/$b" >&2; exit 1; fi
    done
    ssh "${SSH_OPTS[@]}" "$VPS_TARGET" "mkdir -p /tmp/relaisdesk-$STAMP" \
        || { echo "Impossible de preparer /tmp sur Oracle" >&2; exit 1; }
    scp "${SSH_OPTS[@]}" "$PREBUILT_DIR/api" "$PREBUILT_DIR/relaisdesk-backup" "$PREBUILT_DIR/relaisdesk-emailcheck" "$PREBUILT_DIR/relaisdesk-dbcheck" "$PREBUILT_DIR/SHA256SUMS.txt" "$VPS_TARGET:/tmp/relaisdesk-$STAMP/" \
        || { echo "Transfert scp de l'API echoue" >&2; exit 1; }

    REMOTE_API=$(mktemp /tmp/relaisdesk-api-XXXXXXXX.sh)
    trap 'rm -f "$REMOTE_API"' EXIT
    sed "s/__STAMP__/$STAMP/g" > "$REMOTE_API" <<'REMOTE_EOF'
set -euo pipefail
STAMP="__STAMP__"
INCOMING="/tmp/relaisdesk-${STAMP}"
DEPLOY="/opt/relaisdesk/deployments/${STAMP}/rollback-api"
DB="/data/relaisdesk/licences.db"
cd "$INCOMING"
sha256sum --check SHA256SUMS.txt
sudo install -d -m 0700 -o root -g root "$DEPLOY"
for b in api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck; do
  if [ -f "/opt/relaisdesk/api/$b" ] && [ ! -f "$DEPLOY/$b" ]; then sudo cp -p "/opt/relaisdesk/api/$b" "$DEPLOY/$b"; fi
  sudo install -m 0755 "$INCOMING/$b" "/opt/relaisdesk/api/$b.new"
  sudo mv -f "/opt/relaisdesk/api/$b.new" "/opt/relaisdesk/api/$b"
done
if [ ! -f "$DEPLOY/licences-before.db" ]; then sudo sqlite3 "$DB" ".backup '$DEPLOY/licences-before.db'"; fi
sudo chmod 0600 "$DEPLOY/licences-before.db"
CHECK=$(sudo sqlite3 "$DEPLOY/licences-before.db" "PRAGMA integrity_check;")
[ "$CHECK" = "ok" ] || { echo "integrity_check: $CHECK"; exit 1; }
sudo systemctl restart relaisdesk-api
sleep 5
[ "$(systemctl is-active relaisdesk-api)" = "active" ] || { sudo systemctl status relaisdesk-api --no-pager; exit 1; }
curl --fail --silent --show-error --max-time 10 --resolve api.relaisdesk.fr:8443:127.0.0.1 https://api.relaisdesk.fr:8443/api/v1/health
echo ""
PID=$(systemctl show relaisdesk-api -p MainPID --value)
test "$PID" -gt 0
EXE_SHA=$(sudo sha256sum "/proc/$PID/exe" | awk '{print $1}')
DISK_SHA=$(sudo sha256sum /opt/relaisdesk/api/api | awk '{print $1}')
[ "$EXE_SHA" = "$DISK_SHA" ] || { echo "MISMATCH exe=$EXE_SHA disk=$DISK_SHA"; exit 1; }
echo "API ACTIVE exe_sha=$EXE_SHA"
REMOTE_EOF
    step "Phase 2 - Installation et redemarrage API"
    ssh "${SSH_OPTS[@]}" "$VPS_TARGET" 'bash -s' < "$REMOTE_API" \
        || { echo "Deploiement API echoue (rollback: /opt/relaisdesk/deployments/$STAMP/rollback-api/)" >&2; exit 1; }
    ok "API deployee et saine sur Oracle"
else
    echo "  [INFO] Deploiement API ignore (--skip-api-deploy)"
fi

# ---------------------------------------------------------------------------
# Phase 4 - Distribution configurateur (downloads Oracle)
# ---------------------------------------------------------------------------
if [ "$SKIP_DOWNLOADS" -eq 0 ]; then
    step "Phase 4 - Transfert downloads vers Oracle"
    DL_FILES="RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Linux RelaisDesk_Technicien.deb RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_Setup.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux SHA256SUMS.txt release-manifest.json"
    for p in $DL_FILES; do
        if [ ! -f "$DOWNLOADS_DIR/$p" ]; then echo "Fichier manquant : $DOWNLOADS_DIR/$p" >&2; exit 1; fi
    done
    ssh "${SSH_OPTS[@]}" "$VPS_TARGET" "mkdir -p /tmp/relaisdesk-$STAMP-downloads" \
        || { echo "Impossible de preparer /tmp sur Oracle pour les downloads" >&2; exit 1; }
    DL_ARGS=()
    for p in $DL_FILES; do DL_ARGS+=("$DOWNLOADS_DIR/$p"); done
    scp "${SSH_OPTS[@]}" "${DL_ARGS[@]}" "$VPS_TARGET:/tmp/relaisdesk-$STAMP-downloads/" \
        || { echo "Transfert scp des downloads echoue" >&2; exit 1; }

    REMOTE_DL=$(mktemp /tmp/relaisdesk-dl-XXXXXXXX.sh)
    trap 'rm -f "$REMOTE_API" "$REMOTE_DL"' EXIT
    sed "s/__STAMP__/$STAMP/g" > "$REMOTE_DL" <<'REMOTE_EOF'
set -euo pipefail
STAMP="__STAMP__"
INCOMING="/tmp/relaisdesk-${STAMP}-downloads"
DEPLOY="/opt/relaisdesk/deployments/${STAMP}/rollback-downloads"
TARGET="/opt/relaisdesk/downloads"
FILES="RelaisDesk_Technicien_Portable.exe RelaisDesk_Technicien_Linux RelaisDesk_Technicien.deb RelaisDesk_Technicien_Setup_1.0.0.exe RelaisDesk_Setup.exe RelaisDesk_viewer.deb RelaisDesk_Viewer_Linux SHA256SUMS.txt release-manifest.json"
sudo install -d -m 0700 -o root -g root "$DEPLOY"
for f in $FILES; do
  [ -f "$INCOMING/$f" ] || { echo "manquant: $f"; exit 1; }
  if [ -f "$TARGET/$f" ] && [ ! -f "$DEPLOY/$f" ]; then sudo cp -p "$TARGET/$f" "$DEPLOY/$f"; fi
  sudo install -m 0644 --owner=$(stat -c '%U' "$TARGET") --group=$(stat -c '%G' "$TARGET") "$INCOMING/$f" "$TARGET/$f.new"
  sudo mv -f "$TARGET/$f.new" "$TARGET/$f"
done
(cd "$TARGET" && sudo -u "$(stat -c '%U' .)" sha256sum --check SHA256SUMS.txt)
echo "DOWNLOADS OK"
REMOTE_EOF
    ssh "${SSH_OPTS[@]}" "$VPS_TARGET" 'bash -s' < "$REMOTE_DL" \
        || { echo "Deploiement downloads echoue" >&2; exit 1; }

    LOCAL_SIG=$(python3 -c "import json; print(json.load(open('$DOWNLOADS_DIR/release-manifest.json'))['signature'])")
    REMOTE_SIG=$(curl -fsS -m 30 "https://api.relaisdesk.fr/api/v1/downloads/release-manifest.json" | python3 -c "import json,sys; print(json.load(sys.stdin)['signature'])")
    if [ "$REMOTE_SIG" != "$LOCAL_SIG" ]; then echo "Manifeste public different du manifeste signe local" >&2; exit 1; fi
    ok "Downloads Oracle a jour, manifeste public verifie"
else
    echo "  [INFO] Downloads ignores (--skip-downloads)"
fi

# ---------------------------------------------------------------------------
# Phase 3 - OVH (transfert manuel + verification)
# ---------------------------------------------------------------------------
if [ "$SKIP_OVH" -eq 0 ]; then
    step "Phase 3 - Transfert OVH (manuel)"
    echo "  Transferez via FTP OVH / SFTP / gestionnaire OVH (binaire, sans conversion) :"
    echo "    OBLIGATOIRE : relaisdesk/client/app.js  ->  <racine web>/client/app.js"
    echo "    OBLIGATOIRE : relaisdesk/client/index.html  ->  <racine web>/client/index.html"
    echo "    OBLIGATOIRE : relaisdesk/technicien/app.js  ->  <racine web>/technicien/app.js"
    echo "    OBLIGATOIRE : relaisdesk/technicien/index.html  ->  <racine web>/technicien/index.html"
    echo "    MIROIR (recommande) : les 9 fichiers relaisdesk/downloads/ mis a jour -> <racine web>/downloads/"
    read -r -p "  Appuyez sur Entree apres le transfert"
    if ! curl -fsS -m 30 "https://relaisdesk.fr/client/index.html" | grep -q "refreshInterventionsButton"; then
        echo "OVH : refreshInterventionsButton absent de client/index.html en ligne" >&2; exit 1
    fi
    if ! curl -fsS -m 30 "https://relaisdesk.fr/client/app.js" | grep -q "fetchInterventions"; then
        echo "OVH : fetchInterventions absent de client/app.js en ligne" >&2; exit 1
    fi
    if ! curl -fsS -m 30 "https://relaisdesk.fr/technicien/app.js" | grep -q "complete-intervention"; then
        echo "OVH : complete-intervention absent de technicien/app.js en ligne" >&2; exit 1
    fi
    ok "Fichiers OVH verifies en ligne"
else
    echo "  [INFO] OVH ignore (--skip-ovh)"
fi

# ---------------------------------------------------------------------------
# Phase 5 - Recette (manuelle, guidee)
# ---------------------------------------------------------------------------
step "Phase 5 - Recette en situation reelle (manuelle)"
echo "  1. Configurateur : connexion au poste DEV-AYWM-QARC-PKWA-Q5AR."
echo "  2. Sur Oracle : sqlite3 /data/relaisdesk/licences.db \"SELECT intervention_id,status,started_at FROM interventions ORDER BY id DESC LIMIT 3;\""
echo "     attendu : status=in_progress + started_at ; apres fermeture : completed + duree >= 1."
echo "  3. Double-clic Se connecter : une seule fiche (deduplication 5 min)."
echo "  4. Code viewer : creation -> client_ready -> Connexion directe (in_progress) -> fin (completed)."
echo "  5. Espace client #interventions : fiche immediate, badge En cours, bouton Actualiser."
HEALTH_JSON=$(curl -fsS -m 30 "https://api.relaisdesk.fr/api/v1/health")
echo "$HEALTH_JSON" | python3 -c "import json,sys; d=json.load(sys.stdin); print('  Sante API publique : status='+d.get('status','')+' database='+d.get('database',''))"
echo ""
echo "Mise en production terminee."
