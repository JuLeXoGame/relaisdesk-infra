#requires -Version 5.1
<#
.SYNOPSIS
    Pipeline release Windows : NSIS + signature + synchro WSL, en une commande.

.DESCRIPTION
    Pre-requis : build-rustdesk-windows.ps1 (et build_linux_*.ps1 pour Linux)
    executes avant, sur cette machine Windows qui est la source de verite
    pour tout artefact compile.
    1. Copie depuis l'arbre WSL les artefacts NON construits ici (DMG macOS
       issus de la CI, LIRE-MOI) + sign_manifest.ps1 de reference. Les
       artefacts Windows et Linux locaux (frais du build) sont preserves.
    2. Rebuild les 2 installateurs NSIS.
    3. Regenere SHA256SUMS.txt + release-manifest.json signe.
    4. Recopie Setups + portables + artefacts Linux + SHA + manifeste vers WSL.
    5. Verifie les empreintes.

    Lancement (contourne la strategie d'execution pour toute la chaine) :
      powershell -ExecutionPolicy Bypass -File "C:\Users\Administrator\Documents\Projets\projet\installer\release_windows.ps1"
#>
param(
    [string]$WslDistro = "Ubuntu-22.04",
    [string]$WinProject = "C:\Users\Administrator\Documents\Projets\projet",
    [string]$Version = "1.0.0",
    [switch]$TechnicianOnly
)
$ErrorActionPreference = "Stop"

$WslProject = "\\wsl$\$WslDistro\home\julien\projet"
$WinInstaller = Join-Path $WinProject "installer"
$WinBuild = Join-Path $WinInstaller "build"
$WinNsis = Join-Path $WinInstaller "nsis"
$WinDl = Join-Path $WinProject "relaisdesk\downloads"
$WslDl = "$WslProject\relaisdesk\downloads"
$SigningKey = "C:\RelaisDesk-Secrets\release-signing-ed25519"

function Step($msg) { Write-Host ""; Write-Host "=== $msg ===" -ForegroundColor Cyan }

function Require-File($path, $label) {
    if (-not (Test-Path -LiteralPath $path)) { throw "$label introuvable : $path" }
}

Step "0/5 - Verification des acces"
Require-File (Join-Path $WinBuild "viewer.exe") "viewer.exe local (lancez build-rustdesk-windows.ps1 d'abord)"
Require-File (Join-Path $WinBuild "configurator.exe") "configurator.exe local (lancez build-rustdesk-windows.ps1 d'abord)"
Require-File (Join-Path $WinInstaller "rebuild_nsis.ps1") "rebuild_nsis.ps1"
Require-File "$WslProject\installer\sign_manifest.ps1" "sign_manifest.ps1 (WSL, reference)"
Require-File $SigningKey "cle de signature"
foreach ($p in @("C:\Program Files (x86)\NSIS", "C:\Program Files\NSIS")) {
    if ((Test-Path (Join-Path $p "makensis.exe")) -and ($env:PATH -notlike "*$p*")) {
        $env:PATH = "$p;" + $env:PATH
    }
}
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) { throw "makensis introuvable (ni PATH ni C:\Program Files*\NSIS)" }
Write-Host "OK"

Step "1/5 - Copie WSL -> Windows (artefacts non construits ici)"
# Les lanceurs et artefacts Windows/Linux locaux sont FRAIS (build prealable) :
# on ne les ecrase jamais avec les copies WSL. Seuls les DMG (CI), LIRE-MOI
# et .htaccess transitent dans ce sens.
$LocallyBuilt = @(
    "RelaisDesk_Setup.exe", "RelaisDesk_Technicien_Setup_1.0.0.exe",
    "RelaisDesk_Portable.exe", "RelaisDesk_Technicien_Portable.exe",
    "RelaisDesk_Technicien.deb", "RelaisDesk_Technicien_Linux",
    "RelaisDesk_viewer.deb", "RelaisDesk_Viewer_Linux",
    "SHA256SUMS.txt", "release-manifest.json"
)
Copy-Item "$WslDl\*" -Destination $WinDl -Force -Exclude $LocallyBuilt
Copy-Item "$WslProject\installer\sign_manifest.ps1" -Destination $WinInstaller -Force
Get-ChildItem (Join-Path $WinInstaller "*.ps1") | Unblock-File -ErrorAction SilentlyContinue
Write-Host "OK"

