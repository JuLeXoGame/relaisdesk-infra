[CmdletBinding()]
param(
    [string]$ProjectRoot = (Split-Path -Parent $PSScriptRoot),
    [switch]$SkipNativeDependencies,
    [switch]$SkipTests,
    [switch]$SkipLaunchers,
    [string]$DestinationPath,
    [string]$ToolchainRoot,
    [string]$ResultPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$VcpkgToolCommit = "0ac8df3b98e3afcd8bf075fa74a6bd2c32613345"
$SciterCommit = "f33df075d9eb2f8d252cb88f1b2c8096e56197ed"
$SciterSha256 = "4D97528E157C55EF1FABE9E37A9697116AB66660D7DA6163F90A3A7ABF80DD56"
$ClientVersion = "1.4.9"
$ApiUrl = "https://api.relaisdesk.fr"

$ProjectRoot = [IO.Path]::GetFullPath($ProjectRoot)
$RustDeskDir = Join-Path $ProjectRoot "rustdesk"
$VcpkgRoot = Join-Path $ProjectRoot ".tools\vcpkg"
$LibClangRoot = Join-Path $ProjectRoot ".tools\libclang-python"
$LibClangPath = Join-Path $LibClangRoot "clang\native"
if ($ToolchainRoot) {
    $VcpkgRoot = Join-Path $ToolchainRoot 'vcpkg'
    $LibClangRoot = Join-Path $ToolchainRoot 'libclang-python'
    $LibClangPath = Join-Path $LibClangRoot 'clang/native'
}
if ($ResultPath -and (Test-Path -LiteralPath $ResultPath)) { throw 'Le recu moteur existe deja.' }
$CargoTargetDir = Join-Path $ProjectRoot ".cache\rustdesk-target"
$PythonPackages = Join-Path $ProjectRoot ".cache\python-packages"
$PackageRoot = Join-Path $ProjectRoot ".cache\relaisdesk-windows-package"
$OutputDir = Join-Path $ProjectRoot "bin\rustdesk-fork"
$OutputPath = Join-Path $OutputDir "RelaisDesk-RustDesk-$ClientVersion-windows-x64.exe"
if ($DestinationPath) {
    $OutputPath = [IO.Path]::GetFullPath($DestinationPath)
    $OutputDir = Split-Path -Parent $OutputPath
    if (Test-Path -LiteralPath $OutputPath) { throw "Destination déjà existante : $OutputPath" }
}
$ManifestDir = Join-Path $ProjectRoot "scripts\rustdesk-sciter-vcpkg"
$ManifestPath = Join-Path $ManifestDir "vcpkg.json"
$NasmSelector = Join-Path $ManifestDir "nasm-2.16.03.cmake"

function Invoke-NativeChecked {
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [string[]]$Arguments = @()
    )
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Échec ($LASTEXITCODE) : $FilePath $($Arguments -join ' ')"
    }
}

function Invoke-InDirectory {
    param(
        [Parameter(Mandatory)] [string]$Directory,
        [Parameter(Mandatory)] [string]$FilePath,
        [string[]]$Arguments = @()
    )
    Push-Location $Directory
    try {
        Invoke-NativeChecked -FilePath $FilePath -Arguments $Arguments
    } finally {
        Pop-Location
    }
}

foreach ($requiredPath in @($RustDeskDir, $ManifestPath, $NasmSelector)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Chemin requis absent : $requiredPath"
    }
}

if (-not $SkipNativeDependencies) {
    if (-not (Test-Path -LiteralPath (Join-Path $VcpkgRoot ".git"))) {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $VcpkgRoot) | Out-Null
        Invoke-NativeChecked -FilePath "git" -Arguments @(
            "clone", "--filter=blob:none", "https://github.com/microsoft/vcpkg.git", $VcpkgRoot
        )
    }

    Invoke-InDirectory -Directory $VcpkgRoot -FilePath "git" -Arguments @("checkout", $VcpkgToolCommit)
    Invoke-NativeChecked -FilePath (Join-Path $VcpkgRoot "bootstrap-vcpkg.bat") -Arguments @("-disableMetrics")

    $VcpkgNasmSelector = Join-Path $VcpkgRoot "scripts\cmake\vcpkg_find_acquire_program(NASM).cmake"
    Copy-Item -LiteralPath $NasmSelector -Destination $VcpkgNasmSelector -Force

    $GitPerlRoot = "C:\Program Files\Git\usr"
    $PerlParent = Join-Path $VcpkgRoot "downloads\tools\perl\5.42.2.1"
    $PerlLink = Join-Path $PerlParent "perl"
    if ((Test-Path -LiteralPath $GitPerlRoot) -and -not (Test-Path -LiteralPath $PerlLink)) {
        New-Item -ItemType Directory -Force -Path $PerlParent | Out-Null
        New-Item -ItemType Junction -Path $PerlLink -Target $GitPerlRoot | Out-Null
    }

    if (-not (Test-Path -LiteralPath (Join-Path $LibClangPath "libclang.dll"))) {
        New-Item -ItemType Directory -Force -Path $LibClangRoot | Out-Null
        Invoke-NativeChecked -FilePath "python" -Arguments @(
            "-m", "pip", "install", "--disable-pip-version-check", "--target", $LibClangRoot,
            "libclang==18.1.1"
        )
    }

    Invoke-NativeChecked -FilePath (Join-Path $VcpkgRoot "vcpkg.exe") -Arguments @(
        "install",
        "--triplet=x64-windows-static",
        "--x-manifest-root=$ManifestDir",
        "--x-install-root=$(Join-Path $VcpkgRoot 'installed')",
        "--clean-after-build"
    )
}

