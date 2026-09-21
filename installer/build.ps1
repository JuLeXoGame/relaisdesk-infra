# =============================================================================
# RelaisDesk - Build Script (Windows & Debian/Linux)
# =============================================================================
# This script compiles the Go configurator & viewer for both Windows and Linux,
# packages native Debian .deb installers, builds the NSIS Windows installer,
# and deploys all binaries directly into the 'relaisdesk/downloads' directory.
#
# Usage:
#   .\build.ps1 -RustDeskForkWindowsPath <exe> -RustDeskForkLinuxDebPath <deb>
#       -RustDeskForkLinuxBinaryPath <elf> [-SkipNSIS] [-Verbose]
# =============================================================================

param(
    [switch]$SkipNSIS,
    [switch]$Clean,
    [switch]$Verbose,
    [string]$RustDeskForkWindowsPath,
    [string]$RustDeskForkWindowsServicePath,
    [string]$RustDeskForkLinuxDebPath,
    [string]$RustDeskForkLinuxBinaryPath,
    [string]$RustDeskForkLinuxSoPath,
    [string]$RustDeskForkMacDmgPath,
    [string]$RustDeskForkTechMacDmgPath,
    [string]$ReleaseSigningKeyPath = "C:\RelaisDesk-Secrets\release-signing-ed25519",
    [string]$ReleasePublicKey = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo",
    [string]$ReleaseKeyId = "release-1",
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# ---------------------------------------------------------------------------
# Configuration des Répertoires
# ---------------------------------------------------------------------------
$ProjectRoot = $PSScriptRoot
$ApiDir = Join-Path (Join-Path $ProjectRoot "..") "api"
$ConfiguratorDir = Join-Path $ProjectRoot "configurator"
$ViewerDir = Join-Path $ProjectRoot "viewer"
$DebpackDir = Join-Path $ProjectRoot "debpack"
$NsisDir = Join-Path $ProjectRoot "nsis"
$AssetsDir = Join-Path $ProjectRoot "assets"
$BuildDir = Join-Path $ProjectRoot "build"
$DownloadsDir = Join-Path (Join-Path $ProjectRoot "..") "relaisdesk\downloads"

$ApiUrl = "https://api.relaisdesk.fr"

# ---------------------------------------------------------------------------
# Fonctions d'affichage
# ---------------------------------------------------------------------------
function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host ("===> " + $Message) -ForegroundColor Cyan
    Write-Host ("-" * 60) -ForegroundColor DarkGray
}

function Write-Success {
    param([string]$Message)
    Write-Host ("  [OK] " + $Message) -ForegroundColor Green
}

function Write-Failure {
    param([string]$Message)
    Write-Host ("  [ERREUR] " + $Message) -ForegroundColor Red
}

# ---------------------------------------------------------------------------
# Nettoyage (-Clean)
# ---------------------------------------------------------------------------
if ($Clean) {
    Write-Step "Nettoyage des répertoires de build et de downloads"
    if (Test-Path $BuildDir) {
        Remove-Item -Recurse -Force $BuildDir
        Write-Success "Répertoire build supprimé"
    }
    if (Test-Path $DownloadsDir) {
        Remove-Item -Recurse -Force $DownloadsDir
        Write-Success "Répertoire relaisdesk\downloads nettoyé"
    }
    if (-not $SkipNSIS) {
        $installerFiles = Get-ChildItem -Path $NsisDir -Filter "*.exe" -ErrorAction SilentlyContinue
        foreach ($f in $installerFiles) {
            Remove-Item -Force $f.FullName
            Write-Success ("Supprimé: " + $f.Name)
        }
    }
    exit 0
}

# ---------------------------------------------------------------------------
# Vérification des Prérequis
# ---------------------------------------------------------------------------
Write-Step "Vérification des prérequis"

