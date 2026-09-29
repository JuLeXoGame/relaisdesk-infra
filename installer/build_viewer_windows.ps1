$ErrorActionPreference = "Stop"

$ViewerDir = "C:\Users\Administrator\Documents\Projets\projet\installer\viewer"
$AssetsDir = "C:\Users\Administrator\Documents\Projets\projet\installer\assets"
$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"
$BuildDir = "C:\Users\Administrator\Documents\Projets\projet\installer\build"
$ApiUrl = "https://api.relaisdesk.fr"

Push-Location $ViewerDir
try {
    $manifestPath = (Join-Path $ViewerDir "app.manifest").Replace("\", "/")
    $iconPath = (Join-Path $AssetsDir "icon.ico").Replace("\", "/")
    $rcPath = Join-Path $ViewerDir "version.rc"
    Set-Content -Path $rcPath -Value "100 ICON `"$iconPath`"`r`n1 24 `"$manifestPath`"" -Encoding UTF8
    & windres --target=pe-x86-64 -i $rcPath -O coff -o rsrc_windows_amd64.syso

    $winViewerBuild = Join-Path $BuildDir "viewer.exe"
    $winViewerDl = Join-Path $DownloadsDir "RelaisDesk_Portable.exe"

    $forkWindowsSha256 = "b8ff64630042a8f41492d7893163de65f9376b2a2d27d404ebf3812033124ae6"
    $forkWindowsServiceSha256 = "260cb7c32e7b929c88262c0460de94ae86dfaf0123ff03337707a17b7e4a0251"

    $version = "1.0.0"
    $windowsLdFlags = "-s -w -H windowsgui -X main.APIURL=$ApiUrl -X main.APP_VERSION=$version -X main.RUSTDESK_EXPECTED_SHA256=$forkWindowsSha256 -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$forkWindowsServiceSha256"
    & go build -trimpath -buildvcs=false -ldflags $windowsLdFlags -o $winViewerBuild .
    if ($LASTEXITCODE -ne 0) { throw "Build viewer failed" }

    Copy-Item -Path $winViewerBuild -Destination $winViewerDl -Force
    Write-Host "Success! RelaisDesk_Portable.exe built: $((Get-Item $winViewerDl).Length) bytes"
} finally {
    Pop-Location
}
