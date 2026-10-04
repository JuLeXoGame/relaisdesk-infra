# =============================================================================
# RelaisDesk - Deploiement API sur Oracle (binaires precompiles du 27/09/2026)
# =============================================================================
# A executer sur Windows (PowerShell) depuis le poste d'exploitation,
# dans sa propre fenetre (aucune limite de temps) :
#
#   1. Uploader les fichiers web sur OVH (FTP/gestionnaire OVH) — voir § OVH ci-dessous.
#   2. Puis ici (la synchro WSL -> Windows est lancee automatiquement, sauf -SkipSync) :
#   cd C:\Users\Administrator\Documents\Projets\projet
#   .\scripts\deploy-api-20260927.ps1 -VpsTarget "ubuntu@79.72.27.213"
#
# Contenu deployee : portail Stripe, relances paniers/essais, statut public,
# e-facturation CII, manifeste backup + sonde, CGV 2026-09-27 (clause SLA).
#
# La cle par defaut est .secrets\oracle private.ppk (parametre -PpkPath).
# Rollback : les anciens binaires sont copies dans
# /opt/relaisdesk/deployments/<stamp>/rollback-api/ avant bascule.
# En cas d'echec APRES bascule, restaurer a la main :
#   sudo cp -p /opt/relaisdesk/deployments/<stamp>/rollback-api/api /opt/relaisdesk/api/api.new
#   sudo mv -f /opt/relaisdesk/api/api.new /opt/relaisdesk/api/api
#   sudo systemctl restart relaisdesk-api
# (idem pour les 3 autres binaires si necessaire)
#
# § OVH — fichiers relaisdesk/ a transferer (FTP ou gestionnaire OVH) :
#   index.html, app.js, cgv.html, sitemap.xml,
#   comparatif.html, comparatif.js, guide-demarrage.html,
#   client/app.js, admin/app.js, admin/index.html, admin/styles.css
# =============================================================================

