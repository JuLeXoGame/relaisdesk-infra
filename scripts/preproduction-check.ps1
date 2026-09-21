param(
    [string]$LiveApiUrl = "",
    [string]$RustDeskHost = "api.relaisdesk.fr",
    [int]$RendezvousPort = 21116,
    [int]$RelayPort = 21117,
    [string]$EmailDomain = "relaisdesk.fr",
    [string]$DKIMSelector = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$projectRoot = Split-Path -Parent $PSScriptRoot
$goModules = @("database", "api", "tests", "keygen", "installer/configurator", "installer/viewer", "installer/debpack", "dashboard_admin")

foreach ($module in $goModules) {
    $modulePath = Join-Path $projectRoot $module
    Write-Host "[TEST] $module" -ForegroundColor Cyan
    Push-Location $modulePath
    try {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Échec des tests Go dans $module" }
    } finally {
        Pop-Location
    }
}

if (-not [string]::IsNullOrWhiteSpace($DKIMSelector)) {
    Write-Host "[EMAIL] Vérification ponctuelle SPF/DKIM/DMARC" -ForegroundColor Cyan
    Push-Location (Join-Path $projectRoot "api")
    try {
        & go run ./cmd/emailcheck -domain $EmailDomain -selector $DKIMSelector -strict=true
        if ($LASTEXITCODE -ne 0) { throw "Échec de l'authentification DNS des courriels" }
    } finally {
        Pop-Location
    }
}

Write-Host "[TEST] rustdesk-server (autorisation et quota simultané)" -ForegroundColor Cyan
Push-Location (Join-Path $projectRoot "rustdesk-server")
try {
    & cargo test --locked --lib relaisdesk_auth
    if ($LASTEXITCODE -ne 0) { throw "Échec des tests du serveur RustDesk" }
} finally {
    Pop-Location
}

if (-not [string]::IsNullOrWhiteSpace($LiveApiUrl)) {
    $healthUrl = $LiveApiUrl.TrimEnd('/') + "/api/v1/health"
    Write-Host "[LIVE] $healthUrl" -ForegroundColor Cyan
    $health = Invoke-RestMethod -Method Get -Uri $healthUrl -TimeoutSec 10
    if ($health.status -ne "healthy") { throw "API live dégradée" }

    foreach ($port in @($RendezvousPort, $RelayPort)) {
        $probe = Test-NetConnection -ComputerName $RustDeskHost -Port $port -WarningAction SilentlyContinue
        if (-not $probe.TcpTestSucceeded) { throw "Port TCP $RustDeskHost`:$port inaccessible" }
    }
}

Write-Host "Préproduction validée : achat, activation, autorisations signées et quota simultané." -ForegroundColor Green
