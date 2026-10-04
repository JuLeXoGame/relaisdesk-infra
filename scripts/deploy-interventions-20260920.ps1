# =============================================================================
# RelaisDesk - Mise en production du suivi d'historique des connexions
# =============================================================================
# A executer sur Windows (PowerShell) depuis le poste d'exploitation :
#
#   cd C:\Users\Administrator\Documents\Projets\projet
#   .\scripts\deploy-interventions-20260920.ps1 -VpsTarget "ubuntu@<IP_ORACLE>"
#
# -VpsTarget peut aussi etre le nom d'une session PuTTY enregistree.
# La cle par defaut est .secrets\oracle private.ppk (parametre -PpkPath).
# Options : -SkipTests -SkipBuild -SkipApiDeploy -SkipDownloads -SkipOvh
# Exemple re-verification OVH seule :
#   .\scripts\deploy-interventions-20260920.ps1 -SkipTests -SkipBuild -SkipApiDeploy -SkipDownloads
# =============================================================================

param(
    [string]$VpsTarget = "",
    [string]$PpkPath = "",
    [switch]$SkipTests,
    [switch]$SkipBuild,
    [switch]$SkipApiDeploy,
    [switch]$SkipDownloads,
    [switch]$SkipOvh,
    [string]$SigningKey = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$Stamp = "interventions-20260920"
$PrebuiltDir = Join-Path $Root "prebuilt\api-$Stamp"
$DownloadsDir = Join-Path $Root "relaisdesk\downloads"
if ([string]::IsNullOrWhiteSpace($SigningKey)) {
    $SigningKey = "C:\RelaisDesk-Secrets\release-signing-ed25519"
}
if ([string]::IsNullOrWhiteSpace($PpkPath)) {
    $PpkPath = Join-Path $Root ".secrets\oracle private.ppk"
}

function Write-Step($Message) {
    Write-Host ""
    Write-Host ("===> " + $Message) -ForegroundColor Cyan
}

function Write-Ok($Message) {
    Write-Host ("  [OK] " + $Message) -ForegroundColor Green
}

function Assert-Command($Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Commande introuvable : $Name (ajoutez-la au PATH puis relancez)"
    }
}

function Invoke-GoTest($ModuleDir) {
    Push-Location (Join-Path $Root $ModuleDir)
    try {
        & go test -count=1 ./...
        if ($LASTEXITCODE -ne 0) { throw "Tests en echec : $ModuleDir" }
        Write-Ok "Tests verts : $ModuleDir"
    } finally {
        Pop-Location
    }
}

# ---------------------------------------------------------------------------
# Phase 0 - Prerequis + tests
# ---------------------------------------------------------------------------
Write-Step "Phase 0 - Prerequis"
Assert-Command "go"
if (-not $SkipApiDeploy -or -not $SkipDownloads) {
    Assert-Command "pscp"
    Assert-Command "plink"
    if ([string]::IsNullOrWhiteSpace($VpsTarget)) { throw "Parametre -VpsTarget requis (ex : ubuntu@1.2.3.4)" }
    if (-not (Test-Path -LiteralPath $PpkPath)) { throw "Cle introuvable : $PpkPath" }
}
if (-not $SkipBuild) {
    Assert-Command "windres"
    if (-not (Test-Path -LiteralPath $SigningKey)) { throw "Cle de signature introuvable : $SigningKey" }
    if (-not (Test-Path -LiteralPath (Join-Path $Root "installer\build\debpack.exe"))) { throw "debpack.exe introuvable dans installer\build" }
}

