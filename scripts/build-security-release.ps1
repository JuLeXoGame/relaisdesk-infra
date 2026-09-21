param(
    [Parameter(Mandatory=$true)][string]$WindowsNativeExecutable,
    [Parameter(Mandatory)][string]$WindowsPortable,
    [Parameter(Mandatory)][string]$LinuxDeb,
    [Parameter(Mandatory)][string]$LinuxRunner,
    [Parameter(Mandatory)][string]$LinuxLibrary,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [Parameter(Mandatory)][string]$SigningKeyPath,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_-]{43}$')][string]$PublicKey,
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version = '1.0.1',
    [string]$GoCacheDirectory,
    [string]$SourceRoot,
    [switch]$SkipNSIS
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$project = Split-Path -Parent $PSScriptRoot
if ($SourceRoot) { $project = (Resolve-Path -LiteralPath $SourceRoot).Path }
$apiURL = 'https://api.relaisdesk.fr'
$output = [IO.Path]::GetFullPath($OutputDirectory)
$website = [IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $PSScriptRoot) 'relaisdesk'))
if ($output -eq $website -or $output.StartsWith($website + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'A release candidate must stay outside the website directory.'
}
if (Test-Path -LiteralPath $output) { throw 'Use a new output directory; existing releases are never overwritten.' }
foreach ($file in @($WindowsNativeExecutable, $WindowsPortable, $LinuxDeb, $LinuxRunner, $LinuxLibrary, $SigningKeyPath)) {
    if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Required file missing: $file" }
}
foreach ($payload in @(
    @{ Path = $WindowsPortable; Magic = [byte[]](0x4d, 0x5a) },
    @{ Path = $LinuxDeb; Magic = [Text.Encoding]::ASCII.GetBytes("!<arch>`n") },
    @{ Path = $LinuxRunner; Magic = [byte[]](0x7f, 0x45, 0x4c, 0x46) },
    @{ Path = $LinuxLibrary; Magic = [byte[]](0x7f, 0x45, 0x4c, 0x46) }
)) {
    $stream = [IO.File]::OpenRead($payload.Path)
    try {
        $header = [byte[]]::new($payload.Magic.Length)
        if ($stream.Read($header, 0, $header.Length) -ne $header.Length -or
            [Convert]::ToBase64String($header) -ne [Convert]::ToBase64String($payload.Magic)) {
            throw "Invalid payload format: $($payload.Path)"
        }
    } finally { $stream.Dispose() }
}
$windowsHash = (Get-FileHash -LiteralPath $WindowsPortable -Algorithm SHA256).Hash.ToLowerInvariant()
$debHash = (Get-FileHash -LiteralPath $LinuxDeb -Algorithm SHA256).Hash.ToLowerInvariant()
$runnerHash = (Get-FileHash -LiteralPath $LinuxRunner -Algorithm SHA256).Hash.ToLowerInvariant()
$libraryHash = ''
if ($LinuxLibrary) {
    $libraryHash = (Get-FileHash -LiteralPath $LinuxLibrary -Algorithm SHA256).Hash.ToLowerInvariant()
}
$go = (Get-Command go -ErrorAction Stop).Source
$nsis = 'C:\Program Files (x86)\NSIS\makensis.exe'
if (-not $SkipNSIS -and -not (Test-Path -LiteralPath $nsis)) { throw 'NSIS is required.' }
$build = Join-Path $output 'build'
$downloads = Join-Path $output 'downloads'
New-Item -ItemType Directory -Path $build, $downloads | Out-Null

function Invoke-Go {
    param([string]$Directory, [string[]]$Arguments)
    Push-Location $Directory
    try {
        & $go @Arguments
        if ($LASTEXITCODE -ne 0) { throw "Go failed in $Directory" }
    } finally { Pop-Location }
}

$savedEnv = @{}
foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED', 'GOCACHE', 'GOTOOLCHAIN')) {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
try {
    $env:GOTOOLCHAIN = 'local'
    if ($GoCacheDirectory) { $env:GOCACHE = [IO.Path]::GetFullPath($GoCacheDirectory) }
    else { $env:GOCACHE = Join-Path $project '.cache/legal-go-build' }
    foreach ($app in @('configurator', 'viewer')) {
        $source = Join-Path $project "installer/$app"
        $staged = Join-Path $output "sources/$app"
        New-Item -ItemType Directory -Path (Join-Path $staged 'embedded') -Force | Out-Null
        Get-ChildItem -LiteralPath $source -File | Where-Object {
            $_.Extension -in @('.go', '.syso', '.png', '.ico', '.manifest') -or $_.Name -in @('go.mod', 'go.sum')
        } | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $staged }
        Copy-Item -LiteralPath $WindowsPortable -Destination (Join-Path $staged 'embedded/rustdesk.exe')
        Copy-Item -LiteralPath $LinuxDeb -Destination (Join-Path $staged 'embedded/rustdesk.deb')
        if ((Get-FileHash -LiteralPath (Join-Path $staged 'embedded/rustdesk.exe') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $windowsHash -or
            (Get-FileHash -LiteralPath (Join-Path $staged 'embedded/rustdesk.deb') -Algorithm SHA256).Hash.ToLowerInvariant() -cne $debHash) {
            throw 'Engine input changed while staging.'
        }

        if ($app -eq 'viewer') {
            $nativeHash = & (Join-Path $PSScriptRoot 'prepare-fleet-payload.ps1') -NativeExecutable $WindowsNativeExecutable -Destination (Join-Path $staged 'embedded/fleet')
        }
        $env:GOOS = 'windows'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        $flags = "-s -w -H windowsgui -X main.APIURL=$apiURL -X main.APP_VERSION=$Version -X main.RUSTDESK_EXPECTED_SHA256=$windowsHash"
        if ($app -eq 'configurator') { $flags += " -X main.RELEASE_PUBLIC_KEY=$PublicKey" }
        if ($app -eq 'viewer') {
            $flags += " -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$nativeHash"
        }
        $testFlags = $flags.Replace('-X main.', "-X $app.")
        Invoke-Go $staged @('test', '-trimpath', '-mod=readonly', '-tags', 'ci', '-ldflags', $testFlags, './...')
        Invoke-Go $staged @('vet', '-trimpath', '-mod=readonly', '-tags', 'ci', './...')
        $env:CGO_ENABLED = '1'
        Invoke-Go $staged @('build', '-mod=readonly', '-trimpath', '-ldflags', $flags, '-o', (Join-Path $build "$app.exe"), '.')
        $name = if ($app -eq 'configurator') { 'RelaisDesk_Technicien_Portable.exe' } else { 'RelaisDesk_Portable.exe' }
        Copy-Item -LiteralPath (Join-Path $build "$app.exe") -Destination (Join-Path $downloads $name)

        $env:GOOS = 'linux'
        $env:CGO_ENABLED = '0'
        $flags = "-s -w -X main.APIURL=$apiURL -X main.APP_VERSION=$Version -X main.RUSTDESK_EXPECTED_SHA256=$runnerHash -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$debHash -X main.RUSTDESK_SO_EXPECTED_SHA256=$libraryHash"
        if ($app -eq 'configurator') { $flags += " -X main.RELEASE_PUBLIC_KEY=$PublicKey" }
        $testFlags = $flags.Replace('-X main.', "-X $app.")
        Invoke-Go $staged @('test', '-trimpath', '-mod=readonly', '-tags', 'ci', '-ldflags', $testFlags, '-c', '-o', (Join-Path $build "$app-linux.test"), '.')
        Invoke-Go $staged @('vet', '-trimpath', '-mod=readonly', '-tags', 'ci', './...')
        Invoke-Go $staged @('build', '-mod=readonly', '-trimpath', '-ldflags', $flags, '-o', (Join-Path $build "relaisdesk-$app"), '.')
    }

    $env:GOOS = 'windows'
    $env:CGO_ENABLED = '0'
    Invoke-Go (Join-Path $project 'installer/debpack') @('build', '-mod=readonly', '-trimpath', '-o', (Join-Path $build 'debpack.exe'), '.')
    foreach ($app in @('configurator', 'viewer')) {
        $name = if ($app -eq 'configurator') { 'RelaisDesk_Technicien.deb' } else { 'RelaisDesk_viewer.deb' }
        & (Join-Path $build 'debpack.exe') -bin (Join-Path $build "relaisdesk-$app") -pkg "relaisdesk-$app" -name "relaisdesk-$app" -version $Version -desc 'RelaisDesk - Support a distance' -out (Join-Path $downloads $name)
        if ($LASTEXITCODE -ne 0) { throw "Debian packaging failed: $app" }
    }
    if (-not $SkipNSIS) { foreach ($entry in @(
        @{ Script = 'installer.nsi'; Name = 'RelaisDesk_Setup.exe' },
        @{ Script = 'installer-configurator.nsi'; Name = 'RelaisDesk_Technicien_Setup_1.0.0.exe' }
    )) {
        & $nsis /INPUTCHARSET UTF8 /V2 "/DPRODUCT_VERSION=$Version" "/DPAYLOAD_DIR=$build" "/DOUTPUT_FILE=$(Join-Path $downloads $entry.Name)" (Join-Path $project "installer/nsis/$($entry.Script)")
        if ($LASTEXITCODE -ne 0) { throw "NSIS failed: $($entry.Script)" }
    } }
    $artifacts = @('RelaisDesk_Portable.exe', 'RelaisDesk_Technicien.deb', 'RelaisDesk_Technicien_Portable.exe', 'RelaisDesk_viewer.deb')
    if (-not $SkipNSIS) { $artifacts += @('RelaisDesk_Setup.exe', 'RelaisDesk_Technicien_Setup_1.0.0.exe') }
    $checksums = foreach ($name in ($artifacts | Sort-Object)) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $downloads $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $name"
    }
    [IO.File]::WriteAllText((Join-Path $downloads 'SHA256SUMS.txt'), ($checksums -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
    Invoke-Go (Join-Path $project 'api') @('run', '-mod=readonly', './cmd/release-manifest', '-downloads', $downloads, '-version', $Version, '-base-url', "$apiURL/api/v1/downloads", '-key-id', 'release-1', '-private-key', $SigningKeyPath, '-public-key', $PublicKey, '-include', (($artifacts + 'SHA256SUMS.txt') -join ','))
    $verifyArgs = @((Join-Path $PSScriptRoot 'verify-security-release.mjs'), $downloads, $PublicKey, $Version)
    if ($SkipNSIS) { $verifyArgs += '--without-nsis' }
    & node @verifyArgs
    if ($LASTEXITCODE -ne 0) { throw 'Signed release verification failed; do not publish this candidate.' }
    Write-Host 'Linux tests were cross-compiled, not executed. Manual Windows/Linux testing remains mandatory.'
    Write-Host "Staged and signed: $downloads. Nothing has been deployed."
} finally {
    foreach ($name in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($name, $savedEnv[$name], 'Process') }
}