param(
    [Parameter(Mandatory = $true)][string]$VpsTarget,
    [string]$PpkPath = "",
    [string]$Stamp = "api-20260927",
    [string]$WslDistro = "",
    [string]$WslProject = "~/projet",
    [switch]$SkipSync
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinDir = Join-Path $Root "bin"
if ([string]::IsNullOrWhiteSpace($PpkPath)) {
    $PpkPath = Join-Path $Root ".secrets\oracle private.ppk"
}

if ($Stamp -notmatch '^[A-Za-z0-9._-]+$') { throw "Stamp invalide." }
if ($WslProject -notmatch '^[A-Za-z0-9_~./-]+$') { throw "WslProject invalide (interpolé dans bash -lc)." }

$files = @("api", "relaisdesk-backup", "relaisdesk-emailcheck", "relaisdesk-dbcheck", "SHA256SUMS.txt")

if (-not $SkipSync) {
    Write-Host "===> S/4 Synchro WSL -> Windows"
    $wslArgs = @()
    if (-not [string]::IsNullOrWhiteSpace($WslDistro)) { $wslArgs += @("-d", $WslDistro) }
    $wslArgs += @("--exec", "bash", "-lc", "python3 `"$WslProject/scripts/sync-wsl-to-windows.py`"")
    & wsl @wslArgs
    if ($LASTEXITCODE -ne 0) { throw "Synchro WSL echouee (ou -WslProject/-WslDistro a ajuster)" }
    Write-Host "  [OK] Miroir Windows synchronise"
}

foreach ($f in $files) {
    $p = Join-Path $BinDir $f
    if (-not (Test-Path -LiteralPath $p)) { throw "Fichier manquant : $p (relancer sans -SkipSync)" }
}
$canary = Join-Path $Root "relaisdesk\comparatif.html"
if (-not (Test-Path -LiteralPath $canary)) { throw "Miroir Windows obsolete : $canary absent (relancer sans -SkipSync)" }

Write-Host "===> 0/4 Controle local des empreintes"
$sums = @{}
foreach ($line in (Get-Content -LiteralPath (Join-Path $BinDir "SHA256SUMS.txt"))) {
    if ($line -match '^([0-9a-f]{64})\s+(\S+)$') { $sums[$Matches[2]] = $Matches[1] }
}
foreach ($f in $files) {
    if ($f -eq "SHA256SUMS.txt") { continue }
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $BinDir $f)).Hash.ToLower()
    if ($sums[$f] -ne $actual) { throw "Empreinte locale inattendue : $f" }
    Write-Host ("  [OK] {0} {1}" -f $f, $actual.Substring(0, 16))
}

Write-Host "===> 1/4 Transfert vers Oracle ($VpsTarget)"
& plink -batch -i $PpkPath $VpsTarget "mkdir -p /tmp/relaisdesk-${Stamp}-api"
if ($LASTEXITCODE -ne 0) { throw "plink mkdir a echoue" }
foreach ($f in $files) {
    Write-Host "  Envoi $f ..."
    & pscp -batch -i $PpkPath (Join-Path $BinDir $f) ($VpsTarget + ":/tmp/relaisdesk-" + $Stamp + "-api/")
    if ($LASTEXITCODE -ne 0) { throw "Transfert echoue : $f" }
}
Write-Host "  [OK] Transfert termine"

Write-Host "===> 2/4 Installation dans /opt/relaisdesk/api (avec rollback)"
$remote = @(
    'set -euo pipefail',
    "STAMP=`"$Stamp`"",
    'INCOMING="/tmp/relaisdesk-${STAMP}-api"',
    'TARGET="/opt/relaisdesk/api"',
    'DEPLOY="/opt/relaisdesk/deployments/${STAMP}/rollback-api"',
    'FILES="api relaisdesk-backup relaisdesk-emailcheck relaisdesk-dbcheck"',
    '(cd "$INCOMING" && sha256sum --check SHA256SUMS.txt)',
    'echo "sommes entrantes OK"',
    'sudo install -d -m 0700 -o root -g root "$DEPLOY"',
    'for f in $FILES; do [ -f "$INCOMING/$f" ] || { echo "manquant: $f"; exit 1; }; if [ -f "$TARGET/$f" ] && [ ! -f "$DEPLOY/$f" ]; then sudo cp -p "$TARGET/$f" "$DEPLOY/$f"; fi; sudo install -m 0755 "$INCOMING/$f" "$TARGET/$f.new"; sudo mv -f "$TARGET/$f.new" "$TARGET/$f"; done',
    'echo "binaires installes, redemarrage du service..."',
    'sudo systemctl daemon-reload',
    'sudo systemctl restart relaisdesk-api',
    'for i in $(seq 1 24); do [ "$(sudo systemctl is-active relaisdesk-api)" = "active" ] && break; sleep 5; done',
    '[ "$(sudo systemctl is-active relaisdesk-api)" = "active" ] || { echo "service non actif"; sudo systemctl status relaisdesk-api --no-pager | head -30; exit 1; }',
    'PID=$(sudo systemctl show relaisdesk-api -p MainPID --value)',
    'RUNNING=$(sudo sha256sum "/proc/${PID}/exe" | cut -d" " -f1)',
    'ONDISK=$(sudo sha256sum "$TARGET/api" | cut -d" " -f1)',
    '[ "$RUNNING" = "$ONDISK" ] || { echo "le processus ne correspond pas au binaire installe"; exit 1; }',
    'echo "processus aligne sur le nouveau binaire : $RUNNING"',
    'HEALTH=$(curl -sk --max-time 15 https://127.0.0.1:8443/api/v1/health || true)',
    'echo "$HEALTH"',
    'echo "$HEALTH" | grep -q "\"status\":\"healthy\"" || { echo "sante locale inattendue"; exit 1; }',
    'rm -rf "$INCOMING"',
    'echo "API OK"'
)
$remotePath = Join-Path $env:TEMP ("relaisdesk-api-" + $Stamp + ".sh")
[System.IO.File]::WriteAllText($remotePath, ($remote -join "`n"), [System.Text.UTF8Encoding]::new($false))
& plink -batch -i $PpkPath $VpsTarget -m $remotePath
if ($LASTEXITCODE -ne 0) { throw "Installation cote serveur echouee (rollback disponible, voir en-tete du script)" }
Write-Host "  [OK] API installee, redemarree et saine"

Write-Host "===> 3/4 Verification publique"
$health = Invoke-RestMethod -Uri "https://api.relaisdesk.fr/api/v1/health" -TimeoutSec 30 -UseBasicParsing
if ($health.status -ne "healthy") { throw "Sante publique inattendue : $($health.status)" }
Write-Host "  [OK] /api/v1/health = healthy"
$public = Invoke-RestMethod -Uri "https://api.relaisdesk.fr/api/v1/public/status" -TimeoutSec 30 -UseBasicParsing
if ($public.status -ne "operational" -and $public.status -ne "degraded") { throw "Statut public inattendu : $($public.status)" }
Write-Host ("  [OK] /api/v1/public/status = {0}" -f $public.status)
Write-Host ""
Write-Host "Deploiement $Stamp termine et verifie."
Write-Host "Pensez a controler l'espace admin (dont l'alerte sauvegardes) et a lancer un backup :"
Write-Host "  sudo systemctl start relaisdesk-backup.service"
