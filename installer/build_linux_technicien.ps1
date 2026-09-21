$ErrorActionPreference = "Stop"
$ConfiguratorDir = "C:\Users\Administrator\Documents\Projets\projet\installer\configurator"
$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"
$BuildDir = "C:\Users\Administrator\Documents\Projets\projet\installer\build"
$DebpackExe = Join-Path $BuildDir "debpack.exe"

Push-Location $ConfiguratorDir
try {
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    $linuxConfBin = Join-Path $BuildDir "relaisdesk-configurator"
    $rustdeskElfSha = "58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52"
    $rustdeskDebSha = "190b938b370284e091242e915ecebefdacd23eeba142f227d1decb11bc629b30"
    $pubKey = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
    $version = "1.0.0"

    $linuxLdFlags = "-s -w -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$version -X main.RELEASE_PUBLIC_KEY=$pubKey -X main.RUSTDESK_EXPECTED_SHA256=$rustdeskElfSha -X main.RUSTDESK_PACKAGE_EXPECTED_SHA256=$rustdeskDebSha"
    & go build -ldflags $linuxLdFlags -o $linuxConfBin .
    if ($LASTEXITCODE -ne 0) { throw "Linux configurator build failed" }

    $linuxDlBin = Join-Path $DownloadsDir "RelaisDesk_Technicien_Linux"
    Copy-Item $linuxConfBin -Destination $linuxDlBin -Force

    $debConfDl = Join-Path $DownloadsDir "RelaisDesk_Technicien.deb"
    & $DebpackExe -bin $linuxConfBin -pkg "relaisdesk-configurator" -name "relaisdesk-configurator" -version $version -desc "RelaisDesk Technicien - Support a distance" -out $debConfDl
    if ($LASTEXITCODE -ne 0) { throw "Debpack failed" }

    Copy-Item $debConfDl -Destination (Join-Path $BuildDir "RelaisDesk_Technicien.deb") -Force

    Write-Host "Success! Linux binary & DEB package generated."
} finally {
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