foreach ($requiredPath in @(
    (Join-Path $VcpkgRoot "installed\x64-windows-static\lib"),
    (Join-Path $LibClangPath "libclang.dll")
)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Dépendance native absente : $requiredPath"
    }
}

$savedBuildEnvironment = @{}
foreach ($name in @('VCPKG_ROOT','LIBCLANG_PATH','CARGO_TARGET_DIR','PYTHONPATH','GOCACHE')) {
    $savedBuildEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
$env:VCPKG_ROOT = $VcpkgRoot
$env:LIBCLANG_PATH = $LibClangPath
$env:CARGO_TARGET_DIR = $CargoTargetDir

# Compiled Python extensions cannot be shared across interpreter ABIs.
$env:PYTHONPATH = $PythonPackages
$brotliRequirements = Get-Content -LiteralPath (Join-Path $RustDeskDir 'libs/portable/requirements.txt')
$brotliPins = @($brotliRequirements | Where-Object { $_ -match '^Brotli==([0-9]+\.[0-9]+\.[0-9]+)\s*$' })
if ($brotliPins.Count -ne 1) { throw 'Une version Brotli exacte est requise dans requirements.txt.' }
$brotliVersion = ([regex]::Match($brotliPins[0], '^Brotli==([0-9]+\.[0-9]+\.[0-9]+)', 'IgnoreCase')).Groups[1].Value
& python -c "import brotli; assert brotli.__version__ == '$brotliVersion'" 2>$null
if ($LASTEXITCODE -ne 0) {
    $pythonCacheTag = & python -c "import platform, sys; print(sys.implementation.cache_tag + '-' + platform.machine().lower())"
    if ($LASTEXITCODE -ne 0 -or $pythonCacheTag -notmatch '^[a-z0-9_-]+$') {
        throw "Impossible d'identifier le Python de construction."
    }
    $PythonPackages = Join-Path $ProjectRoot ".cache\python-packages-$pythonCacheTag"
    New-Item -ItemType Directory -Force -Path $PythonPackages | Out-Null
    Invoke-NativeChecked -FilePath "python" -Arguments @(
        "-m", "pip", "install", "--disable-pip-version-check", "--target", $PythonPackages,
        "--upgrade", "Brotli==$brotliVersion"
    )
    $env:PYTHONPATH = $PythonPackages
    Invoke-NativeChecked -FilePath "python" -Arguments @("-c", "import brotli; assert brotli.__version__ == '$brotliVersion'")
}

if (-not $SkipTests) {
    # This upstream test edits the host's Windows certificate stores.
    Invoke-InDirectory -Directory $RustDeskDir -FilePath "cargo" -Arguments @("test", "--locked", "--lib", "--", "--skip", "platform::windows::tests::test_uninstall_cert")
}

Invoke-InDirectory -Directory $RustDeskDir -FilePath "python" -Arguments @("res\inline-sciter.py")
Invoke-InDirectory -Directory $RustDeskDir -FilePath "cargo" -Arguments @(
    "build", "--locked", "--release", "--features", "inline"
)
Invoke-InDirectory -Directory (Join-Path $RustDeskDir "libs\virtual_display\dylib") `
    -FilePath "cargo" -Arguments @("build", "--locked", "--release")

New-Item -ItemType Directory -Force -Path $PackageRoot, $OutputDir | Out-Null
$SciterPath = Join-Path $PackageRoot "sciter.dll"
if (-not (Test-Path -LiteralPath $SciterPath)) {
    $SciterUrl = "https://raw.githubusercontent.com/c-smile/sciter-sdk/$SciterCommit/bin.win/x64/sciter.dll"
    Invoke-WebRequest -Uri $SciterUrl -OutFile $SciterPath
}
$ActualSciterSha256 = (Get-FileHash -LiteralPath $SciterPath -Algorithm SHA256).Hash
if ($ActualSciterSha256 -ne $SciterSha256) {
    throw "Empreinte Sciter invalide : $ActualSciterSha256"
}

$PayloadDir = Join-Path $PackageRoot ("payload-{0}-{1}" -f $PID, [DateTime]::UtcNow.Ticks)
New-Item -ItemType Directory -Path $PayloadDir | Out-Null
Copy-Item -LiteralPath (Join-Path $CargoTargetDir "release\rustdesk.exe") `
    -Destination (Join-Path $PayloadDir "rustdesk.exe")
Copy-Item -LiteralPath (Join-Path $CargoTargetDir "release\dylib_virtual_display.dll") `
    -Destination (Join-Path $PayloadDir "dylib_virtual_display.dll")
Copy-Item -LiteralPath $SciterPath -Destination (Join-Path $PayloadDir "sciter.dll")

$PortableDir = Join-Path $RustDeskDir "libs\portable"
Invoke-NativeChecked -FilePath "python" -Arguments @(
    (Join-Path $PortableDir "generate.py"),
    "-f", $PayloadDir,
    "-o", $PortableDir,
    "-e", (Join-Path $PayloadDir "rustdesk.exe"),
    "-l", "11"
)

$PackerPath = Join-Path $CargoTargetDir "release\rustdesk-portable-packer.exe"
if (-not (Test-Path -LiteralPath $PackerPath)) {
    throw "Packer absent après compilation : $PackerPath"
}
Copy-Item -LiteralPath $PackerPath -Destination $OutputPath -Force
$ForkSha256 = (Get-FileHash -LiteralPath $OutputPath -Algorithm SHA256).Hash.ToLowerInvariant()

if (-not $SkipLaunchers) {
    $ConfiguratorDir = Join-Path $ProjectRoot "installer\configurator"
    $ViewerDir = Join-Path $ProjectRoot "installer\viewer"
    $InstallerBuildDir = Join-Path $ProjectRoot "installer\build"
    $DownloadsDir = Join-Path $ProjectRoot "relaisdesk\downloads"
    $GoCache = Join-Path $ProjectRoot ".cache\go-build"
    New-Item -ItemType Directory -Force -Path $InstallerBuildDir, $DownloadsDir, $GoCache | Out-Null
    $env:GOCACHE = $GoCache

    Copy-Item -LiteralPath $OutputPath -Destination (Join-Path $ConfiguratorDir "embedded\rustdesk.exe") -Force
    Copy-Item -LiteralPath $OutputPath -Destination (Join-Path $ViewerDir "embedded\rustdesk.exe") -Force

    $ServiceSha256 = & (Join-Path $PSScriptRoot 'prepare-fleet-payload.ps1') -NativeExecutable (Join-Path $PayloadDir 'rustdesk.exe') -Destination (Join-Path $ViewerDir 'embedded/fleet')
    $LdFlags = "-s -w -H windowsgui -X main.APIURL=$ApiUrl -X main.RUSTDESK_EXPECTED_SHA256=$ForkSha256 -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$ServiceSha256"
    $ConfiguratorOutput = Join-Path $InstallerBuildDir "configurator.exe"
    $ViewerOutput = Join-Path $InstallerBuildDir "viewer.exe"
    Invoke-InDirectory -Directory $ConfiguratorDir -FilePath "go" -Arguments @(
        "build", "-trimpath", "-ldflags", $LdFlags, "-o", $ConfiguratorOutput, "."
    )
    Invoke-InDirectory -Directory $ViewerDir -FilePath "go" -Arguments @(
        "build", "-trimpath", "-ldflags", $LdFlags, "-o", $ViewerOutput, "."
    )
    Invoke-InDirectory -Directory $ConfiguratorDir -FilePath "go" -Arguments @("test", "./...")
    $env:RELAISDESK_TEST_NATIVE_SHA256 = $ServiceSha256
    try {
        Invoke-InDirectory -Directory $ViewerDir -FilePath "go" -Arguments @("test", "./...")
    } finally {
        Remove-Item Env:\RELAISDESK_TEST_NATIVE_SHA256 -ErrorAction SilentlyContinue
    }

    Copy-Item -LiteralPath $ConfiguratorOutput `
        -Destination (Join-Path $DownloadsDir "RelaisDesk_Technicien_Portable.exe") -Force
    Copy-Item -LiteralPath $ViewerOutput `
        -Destination (Join-Path $DownloadsDir "RelaisDesk_Portable.exe") -Force
}

Write-Host "Fork Windows : $OutputPath"
Write-Host "SHA-256      : $ForkSha256"
Write-Host "Note          : artefacts sans Authenticode ; annoncer cet état et signer le manifeste Ed25519 avant distribution."
if ($ResultPath) {
    $receipt = @{
        portable = $OutputPath
        portable_sha256 = $ForkSha256
        native = (Join-Path $PayloadDir 'rustdesk.exe')
        native_sha256 = (Get-FileHash -LiteralPath (Join-Path $PayloadDir 'rustdesk.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
    }
    [IO.File]::WriteAllText([IO.Path]::GetFullPath($ResultPath), ($receipt | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
}
} finally {
    foreach ($name in $savedBuildEnvironment.Keys) { [Environment]::SetEnvironmentVariable($name, $savedBuildEnvironment[$name], 'Process') }
}
