#requires -Version 5.1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$source = Join-Path $PSScriptRoot 'migrate-windows-permanent-1.0.3.ps1'
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile($source, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
# Load definitions only. NEVER invoke the migration or query/control real services.
foreach ($fn in $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $false)) {
    . ([ScriptBlock]::Create($fn.Extent.Text))
}
function Assert-Test([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Assert-Throws([scriptblock]$Action, [string]$Message) {
    $threw = $false
    try { & $Action } catch { $threw = $true }
    Assert-Test $threw $Message
}
$fixtures = Join-Path (Split-Path $PSScriptRoot -Parent) ('.cache\migration-check-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $fixtures | Out-Null

# Exercise the exact parameter-binding and initialization code via -File in a
# fresh Windows PowerShell 5.1 process. The probe excludes ALL migration functions.
$header = (Get-Content -LiteralPath $source -Raw).Split(@('function Assert-RDPlainPath'), [StringSplitOptions]::None)[0]
$probe = Join-Path $fixtures 'parameter-probe.ps1'
[IO.File]::WriteAllText($probe, $header + "`r`nWrite-Output `$PayloadDirectory`r`n", [Text.Encoding]::UTF8)
$windowsPowerShell = Join-Path ([Environment]::GetFolderPath('Windows')) 'System32\WindowsPowerShell\v1.0\powershell.exe'
Push-Location -LiteralPath (Split-Path $fixtures -Parent)
try {
    $resolvedDefault = & $windowsPowerShell -NoProfile -ExecutionPolicy Bypass -File $probe
    Assert-Test ($LASTEXITCODE -eq 0) 'Default argument failed under Windows PowerShell -File.'
    Assert-Test ($resolvedDefault -eq (Join-Path $fixtures 'payload')) 'Default payload must be next to the script, not the current directory.'
    $resolvedExplicit = & $windowsPowerShell -NoProfile -ExecutionPolicy Bypass -File $probe -PayloadDirectory '.\custom-payload'
    Assert-Test ($LASTEXITCODE -eq 0) 'Explicit argument failed under Windows PowerShell -File.'
    Assert-Test ($resolvedExplicit -eq (Join-Path (Get-Location).Path 'custom-payload')) 'Explicit relative path must resolve against the caller directory.'
} finally { Pop-Location }

$inputFile = Join-Path $fixtures 'source.bin'
$destination = Join-Path $fixtures 'target.bin'
[IO.File]::WriteAllBytes($inputFile, [byte[]](1, 2, 3, 4))
$hash = (Get-FileHash -LiteralPath $inputFile -Algorithm SHA256).Hash
Assert-RDHash $inputFile $hash
Assert-Throws { Assert-RDHash $inputFile ('0' * 64) } 'Bad hash accepted.'
Assert-Throws { Assert-RDHash $fixtures $hash } 'Directory accepted as binary.'
Assert-Throws { Assert-RDHash (Join-Path $fixtures 'missing.bin') $hash } 'Missing file accepted.'

$oldCommand = '"C:\Program Files\RustDesk\rustdesk.exe" --service'
$newCommand = '"C:\Program Files\RelaisDeskEngine\rustdesk.exe" --service'
$service = [pscustomobject]@{Name='RustDesk'; StartName='LocalSystem'; PathName=$oldCommand}
Assert-RDService $service 'RustDesk' @($oldCommand, $newCommand)
$service.PathName = $newCommand
Assert-RDService $service 'RustDesk' @($oldCommand, $newCommand)
$service.PathName = $newCommand + ' --extra'
Assert-Throws { Assert-RDService $service 'RustDesk' @($oldCommand, $newCommand) } 'Extra arguments accepted.'
$service.PathName = 'C:\Users\Public\rustdesk.exe --service'
Assert-Throws { Assert-RDService $service 'RustDesk' @($oldCommand, $newCommand) } 'Foreign service path accepted.'
$service.PathName = $newCommand; $service.StartName = 'SomeUser'
Assert-Throws { Assert-RDService $service 'RustDesk' @($oldCommand, $newCommand) } 'Custom account accepted.'
Assert-Throws { Assert-RDService $null 'RustDesk' @($oldCommand) } 'Missing service accepted.'

# Test actual copy/hash/atomic-replace behavior on inert fixture files only.
# Mock ACL application so tests need no elevation and never modify a real ACL.
$script:aclCalls = 0
function Set-Acl { param($LiteralPath, $AclObject) $script:aclCalls++ }
Install-RDVerifiedFile $inputFile $destination $hash
Assert-RDHash $destination $hash
[IO.File]::WriteAllBytes($inputFile, [byte[]](5, 6, 7, 8))
$newHash = (Get-FileHash -LiteralPath $inputFile -Algorithm SHA256).Hash
Install-RDVerifiedFile $inputFile $destination $newHash
Assert-RDHash $destination $newHash
Assert-Throws { Install-RDVerifiedFile $inputFile $destination $hash } 'Tampered package installed.'
Assert-RDHash $destination $newHash
Assert-Test ($script:aclCalls -eq 2) 'Destination ACL not set.'

# A failed service reconfiguration must fail, never pretend success.
function Get-CimInstance { param($ClassName, $Filter) return [pscustomobject]@{Name='RustDesk'} }
function Invoke-CimMethod { param($InputObject, $MethodName, $Arguments) return [pscustomobject]@{ReturnValue=2} }
Assert-Throws { Set-RDServicePath 'RustDesk' $newCommand } 'Service failure ignored.'

$text = Get-Content -LiteralPath $source -Raw
Assert-Test ($text.IndexOf('if (-not $Apply)') -lt $text.IndexOf('Set-RDProtectedDirectory $engine')) 'Dry run could modify the machine.'
Assert-Test (-not ($text -match '(?im)^\s*(Remove-Item|Remove-Service|Invoke-WebRequest|Invoke-RestMethod)\b')) 'Unexpected delete/network command.'
Assert-Test ($text.Contains("Set-Service -Name `$name -StartupType Disabled; Stop-RDService `$name")) 'Fail-closed recovery absent.'
Assert-Test ($text.Contains('Assert-RDHash $path $identityHashes[$path]')) 'Identity preservation check absent.'
Write-Host 'OK : syntaxe PowerShell, services refuses si inattendus, empreintes, copie et remplacement atomique, conservation sur erreur, controles sans mutation des services.'
