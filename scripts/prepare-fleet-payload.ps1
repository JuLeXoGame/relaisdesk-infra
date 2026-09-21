param(
    [Parameter(Mandatory=$true)][string]$NativeExecutable,
    [Parameter(Mandatory=$true)][string]$Destination
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$native = Get-Item -LiteralPath $NativeExecutable
if ($native.PSIsContainer -or $native.Name -ne 'rustdesk.exe') { throw 'Un rustdesk.exe natif, issu de la compilation du fork, est requis.' }
$names = @('rustdesk.exe', 'sciter.dll', 'dylib_virtual_display.dll')
# All required inputs are checked before copying; never execute the portable
# wrapper to obtain service components from a user's extraction cache.
foreach ($name in $names) {
    $source = Get-Item -LiteralPath (Join-Path $native.DirectoryName $name)
    if ($source.PSIsContainer -or $source.Length -lt 2 -or $source.LinkType) { throw "Composant natif invalide : $name" }
    $stream = [IO.File]::OpenRead($source.FullName)
    try {
        if ($stream.ReadByte() -ne 0x4d -or $stream.ReadByte() -ne 0x5a) { throw "Composant PE invalide : $name" }
    } finally { $stream.Dispose() }
}
New-Item -ItemType Directory -Path $Destination -Force | Out-Null
$destinationItem = Get-Item -LiteralPath $Destination
if (-not $destinationItem.PSIsContainer -or ($destinationItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Le dossier du paquet natif ne doit pas être un lien.' }
foreach ($entry in Get-ChildItem -LiteralPath $Destination) {
    if ($entry.Name -notin ($names + 'README.txt')) { throw "Fichier inattendu dans le paquet natif : $($entry.Name)" }
    if ($entry.PSIsContainer -or ($entry.Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw "Composant de destination non sûr : $($entry.Name)" }
}
foreach ($name in $names) {
    Copy-Item -LiteralPath (Join-Path $native.DirectoryName $name) -Destination (Join-Path $Destination $name) -Force
}
(Get-FileHash -LiteralPath (Join-Path $Destination 'rustdesk.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
