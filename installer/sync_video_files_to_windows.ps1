# Synchronise WSL -> arbre Windows natif pour le build (lanceurs + scripts +
# fork au niveau HEAD), puis verifie des marqueurs de contenu.
# Usage (une ligne chacune) :
#   Copy-Item "\\wsl$\Ubuntu-22.04\home\julien\projet\installer\sync_video_files_to_windows.ps1" -Destination "C:\Users\Administrator\Documents\Projets\projet\installer\" -Force
#   powershell -ExecutionPolicy Bypass -File "C:\Users\Administrator\Documents\Projets\projet\installer\sync_video_files_to_windows.ps1"
$ErrorActionPreference = "Stop"
$Wsl = "\\wsl$\Ubuntu-22.04\home\julien\projet"
$Win = "C:\Users\Administrator\Documents\Projets\projet"
$files = @(
  "docs/FORK_IMPLEMENTATION_STATUS.md",
  "installer/configurator/codec_perf.go",
  "installer/configurator/codec_perf_test.go",
  "installer/configurator/main.go",
  "installer/configurator/rustdesk_darwin.go",
  "installer/configurator/rustdesk_linux.go",
  "installer/configurator/rustdesk_windows.go",
  "installer/configurator/config_windows.go",
  "installer/configurator/i18n.go",
  "installer/configurator/service_billing_linux.go",
  "installer/configurator/gui.go",
  "installer/configurator/fleet_uri.go",
  "installer/configurator/api.go",
  "installer/configurator/google_auth.go",
  "installer/configurator/service_billing_gui.go",
  "installer/configurator/config_linux.go",
  "installer/configurator/gui_linux.go",
  "installer/configurator/fleet_gui.go",
  "installer/configurator/folders_test.go",
  "installer/configurator/rustdesk_darwin_test.go",
  "installer/configurator/folders.go",
  "installer/configurator/fleet_uri_test.go",
  "installer/configurator/go.mod",
  "installer/configurator/go.sum",
  "installer/configurator/customer_extras_test.go",
  "installer/configurator/customer_team.go",
  "installer/configurator/customer_gui.go",
  "installer/configurator/customer_api_test.go",
  "installer/configurator/customer_billing.go",
  "installer/configurator/customer_silent_test.go",
  "installer/configurator/customer_gui_test.go",
  "installer/configurator/customer_api.go",
  "installer/configurator/customer_history.go",
  "installer/configurator/customer_gui_panels.go",
  "installer/configurator/customer_services.go",
  "installer/configurator/pin_consistency_test.go",
  "installer/viewer/codec_perf.go",
  "installer/viewer/codec_perf_test.go",
  "installer/viewer/main.go",
  "installer/viewer/rustdesk_darwin.go",
  "installer/viewer/rustdesk_linux.go",
  "installer/viewer/rustdesk_windows.go",
  "installer/viewer/config_windows.go",
  "installer/build_linux_technicien.ps1",
  "installer/build_technicien_portable.ps1",
  "installer/build_viewer_windows.ps1",
  "scripts/build-rustdesk-windows.ps1",
  "scripts/rustdesk-sciter-vcpkg/vcpkg.json",
  "scripts/mirror-portable-oracle.ps1",
  "scripts/deploy-api-20260927.ps1",
  "rustdesk/.github/workflows/flutter-build.yml",
  "rustdesk/.github/workflows/relaisdesk-mac.yml",
  "rustdesk/Cargo.lock",
  "rustdesk/Cargo.toml",
  "rustdesk/build.py",
  "rustdesk/src/client.rs",
  "rustdesk/src/client/io_loop.rs",
  "rustdesk/src/lib.rs",
  "rustdesk/src/platform/gtk_sudo.rs",
  "rustdesk/src/platform/win_device.rs",
  "rustdesk/src/plugin/manager.rs",
  "rustdesk/src/plugin/native_handlers/macros.rs",
  "rustdesk/src/plugin/native_handlers/session.rs",
  "rustdesk/src/relaisdesk_intervention.rs",
  "rustdesk/src/server/connection.rs",
  "rustdesk/src/server/video_qos.rs"
)
foreach ($f in $files) {
  $src = "$Wsl/$f"
  if (-not (Test-Path -LiteralPath $src)) { throw "Source absente : $src" }
  $dest = Join-Path $Win ($f -replace '/', '\')
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $dest) | Out-Null
  Copy-Item $src -Destination $dest -Force
}
# Marqueurs : @(chemin, motif, estRegex)
$checks = @(
  @("installer\viewer\codec_perf.go", "CodecPreferenceAuto", $false),
  @("installer\configurator\codec_perf.go", "setHwCodecEverywhere", $false),
  @("installer\configurator\fleet_uri.go", "splitCLIArgs", $false),
  @("installer\configurator\customer_api.go", "package main", $false),
  @("scripts\build-rustdesk-windows.ps1", "inline,vram,hwcodec", $false),
  @("scripts\rustdesk-sciter-vcpkg\vcpkg.json", "mfx-dispatch", $false),
  @("rustdesk\src\server\video_qos.rs", "INIT_FPS: u32 = 30", $false),
  @("rustdesk\Cargo.lock", 'name = "ringbuf"\r?\nversion = "0.5.2"', $true),
  @("installer\viewer\config_windows.go", "6e13fd769c0eb77ae899f6e96a4d112249aa42e3e285494ac6030d0d4b9900eb", $false)
)
foreach ($c in $checks) {
  $p = Join-Path $Win $c[0]
  $pattern = $c[1]
  if (-not $c[2]) { $pattern = [regex]::Escape($pattern) }
  if ((Get-Content -LiteralPath $p -Raw) -notmatch $pattern) {
    throw "Marqueur absent dans $($c[0]) : $($c[1])"
  }
}
Write-Host ("SYNC OK : {0} fichiers copies, {1} marqueurs verifies." -f $files.Count, $checks.Count)