try {
    $goVersion = & go version 2>&1
    Write-Success ("Go trouvé: " + $goVersion)
} catch {
    Write-Failure "Go n'est pas installé ou n'est pas dans le PATH"
    exit 1
}

# Auto-détection de NSIS dans les répertoires standards
$nsisStandardPaths = @(
    "C:\Program Files (x86)\NSIS",
    "C:\Program Files\NSIS"
)
foreach ($p in $nsisStandardPaths) {
    if ((Test-Path (Join-Path $p "makensis.exe")) -and ($env:PATH -notlike "*$p*")) {
        $env:PATH = "$p;" + $env:PATH
    }
}

if (-not $SkipNSIS) {
    try {
        $nsisVersion = & makensis /VERSION 2>&1
        Write-Success ("NSIS trouvé: " + $nsisVersion)
    } catch {
        Write-Failure "NSIS (makensis) n'est pas dans le PATH. Utilisez -SkipNSIS pour continuer sans NSIS."
        exit 1
    }
}

# ---------------------------------------------------------------------------
# Préparation des Répertoires de Sortie
# ---------------------------------------------------------------------------
Write-Step "Préparation des dossiers de sortie"

if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
}
if (-not (Test-Path $DownloadsDir)) {
    New-Item -ItemType Directory -Path $DownloadsDir | Out-Null
}
Write-Success ("Dossier build : " + $BuildDir)
Write-Success ("Dossier downloads : " + $DownloadsDir)

# ---------------------------------------------------------------------------
# Préparation des binaires forkés embarqués (Portable All-in-One)
# ---------------------------------------------------------------------------
if ([string]::IsNullOrWhiteSpace($RustDeskForkWindowsPath) -or
    [string]::IsNullOrWhiteSpace($RustDeskForkLinuxDebPath) -or
    [string]::IsNullOrWhiteSpace($RustDeskForkLinuxBinaryPath)) {
    throw "Les trois artefacts du fork RustDesk sont obligatoires (EXE Windows, DEB Linux et binaire ELF Linux)."
}
if ([string]::IsNullOrWhiteSpace($ReleaseSigningKeyPath) -or [string]::IsNullOrWhiteSpace($ReleasePublicKey)) {
    throw "La clé privée de signature des versions et sa clé publique sont obligatoires (-ReleaseSigningKeyPath et -ReleasePublicKey)."
}
if (-not (Test-Path -LiteralPath $ReleaseSigningKeyPath -PathType Leaf)) {
    throw "Clé privée de signature des versions introuvable : $ReleaseSigningKeyPath"
}
if ($ReleasePublicKey -notmatch '^[A-Za-z0-9_-]{43}$') {
    throw "ReleasePublicKey doit être une clé publique Ed25519 base64url sans padding."
}
if ($ReleaseKeyId -notmatch '^[A-Za-z0-9_-]{1,64}$') {
    throw "ReleaseKeyId est invalide."
}
$ReleaseSigningKeyPath = (Resolve-Path -LiteralPath $ReleaseSigningKeyPath).Path

$forkArtifacts = @(
    @{ Label = "EXE Windows"; Path = $RustDeskForkWindowsPath },
    @{ Label = "DEB Linux"; Path = $RustDeskForkLinuxDebPath },
    @{ Label = "binaire Linux"; Path = $RustDeskForkLinuxBinaryPath }
)
foreach ($artifact in $forkArtifacts) {
    if (-not (Test-Path -LiteralPath $artifact.Path -PathType Leaf)) {
        throw "Artefact du fork absent ($($artifact.Label)) : $($artifact.Path)"
    }
}