if (-not $SkipTests) {
    Write-Step "Phase 0 - Tests Go"
    foreach ($mod in @("database", "keygen", "api", "tests", "installer\configurator")) {
        Invoke-GoTest $mod
    }
    Write-Step "Phase 0 - Syntaxe des fichiers macOS (configurateur)"
    Push-Location (Join-Path $Root "installer\configurator")
    try {
        # Vet/build croise darwin impossible sur Windows (fyne exige CGO sous
        # macOS) : verification syntaxique stricte a la place. Les fichiers
        # windows sont types par les tests, les fichiers linux par le build
        # linux de la phase 1.
        $darwinFiles = @((Get-ChildItem -Path . -Filter "*darwin*.go" | ForEach-Object { $_.FullName }))
        if ($darwinFiles.Count -eq 0) { throw "Aucun fichier darwin trouve" }
        $prevEAP = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        try {
            $gofmtErr = @( & gofmt -e $darwinFiles 2>&1 | Where-Object { $_ -isnot [string] -or $_.Trim() -ne "" })
        } finally {
            $ErrorActionPreference = $prevEAP
        }
        # gofmt -e renvoie le source formate sur stdout en cas de succes ; on
        # ne garde que les enregistrements d'erreur (stderr fusionne).
        $parseErrors = @($gofmtErr | Where-Object { $_ -isnot [string] })
        if ($parseErrors.Count -gt 0) {
            throw ("Erreur de syntaxe darwin : " + ($parseErrors -join " | "))
        }
        Write-Ok "Fichiers darwin syntaxiquement valides"
    } finally {
        Pop-Location
    }
    foreach ($mod in @("database", "api", "installer\configurator")) {
        Push-Location (Join-Path $Root $mod)
        try {
            $prevEAP = $ErrorActionPreference
            $ErrorActionPreference = "Continue"
            try {
                $gofmtAll = @( & gofmt -l . 2>&1 )
            } finally {
                $ErrorActionPreference = $prevEAP
            }
            $unformatted = @($gofmtAll | Where-Object { $_ -is [string] -and $_.Trim() -ne "" })
            $parseErrors = @($gofmtAll | Where-Object { $_ -isnot [string] })
            if ($unformatted) { Write-Host ("  [AVERTISSEMENT] gofmt signale : " + ($unformatted -join ", ")) -ForegroundColor Yellow }
            foreach ($pe in $parseErrors) { Write-Host ("  [AVERTISSEMENT] gofmt ne peut pas analyser : " + $pe) -ForegroundColor Yellow }
        } finally {
            Pop-Location
        }
    }
} else {
    Write-Host "  [INFO] Tests ignores (-SkipTests)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Phase 1 - Compilation
# ---------------------------------------------------------------------------
if (-not $SkipBuild) {
    Write-Step "Phase 1a - Compilation API Linux"
    New-Item -ItemType Directory -Force -Path $PrebuiltDir | Out-Null
    Push-Location (Join-Path $Root "api")
    try {
        $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
        & go build -trimpath -o (Join-Path $PrebuiltDir "api") .
        if ($LASTEXITCODE -ne 0) { throw "Build api echoue" }
        foreach ($tool in @(@("backup", "relaisdesk-backup"), @("emailcheck", "relaisdesk-emailcheck"), @("dbcheck", "relaisdesk-dbcheck"))) {
            & go build -trimpath -o (Join-Path $PrebuiltDir $tool[1]) ("./cmd/" + $tool[0])
            if ($LASTEXITCODE -ne 0) { throw ("Build " + $tool[1] + " echoue") }
        }
    } finally {
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }
    $sums = Get-ChildItem -LiteralPath $PrebuiltDir -File | Where-Object { $_.Name -ne "SHA256SUMS.txt" } | ForEach-Object {
        ((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + "  " + $_.Name)
    }
    [System.IO.File]::WriteAllText((Join-Path $PrebuiltDir "SHA256SUMS.txt"), (($sums -join "`n") + "`n"), [System.Text.UTF8Encoding]::new($false))
    Write-Ok "API Linux compilee : $PrebuiltDir"

    Write-Step "Phase 1b - Compilation configurateur technicien"
    & (Join-Path $Root "installer\build_technicien_portable.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Build portable technicien echoue" }
    & (Join-Path $Root "installer\build_linux_technicien.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Build linux technicien echoue" }
    & (Join-Path $Root "installer\rebuild_nsis.ps1") -TechnicianOnly
    if ($LASTEXITCODE -ne 0) { throw "Build NSIS technicien echoue" }
    $portable = Join-Path $DownloadsDir "RelaisDesk_Technicien_Portable.exe"
    if (-not (Select-String -LiteralPath $portable -Pattern "/connect" -SimpleMatch -Quiet)) {
        throw "Le binaire reconstruit ne contient pas le suivi d'intervention (/connect absent)"
    }
    Write-Ok "Configurateur reconstruit avec le suivi d'intervention"

    Write-Step "Phase 1c - Signature du manifeste"
    & (Join-Path $Root "installer\sign_manifest.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Signature du manifeste echouee" }
    Write-Ok "SHA256SUMS.txt et release-manifest.json regeneres et signes"
} else {
    Write-Host "  [INFO] Compilation ignoree (-SkipBuild)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Phase 2 - Deploiement API sur Oracle
# ---------------------------------------------------------------------------
if (-not $SkipApiDeploy) {
    Write-Step "Phase 2 - Transfert API vers Oracle"
    & plink -batch -i $PpkPath $VpsTarget "mkdir -p /tmp/relaisdesk-$Stamp"
    if ($LASTEXITCODE -ne 0) { throw "Impossible de preparer /tmp sur Oracle (cle hotes ? connectez-vous une fois via PuTTY)" }
    & pscp -batch -i $PpkPath (Join-Path $PrebuiltDir "api") (Join-Path $PrebuiltDir "relaisdesk-backup") (Join-Path $PrebuiltDir "relaisdesk-emailcheck") (Join-Path $PrebuiltDir "relaisdesk-dbcheck") (Join-Path $PrebuiltDir "SHA256SUMS.txt") ($VpsTarget + ":/tmp/relaisdesk-" + $Stamp + "/")
    if ($LASTEXITCODE -ne 0) { throw "Transfert pscp de l'API echoue" }

    $remoteApi = @'
set -euo pipefail
STAMP="interventions-20260920"
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
'@
    $remoteApiPath = Join-Path $env:TEMP "relaisdesk-api-$Stamp.sh"
    [System.IO.File]::WriteAllText($remoteApiPath, ($remoteApi -replace "`r", ""), [System.Text.UTF8Encoding]::new($false))
    Write-Step "Phase 2 - Installation et redemarrage API"
    & plink -batch -i $PpkPath $VpsTarget -m $remoteApiPath
    if ($LASTEXITCODE -ne 0) { throw "Deploiement API echoue - voir sortie ci-dessus (rollback: /opt/relaisdesk/deployments/$Stamp/rollback-api/)" }
    Write-Ok "API deployee et saine sur Oracle"
} else {
    Write-Host "  [INFO] Deploiement API ignore (-SkipApiDeploy)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Phase 4 - Distribution configurateur (downloads Oracle)
# ---------------------------------------------------------------------------
if (-not $SkipDownloads) {
    Write-Step "Phase 4 - Transfert downloads vers Oracle"
    $dlFiles = @(
        "RelaisDesk_Technicien_Portable.exe",
        "RelaisDesk_Technicien_Linux",
        "RelaisDesk_Technicien.deb",
        "RelaisDesk_Technicien_Setup_1.0.0.exe",
        "RelaisDesk_Setup.exe",
        "RelaisDesk_viewer.deb",
        "RelaisDesk_Viewer_Linux",
        "SHA256SUMS.txt",
        "release-manifest.json"
    )
    $dlPaths = $dlFiles | ForEach-Object { Join-Path $DownloadsDir $_ }
    foreach ($p in $dlPaths) {
        if (-not (Test-Path -LiteralPath $p)) { throw "Fichier manquant : $p" }
    }
    & plink -batch -i $PpkPath $VpsTarget "mkdir -p /tmp/relaisdesk-$Stamp-downloads"
    if ($LASTEXITCODE -ne 0) { throw "Impossible de preparer /tmp sur Oracle pour les downloads" }
    & pscp -batch -i $PpkPath @dlPaths ($VpsTarget + ":/tmp/relaisdesk-" + $Stamp + "-downloads/")
    if ($LASTEXITCODE -ne 0) { throw "Transfert pscp des downloads echoue" }

    $remoteDl = @'
set -euo pipefail
STAMP="interventions-20260920"
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
'@
    $remoteDlPath = Join-Path $env:TEMP "relaisdesk-dl-$Stamp.sh"
    [System.IO.File]::WriteAllText($remoteDlPath, ($remoteDl -replace "`r", ""), [System.Text.UTF8Encoding]::new($false))
    & plink -batch -i $PpkPath $VpsTarget -m $remoteDlPath
    if ($LASTEXITCODE -ne 0) { throw "Deploiement downloads echoue - voir sortie ci-dessus" }

    $localManifest = Get-Content -Raw -LiteralPath (Join-Path $DownloadsDir "release-manifest.json") | ConvertFrom-Json
    $remoteManifest = Invoke-RestMethod -Uri "https://api.relaisdesk.fr/api/v1/downloads/release-manifest.json" -TimeoutSec 30 -UseBasicParsing
    if ($remoteManifest.signature -ne $localManifest.signature) { throw "Manifeste public different du manifeste signe local" }
    Write-Ok "Downloads Oracle a jour, manifeste public verifie"
} else {
    Write-Host "  [INFO] Downloads ignores (-SkipDownloads)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Phase 3 - OVH (transfert manuel + verification)
# ---------------------------------------------------------------------------
if (-not $SkipOvh) {
    Write-Step "Phase 3 - Transfert OVH (manuel)"
    Write-Host "  Transferez via FTP OVH / SFTP / gestionnaire OVH (binaire, sans conversion) :" -ForegroundColor Yellow
    Write-Host "    OBLIGATOIRE : relaisdesk/client/app.js  ->  <racine web>/client/app.js"
    Write-Host "    OBLIGATOIRE : relaisdesk/client/index.html  ->  <racine web>/client/index.html"
    Write-Host "    OBLIGATOIRE : relaisdesk/technicien/app.js  ->  <racine web>/technicien/app.js"
    Write-Host "    OBLIGATOIRE : relaisdesk/technicien/index.html  ->  <racine web>/technicien/index.html"
    Write-Host "    MIROIR (recommande) : les 9 fichiers relaisdesk/downloads/ mis a jour -> <racine web>/downloads/"
    Read-Host "  Appuyez sur Entree apres le transfert"
    $ovhIndex = (Invoke-WebRequest -Uri "https://relaisdesk.fr/client/index.html" -TimeoutSec 30 -UseBasicParsing).Content
    if ($ovhIndex -notmatch "refreshInterventionsButton") { throw "OVH : refreshInterventionsButton absent de client/index.html en ligne" }
    $ovhApp = (Invoke-WebRequest -Uri "https://relaisdesk.fr/client/app.js" -TimeoutSec 30 -UseBasicParsing).Content
    if ($ovhApp -notmatch "fetchInterventions") { throw "OVH : fetchInterventions absent de client/app.js en ligne" }
    $ovhTechApp = (Invoke-WebRequest -Uri "https://relaisdesk.fr/technicien/app.js" -TimeoutSec 30 -UseBasicParsing).Content
    if ($ovhTechApp -notmatch "complete-intervention") { throw "OVH : complete-intervention absent de technicien/app.js en ligne" }
    Write-Ok "Fichiers OVH verifies en ligne"
} else {
    Write-Host "  [INFO] OVH ignore (-SkipOvh)" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# Phase 5 - Recette (manuelle, guidee)
# ---------------------------------------------------------------------------
Write-Step "Phase 5 - Recette en situation reelle (manuelle)"
Write-Host "  1. Configurateur : connexion au poste DEV-AYWM-QARC-PKWA-Q5AR."
Write-Host "  2. Sur Oracle : sqlite3 /data/relaisdesk/licences.db `"SELECT intervention_id,status,started_at FROM interventions ORDER BY id DESC LIMIT 3;`""
Write-Host "     attendu : status=in_progress + started_at ; apres fermeture : completed + duree >= 1."
Write-Host "  3. Double-clic Se connecter : une seule fiche (deduplication 5 min)."
Write-Host "  4. Code viewer : creation -> client_ready -> Connexion directe (in_progress) -> fin (completed)."
Write-Host "  5. Espace client #interventions : fiche immediate, badge En cours, bouton Actualiser."
$health = Invoke-RestMethod -Uri "https://api.relaisdesk.fr/api/v1/health" -TimeoutSec 30 -UseBasicParsing
Write-Host ("  Sante API publique : status=" + $health.status + " database=" + $health.database)
Write-Host ""
Write-Host "Mise en production terminee." -ForegroundColor Green
