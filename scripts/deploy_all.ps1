# Déploiement complet RelaisDesk depuis Windows, en une seule commande :
#   1. Sync WSL -> Windows (sources + scripts, dont ce script : auto-à-jour)
#   2. Build Rust + lanceurs Go + tests + portables
#   3. Récupération des artefacts Linux (construits sur Linux natif)
#   4. Release : Setups NSIS + SHA256SUMS + manifeste signé + retour WSL
#   5. Miroir des téléchargements vers Oracle (via WSL + SSH)
#   6. (optionnel, -DeployApi) Redéploiement de l'API sur Oracle
#
# Usage :
#   powershell -ExecutionPolicy Bypass -File "C:\...\projet\scripts\deploy_all.ps1"
# Le build complet dure 15 à 30 minutes (Rust). Chaque étape s'arrête nette
# en cas d'échec avec un message explicite : relancer après correction.
param(
    [string]$WslDistro = "Ubuntu-22.04",
    [string]$WslProject = "/home/julien/projet",
    [string]$WinProject = "C:\Users\Administrator\Documents\Projets\projet",
    [string]$VpsTarget = "ubuntu@79.72.27.213",
    [string]$Stamp = "",
    [switch]$SkipBuild,
    [switch]$SkipMirror,
    [switch]$DeployApi
)
$ErrorActionPreference = "Stop"

function Step($label) {
    Write-Host ""
    Write-Host "########## $label ##########"
}

function Require-ZeroExit($code, $label) {
    if ($code -ne 0) { throw "$label a échoué (code $code). Corrigez puis relancez deploy_all.ps1." }
}

# ---------- 0/5 - Prérequis ----------
Step "0/5 - Prérequis"
if (-not (Get-Command wsl -ErrorAction SilentlyContinue)) { throw "wsl.exe introuvable : WSL est requis pour le miroir Oracle." }
$WslRoot = "\\wsl$\$WslDistro" + ($WslProject -replace '/', '\')
if (-not (Test-Path -LiteralPath $WslRoot)) { throw "Arbre WSL inaccessible : $WslRoot (la distribution $WslDistro tourne-t-elle ?)" }
if ($VpsTarget -notmatch '^[A-Za-z0-9.@-]+$') { throw "VpsTarget invalide : $VpsTarget" }
$stamp = $Stamp
if ([string]::IsNullOrWhiteSpace($stamp)) { $stamp = "release-" + (Get-Date -Format "yyyyMMdd-HHmm") }
if ($stamp -notmatch '^[A-Za-z0-9._-]+$') { throw "Stamp invalide : $stamp" }
Write-Host "OK (stamp Oracle : $stamp)"

# ---------- 1/5 - Sync WSL -> Windows ----------
Step "1/5 - Sync WSL -> Windows"
$syncSrc = Join-Path $WslRoot "installer\sync_video_files_to_windows.ps1"
$syncDst = Join-Path $WinProject "installer\sync_video_files_to_windows.ps1"
Copy-Item -LiteralPath $syncSrc -Destination $syncDst -Force
$syncOut = powershell -ExecutionPolicy Bypass -File $syncDst *>&1 | Out-String
Write-Host $syncOut
Require-ZeroExit $LASTEXITCODE "La synchronisation"
if ($syncOut -notmatch 'SYNC OK') { throw "La synchronisation n'a pas confirmé 'SYNC OK'." }

# ---------- 2/5 - Build Rust + Go ----------
if (-not $SkipBuild) {
    Step "2/5 - Build Rust + Go (long : 15 à 30 minutes)"
    & (Join-Path $WinProject "scripts\build-rustdesk-windows.ps1")
    Require-ZeroExit $LASTEXITCODE "Le build"
} else {
    Step "2/5 - Build ignoré (-SkipBuild)"
}

# ---------- 3/5 - Artefacts Linux ----------
Step "3/5 - Artefacts Linux depuis WSL"
& (Join-Path $WinProject "installer\build_linux_technicien.ps1")
Require-ZeroExit $LASTEXITCODE "La récupération Linux technicien"
& (Join-Path $WinProject "installer\build_linux_viewer.ps1")
Require-ZeroExit $LASTEXITCODE "La récupération Linux viewer"

# ---------- 4/5 - Release ----------
Step "4/5 - Release (NSIS + manifeste + retour WSL)"
$relOut = & (Join-Path $WinProject "installer\release_windows.ps1") *>&1 | Out-String
Write-Host $relOut
Require-ZeroExit $LASTEXITCODE "Le release"
if ($relOut -notmatch 'TERMINE') { throw "Le release n'a pas confirmé 'TERMINE'." }

# ---------- 5/5 - Miroir Oracle ----------
if (-not $SkipMirror) {
    Step "5/5 - Miroir Oracle ($VpsTarget)"
    $mirrorCmd = "cd '$WslProject' && bash scripts/mirror-portable-oracle.sh --vps-target '$VpsTarget' --full --stamp '$stamp'"
    wsl -d $WslDistro bash -lc $mirrorCmd
    Require-ZeroExit $LASTEXITCODE "Le miroir Oracle"
    if ($DeployApi) {
        Step "5b/5 - Redéploiement API Oracle"
        $apiCmd = "cd '$WslProject' && bash scripts/deploy_api_oracle.sh --vps-target '$VpsTarget'"
        wsl -d $WslDistro bash -lc $apiCmd
        Require-ZeroExit $LASTEXITCODE "Le déploiement API"
    }
} else {
    Step "5/5 - Miroir Oracle ignoré (-SkipMirror)"
}

Write-Host ""
Write-Host "DÉPLOIEMENT TERMINÉ : build + release + Oracle (stamp $stamp) à jour."
Write-Host "Reste manuel : transfert OVH des fichiers du site + DMG Mac (CI, sur token GitHub)."