# The Flutter Linux executable is only a small native runner; the Rust code is
# carried by librustdesk.so. A blanket 1 MiB minimum therefore rejects a valid
# package. Validate the actual file formats and target architecture instead.
$windowsItem = Get-Item -LiteralPath $RustDeskForkWindowsPath
$linuxPackageItem = Get-Item -LiteralPath $RustDeskForkLinuxDebPath
$linuxBinaryItem = Get-Item -LiteralPath $RustDeskForkLinuxBinaryPath
if ($windowsItem.Length -lt 1MB) {
    throw "EXE Windows anormalement petit : $RustDeskForkWindowsPath"
}
if ($linuxPackageItem.Length -lt 1MB) {
    throw "Paquet DEB anormalement petit : $RustDeskForkLinuxDebPath"
}
if ($linuxBinaryItem.Length -lt 20) {
    throw "Binaire ELF Linux anormalement petit : $RustDeskForkLinuxBinaryPath"
}

function Read-ArtifactHeader {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][int]$Length
    )
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $header = New-Object byte[] $Length
        if ($stream.Read($header, 0, $header.Length) -ne $header.Length) {
            throw "En-tete de fichier incomplet : $Path"
        }
        return $header
    } finally {
        $stream.Dispose()
    }
}

[byte[]]$windowsHeader = Read-ArtifactHeader -Path $RustDeskForkWindowsPath -Length 2
if ($windowsHeader[0] -ne 0x4D -or $windowsHeader[1] -ne 0x5A) {
    throw "L'artefact Windows n'est pas un executable PE : $RustDeskForkWindowsPath"
}
[byte[]]$debHeader = Read-ArtifactHeader -Path $RustDeskForkLinuxDebPath -Length 8
if ([System.Text.Encoding]::ASCII.GetString($debHeader) -ne "!<arch>`n") {
    throw "L'artefact Linux n'est pas un paquet DEB : $RustDeskForkLinuxDebPath"
}
[byte[]]$elfHeader = Read-ArtifactHeader -Path $RustDeskForkLinuxBinaryPath -Length 20
$elfMachine = [System.BitConverter]::ToUInt16($elfHeader, 18)
if ($elfHeader[0] -ne 0x7F -or $elfHeader[1] -ne 0x45 -or
    $elfHeader[2] -ne 0x4C -or $elfHeader[3] -ne 0x46 -or
    $elfHeader[4] -ne 2 -or $elfHeader[5] -ne 1 -or $elfMachine -ne 62) {
    throw "Le binaire Linux doit etre un ELF64 x86_64 little-endian : $RustDeskForkLinuxBinaryPath"
}

