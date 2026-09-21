param(
    [Parameter(Mandatory)][string]$WindowsPortable,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$PortableSHA256,
    [Parameter(Mandatory)][string]$WindowsNativeExecutable,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$NativeSHA256,
    [Parameter(Mandatory)][string]$WorkingDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_-]{43}$')][string]$ReleasePublicKey,
    [string]$GoExecutable = 'go',
    [string]$GoCacheDirectory,
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version = '1.0.3'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$project = Split-Path -Parent $PSScriptRoot
$work = [IO.Path]::GetFullPath($WorkingDirectory)
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $work) { throw 'Use a new working directory.' }
if (Test-Path -LiteralPath $output) { throw 'Use a new output directory; existing downloads are never overwritten.' }
$web = [IO.Path]::GetFullPath((Join-Path $project 'relaisdesk'))
foreach ($destination in @($work,$output)) {
    if ($destination -eq $web -or $destination.StartsWith($web+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Test builds must stay outside the website directory.' }
}
if ((Get-FileHash -LiteralPath $WindowsPortable).Hash -ne $PortableSHA256) { throw 'Unexpected portable engine.' }
if ((Get-FileHash -LiteralPath $WindowsNativeExecutable).Hash -ne $NativeSHA256) { throw 'Unexpected native engine.' }
$nsis = 'C:/Program Files (x86)/NSIS/makensis.exe'
if (-not (Test-Path -LiteralPath $nsis)) { throw 'NSIS is required.' }
New-Item -ItemType Directory -Path $work,$output | Out-Null
$build = Join-Path $work 'build'
New-Item -ItemType Directory -Path $build | Out-Null

function Invoke-Go([string]$Directory,[string[]]$Arguments) {
    Push-Location $Directory
    try {
        & $GoExecutable @Arguments
        if ($LASTEXITCODE -ne 0) { throw "Go failed in $Directory" }
    } finally { Pop-Location }
}
$saved = @{}
foreach ($key in @('GOOS','GOARCH','CGO_ENABLED','GOCACHE','GOTOOLCHAIN')) {
    $saved[$key] = [Environment]::GetEnvironmentVariable($key,'Process')
}
try {
    $env:GOOS='windows'
    $env:GOARCH='amd64'
    if (-not $env:GOTOOLCHAIN -or $env:GOTOOLCHAIN -eq 'local') { $env:GOTOOLCHAIN = 'auto' }
    if ($GoCacheDirectory) { $env:GOCACHE=[IO.Path]::GetFullPath($GoCacheDirectory) }
    foreach ($app in @('configurator','viewer')) {
        $source=Join-Path $project "installer/$app"
        $staged=Join-Path $work "sources/$app"
        New-Item -ItemType Directory -Path (Join-Path $staged 'embedded') -Force | Out-Null
        Get-ChildItem -LiteralPath $source -File | Where-Object {
            $_.Extension -in @('.go','.syso','.png','.ico','.manifest') -or $_.Name -in @('go.mod','go.sum')
        } | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $staged }
        Copy-Item -LiteralPath $WindowsPortable -Destination (Join-Path $staged 'embedded/rustdesk.exe')
        $flags="-s -w -H windowsgui -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$Version -X main.RUSTDESK_EXPECTED_SHA256=$($PortableSHA256.ToLowerInvariant())"
        if ($app -eq 'viewer') {
            $pinned=& (Join-Path $PSScriptRoot 'prepare-fleet-payload.ps1') -NativeExecutable $WindowsNativeExecutable -Destination (Join-Path $staged 'embedded/fleet')
            if ($pinned -ne $NativeSHA256) { throw 'Native payload changed while staging.' }
            $flags+=" -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$pinned"
        } else {
            $flags+=" -X main.RELEASE_PUBLIC_KEY=$ReleasePublicKey"
        }
        Write-Output "TEST_AND_VET $app"
        $env:CGO_ENABLED='0'
        # Test binaries import package main by its module path, not as main.
        $testFlags=$flags.Replace('-X main.',"-X $app.")
        Invoke-Go $staged @('test','-trimpath','-mod=readonly','-tags','ci','-ldflags',$testFlags,'./...')
        Invoke-Go $staged @('vet','-trimpath','-mod=readonly','-tags','ci','./...')
        Write-Output "BUILD_NATIVE_WINDOWS $app"
        $env:CGO_ENABLED='1'
        Invoke-Go $staged @('build','-trimpath','-mod=readonly','-ldflags',$flags,'-o',(Join-Path $build "$app.exe"),'.')
        $name=if($app -eq 'viewer'){'RelaisDesk_Portable.exe'}else{'RelaisDesk_Technicien_Portable.exe'}
        Copy-Item -LiteralPath (Join-Path $build "$app.exe") -Destination (Join-Path $output $name)
    }
    foreach ($entry in @(
        @{Script='installer.nsi';Name='RelaisDesk_Setup.exe'},
        @{Script='installer-configurator.nsi';Name="RelaisDesk_Technicien_Setup_$Version.exe"}
    )) {
        Write-Output "PACKAGE $($entry.Name)"
        & $nsis /INPUTCHARSET UTF8 /V2 "/DPRODUCT_VERSION=$Version" "/DPAYLOAD_DIR=$build" "/DOUTPUT_FILE=$(Join-Path $output $entry.Name)" (Join-Path $project "installer/nsis/$($entry.Script)")
        if ($LASTEXITCODE -ne 0) { throw 'NSIS packaging failed.' }
    }
    $hashes=Get-ChildItem -LiteralPath $output -File -Filter '*.exe' | Sort-Object Name | ForEach-Object {
        "$((Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant())  $($_.Name)"
    }
    [IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS.txt'),($hashes -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
    Write-Output "TEST_BUILD_READY $output"
    Write-Output 'No publication, release signing or system installation was performed.'
} finally {
    foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key],'Process') }
}
