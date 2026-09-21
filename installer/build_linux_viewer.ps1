$ErrorActionPreference = "Stop"
$ViewerDir = "C:\Users\Administrator\Documents\Projets\projet\installer\viewer"
$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"
$BuildDir = "C:\Users\Administrator\Documents\Projets\projet\installer\build"
$DebpackExe = Join-Path $BuildDir "debpack.exe"

Push-Location $ViewerDir
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    $linuxViewerBin = Join-Path $BuildDir "relaisdesk-viewer"
    $rustdeskElfSha = "36421cadea72a2a61b50c1b1bda247541c20163f3d501013eb2521cc6c56abff"
    $rustdeskDebSha = "b40be3e770028e83623a4ad97d879eb8080205c52ce00a7091cc76c4741bdd91"
    $rustdeskSoSha = "2c331c9fe6e99a14294dabc1a628b697294ca32f40f36eb1d92247c5087b9087"
    $version = "1.0.0"

    $linuxLdFlags = "-s -w -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$version -X main.RUSTDESK_EXPECTED_SHA256=$rustdeskElfSha -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$rustdeskDebSha -X main.RUSTDESK_SO_EXPECTED_SHA256=$rustdeskSoSha"
    & go build -trimpath -ldflags $linuxLdFlags -o $linuxViewerBin .
    if ($LASTEXITCODE -ne 0) { throw "Linux viewer build failed" }

    Copy-Item $linuxViewerBin -Destination (Join-Path $DownloadsDir "RelaisDesk_Viewer_Linux") -Force

    $debViewerDl = Join-Path $DownloadsDir "RelaisDesk_viewer.deb"
    & $DebpackExe -bin $linuxViewerBin -pkg "relaisdesk-viewer" -name "relaisdesk-viewer" -version $version -desc "RelaisDesk Viewer" -out $debViewerDl
    if ($LASTEXITCODE -ne 0) { throw "Debpack failed" }

    Copy-Item $debViewerDl -Destination (Join-Path $BuildDir "RelaisDesk_viewer.deb") -Force

    Write-Host "Success! RelaisDesk_viewer.deb generated successfully."
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
