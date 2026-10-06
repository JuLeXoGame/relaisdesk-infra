# =============================================================================
# RelaisDesk - Miroir Oracle : RelaisDesk_Portable.exe + sommes + manifeste
# =============================================================================
# A executer sur Windows (PowerShell) depuis le poste d'exploitation,
# dans sa propre fenetre (aucune limite de temps) :
#
#   cd C:\Users\Administrator\Documents\Projets\projet
#   .\scripts\mirror-portable-oracle.ps1 -VpsTarget "ubuntu@79.72.27.213"
#   .\scripts\mirror-portable-oracle.ps1 -VpsTarget "ubuntu@79.72.27.213" -Full -Stamp "release-1.0.0-20260924"
#
# La cle par defaut est .secrets\oracle private.ppk (parametre -PpkPath).
# Par defaut, le script transfere 3 fichiers (RelaisDesk_Portable.exe et
# metadonnees). Avec -Full, il transfere la release complete (12 binaires
# dont 4 DMG macOS + SHA256SUMS.txt + release-manifest.json). Dans les deux cas, il les
# installe dans /opt/relaisdesk/downloads (avec rollback), verifie les
# sommes cote serveur et controle que le manifeste public correspond au local.
# =============================================================================

param(
    [Parameter(Mandatory = $true)][string]$VpsTarget,
    [string]$PpkPath = "",
    [string]$Stamp = "viewer-reenroll-20260921",
    [switch]$Full
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($Stamp -notmatch '^[A-Za-z0-9._-]+$') { throw "Stamp invalide." }

$Root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$DownloadsDir = Join-Path $Root "relaisdesk\downloads"
if ([string]::IsNullOrWhiteSpace($PpkPath)) {
    $PpkPath = Join-Path $Root ".secrets\oracle private.ppk"
}

$files = @("RelaisDesk_Portable.exe", "SHA256SUMS.txt", "release-manifest.json")
if ($Full) {
    $files = @(
        "RelaisDesk_Portable.exe",
        "RelaisDesk_Setup.exe",
        "RelaisDesk_Technicien.deb",
        "RelaisDesk_Technicien_Linux",
        "RelaisDesk_Technicien_Portable.exe",
        "RelaisDesk_Technicien_Setup_1.0.0.exe",
        "RelaisDesk_viewer.deb",
        "RelaisDesk_Viewer_Linux",
        "RelaisDesk_Mac.dmg",
        "RelaisDesk_Technicien_Mac.dmg",
        "RelaisDesk_Mac_Intel.dmg",
        "RelaisDesk_Technicien_Mac_Intel.dmg",
        "SHA256SUMS.txt",
        "release-manifest.json"
    )
}
foreach ($f in $files) {
    $p = Join-Path $DownloadsDir $f
    if (-not (Test-Path -LiteralPath $p)) { throw "Fichier manquant : $p" }
}

Write-Host "===> 1/3 Transfert vers Oracle ($VpsTarget)"
& plink -batch -i $PpkPath $VpsTarget "mkdir -p /tmp/relaisdesk-$Stamp-downloads"
if ($LASTEXITCODE -ne 0) { throw "plink mkdir a echoue" }
foreach ($f in $files) {
    Write-Host "  Envoi $f ..."
    & pscp -batch -i $PpkPath (Join-Path $DownloadsDir $f) ($VpsTarget + ":/tmp/relaisdesk-" + $Stamp + "-downloads/")
    if ($LASTEXITCODE -ne 0) { throw "Transfert echoue : $f" }
}
Write-Host "  [OK] Transfert termine"

Write-Host "===> 2/3 Installation dans /opt/relaisdesk/downloads"
$remote = @(
    'set -euo pipefail',
    "STAMP=`"$Stamp`"",
    'INCOMING="/tmp/relaisdesk-${STAMP}-downloads"',
    'TARGET="/opt/relaisdesk/downloads"',
    'DEPLOY="/opt/relaisdesk/deployments/${STAMP}/rollback-downloads"',
    "FILES=`"$($files -join ' ')`"",
    'cleanup() { rm -rf "$INCOMING"; }',
    'trap cleanup EXIT',
    'for f in $FILES; do [ -f "$INCOMING/$f" ] || { echo "manquant: $f"; exit 1; }; done',
    'case " $FILES " in *" SHA256SUMS.txt "*) ;; *) echo "SHA256SUMS.txt non transféré"; exit 1;; esac',
    '# Pre-verification AVANT toute ecriture : chaque binaire entrant doit correspondre a la somme entree.',
    'for f in $FILES; do',
    '  case "$f" in SHA256SUMS.txt|release-manifest.json) continue;; esac',
    '  want=$(awk -v f="$f" ''$2==f{print $1; exit}'' "$INCOMING/SHA256SUMS.txt")',
    '  [ -n "$want" ] || { echo "pas d''entrée SHA pour: $f"; exit 1; }',
    '  got=$(sha256sum "$INCOMING/$f" | awk ''{print $1}'')',
    '  [ "$got" = "$want" ] || { echo "somme entrante invalide: $f"; exit 1; }',
    'done',
    'sudo install -d -m 0700 -o root -g root "$DEPLOY"',
    'for f in $FILES; do if [ -f "$TARGET/$f" ] && [ ! -f "$DEPLOY/$f" ]; then sudo cp -p "$TARGET/$f" "$DEPLOY/$f"; fi; sudo install -m 0644 --owner=$(stat -c "%U" "$TARGET") --group=$(stat -c "%G" "$TARGET") "$INCOMING/$f" "$TARGET/$f.new"; sudo mv -f "$TARGET/$f.new" "$TARGET/$f"; done',
    '# Verification post-install : uniquement les fichiers transferes.',
    'for f in $FILES; do',
    '  case "$f" in SHA256SUMS.txt|release-manifest.json) continue;; esac',
    '  want=$(awk -v f="$f" ''$2==f{print $1; exit}'' "$TARGET/SHA256SUMS.txt")',
    '  got=$(sha256sum "$TARGET/$f" | awk ''{print $1}'')',
    '  [ "$got" = "$want" ] || { echo "somme installee invalide: $f"; exit 1; }',
    'done',
    'echo "DOWNLOADS OK"'
)
$remotePath = Join-Path $env:TEMP ("relaisdesk-dl-" + $Stamp + ".sh")
[System.IO.File]::WriteAllText($remotePath, ($remote -join "`n"), [System.Text.UTF8Encoding]::new($false))
& plink -batch -i $PpkPath $VpsTarget -m $remotePath
if ($LASTEXITCODE -ne 0) { throw "Installation cote serveur echouee" }
Write-Host "  [OK] Miroir installe et sommes verifiees"

Write-Host "===> 3/3 Verification du manifeste public"
$localManifest = Get-Content -Raw -LiteralPath (Join-Path $DownloadsDir "release-manifest.json") | ConvertFrom-Json
$remoteManifest = Invoke-RestMethod -Uri "https://api.relaisdesk.fr/api/v1/downloads/release-manifest.json" -TimeoutSec 30 -UseBasicParsing
if ($remoteManifest.signature -ne $localManifest.signature) { throw "Manifeste public different du manifeste signe local" }
Write-Host "  [OK] Miroir Oracle a jour et verifie"
