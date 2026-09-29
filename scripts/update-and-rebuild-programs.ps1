<#
.SYNOPSIS
    Maintenance locale et preparation de programmes de test dans update/.
.DESCRIPTION
    Ne publie rien, ne contacte pas Oracle et ne regenere aucune cle.
    Les moteurs reutilises doivent etre explicitement identifies par un manifeste
    d'entrees SHA-256. Aucun fichier n'est choisi dans un cache par sa date.
    Voir docs/MAINTENANCE_SECURITE.md.
#>
[CmdletBinding()]
param(
    [switch]$ApplySecurityFixes,
    [switch]$SkipSecurityAudit,
    [switch]$SkipEngineBuild,
    [switch]$SkipNSIS,
    [switch]$Rollback,
    [string]$RunId,
    [ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version,
    [string]$EngineManifest,
    [string]$ReleaseSigningKeyPath,
    [ValidatePattern('^[A-Za-z0-9_-]{43}$')][string]$ReleasePublicKey,
    [string]$PythonPath = 'python',
    [switch]$InstallAuditTools,
    [switch]$AuditOnly,
    [switch]$PrepareOnly,
    [switch]$DryRun,
    [switch]$Force,
    [switch]$ShowReport,
    [switch]$Interactive
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$projectRoot = Split-Path -Parent $PSScriptRoot

function Invoke-PythonChecked([string[]]$Arguments) {
    & $PythonPath -B @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Operation Python en echec (code $LASTEXITCODE)." }
}
function Assert-NoLinks([string]$Path) {
    $current = [IO.Path]::GetFullPath($Path)
    while ($current) {
        if (Test-Path -LiteralPath $current) {
            if ((Get-Item -Force -LiteralPath $current).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw "Lien/jonction interdit : $Path"
            }
        }
        $current = Split-Path -Parent $current
    }
}
function Stage-Engine([object]$Manifest, [string]$Name, [string]$Destination, [string]$ManifestDirectory) {
    if ($Manifest.artifacts.PSObject.Properties.Name -notcontains $Name) { throw "Entree moteur manquante : $Name" }
    $entry = $Manifest.artifacts.$Name
    if ($entry.sha256 -cnotmatch '^[a-f0-9]{64}$') { throw "Empreinte invalide : $Name" }
    $path = [string]$entry.path
    if (-not [IO.Path]::IsPathRooted($path)) { $path = Join-Path $ManifestDirectory $path }
    Assert-NoLinks $path
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Moteur absent : $Name" }
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.sha256) {
        throw "Moteur different de l'entree approuvee : $Name"
    }
    New-Item -ItemType Directory -Path (Split-Path -Parent $Destination) -Force | Out-Null
    Copy-Item -LiteralPath $path -Destination $Destination
    if ((Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant() -cne $entry.sha256) {
        throw "Entree modifiee pendant la copie : $Name"
    }
}
function Show-ProductionPolicy {
    Write-Host 'Deploiement direct et rotation automatique retires du script de maintenance.'
    Write-Host 'Aucune connexion SSH. Aucun changement de VPN, de systeme, de cle ou de telechargement public.'
    Write-Host 'Une livraison exige une recette puis une procedure specifique : sauvegarde DB, preflight, empreintes, bascule et retour arriere.'
    Write-Host 'Les anciens scripts deploy-*-20260919.sh sont historiques, pas des deployeurs generiques a relancer.'
    Write-Host 'Lire docs/MAINTENANCE_SECURITE.md, section Deploiement et cles.'
}
function Invoke-Maintenance {
    if ($AuditOnly -and $PrepareOnly) { throw 'Choisir AuditOnly OU PrepareOnly.' }
    if ($ApplySecurityFixes -and ($AuditOnly -or $PrepareOnly -or $SkipSecurityAudit)) {
        throw 'Application interdite sans maintenance complete et tests.'
    }
    if ($AuditOnly -or $PrepareOnly) {
        $auditArgs = @{PythonPath=$PythonPath; ShowReport=[bool]$ShowReport; InstallAuditTools=[bool]$InstallAuditTools}
        if ($AuditOnly) { $auditArgs.AuditOnly=$true } else { $auditArgs.PrepareOnly=$true }
        & (Join-Path $PSScriptRoot 'update-security.ps1') @auditArgs
        if ($LASTEXITCODE -ne 0) { throw 'Audit incomplet/en echec : lire le rapport. Aucun build lance.' }
        return
    }
    foreach ($required in @('Version','EngineManifest','ReleaseSigningKeyPath','ReleasePublicKey')) {
        if (-not (Get-Variable -Name $required -ValueOnly)) { throw "Parametre obligatoire pour compiler : -$required" }
    }
    Assert-NoLinks $EngineManifest
    Assert-NoLinks $ReleaseSigningKeyPath
    if (-not (Test-Path -LiteralPath $ReleaseSigningKeyPath -PathType Leaf)) { throw 'Cle de signature introuvable ; aucun repli implicite.' }
    $manifestFile = (Resolve-Path -LiteralPath $EngineManifest).Path
    $manifest = Get-Content -LiteralPath $manifestFile -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($manifest.schema_version -ne 1) { throw 'Schema du manifeste moteur inconnu.' }
    $releaseRun = (Get-Date -Format 'yyyyMMdd-HHmmss-') + [Guid]::NewGuid().ToString('N')
    $run = Join-Path $projectRoot "update/$releaseRun"
    $auditArgs = @{
        RunId=$releaseRun; PythonPath=$PythonPath; ShowReport=[bool]$ShowReport
        InstallAuditTools=[bool]$InstallAuditTools; RunBuildTests=$true
    }
    if ($SkipSecurityAudit) {
        $auditArgs.PrepareOnly=$true
        Write-Warning 'Audit ignore explicitement : cette compilation ne sera pas une validation de securite.'
    } else {
        $auditArgs.ApplyFixes=[bool]$ApplySecurityFixes
        # -ApplySecurityFixes vaut déjà consentement explicite : le transmettre
        # pour la confirmation exigée par update-security.ps1 avant écriture.
        if ($ApplySecurityFixes) { $auditArgs.Force=$true }
    }
    & (Join-Path $PSScriptRoot 'update-security.ps1') @auditArgs
    if ($LASTEXITCODE -ne 0) { throw 'Audit/tests/application en echec : compilation bloquee, candidats conserves dans update/.' }
    Assert-NoLinks $run
    $report = Get-Content -LiteralPath (Join-Path $run 'rapport.json') -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $SkipSecurityAudit -and $report.do_not_apply_batch) { throw 'Le rapport interdit ce lot.' }
    $source = Join-Path $run 'projet'
    # Bind reused native engines to the dependency manifests of the audited sources.
    # This is a provenance declaration, not proof that a third-party engine was built honestly.
    foreach ($item in @(
        @{File='rustdesk/Cargo.lock';Field='rustdesk_cargo_lock_sha256'},
        @{File='rustdesk/Cargo.toml';Field='rustdesk_cargo_toml_sha256'}
    )) {
        $expected = $manifest.($item.Field)
        if ($expected -cnotmatch '^[a-f0-9]{64}$' -or
            (Get-FileHash -LiteralPath (Join-Path $source $item.File) -Algorithm SHA256).Hash.ToLowerInvariant() -cne $expected) {
            throw "Moteurs natifs non alignes sur $($item.File). Recompiler les moteurs concernes et mettre a jour le manifeste d'entrees."
        }
    }
    $inputs = Join-Path $run 'engine-inputs'
    $manifestDirectory = Split-Path -Parent $manifestFile
    foreach ($item in @(
        @{Name='linux_deb';File='linux/rustdesk.deb'},
        @{Name='linux_runner';File='linux/rustdesk'},
        @{Name='linux_library';File='linux/librustdesk.so'}
    )) {
        Stage-Engine $manifest $item.Name (Join-Path $inputs $item.File) $manifestDirectory
    }
    if ($SkipEngineBuild) {
        foreach ($item in @(
            @{Name='windows_portable';File='windows/portable.exe'},
            @{Name='windows_native';File='windows/native/rustdesk.exe'},
            @{Name='windows_sciter';File='windows/native/sciter.dll'},
            @{Name='windows_virtual_display';File='windows/native/dylib_virtual_display.dll'}
        )) {
            Stage-Engine $manifest $item.Name (Join-Path $inputs $item.File) $manifestDirectory
        }
        $windowsPortable = Join-Path $inputs 'windows/portable.exe'
        $windowsNative = Join-Path $inputs 'windows/native/rustdesk.exe'
    } else {
        # Compile in the snapshot, never over the project's embedded payloads/downloads.
        $engineReceipt = Join-Path $run 'windows-engine.json'
        $engineArgs = @{
            ProjectRoot=$source; ToolchainRoot=(Join-Path $projectRoot '.tools')
            SkipNativeDependencies=$true; SkipLaunchers=$true
            DestinationPath=(Join-Path $inputs 'windows/portable.exe'); ResultPath=$engineReceipt
        }
        & (Join-Path $PSScriptRoot 'build-rustdesk-windows.ps1') @engineArgs
        if (-not (Test-Path -LiteralPath $engineReceipt -PathType Leaf)) { throw 'Compilation moteur incomplete.' }
        $receipt = Get-Content -LiteralPath $engineReceipt -Raw -Encoding UTF8 | ConvertFrom-Json
        $windowsPortable = $receipt.portable
        $windowsNative = $receipt.native
        foreach ($item in @(@{Path=$windowsPortable;Hash=$receipt.portable_sha256}, @{Path=$windowsNative;Hash=$receipt.native_sha256})) {
            Assert-NoLinks $item.Path
            if ((Get-FileHash -LiteralPath $item.Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $item.Hash) { throw 'Moteur modifie apres construction.' }
        }
    }
    $buildArgs = @{
        SourceRoot=$source; WindowsPortable=$windowsPortable; WindowsNativeExecutable=$windowsNative
        LinuxDeb=(Join-Path $inputs 'linux/rustdesk.deb'); LinuxRunner=(Join-Path $inputs 'linux/rustdesk')
        LinuxLibrary=(Join-Path $inputs 'linux/librustdesk.so'); OutputDirectory=(Join-Path $run 'release-candidate')
        SigningKeyPath=(Resolve-Path -LiteralPath $ReleaseSigningKeyPath).Path; PublicKey=$ReleasePublicKey
        Version=$Version; SkipNSIS=[bool]$SkipNSIS; GoCacheDirectory=(Join-Path $projectRoot '.cache/maintenance-go-build')
    }
    & (Join-Path $PSScriptRoot 'build-security-release.ps1') @buildArgs
    $downloads = Join-Path $run 'release-candidate/downloads'
    if (-not (Test-Path -LiteralPath (Join-Path $downloads 'release-manifest.json') -PathType Leaf)) { throw 'Manifeste signe absent : build incomplet.' }
    $summary = @{
        status='CANDIDAT_A_TESTER'; version=$Version; audit_skipped=[bool]$SkipSecurityAudit
        source_snapshot=$source; report=(Join-Path $run 'rapport.json'); downloads=$downloads
        linux_tests_executed=$false; published=$false; engines=$manifest
    }
    [IO.File]::WriteAllText((Join-Path $run 'build-result.json'), ($summary | ConvertTo-Json -Depth 12), [Text.UTF8Encoding]::new($false))
    Write-Host "Candidat prepare pour vos tests : $downloads"
    Write-Host 'Aucune publication. Linux : tests compiles, execution et recette restant a faire.'
    if ($ShowReport) { Get-Content -LiteralPath (Join-Path $downloads 'SHA256SUMS.txt') }
}
try {
    # Must precede rollback, interactive prompts, Python and all writes.
    if ($DryRun) {
        Write-Host 'SIMULATION UNIQUEMENT : aucune commande externe, lecture de secret ou modification.'
        if ($Rollback) { Write-Host "Plan : verifier puis restaurer uniquement les dependances du lot explicite $RunId." }
        else { Write-Host 'Plan : audit isole -> moteurs explicites -> compilation signee dans update/ -> tests humains. Pas de deploiement.' }
        return
    }
    if ($Rollback) {
        if (-not $RunId) { throw '-Rollback exige -RunId ; aucune selection du dernier dossier.' }
        $rollbackArgs = @{RunId=$RunId; Rollback=$true; PythonPath=$PythonPath; ShowReport=[bool]$ShowReport}
        if ($Force) { $rollbackArgs.Force=$true }
        & (Join-Path $PSScriptRoot 'update-security.ps1') @rollbackArgs
        if ($LASTEXITCODE -ne 0) { throw 'Restauration incomplete/en echec : lire le rapport et application/receipt.json.' }
        return
    }
    if ($Interactive -or $PSBoundParameters.Count -eq 0) {
        Write-Host '1. Simulation ; 2. Maintenance dependances ; 3. Aide compilation ; 4. Deploiement ; 5. Cles ; 6. Restauration ; 0. Quitter'
        $choice = Read-Host 'Choix'
        switch ($choice) {
            '1' { Write-Host 'Audit -> candidats dans update/ -> recette. Aucun deploiement.' }
            '2' {
                $auditArgs=@{PythonPath=$PythonPath;RunBuildTests=$true;ShowReport=$true;InstallAuditTools=[bool]$InstallAuditTools}
                & (Join-Path $PSScriptRoot 'update-security.ps1') @auditArgs
                if ($LASTEXITCODE -ne 0) { throw 'Maintenance incomplete/en echec : lire le rapport ; rien applique.' }
            }
            '3' { Write-Host 'Utiliser -Version -EngineManifest -ReleaseSigningKeyPath -ReleasePublicKey. Exemple dans docs/MAINTENANCE_SECURITE.md.' }
            '4' { Show-ProductionPolicy }
            '5' { Invoke-PythonChecked @((Join-Path $PSScriptRoot 'manage-secrets.py'),'--action','plan') }
            '6' { Write-Host 'Restauration explicite : -Rollback -RunId IDENTIFIANT. Les anciennes sauvegardes partielles des executables ne sont jamais restaurees automatiquement.' }
            '0' { return }
            default { throw 'Choix invalide.' }
        }
        return
    }
    Invoke-Maintenance
} catch {
    Write-Error -ErrorAction Continue $_
    exit 1
}
