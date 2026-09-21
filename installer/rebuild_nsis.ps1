param([switch]$TechnicianOnly)

$ErrorActionPreference = "Stop"

$NsisDir = "C:\Users\Administrator\Documents\Projets\projet\installer\nsis"
$BuildDir = "C:\Users\Administrator\Documents\Projets\projet\installer\build"
$AssetsDir = "C:\Users\Administrator\Documents\Projets\projet\installer\assets"
$DownloadsDir = "C:\Users\Administrator\Documents\Projets\projet\relaisdesk\downloads"

$nsisStandardPaths = @("C:\Program Files (x86)\NSIS", "C:\Program Files\NSIS")
foreach ($p in $nsisStandardPaths) {
    if ((Test-Path (Join-Path $p "makensis.exe")) -and ($env:PATH -notlike "*$p*")) {
        $env:PATH = "$p;" + $env:PATH
    }
}

foreach ($asset in @("icon.ico", "license.txt")) {
    Copy-Item (Join-Path $AssetsDir $asset) (Join-Path $BuildDir $asset) -Force
}

Push-Location $NsisDir
try {
    $version = "1.0.0"
    $outTech = Join-Path $NsisDir "RelaisDesk_Technicien_Setup_1.0.0.exe"
    $scriptTech = Join-Path $NsisDir "installer-configurator.nsi"
    $argsTech = @("/INPUTCHARSET", "UTF8", "/V2", "/DPRODUCT_VERSION=$version", "/DOUTPUT_FILE=$outTech", $scriptTech)
    & makensis @argsTech
    if ($LASTEXITCODE -ne 0) { throw "Technicien NSIS failed" }
    Copy-Item $outTech $DownloadsDir -Force
    Copy-Item $outTech $BuildDir -Force

    if (-not $TechnicianOnly) {
        $outClient = Join-Path $NsisDir "RelaisDesk_Setup.exe"
        $scriptClient = Join-Path $NsisDir "installer.nsi"
        $argsClient = @("/INPUTCHARSET", "UTF8", "/V2", "/DPRODUCT_VERSION=$version", "/DOUTPUT_FILE=$outClient", $scriptClient)
        & makensis @argsClient
        if ($LASTEXITCODE -ne 0) { throw "Client NSIS failed" }
        Copy-Item $outClient $DownloadsDir -Force
        Copy-Item $outClient $BuildDir -Force
        Write-Host "Both NSIS installers rebuilt successfully."
    } else {
        Write-Host "Technician NSIS installer rebuilt successfully."
    }
} finally {
    Pop-Location
}
