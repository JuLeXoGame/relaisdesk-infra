<#
.SYNOPSIS
    Lance la maintenance et l'audit de sécurité des dépendances de RelaisDesk.

.DESCRIPTION
    Recherche un environnement Python 3.11+, configure l'encodage UTF-8 et exécute
    scripts/security_maintenance.py dans un espace de travail temporaire isolé (update/).
    Aucune modification n'est appliquée en direct sur les sources du projet sans confirmation :
    -ApplyFixes et -Rollback demandent une validation interactive, sauf -Force ou -DryRun.

.PARAMETER AuditOnly
    Effectue uniquement l'audit des vulnérabilités sans préparer de fichiers corrigés.

.PARAMETER PrepareOnly
    Copie et prépare les sources isolées sans exécuter les scanners de vulnérabilités.

.PARAMETER RunBuildTests
    Exécute les tests de compilation des modules audités.

.PARAMETER InstallAuditTools
    Télécharge et installe automatiquement les outils d'audit manquants (govulncheck, etc.).

.PARAMETER ApplyFixes
    Applique les manifestes et fichiers corrigés (fichiers-corriges/) dans le projet,
    après confirmation interactive (sauf -Force ou -DryRun).

.PARAMETER DryRun
    Simule l'application (-ApplyFixes, implicite) ou l'annulation (-Rollback)
    sans modifier le projet.

.PARAMETER Rollback
    Annule un lot précédemment appliqué, désigné par -RunId (obligatoire).
    N'exécute aucun audit ; demande confirmation sauf -Force ou -DryRun.

.PARAMETER Force
    Applique ou annule sans demander de confirmation (usage non interactif).

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
    Lance l'audit puis applique les dépendances corrigées après confirmation.

.EXAMPLE
    .\update-security.ps1 -ApplyFixes -DryRun -ShowReport
    Lance l'audit puis simule l'application, sans modifier le projet.

.EXAMPLE
    .\update-security.ps1 -Rollback -RunId '20260919-120000-1234abcd' -ShowReport
    Annule un lot précédemment appliqué, après confirmation.
#>
[CmdletBinding()]
param(
    [switch]$AuditOnly,
    [switch]$PrepareOnly,
    [switch]$RunBuildTests,
    [switch]$InstallAuditTools,
    [switch]$ApplyFixes,
    [switch]$DryRun,
    [switch]$Rollback,
    [switch]$Force,
    [switch]$ShowReport,
    [switch]$OpenReport,
    [ValidateRange(30, 7200)][int]$CommandTimeoutSeconds = 900,
    [string]$PythonPath = "",
    [ValidatePattern('^\d{8}-\d{6}-[a-f0-9]{8,32}$')][string]$RunId
)

function Confirm-ProjectWrite {
    param([string]$Question)
    if ($Force) { return $true }
    if (-not [Environment]::UserInteractive) {
        throw 'Session non interactive : relancer avec -Force pour confirmer explicitement.'
    }
    return ((Read-Host "$Question [o/N]") -match '^(?i)(o|oui|y|yes)$')
}

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$projectRoot = Split-Path -Parent $PSScriptRoot

if ($AuditOnly -and $PrepareOnly) { 
    throw 'Choisir -AuditOnly OU -PrepareOnly.' 
}
if ($Rollback) {
    if (-not $PSBoundParameters.ContainsKey('RunId')) {
        throw '-Rollback exige -RunId : identifiant du lot appliqué à annuler.'
    }
    if ($ApplyFixes -or $AuditOnly -or $PrepareOnly -or $RunBuildTests -or $InstallAuditTools) {
        throw '-Rollback est exclusif : aucun audit ni -ApplyFixes.'
    }
} else {
    if ($DryRun) { $ApplyFixes = $true }
    if ($ApplyFixes -and ($AuditOnly -or $PrepareOnly)) {
        throw '-ApplyFixes exige une maintenance complete, pas -AuditOnly/-PrepareOnly.'
    }
    if ($ApplyFixes) { $RunBuildTests = $true }
    if (-not $RunId) { $RunId = (Get-Date -Format 'yyyyMMdd-HHmmss-') + [Guid]::NewGuid().ToString('N') }
}
$runDirectory = Join-Path $projectRoot "update/$RunId"
if ($Rollback) {
    if (-not (Test-Path -LiteralPath $runDirectory -PathType Container)) {
        throw "Lot '$RunId' introuvable dans update/ : rien à annuler."
    }
} elseif (Test-Path -LiteralPath $runDirectory) {
    throw 'Identifiant deja utilise ; aucun lot ne sera ecrase.'
}

# Assurer un encodage UTF-8 propre dans la console Windows
$prevConsoleEncoding = [Console]::OutputEncoding
$prevOutputEncoding = $OutputEncoding
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

try {
    if ($PythonPath -and -not (Test-Path -LiteralPath $PythonPath -PathType Leaf) -and -not (Get-Command $PythonPath -ErrorAction SilentlyContinue)) {
        throw "Chemin Python invalide : '$PythonPath' est introuvable (ni fichier ni commande)."
    }
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
    $exitCode = 0
    if (-not $Rollback) {
        & $selectedPython[0] @prefix @arguments
        $exitCode = $LASTEXITCODE
    }
    # Application (-ApplyFixes) ou annulation (-Rollback) du lot
    if (($ApplyFixes -or $Rollback) -and $exitCode -eq 0) {
        $applyArguments = @('-B', (Join-Path $PSScriptRoot 'maintenance_apply.py'), '--project-root', $projectRoot, '--run-id', $RunId)
        if ($Rollback) { $applyArguments += '--rollback' }
        if ($DryRun) { $applyArguments += '--dry-run' }
        $question = if ($Rollback) { 'Annuler le lot appliqué sur le projet' } else { 'Appliquer les correctifs au projet' }
        if ($DryRun -or (Confirm-ProjectWrite -Question $question)) {
            & $selectedPython[0] @prefix @applyArguments
            $exitCode = $LASTEXITCODE
        } else {
            Write-Host 'Abandon sur confirmation : aucune modification du projet.' -ForegroundColor Yellow
        }
    }

    # Actions post-exécution (Affichage ou ouverture du rapport)
    if ($ShowReport -or $OpenReport) {
        $latestReport = Get-Item -LiteralPath (Join-Path $runDirectory 'rapport.md') -ErrorAction SilentlyContinue

        if ($latestReport) {
            if ($ShowReport) {
                Write-Host "`n--- SYNTHÈSE DU RAPPORT ($($latestReport.FullName)) ---" -ForegroundColor Cyan
                $reportLines = Get-Content -LiteralPath $latestReport.FullName -Encoding UTF8
                $synthesis = @()
                foreach ($reportLine in $reportLines) {
                    if ($reportLine -match '^## Signalements restants') { break }
                    $synthesis += $reportLine
                }
                if (-not $synthesis) { $synthesis = @($reportLines | Select-Object -First 35) }
                $synthesis | ForEach-Object { Write-Host $_ }
                $reportLines | Where-Object { $_ -match '^\*\*NE PAS appliquer' } | ForEach-Object { Write-Host $_ -ForegroundColor Yellow }
                Write-Host "Détail complet : $($latestReport.FullName)" -ForegroundColor Cyan
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
