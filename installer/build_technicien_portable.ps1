$ErrorActionPreference = "Stop"
$ConfiguratorDir = "C:\Users\Administrator\Documents\Projets\projet\installer\configurator"
$AssetsDir = "C:\Users\Administrator\Documents\Projets\projet\installer\assets"
$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"
$BuildDir = "C:\Users\Administrator\Documents\Projets\projet\installer\build"

Push-Location $ConfiguratorDir
try {
    $manifestPath = (Join-Path $ConfiguratorDir "app.manifest").Replace("\", "/")
    $iconPath = (Join-Path $AssetsDir "icon.ico").Replace("\", "/")
    $rcPath = Join-Path $ConfiguratorDir "version.rc"
    
    Set-Content -Path $rcPath -Value "100 ICON `"$iconPath`"`r`n1 24 `"$manifestPath`"" -Encoding UTF8
    & windres --target=pe-x86-64 -i $rcPath -O coff -o rsrc_windows_amd64.syso

    $windowsHash = "b8ff64630042a8f41492d7893163de65f9376b2a2d27d404ebf3812033124ae6"
    $version = "1.0.0"
    $flags = "-s -w -H windowsgui -X main.APIURL=https://api.relaisdesk.fr -X main.APP_VERSION=$version -X main.RELEASE_PUBLIC_KEY=K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo -X main.RUSTDESK_EXPECTED_SHA256=$windowsHash"

    & go build -trimpath -ldflags $flags -o RelaisDesk_Technicien_Portable.exe .
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }

    Copy-Item RelaisDesk_Technicien_Portable.exe -Destination (Join-Path $BuildDir "configurator.exe") -Force
    Copy-Item RelaisDesk_Technicien_Portable.exe -Destination (Join-Path $DownloadsDir "RelaisDesk_Technicien_Portable.exe") -Force

    $installedTarget = "C:\Program Files (x86)\RelaisDesk Technicien\configurator.exe"
    if (Test-Path "C:\Program Files (x86)\RelaisDesk Technicien") {
        Copy-Item RelaisDesk_Technicien_Portable.exe -Destination $installedTarget -Force
        Write-Host "Updated installed binary: $installedTarget"
    }

    Write-Host "Success! RelaisDesk_Technicien_Portable.exe built: $((Get-Item RelaisDesk_Technicien_Portable.exe).Length) bytes"
} finally {
    Pop-Location
}
