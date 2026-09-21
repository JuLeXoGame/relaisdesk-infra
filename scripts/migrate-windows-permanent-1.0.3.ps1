#requires -Version 5.1
<#
Local migration of the confirmed legacy RelaisDesk service, without unenrolling.
No old-engine backup, no recursive deletion, no changes to Oracle or the API.
Run without -Apply for read-only checks. Run locally as administrator with -Apply
to interrupt remote access and replace the service components. No automatic rollback.
#>
[CmdletBinding()]
param([switch]$Apply, [string]$PayloadDirectory)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Windows PowerShell 5.1 -File may not populate PSScriptRoot while binding a
# default parameter. Resolve it in the script body, before any service operation.
if ([string]::IsNullOrWhiteSpace($PayloadDirectory)) {
    if ([string]::IsNullOrWhiteSpace($PSScriptRoot)) { throw 'Chemin du script indisponible : lancer le fichier .ps1 avec -File.' }
    $PayloadDirectory = Join-Path -Path $PSScriptRoot -ChildPath 'payload'
}
$PayloadDirectory = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($PayloadDirectory)

function Assert-RDPlainPath([string]$Path) {
    $cursor = [IO.Path]::GetFullPath($Path)
    while ($cursor) {
        $item = Get-Item -LiteralPath $cursor -Force -ErrorAction Stop
        if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw "Lien ou jonction refuse : $cursor"
        }
        $parent = Split-Path -Path $cursor -Parent
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}