Step "2/5 - Rebuild NSIS"
$nsisArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", (Join-Path $WinInstaller "rebuild_nsis.ps1"))
if ($TechnicianOnly) { $nsisArgs += "-TechnicianOnly" }
$t0 = (Get-Date).AddMinutes(-2)
Push-Location $WinInstaller
try {
    & powershell @nsisArgs
    if ($LASTEXITCODE -ne 0) { throw "rebuild_nsis.ps1 a echoue (exit $LASTEXITCODE)" }
} finally { Pop-Location }
$setupViewer = Join-Path $WinDl "RelaisDesk_Setup.exe"
$setupTech = Join-Path $WinDl "RelaisDesk_Technicien_Setup_1.0.0.exe"
Require-File $setupViewer "Setup viewer"
if (-not $TechnicianOnly) { Require-File $setupTech "Setup technicien" }
$wt = (Get-Item $setupViewer).LastWriteTime
if ($wt -lt $t0) { throw "RelaisDesk_Setup.exe anterieur au lancement ($wt) : le rebuild a-t-il tourne ?" }
$hNsis = (Get-FileHash -LiteralPath (Join-Path $WinNsis "RelaisDesk_Setup.exe") -Algorithm SHA256).Hash
$hDl = (Get-FileHash -LiteralPath $setupViewer -Algorithm SHA256).Hash
if ($hNsis -ne $hDl) { throw "Setup downloads different de la sortie NSIS !" }
Write-Host "OK"

Step "3/5 - Signature SHA + manifeste"
Push-Location $WinInstaller
try {
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $WinInstaller "sign_manifest.ps1") -Version $Version
    if ($LASTEXITCODE -ne 0) { throw "sign_manifest.ps1 a echoue (exit $LASTEXITCODE)" }
} finally { Pop-Location }
Write-Host "OK"

Step "4/5 - Retour Windows -> WSL"
$back = @(
    $setupViewer,
    (Join-Path $WinDl "RelaisDesk_Portable.exe"),
    (Join-Path $WinDl "RelaisDesk_Technicien_Portable.exe"),
    (Join-Path $WinDl "RelaisDesk_Technicien.deb"),
    (Join-Path $WinDl "RelaisDesk_Technicien_Linux"),
    (Join-Path $WinDl "RelaisDesk_viewer.deb"),
    (Join-Path $WinDl "RelaisDesk_Viewer_Linux"),
    (Join-Path $WinDl "SHA256SUMS.txt"),
    (Join-Path $WinDl "release-manifest.json")
)
if (-not $TechnicianOnly) { $back += $setupTech }
foreach ($f in $back) {
    if (-not (Test-Path -LiteralPath $f)) {
        throw "$(Split-Path $f -Leaf) introuvable cote Windows : lancez build-rustdesk-windows.ps1 (et build_linux_technicien.ps1 + build_linux_viewer.ps1 pour Linux) avant release_windows.ps1"
    }
}
Copy-Item $back -Destination $WslDl -Force
Copy-Item (Join-Path $WinBuild "viewer.exe"), (Join-Path $WinBuild "configurator.exe") -Destination "$WslProject\installer\build" -Force
Write-Host "OK"

Step "5/5 - Verification croisee"
foreach ($f in $back) {
    $leaf = Split-Path $f -Leaf
    $hWin = (Get-FileHash -LiteralPath $f -Algorithm SHA256).Hash
    $hWsl = (Get-FileHash -LiteralPath "$WslDl\$leaf" -Algorithm SHA256).Hash
    if ($hWin -ne $hWsl) { throw "empreinte differente entre Windows et WSL pour $leaf !" }
}
Write-Host ""
Write-Host "TERMINE : Setups + portables + Linux + SHA256SUMS.txt + release-manifest.json a jour des deux cotes." -ForegroundColor Green
