<#
.SYNOPSIS
    Lance la maintenance et l'audit de sécurité des dépendances de RelaisDesk.

.DESCRIPTION
    Recherche un environnement Python 3.11+, configure l'encodage UTF-8 et exécute
    scripts/security_maintenance.py dans un espace de travail temporaire isolé (update/).
    Aucune modification n'est appliquée en direct sur les sources du projet sans confirmation.

.PARAMETER AuditOnly
    Effectue uniquement l'audit des vulnérabilités sans préparer de fichiers corrigés.

.PARAMETER PrepareOnly
    Copie et prépare les sources isolées sans exécuter les scanners de vulnérabilités.

.PARAMETER RunBuildTests
    Exécute les tests de compilation des modules audités.

.PARAMETER InstallAuditTools
    Télécharge et installe automatiquement les outils d'audit manquants (govulncheck, etc.).

.PARAMETER ApplyFixes
    Applique automatiquement les manifestes et fichiers corrigés (fichiers-corriges/) directement dans le projet.

.PARAMETER ShowReport
    Affiche le tableau de synthèse du rapport dans la console à la fin de l'exécution.

.PARAMETER OpenReport
    Ouvre automatiquement le rapport Markdown dans l'application par défaut.

.PARAMETER CommandTimeoutSeconds
    Délai maximal alloué à chaque commande d'audit/build (par défaut 900s, entre 30s et 7200s).

.PARAMETER PythonPath
    Chemin direct vers un exécutable Python 3.11+ spécifique (outrepasse la détection automatique).

.EXAMPLE
    .\update-security.ps1 -AuditOnly -ShowReport
    Lance un audit rapide et affiche le rapport de synthèse dans la console.

.EXAMPLE
    .\update-security.ps1 -RunBuildTests -ShowReport
    Lance l'audit complet avec compilation et tests des modules.

.EXAMPLE
    .\update-security.ps1 -ApplyFixes -ShowReport
    Lance l'audit et applique directement les dépendances corrigées sur le projet.