function Assert-RDHash([string]$Path, [string]$Expected) {
    Assert-RDPlainPath $Path
    $item = Get-Item -LiteralPath $Path -Force
    if ($item.PSIsContainer -or (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $Expected) {
        throw "Empreinte incorrecte : $Path. Aucun fichier non verifie ne sera execute."
    }
}

function Assert-RDService($Service, [string]$Name, [string[]]$Commands) {
    if ($null -eq $Service -or $Service.Name -cne $Name) { throw "Service $Name introuvable." }
    if ($Service.StartName -ne 'LocalSystem') { throw "Compte personnalise pour $Name : migration automatique refusee." }
    if ($Service.PathName.Trim() -notin $Commands) { throw "Chemin du service $Name inattendu : migration refusee." }
}

function Set-RDProtectedDirectory([string]$Path) {
    Assert-RDPlainPath $Path
    $acl = New-Object System.Security.AccessControl.DirectorySecurity
    $acl.SetSecurityDescriptorSddlForm('O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;LS)')
    Set-Acl -LiteralPath $Path -AclObject $acl
}

function Stop-RDService([string]$Name) {
    $service = Get-Service -Name $Name
    if ($service.Status -ne 'Stopped') {
        if ($service.Status -ne 'StopPending') { $service.Stop() }
        $service.WaitForStatus([System.ServiceProcess.ServiceControllerStatus]::Stopped, [TimeSpan]::FromSeconds(30))
    }
}

function Install-RDVerifiedFile([string]$Source, [string]$Destination, [string]$Hash) {
    $dir = Split-Path -Path $Destination -Parent
    Assert-RDPlainPath $dir
    if (Test-Path -LiteralPath $Destination) { Assert-RDPlainPath $Destination }
    $staged = Join-Path $dir ('relaisdesk-' + [Guid]::NewGuid().ToString('N') + '.new')
    Copy-Item -LiteralPath $Source -Destination $staged
    Assert-RDHash $staged $Hash
    # Atomic replacement, never truncate an existing executable or follow a hard link.
    if (Test-Path -LiteralPath $Destination) {
        # PowerShell 5.1 converts a direct null string argument to "". Reflection
        # preserves a genuine null backup path: no old executable is copied.
        $replace = [IO.File].GetMethod('Replace', [type[]]@([string], [string], [string]))
        $replace.Invoke($null, [object[]]@([string]$staged, [string]$Destination, $null))
    } else { [IO.File]::Move($staged, $Destination) }
    $acl = New-Object System.Security.AccessControl.FileSecurity
    $acl.SetSecurityDescriptorSddlForm('O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;GRGX;;;LS)')
    Set-Acl -LiteralPath $Destination -AclObject $acl
    Assert-RDHash $Destination $Hash
}

function Set-RDServicePath([string]$Name, [string]$Command) {
    $service = Get-CimInstance Win32_Service -Filter "Name='$Name'"
    $result = Invoke-CimMethod -InputObject $service -MethodName Change -Arguments @{ PathName = $Command }
    if ($result.ReturnValue -ne 0) { throw "Modification du service $Name refusee (code $($result.ReturnValue))." }
}

function Invoke-RDMigration {
    if (-not [Environment]::Is64BitProcess) { throw 'Ouvrez PowerShell 64 bits.' }
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        throw 'Ouvrez PowerShell en tant qu administrateur.'
    }
    # This kit is intentionally limited to the paths confirmed by the operator.
    if ([Environment]::GetFolderPath('ProgramFiles') -ne 'C:\Program Files' -or
        [Environment]::GetFolderPath('CommonApplicationData') -ne 'C:\ProgramData' -or
        [Environment]::GetFolderPath('Windows') -ne 'C:\Windows') {
        throw 'Disposition Windows differente de celle verifiee : migration refusee.'
    }
    $engine = 'C:\Program Files\RelaisDeskEngine'
    $legacyEngine = 'C:\Program Files\RustDesk\rustdesk.exe'
    $fleet = 'C:\ProgramData\RelaisDeskFleet'
    $agent = Join-Path $fleet 'viewer-agent.exe'
    $config = 'C:\Windows\ServiceProfiles\LocalService\AppData\Roaming\RustDesk\config'
    $engineCommand = '"C:\Program Files\RelaisDeskEngine\rustdesk.exe" --service'
    $fleetCommand = '"C:\ProgramData\RelaisDeskFleet\viewer-agent.exe" --fleet-service'
    $hashes = [ordered]@{
        'rustdesk.exe' = 'f67cbc42a91f3bc4bfded8409937bca9a3c815d47c7163ba7f1302a630593ac7'
        'sciter.dll' = '4d97528e157c55ef1fabe9e37a9697116ab66660d7da6163f90a3a7abf80dd56'
        'dylib_virtual_display.dll' = '5eaab07d5a200c3ff679fe37f6380b1540af70d03e21e12f07a8f34920b4ac7a'
        'RelaisDesk_Portable.exe' = '67a8648a5e9e1545c63f479c2908219e63b139786e0e0003e61fa511860ec0b8'
    }
    foreach ($name in $hashes.Keys) { Assert-RDHash (Join-Path $PayloadDirectory $name) $hashes[$name] }
    $rd = Get-CimInstance Win32_Service -Filter "Name='RustDesk'"
    $fl = Get-CimInstance Win32_Service -Filter "Name='RelaisDeskFleet'"
    Assert-RDService $rd 'RustDesk' @('"C:\Program Files\RustDesk\rustdesk.exe" --service', $engineCommand)
    Assert-RDService $fl 'RelaisDeskFleet' @('C:\ProgramData\RelaisDeskFleet\viewer-agent.exe --fleet-service', $fleetCommand)
    foreach ($path in @($fleet, $agent, $config)) { Assert-RDPlainPath $path }
    if ($rd.PathName.Trim() -ne $engineCommand) { Assert-RDPlainPath $legacyEngine }
    $identityHashes = @{}
    foreach ($path in @((Join-Path $fleet 'state.json'), (Join-Path $fleet 'proof-key'), (Join-Path $config 'RustDesk.toml'))) {
        Assert-RDPlainPath $path
        if ((Get-Item -LiteralPath $path).PSIsContainer) { throw 'Fichier identite invalide.' }
        $identityHashes[$path] = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
    }
    # Validate shape, never print the identity, keys or configuration contents.
    $state = Get-Content -LiteralPath (Join-Path $fleet 'state.json') -Raw | ConvertFrom-Json
    if ($state.device_id -notmatch '^DEV-[A-Za-z0-9-]+$' -or $state.rustdesk_id -notmatch '^\d+$') {
        throw 'Identite de parc invalide. Aucun nouvel enrolement ne sera cree.'
    }
    if (Test-Path -LiteralPath $engine) {
        Assert-RDPlainPath $engine
        if (-not (Get-Item -LiteralPath $engine).PSIsContainer) { throw 'Destination moteur invalide.' }
        foreach ($entry in Get-ChildItem -LiteralPath $engine -Force) {
            if ($entry.Name -notin @('rustdesk.exe', 'sciter.dll', 'dylib_virtual_display.dll')) { throw 'Fichier inattendu dans RelaisDeskEngine.' }
            Assert-RDHash $entry.FullName $hashes[$entry.Name]
        }
    } else { Assert-RDPlainPath 'C:\Program Files' }
    Write-Host 'Controles OK : paquet 1.0.3 verifie, services identifies, identite presente.'
    if (-not $Apply) {
        Write-Host 'Aucune modification. Ajouter -Apply pour migrer, depuis le PC physique.'
        return
    }

    Write-Host 'Migration locale sans sauvegarde de l ancien moteur. Coupure de l acces permanent.'
    $servicesTouched = $false
    try {
        if (-not (Test-Path -LiteralPath $engine)) { New-Item -ItemType Directory -Path $engine | Out-Null }
        Set-RDProtectedDirectory $engine
        Set-RDProtectedDirectory $fleet
        $servicesTouched = $true
        foreach ($name in @('RelaisDeskFleet', 'RustDesk')) { Set-Service -Name $name -StartupType Disabled }
        Stop-RDService 'RelaisDeskFleet'
        Stop-RDService 'RustDesk'
        # Only processes from the two confirmed service-engine paths, never every rustdesk.exe.
        foreach ($process in Get-CimInstance Win32_Process -Filter "Name='rustdesk.exe'") {
            if ($process.ExecutablePath -in @($legacyEngine, (Join-Path $engine 'rustdesk.exe'))) {
                $result = Invoke-CimMethod -InputObject $process -MethodName Terminate -Arguments @{ Reason = [uint32]0 }
                if ($result.ReturnValue -ne 0) { throw 'Fermez les fenetres de l ancien moteur puis relancez la migration.' }
            }
        }
        foreach ($name in @('rustdesk.exe', 'sciter.dll', 'dylib_virtual_display.dll')) {
            Install-RDVerifiedFile (Join-Path $PayloadDirectory $name) (Join-Path $engine $name) $hashes[$name]
        }
        # Replace only the stopped agent. No backup of an old executable is made.
        Install-RDVerifiedFile (Join-Path $PayloadDirectory 'RelaisDesk_Portable.exe') $agent $hashes['RelaisDesk_Portable.exe']
        Set-RDServicePath 'RustDesk' $engineCommand
        Set-RDServicePath 'RelaisDeskFleet' $fleetCommand
        foreach ($path in $identityHashes.Keys) { Assert-RDHash $path $identityHashes[$path] }
        foreach ($name in @('RustDesk', 'RelaisDeskFleet')) { Set-Service -Name $name -StartupType Automatic }
        # The fleet agent obtains authorization before starting the native engine.
        $startTime = [DateTime]::UtcNow
        Start-Service -Name 'RelaisDeskFleet'
        $deadline = $startTime.AddSeconds(60)
        $ready = Join-Path $fleet 'ready'
        $authorized = $false
        do {
            $rdState = (Get-Service -Name 'RustDesk').Status
            $flState = (Get-Service -Name 'RelaisDeskFleet').Status
            if ((Test-Path -LiteralPath $ready) -and $rdState -eq 'Running' -and $flState -eq 'Running') {
                Assert-RDPlainPath $ready
                if ((Get-Item -LiteralPath $ready).LastWriteTimeUtc -ge $startTime.AddSeconds(-1)) { $authorized = $true; break }
            }
            Start-Sleep -Milliseconds 500
        } while ([DateTime]::UtcNow -lt $deadline)
        if (-not $authorized) { throw 'Autorisation non confirmee en 60 secondes.' }
        foreach ($path in @((Join-Path $fleet 'state.json'), (Join-Path $fleet 'proof-key'))) { Assert-RDHash $path $identityHashes[$path] }
        Write-Host 'Migration terminee : services demarres, autorisation recue, identite du parc conservee.'
        Write-Host 'Aucun ancien moteur sauvegarde ou supprime. Aucun nouvel enrolement.'
        Write-Host 'Verifiez maintenant une connexion distante, puis un redemarrage Windows et une reconnexion.'
    } catch {
        if ($servicesTouched) {
            foreach ($name in @('RelaisDeskFleet', 'RustDesk')) {
                try { Set-Service -Name $name -StartupType Disabled; Stop-RDService $name } catch { Write-Warning "Arret de $name non confirme : controle manuel necessaire." }
            }
            Write-Warning 'Migration interrompue. Services desactives par precaution, identite non supprimee. Aucun retour automatique a l ancienne version.'
        }
        throw
    }
}

Invoke-RDMigration