$forkWindowsSha256 = (Get-FileHash -LiteralPath $RustDeskForkWindowsPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ([string]::IsNullOrWhiteSpace($RustDeskForkWindowsServicePath)) {
    throw 'Indiquez -RustDeskForkWindowsServicePath : rustdesk.exe natif et ses deux DLL du même build sont requis pour le service permanent.'
}
$forkWindowsServiceSha256 = & (Join-Path (Split-Path $ProjectRoot -Parent) 'scripts/prepare-fleet-payload.ps1') -NativeExecutable $RustDeskForkWindowsServicePath -Destination (Join-Path $ViewerDir 'embedded/fleet')
$forkLinuxPackageSha256 = (Get-FileHash -LiteralPath $RustDeskForkLinuxDebPath -Algorithm SHA256).Hash.ToLowerInvariant()
$forkLinuxBinarySha256 = (Get-FileHash -LiteralPath $RustDeskForkLinuxBinaryPath -Algorithm SHA256).Hash.ToLowerInvariant()
$forkLinuxSoSha256 = ''
if (-not [string]::IsNullOrWhiteSpace($RustDeskForkLinuxSoPath)) {
    [byte[]]$soHeader = Read-ArtifactHeader -Path $RustDeskForkLinuxSoPath -Length 20
    if ($soHeader[0] -ne 0x7F -or $soHeader[1] -ne 0x45 -or $soHeader[2] -ne 0x4C -or $soHeader[3] -ne 0x46 -or
        $soHeader[4] -ne 2 -or $soHeader[5] -ne 1 -or [BitConverter]::ToUInt16($soHeader, 18) -ne 62) {
        throw 'La bibliothèque du fork Linux doit être un ELF64 x86_64.'
    }
    $forkLinuxSoSha256 = (Get-FileHash -LiteralPath $RustDeskForkLinuxSoPath -Algorithm SHA256).Hash.ToLowerInvariant()
}

$confEmbedDir = Join-Path $ConfiguratorDir "embedded"
$viewEmbedDir = Join-Path $ViewerDir "embedded"
if (-not (Test-Path $confEmbedDir)) { New-Item -ItemType Directory -Path $confEmbedDir | Out-Null }
if (-not (Test-Path $viewEmbedDir)) { New-Item -ItemType Directory -Path $viewEmbedDir | Out-Null }

function Copy-IfDifferent {
    param([string]$Source, [string]$Destination)
    $srcResolved = (Resolve-Path -LiteralPath $Source).Path
    $dstDir = Split-Path -Parent $Destination
    if (-not (Test-Path -LiteralPath $dstDir)) { New-Item -ItemType Directory -Path $dstDir | Out-Null }
    if (Test-Path -LiteralPath $Destination) {
        $dstResolved = (Resolve-Path -LiteralPath $Destination).Path
        if ($srcResolved -eq $dstResolved) { return }
    }
    Copy-Item -LiteralPath $srcResolved -Destination $Destination -Force
}

Copy-IfDifferent -Source $RustDeskForkWindowsPath -Destination (Join-Path $confEmbedDir "rustdesk.exe")
Copy-IfDifferent -Source $RustDeskForkWindowsPath -Destination (Join-Path $viewEmbedDir "rustdesk.exe")
Copy-IfDifferent -Source $RustDeskForkLinuxDebPath -Destination (Join-Path $confEmbedDir "rustdesk.deb")
Copy-IfDifferent -Source $RustDeskForkLinuxDebPath -Destination (Join-Path $viewEmbedDir "rustdesk.deb")
Write-Success "Artefacts RustDesk RelaisDesk intégrés et empreintes calculées"

# ---------------------------------------------------------------------------
# 1. Compilation Windows : Configurateur Technicien (.exe)
# ---------------------------------------------------------------------------
Write-Step "1/4. Compilation Windows - Configurateur Technicien"

Push-Location $ConfiguratorDir
try {
    & go mod tidy
    & go mod download

    # Embedding manifest and icon using windres
    $manifestPath = (Join-Path $ConfiguratorDir "app.manifest").Replace("\", "/")
    $iconPath = (Join-Path $AssetsDir "icon.ico").Replace("\", "/")
    $rcPath = Join-Path $ConfiguratorDir "version.rc"
    "100 ICON `"$iconPath`"`n1 24 `"$manifestPath`"" | Set-Content -Path $rcPath -Encoding UTF8
    if (Test-Path $manifestPath) {
        try {
            & windres --target=pe-x86-64 -i $rcPath -O coff -o rsrc_windows_amd64.syso
            Write-Success "Ressources Windows du configurateur générées"
        } catch {
            Write-Host "  [AVERT] windres non exécuté pour le configurateur" -ForegroundColor Yellow
        }
    }

    $winConfBuild = Join-Path $BuildDir "configurator.exe"
    $winConfDl = Join-Path $DownloadsDir "RelaisDesk_Technicien_Portable.exe"

    $windowsLdFlags = "-s -w -H windowsgui -X main.APIURL=$ApiUrl -X main.APP_VERSION=$Version -X main.RELEASE_PUBLIC_KEY=$ReleasePublicKey -X main.RUSTDESK_EXPECTED_SHA256=$forkWindowsSha256"
    $args = @("build", "-ldflags", $windowsLdFlags, "-o", $winConfBuild, ".")
    if ($Verbose) { $args = @("build", "-v", "-ldflags", $windowsLdFlags, "-o", $winConfBuild, ".") }
    & go @args
    if ($LASTEXITCODE -ne 0) { throw "Échec de compilation du configurateur Windows" }

    Copy-Item -Path $winConfBuild -Destination $winConfDl -Force
    $size = [math]::Round(((Get-Item $winConfDl).Length / 1MB), 2)
    Write-Success "Configurateur Windows compilé dans relaisdesk/downloads ($((Get-Item $winConfDl).Name)) : $size Mo"
} catch {
    Write-Failure $_.Exception.Message
    Pop-Location
    exit 1
} finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# 2. Compilation Windows : Viewer Client (.exe)
# ---------------------------------------------------------------------------
Write-Step "2/4. Compilation Windows - Client Viewer"

if (Test-Path $ViewerDir) {
    Push-Location $ViewerDir
    try {
        & go mod tidy
        # Embedding manifest and icon using windres
        $manifestPath = (Join-Path $ViewerDir "app.manifest").Replace("\", "/")
        $iconPath = (Join-Path $AssetsDir "icon.ico").Replace("\", "/")
        $rcPath = Join-Path $ViewerDir "version.rc"
        "100 ICON `"$iconPath`"`n1 24 `"$manifestPath`"" | Set-Content -Path $rcPath -Encoding UTF8
        if (Test-Path $manifestPath) {
            try {
                & windres --target=pe-x86-64 -i $rcPath -O coff -o rsrc_windows_amd64.syso
                Write-Success "Ressources Windows du viewer générées"
            } catch {
                Write-Host "  [AVERT] windres non exécuté pour le viewer" -ForegroundColor Yellow
            }
        }

        $winViewerBuild = Join-Path $BuildDir "viewer.exe"
        $winViewerDl = Join-Path $DownloadsDir "RelaisDesk_Portable.exe"

        $windowsLdFlags = "-s -w -H windowsgui -X main.APIURL=$ApiUrl -X main.RUSTDESK_EXPECTED_SHA256=$forkWindowsSha256 -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$forkWindowsServiceSha256"
        $args = @("build", "-ldflags", $windowsLdFlags, "-o", $winViewerBuild, ".")
        if ($Verbose) { $args = @("build", "-v", "-ldflags", $windowsLdFlags, "-o", $winViewerBuild, ".") }
        & go @args
        if ($LASTEXITCODE -ne 0) { throw "Échec de compilation du viewer Windows" }

        Copy-Item -Path $winViewerBuild -Destination $winViewerDl -Force
        $size = [math]::Round(((Get-Item $winViewerDl).Length / 1MB), 2)
        Write-Success "Viewer Windows compilé dans relaisdesk/downloads ($((Get-Item $winViewerDl).Name)) : $size Mo"
    } catch {
        Write-Failure $_.Exception.Message
        Pop-Location
        exit 1
    } finally {
        Pop-Location
    }
}

# ---------------------------------------------------------------------------
# 2.5. Compilation Windows : Dashboard Administrateur (.exe)
# ---------------------------------------------------------------------------
$AdminDir = Join-Path (Join-Path $ProjectRoot "..") "dashboard_admin"
if (Test-Path $AdminDir) {
    Write-Step "Compilation Windows - Dashboard Administrateur"
    Push-Location $AdminDir
    try {
        & go mod tidy
        # Embedding manifest and icon using windres
        $manifestPath = (Join-Path $AdminDir "app.manifest").Replace("\", "/")
        $iconPath = (Join-Path $AdminDir "icon.ico").Replace("\", "/")
        $rcPath = Join-Path $AdminDir "version.rc"
        "100 ICON `"$iconPath`"`n1 24 `"$manifestPath`"" | Set-Content -Path $rcPath -Encoding UTF8
        if (Test-Path $manifestPath) {
            try {
                & windres --target=pe-x86-64 -i $rcPath -O coff -o rsrc_windows_amd64.syso
                Write-Success "Ressources Windows du dashboard admin générées"
            } catch {
                Write-Host "  [AVERT] windres non exécuté pour le dashboard admin" -ForegroundColor Yellow
            }
        }

        $winAdminBuild = Join-Path $BuildDir "dashboard_admin.exe"

        $args = @("build", "-ldflags", "-s -w -H windowsgui -X main.APIURL=$ApiUrl", "-o", $winAdminBuild, ".")
        if ($Verbose) { $args = @("build", "-v", "-ldflags", "-s -w -H windowsgui -X main.APIURL=$ApiUrl", "-o", $winAdminBuild, ".") }
        & go @args
        if ($LASTEXITCODE -ne 0) { throw "Échec de compilation du dashboard admin Windows" }

        Copy-Item -Path $winAdminBuild -Destination (Join-Path $AdminDir "dashboard_admin.exe") -Force
        $size = [math]::Round(((Get-Item $winAdminBuild).Length / 1MB), 2)
        Write-Success "Dashboard Administrateur Windows compilé : $size Mo"
    } catch {
        Write-Failure $_.Exception.Message
        Pop-Location
        exit 1
    } finally {
        Pop-Location
    }
}

# ---------------------------------------------------------------------------
# 3. Compilation Linux (Serveur & Paquets Debian Client)
# ---------------------------------------------------------------------------
Write-Step "3/4. Compilation Serveur Linux (API) & Paquets Debian"

# 0. Serveur Linux : API
$apiDir = Join-Path (Join-Path $ProjectRoot "..") "api"

if (Test-Path $apiDir) {
    Push-Location $apiDir
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"
        $apiLinuxBin = Join-Path $BuildDir "api-linux"
        & go build -ldflags "-s -w" -o $apiLinuxBin .
        if ($LASTEXITCODE -eq 0) {
            $size = [math]::Round(((Get-Item $apiLinuxBin).Length / 1MB), 2)
            Write-Success "Binaire serveur API Linux compilé (api-linux) : $size Mo"
        }
    } finally {
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }
}

# Compiler l'outil d'empaquetage debpack pour l'hôte local (Windows)
$debpackExe = Join-Path $BuildDir "debpack.exe"
Push-Location $DebpackDir
try {
    & go build -o $debpackExe .
    if ($LASTEXITCODE -ne 0) { throw "Échec de compilation de l'outil debpack" }
} finally {
    Pop-Location
}

# A. Configurateur Technicien Debian (.deb)
Push-Location $ConfiguratorDir
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $linuxConfBin = Join-Path $BuildDir "relaisdesk-configurator"
    
    $linuxLdFlags = "-s -w -X main.APIURL=$ApiUrl -X main.APP_VERSION=$Version -X main.RELEASE_PUBLIC_KEY=$ReleasePublicKey -X main.RUSTDESK_EXPECTED_SHA256=$forkLinuxBinarySha256 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$forkLinuxPackageSha256 -X main.RUSTDESK_SO_EXPECTED_SHA256=$forkLinuxSoSha256"
    & go build -ldflags $linuxLdFlags -o $linuxConfBin .
    if ($LASTEXITCODE -ne 0) { throw "Échec de compilation Linux du configurateur" }
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

$debConfDl = Join-Path $DownloadsDir "RelaisDesk_Technicien.deb"
$debConfBuild = Join-Path $BuildDir "RelaisDesk_Technicien.deb"

& $debpackExe -bin $linuxConfBin -pkg "relaisdesk-configurator" -name "relaisdesk-configurator" -version $Version -desc "RelaisDesk Technicien - Support a distance" -out $debConfDl
if ($LASTEXITCODE -ne 0) { throw "Échec de l'empaquetage Debian du configurateur" }
Copy-Item -Path $debConfDl -Destination $debConfBuild -Force

$size = [math]::Round(((Get-Item $debConfDl).Length / 1MB), 2)
Write-Success "Paquet Debian Technicien (.deb) généré dans relaisdesk/downloads ($((Get-Item $debConfDl).Name)) : $size Mo"

# B. Client Viewer Debian (.deb)
Push-Location $ViewerDir
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $linuxViewerBin = Join-Path $BuildDir "relaisdesk-viewer"

    $linuxLdFlags = "-s -w -X main.APIURL=$ApiUrl -X main.RUSTDESK_EXPECTED_SHA256=$forkLinuxBinarySha256 -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$forkLinuxPackageSha256 -X main.RUSTDESK_SO_EXPECTED_SHA256=$forkLinuxSoSha256"
    & go build -ldflags $linuxLdFlags -o $linuxViewerBin .
    if ($LASTEXITCODE -ne 0) { throw "Échec de compilation Linux du viewer" }
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

$debViewerDl = Join-Path $DownloadsDir "RelaisDesk_viewer.deb"
$debViewerBuild = Join-Path $BuildDir "RelaisDesk_viewer.deb"

& $debpackExe -bin $linuxViewerBin -pkg "relaisdesk-viewer" -name "relaisdesk-viewer" -version $Version -desc "RelaisDesk Viewer - Assistance a distance client" -out $debViewerDl
if ($LASTEXITCODE -ne 0) { throw "Échec de l'empaquetage Debian du viewer" }
Copy-Item -Path $debViewerDl -Destination $debViewerBuild -Force

$size = [math]::Round(((Get-Item $debViewerDl).Length / 1MB), 2)
Write-Success "Paquet Debian Viewer (.deb) généré dans relaisdesk/downloads ($((Get-Item $debViewerDl).Name)) : $size Mo"

# ---------------------------------------------------------------------------
# 4. Construction de l'Installeur NSIS (Windows Setup)
# ---------------------------------------------------------------------------
if (-not $SkipNSIS) {
    Write-Step "4/4. Construction de l'installeur NSIS Windows"

    # Copie des assets requis par NSIS
    $assetFiles = @("icon.ico", "license.txt")
    foreach ($asset in $assetFiles) {
        $src = Join-Path $AssetsDir $asset
        $dst = Join-Path $BuildDir $asset
        if (Test-Path $src) {
            Copy-Item -Path $src -Destination $dst -Force
        }
    }

    $nsiScripts = @(
        @{ Script = "installer.nsi"; Target = "RelaisDesk_Setup.exe" },
        @{ Script = "installer-configurator.nsi"; Target = "RelaisDesk_Technicien_Setup_1.0.0.exe" }
    )
    foreach ($entry in $nsiScripts) {
        $nsiScript = Join-Path $NsisDir $entry.Script
        $targetName = $entry.Target
        if (Test-Path $nsiScript) {
            Push-Location $NsisDir
            try {
                $nsisVerbosity = if ($Verbose) { "/V4" } else { "/V2" }
                $nsisArgs = @("/INPUTCHARSET", "UTF8", $nsisVerbosity, "/DPRODUCT_VERSION=$Version", "/DOUTPUT_FILE=$(Join-Path $NsisDir $targetName)", $nsiScript)
                & makensis @nsisArgs
                if ($LASTEXITCODE -ne 0) { throw "La construction NSIS de $($entry.Script) a échoué" }

                $targetPath = Join-Path $NsisDir $targetName
                if (Test-Path $targetPath) {
                    Copy-Item -Path $targetPath -Destination $DownloadsDir -Force
                    Copy-Item -Path $targetPath -Destination $BuildDir -Force
                    $size = [math]::Round(((Get-Item $targetPath).Length / 1MB), 2)
                    Write-Success "Installeur NSIS ($targetName) généré dans relaisdesk/downloads : $size Mo"
                }
            } catch {
                Write-Failure $_.Exception.Message
                Pop-Location
                exit 1
            } finally {
                Pop-Location
            }
        }
    }
}

# ---------------------------------------------------------------------------
# Résumé Final
# ---------------------------------------------------------------------------
if (-not [string]::IsNullOrWhiteSpace($RustDeskForkMacDmgPath) -and (Test-Path -LiteralPath $RustDeskForkMacDmgPath -PathType Leaf)) {
    Copy-Item -LiteralPath $RustDeskForkMacDmgPath -Destination (Join-Path $DownloadsDir "RelaisDesk_Mac.dmg") -Force
    Write-Success "Artefact macOS Client (RelaisDesk_Mac.dmg) copié dans relaisdesk/downloads"
}
if (-not [string]::IsNullOrWhiteSpace($RustDeskForkTechMacDmgPath) -and (Test-Path -LiteralPath $RustDeskForkTechMacDmgPath -PathType Leaf)) {
    Copy-Item -LiteralPath $RustDeskForkTechMacDmgPath -Destination (Join-Path $DownloadsDir "RelaisDesk_Technicien_Mac.dmg") -Force
    Write-Success "Artefact macOS Technicien (RelaisDesk_Technicien_Mac.dmg) copié dans relaisdesk/downloads"
}

$releaseArtifactNames = @(
    "RelaisDesk_Portable.exe",
    "RelaisDesk_Setup.exe",
    "RelaisDesk_Technicien_Portable.exe",
    "RelaisDesk_Technicien_Setup_1.0.0.exe",
    "RelaisDesk_Technicien.deb",
    "RelaisDesk_viewer.deb"
)
$optionalMacArtifacts = @("RelaisDesk_Mac.dmg", "RelaisDesk_Technicien_Mac.dmg")
foreach ($macArtifact in $optionalMacArtifacts) {
    if (Test-Path -LiteralPath (Join-Path $DownloadsDir $macArtifact) -PathType Leaf) {
        $releaseArtifactNames += $macArtifact
    }
}
foreach ($artifactName in $releaseArtifactNames) {
    if (-not (Test-Path -LiteralPath (Join-Path $DownloadsDir $artifactName) -PathType Leaf)) {
        throw "Artefact de distribution absent : $artifactName"
    }
}

$checksumPath = Join-Path $DownloadsDir "SHA256SUMS.txt"
$checksumLines = $releaseArtifactNames |
    Sort-Object |
    ForEach-Object {
        $artifactPath = Join-Path $DownloadsDir $_
        $hash = (Get-FileHash -LiteralPath $artifactPath -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $_"
    }
[System.IO.File]::WriteAllLines($checksumPath, $checksumLines, [System.Text.UTF8Encoding]::new($false))
Write-Success "Empreintes SHA-256 publiées dans SHA256SUMS.txt"

Push-Location $ApiDir
try {
    & go run ./cmd/release-manifest `
        -downloads $DownloadsDir `
        -version $Version `
        -base-url "$ApiUrl/api/v1/downloads" `
        -key-id $ReleaseKeyId `
        -private-key $ReleaseSigningKeyPath `
        -public-key $ReleasePublicKey `
        -include (($releaseArtifactNames + "SHA256SUMS.txt") -join ",")
    if ($LASTEXITCODE -ne 0) { throw "Échec de signature du manifeste de version" }
} finally {
    Pop-Location
}
Write-Success "Manifeste de version signé et auto-vérifié"

Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host "  BUILD TERMINE -- FICHIERS DANS RELAISDESK\DOWNLOADS" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Fichiers prets pour distribution :" -ForegroundColor White
$summaryNames = $releaseArtifactNames + "SHA256SUMS.txt" + "release-manifest.json"
foreach ($artifactName in $summaryNames) {
    $file = Get-Item -LiteralPath (Join-Path $DownloadsDir $artifactName)
    $fSize = [math]::Round(($file.Length / 1MB), 2)
    $line = "    - " + $file.Name + " : " + $fSize + " Mo"
    Write-Host $line -ForegroundColor Cyan
}
Write-Host ""
