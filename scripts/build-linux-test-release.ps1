param(
    [Parameter(Mandatory)][string]$NativePackage,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$PackageSHA256,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$RunnerSHA256,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{64}$')][string]$LibrarySHA256,
    [Parameter(Mandatory)][string]$WorkingDirectory,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_-]{43}$')][string]$ReleasePublicKey,
    [Parameter(Mandatory)][string]$GoCacheDirectory,
    [string]$GoExecutable='go',
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version='1.0.4'
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$project=Split-Path -Parent $PSScriptRoot
$work=[IO.Path]::GetFullPath($WorkingDirectory)
$output=[IO.Path]::GetFullPath($OutputDirectory)
$web=[IO.Path]::GetFullPath((Join-Path $project 'relaisdesk'))
foreach ($destination in @($work,$output)) {
    if (Test-Path -LiteralPath $destination) { throw 'Use new test directories; no existing files may be overwritten.' }
    if ($destination -eq $web -or $destination.StartsWith($web+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Test programs must stay outside the website.' }
}
if ((Get-FileHash -LiteralPath $NativePackage).Hash -ne $PackageSHA256) { throw 'Native package digest mismatch.' }
New-Item -ItemType Directory -Path $work,$output | Out-Null
$saved=@{}
foreach ($key in @('GOOS','GOARCH','CGO_ENABLED','GOCACHE','GOTOOLCHAIN')) { $saved[$key]=[Environment]::GetEnvironmentVariable($key,'Process') }
function Invoke-Go([string]$Directory,[string[]]$Arguments) {
    Push-Location $Directory
    try { & $GoExecutable @Arguments; if ($LASTEXITCODE -ne 0) { throw "Go failed in $Directory" } } finally { Pop-Location }
}
try {
    $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    if (-not $env:GOTOOLCHAIN -or $env:GOTOOLCHAIN -eq 'local') { $env:GOTOOLCHAIN = 'auto' }
    $env:GOCACHE=[IO.Path]::GetFullPath($GoCacheDirectory)
    $debpack=Join-Path $work 'debpack.exe'
    Invoke-Go (Join-Path $project 'installer/debpack') @('build','-trimpath','-mod=readonly','-o',$debpack,'.')
    $env:GOOS='linux'
    foreach ($app in @('viewer','configurator')) {
        $staged=Join-Path $work "sources/$app"
        New-Item -ItemType Directory -Path (Join-Path $staged 'embedded') -Force | Out-Null
        Get-ChildItem -LiteralPath (Join-Path $project "installer/$app") -File | Where-Object {
            $_.Extension -in @('.go','.png','.ico') -or $_.Name -in @('go.mod','go.sum')
        } | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $staged }
        Copy-Item -LiteralPath $NativePackage -Destination (Join-Path $staged 'embedded/rustdesk.deb')
        $flags="-s -w -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$Version -X main.RUSTDESK_EXPECTED_SHA256=$($RunnerSHA256.ToLowerInvariant()) -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$($PackageSHA256.ToLowerInvariant()) -X main.RUSTDESK_SO_EXPECTED_SHA256=$($LibrarySHA256.ToLowerInvariant())"
        if ($app -eq 'configurator') { $flags+=" -X main.RELEASE_PUBLIC_KEY=$ReleasePublicKey" }
        $name=if($app -eq 'viewer'){'RelaisDesk_Viewer_Linux'}else{'RelaisDesk_Technicien_Linux'}
        $binary=Join-Path $output $name
        Invoke-Go $staged @('vet','-trimpath','-mod=readonly','-tags','ci','./...')
        $testFlags=$flags.Replace('-X main.',"-X $app.")
        Invoke-Go $staged @('test','-c','-trimpath','-mod=readonly','-tags','ci','-ldflags',$testFlags,'-o',(Join-Path $work "$app.test"),'.')
        Invoke-Go $staged @('build','-trimpath','-mod=readonly','-ldflags',$flags,'-o',$binary,'.')
        $package=if($app -eq 'viewer'){'RelaisDesk_viewer.deb'}else{'RelaisDesk_Technicien.deb'}
        & $debpack -bin $binary -pkg "relaisdesk-$app" -name "relaisdesk-$app" -version $Version -desc "RelaisDesk $app - security test" -out (Join-Path $output $package)
        if ($LASTEXITCODE -ne 0) { throw 'Debian package creation failed.' }
    }
    $hashes=Get-ChildItem -LiteralPath $output -File | Sort-Object Name | ForEach-Object { "$((Get-FileHash -LiteralPath $_.FullName).Hash.ToLowerInvariant())  $($_.Name)" }
    [IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS.txt'),($hashes -join "`n")+"`n",[Text.UTF8Encoding]::new($false))
    Write-Output "TEST_BUILD_READY $output"
    Write-Output 'Linux tests are cross-compiled, not executed by this script. No publication or installation was performed.'
} finally { foreach ($key in $saved.Keys) { [Environment]::SetEnvironmentVariable($key,$saved[$key],'Process') } }