#>
[CmdletBinding()]
param(
    [switch]$AuditOnly,
    [switch]$PrepareOnly,
    [switch]$RunBuildTests,
    [switch]$InstallAuditTools,
    [switch]$ApplyFixes,
    [switch]$ShowReport,
    [switch]$OpenReport,
    [ValidateRange(30, 7200)][int]$CommandTimeoutSeconds = 900,
    [string]$PythonPath = "",
    [ValidatePattern('^\d{8}-\d{6}-[a-f0-9]{8,32}$')][string]$RunId
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$projectRoot = Split-Path -Parent $PSScriptRoot

if ($AuditOnly -and $PrepareOnly) { 
    throw 'Choisir -AuditOnly OU -PrepareOnly.' 
}
if ($ApplyFixes -and ($AuditOnly -or $PrepareOnly)) {
    throw '-ApplyFixes exige une maintenance complete, pas -AuditOnly/-PrepareOnly.'
}
if ($ApplyFixes) { $RunBuildTests = $true }
if (-not $RunId) { $RunId = (Get-Date -Format 'yyyyMMdd-HHmmss-') + [Guid]::NewGuid().ToString('N') }
$runDirectory = Join-Path $projectRoot "update/$RunId"
if (Test-Path -LiteralPath $runDirectory) { throw 'Identifiant deja utilise ; aucun lot ne sera ecrase.' }

# Assurer un encodage UTF-8 propre dans la console Windows
$prevConsoleEncoding = [Console]::OutputEncoding
$prevOutputEncoding = $OutputEncoding
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

try {
    $candidates = @()
    if ($PythonPath) {
        $candidates += ,@($PythonPath)
    } else {
        # 1. Environnement virtuel actif si présent
        if ($env:VIRTUAL_ENV) {
            $venvPy = Join-Path $env:VIRTUAL_ENV 'Scripts\python.exe'
            if (Test-Path -LiteralPath $venvPy -PathType Leaf) { $candidates += ,@($venvPy) }
        }
        # 2. Commandes python / python3 dans le PATH (en évitant le stub vide WindowsApps)
        foreach ($name in @('python', 'python3')) {
            $command = Get-Command $name -ErrorAction SilentlyContinue
            if ($command -and $command.Source -notmatch '(?i)WindowsApps') { 
                $candidates += ,@($command.Source) 
            }
        }
        # 3. Lanceur officiel py -3
        $launcher = Get-Command py -ErrorAction SilentlyContinue
        if ($launcher) { $candidates += ,@($launcher.Source, '-3') }
        # 4. Répertoires d'installation Windows standards (ex. C:\Python314, C:\Python312...)
        Get-ChildItem -Path 'C:\' -Filter 'Python3*' -Directory -ErrorAction SilentlyContinue | ForEach-Object {
            $stdPy = Join-Path $_.FullName 'python.exe'
            if (Test-Path -LiteralPath $stdPy -PathType Leaf) { $candidates += ,@($stdPy) }
        }
        # 5. Répertoire LocalAppData Programs
        $localPyDir = Join-Path $env:LOCALAPPDATA 'Programs\Python'
        if (Test-Path -LiteralPath $localPyDir -PathType Container) {
            Get-ChildItem -Path $localPyDir -Filter 'Python3*' -Directory -ErrorAction SilentlyContinue | ForEach-Object {
                $userPy = Join-Path $_.FullName 'python.exe'
                if (Test-Path -LiteralPath $userPy -PathType Leaf) { $candidates += ,@($userPy) }
            }
        }
        # 6. Runtime packagé dans le profil utilisateur
        $bundled = Join-Path $env:USERPROFILE '.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'
        if (Test-Path -LiteralPath $bundled -PathType Leaf) { $candidates += ,@($bundled) }
    }

    # Sélection du premier Python validé >= 3.11
    $selectedPython = $null
    foreach ($candidate in $candidates) {
        try {
            $prefix = @($candidate | Select-Object -Skip 1)
            & $candidate[0] @prefix -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 11) else 1)' 2>$null
            if ($LASTEXITCODE -eq 0) { $selectedPython = $candidate; break }
        } catch { }
    }

    if (-not $selectedPython) {
        throw 'Python 3.11 ou plus récent requis. Installer Python ou spécifier -PythonPath "C:\chemin\vers\python.exe".'
    }

    # Préparation des arguments pour security_maintenance.py
    $maintenanceScript = Join-Path $PSScriptRoot 'security_maintenance.py'
    $arguments = @('-B', $maintenanceScript, '--project-root', $projectRoot, '--timeout', [string]$CommandTimeoutSeconds, '--run-id', $RunId)
    if ($AuditOnly) { $arguments += '--audit-only' }
    if ($PrepareOnly) { $arguments += '--prepare-only' }
    if ($RunBuildTests) { $arguments += '--run-build-tests' }
    if ($InstallAuditTools) { $arguments += '--install-audit-tools' }

    $prefix = @($selectedPython | Select-Object -Skip 1)
    & $selectedPython[0] @prefix @arguments
    $exitCode = $LASTEXITCODE
    # Application des correctifs (-ApplyFixes)
    if ($ApplyFixes -and $exitCode -eq 0) {
        & $selectedPython[0] @prefix -B (Join-Path $PSScriptRoot 'maintenance_apply.py') --project-root $projectRoot --run-id $RunId
        $exitCode = $LASTEXITCODE
    }

    # Actions post-exécution (Affichage ou ouverture du rapport)
    if ($ShowReport -or $OpenReport) {
        $latestReport = Get-Item -LiteralPath (Join-Path $runDirectory 'rapport.md') -ErrorAction SilentlyContinue

        if ($latestReport) {
            if ($ShowReport) {
                Write-Host "`n--- SYNTHÈSE DU RAPPORT ($($latestReport.FullName)) ---" -ForegroundColor Cyan
                Get-Content -LiteralPath $latestReport.FullName -Encoding UTF8 | Select-Object -First 35 | ForEach-Object { Write-Host $_ }
            }
            if ($OpenReport) {
                Start-Process -FilePath $latestReport.FullName
            }
        }
    }

    exit $exitCode
}
finally {
    [Console]::OutputEncoding = $prevConsoleEncoding
    $OutputEncoding = $prevOutputEncoding
}
