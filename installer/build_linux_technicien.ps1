# Les binaires Linux incluent la GUI Fyne (CGO + X11) : ils ne peuvent pas
# être compilés sous Windows. Ils sont construits sur Linux natif et
# récupérés ici depuis l'arbre WSL vers les dossiers Windows attendus
# par release_windows.ps1.
param(
    [string]$WslDistro = "Ubuntu-22.04",
    [string]$WinProject = "C:\Users\Administrator\Documents\Projets\projet"
)
$ErrorActionPreference = "Stop"

$WslDl = "\\wsl$\$WslDistro\home\julien\projet\relaisdesk\downloads"
$DownloadsDir = Join-Path $WinProject "relaisdesk\downloads"
$BuildDir = Join-Path $WinProject "installer\build"

function Require-Artifact($path, $label, $magic) {
    if (-not (Test-Path -LiteralPath $path)) { throw "$label introuvable : $path (build Linux manquant côté WSL ?)" }
    $fs = [System.IO.File]::OpenRead($path)
    try {
        $buf = New-Object byte[] $magic.Length
        $n = $fs.Read($buf, 0, $magic.Length)
        if ($n -ne $magic.Length) { throw "$label illisible : $path" }
        for ($i = 0; $i -lt $magic.Length; $i++) {
            if ($buf[$i] -ne $magic[$i]) { throw "$label n'a pas la signature attendue : $path" }
        }
    } finally { $fs.Close() }
    $size = (Get-Item -LiteralPath $path).Length
    if ($size -lt 10MB) { throw "$label anormalement petit ($size octets) : $path" }
}

$elfMagic = [byte[]](0x7F, [byte][char]'E', [byte][char]'L', [byte][char]'F')
$debMagic = [System.Text.Encoding]::ASCII.GetBytes("!<arch>")

# Pins RustDesk attendus dans le binaire (doivent suivre config_linux.go ;
# vérifiés par TestLinuxPinsMatchEmbeddedTechnician et ci-dessous).
$rustdeskElfSha = "58ef1e984727d827836c8ad84ad50a4971db4a3cb80ad7a646982594af155c52"
$rustdeskDebSha = "190b938b370284e091242e915ecebefdacd23eeba142f227d1decb11bc629b30"
$rustdeskSoSha = "0b36cca0afe53cb4f7e7a61710318304db6aa861e583475247445446ebd76eae"

function Require-EmbeddedPins($binPath) {
    $latin1 = [System.Text.Encoding]::GetEncoding("iso-8859-1")
    $text = $latin1.GetString([System.IO.File]::ReadAllBytes($binPath))
    foreach ($pin in @($rustdeskElfSha, $rustdeskDebSha, $rustdeskSoSha)) {
        if ($text.IndexOf($pin, [System.StringComparison]::Ordinal) -lt 0) {
            throw "pin RustDesk $pin absent de $binPath (binaire Linux construit avec d'autres pins ?)"
        }
    }
}

$srcBin = Join-Path $WslDl "RelaisDesk_Technicien_Linux"
$srcDeb = Join-Path $WslDl "RelaisDesk_Technicien.deb"
Require-Artifact $srcBin "RelaisDesk_Technicien_Linux (WSL)" $elfMagic
Require-Artifact $srcDeb "RelaisDesk_Technicien.deb (WSL)" $debMagic
Require-EmbeddedPins $srcBin

Copy-Item $srcBin -Destination (Join-Path $BuildDir "relaisdesk-configurator") -Force
Copy-Item $srcBin -Destination (Join-Path $DownloadsDir "RelaisDesk_Technicien_Linux") -Force
Copy-Item $srcDeb -Destination (Join-Path $DownloadsDir "RelaisDesk_Technicien.deb") -Force
Copy-Item $srcDeb -Destination (Join-Path $BuildDir "RelaisDesk_Technicien.deb") -Force

$h = (Get-FileHash -LiteralPath (Join-Path $DownloadsDir "RelaisDesk_Technicien_Linux") -Algorithm SHA256).Hash
Write-Host "Success! Artefacts Linux technicien récupérés (SHA256=$h)."
