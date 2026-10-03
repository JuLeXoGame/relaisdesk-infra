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

    $forkWindowsSha256 = "26ef612657f0edd0729275a341457cb96f2105eea4b4c5871244641fd0e35b7f"
    $forkWindowsServiceSha256 = "69eea14bb9e3f7e3a5dff3d11a94bc3b3c1977d8b4bc66075b13e0c9e23054c6"

    $version = "1.0.0"
    $windowsLdFlags = "-s -w -H windowsgui -X main.APIURL=$ApiUrl -X main.APP_VERSION=$version -X main.RUSTDESK_EXPECTED_SHA256=$forkWindowsSha256 -X main.RUSTDESK_SERVICE_EXPECTED_SHA256=$forkWindowsServiceSha256"
    & go build -trimpath -buildvcs=false -ldflags $windowsLdFlags -o $winViewerBuild .
    if ($LASTEXITCODE -ne 0) { throw "Build viewer failed" }

    Copy-Item -Path $winViewerBuild -Destination $winViewerDl -Force
    Write-Host "Success! RelaisDesk_Portable.exe built: $((Get-Item $winViewerDl).Length) bytes"
} finally {
    Pop-Location
}
