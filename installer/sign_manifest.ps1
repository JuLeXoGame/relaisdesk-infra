param(
    [string]$Version = "1.0.0"
)
$ErrorActionPreference = "Stop"

$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"
$ApiDir = "C:\Users\Administrator\Documents\Projets\projet\api"
$SigningKey = "C:\RelaisDesk-Secrets\release-signing-ed25519"
$PublicKey = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
$KeyId = "release-1"
$ApiUrl = "https://api.relaisdesk.fr"

$releaseArtifactNames = @(
    "RelaisDesk_Portable.exe",
    "RelaisDesk_Setup.exe",
    "RelaisDesk_Technicien.deb",
    "RelaisDesk_Technicien_Linux",
    "RelaisDesk_Technicien_Portable.exe",
    "RelaisDesk_Technicien_Setup_1.0.0.exe",
    "RelaisDesk_viewer.deb",
    "RelaisDesk_Viewer_Linux"
)

# 1. Update SHA256SUMS.txt
$checksumPath = Join-Path $DownloadsDir "SHA256SUMS.txt"
$checksumLines = $releaseArtifactNames |
    Sort-Object |
    ForEach-Object {
        $artifactPath = Join-Path $DownloadsDir $_
        $hash = (Get-FileHash -LiteralPath $artifactPath -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $_"
    }
[System.IO.File]::WriteAllText($checksumPath, (($checksumLines -join "`n") + "`n"), [System.Text.UTF8Encoding]::new($false))
Write-Host "Updated SHA256SUMS.txt"

# 2. Run release-manifest tool
Push-Location $ApiDir
try {
    $includeList = ($releaseArtifactNames + "SHA256SUMS.txt") -join ","
    & go run ./cmd/release-manifest `
        -downloads $DownloadsDir `
        -version $Version `
        -base-url "$ApiUrl/api/v1/downloads" `
        -key-id $KeyId `
        -private-key $SigningKey `
        -public-key $PublicKey `
        -include $includeList
    if ($LASTEXITCODE -ne 0) { throw "Manifest signing failed" }
    Write-Host "Manifest signed and verified successfully."
} finally {
    Pop-Location
}
