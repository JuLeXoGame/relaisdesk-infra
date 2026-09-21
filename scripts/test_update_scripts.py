"""Offline PowerShell integration tests; all builds/audits are local stubs in temp fixtures."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent
SHELL = shutil.which("powershell.exe") or shutil.which("pwsh")

AUDIT_STUB = r'''
[CmdletBinding()]
param([string]$RunId,[string]$PythonPath,[switch]$ShowReport,[switch]$InstallAuditTools,[switch]$RunBuildTests,[switch]$ApplyFixes,[switch]$PrepareOnly,[switch]$AuditOnly)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if ($env:RELAISDESK_TEST_AUDIT_EXIT) { exit ([int]$env:RELAISDESK_TEST_AUDIT_EXIT) }
if (-not $RunId) { $RunId='20260919-120000-1234abcd' }
$run=Join-Path $root "update/$RunId"
$source=Join-Path $run 'projet'
New-Item -ItemType Directory -Path $source | Out-Null
Copy-Item -LiteralPath (Join-Path $root 'rustdesk') -Destination $source -Recurse
@{do_not_apply_batch=[bool]$env:RELAISDESK_TEST_BLOCK;run_build_tests=[bool]$RunBuildTests} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $run 'rapport.json')
$PSBoundParameters | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $root 'audit-arguments.json')
exit 0
'''
BUILD_STUB = r'''
[CmdletBinding()]
param([string]$SourceRoot,[string]$WindowsPortable,[string]$WindowsNativeExecutable,[string]$LinuxDeb,[string]$LinuxRunner,[string]$LinuxLibrary,[string]$OutputDirectory,[string]$SigningKeyPath,[string]$PublicKey,[string]$Version,[switch]$SkipNSIS,[string]$GoCacheDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$PSBoundParameters | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $root 'build-arguments.json')
if ($env:RELAISDESK_TEST_BUILD_FAIL) { throw 'simulated builder failure' }
$downloads=Join-Path $OutputDirectory 'downloads'
New-Item -ItemType Directory -Path $downloads | Out-Null
'{}' | Set-Content -LiteralPath (Join-Path $downloads 'release-manifest.json')
'''
ENGINE_STUB = r'''
[CmdletBinding()]
param([string]$ProjectRoot,[string]$ToolchainRoot,[switch]$SkipNativeDependencies,[switch]$SkipLaunchers,[string]$DestinationPath,[string]$ResultPath)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$PSBoundParameters | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $root 'engine-arguments.json')
$nativeDir=Join-Path (Split-Path -Parent $DestinationPath) 'native'
New-Item -ItemType Directory -Path $nativeDir -Force | Out-Null
'MZportable' | Set-Content -LiteralPath $DestinationPath
foreach ($name in @('rustdesk.exe','sciter.dll','dylib_virtual_display.dll')) { 'MZnative' | Set-Content -LiteralPath (Join-Path $nativeDir $name) }
$native=Join-Path $nativeDir 'rustdesk.exe'
@{portable=$DestinationPath;native=$native;portable_sha256=(Get-FileHash -LiteralPath $DestinationPath).Hash.ToLowerInvariant();native_sha256=(Get-FileHash -LiteralPath $native).Hash.ToLowerInvariant()} | ConvertTo-Json | Set-Content -LiteralPath $ResultPath
'''


@unittest.skipUnless(SHELL, "PowerShell unavailable")
class ScriptTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="relaisdesk-script-tests-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        (self.root / "scripts").mkdir()
        for name in ("update-and-rebuild-programs.ps1", "update-security.ps1"):
            shutil.copy2(SCRIPTS / name, self.root / "scripts" / name)
        self.write("scripts/build-security-release.ps1", BUILD_STUB)
        self.write("rustdesk/Cargo.lock", "locked fixture")
        self.write("rustdesk/Cargo.toml", "fixture manifest")
        self.write("key.fixture", "NOT A REAL PRIVATE KEY")
        self.write("relaisdesk/downloads/untouched.txt", "public files must stay untouched")
        names = ("linux_deb", "linux_runner", "linux_library", "windows_native", "windows_portable", "windows_sciter", "windows_virtual_display")
        self.manifest = {"schema_version": 1, "artifacts": {},
                         "rustdesk_cargo_lock_sha256": hashlib.sha256(b"locked fixture").hexdigest(),
                         "rustdesk_cargo_toml_sha256": hashlib.sha256(b"fixture manifest").hexdigest()}
        for name in names:
            self.write("inputs/" + name, name)
            self.manifest["artifacts"][name] = {"path": "inputs/" + name, "sha256": hashlib.sha256(name.encode()).hexdigest()}
        self.save_manifest()

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def save_manifest(self):
        self.write("engines.json", json.dumps(self.manifest))

    def invoke(self, args, script="update-and-rebuild-programs.ps1", extra_env=None):
        env = os.environ.copy()
        # PowerShell 7's module path cannot be reused by Windows PowerShell 5.1.
        # Let the child shell initialize its own built-in modules.
        for name in list(env):
            if name.lower() == "psmodulepath":
                env.pop(name)
        env.update(extra_env or {})
        return subprocess.run([SHELL, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", str(self.root / "scripts" / script), *args],
                              cwd=self.root, env=env, capture_output=True, text=True, errors="replace", timeout=30)

    def build_args(self):
        return ["-SkipEngineBuild", "-Version", "9.8.7", "-EngineManifest", str(self.root / "engines.json"),
                "-ReleaseSigningKeyPath", str(self.root / "key.fixture"), "-ReleasePublicKey", "A" * 43, "-SkipNSIS"]

    def stub_audit(self):
        self.write("scripts/update-security.ps1", AUDIT_STUB)

    def test_dry_run_precedes_rollback_and_interactive(self):
        before = sorted(str(p) for p in self.root.rglob("*"))
        result = self.invoke(["-DryRun", "-Rollback", "-Interactive"])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("SIMULATION", result.stdout)
        self.assertEqual(sorted(str(p) for p in self.root.rglob("*")), before)

    def test_rollback_requires_exact_run_id(self):
        result = self.invoke(["-Rollback"])
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "update").exists())

    def test_incompatible_apply_modes_fail_before_tools(self):
        for mode in ("-AuditOnly", "-PrepareOnly"):
            result = self.invoke(["-ApplyFixes", mode], "update-security.ps1")
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((self.root / "update").exists())
        result = self.invoke(["-ApplySecurityFixes", "-SkipSecurityAudit"])
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "update").exists())

    def test_real_prepare_wrapper_uses_exact_run_and_rejects_reuse(self):
        shutil.copy2(SCRIPTS / "security_maintenance.py", self.root / "scripts/security_maintenance.py")
        self.write("database/go.mod", "module fixture\n\ngo 1.26.6\n")
        run_id = "20260919-120000-1234abcd"
        args = ["-PrepareOnly", "-PythonPath", sys.executable, "-RunId", run_id, "-ShowReport"]
        result = self.invoke(args, "update-security.ps1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        report = json.loads((self.root / "update" / run_id / "rapport.json").read_text())
        self.assertTrue(report["do_not_apply_batch"])
        self.assertEqual(report["mode"], "prepare")
        self.assertEqual(Path(report["output"]), self.root / "update" / run_id)
        second = self.invoke(args, "update-security.ps1")
        self.assertNotEqual(second.returncode, 0)
        self.assertEqual((self.root / "database/go.mod").read_text(), "module fixture\n\ngo 1.26.6\n")

    def test_named_arguments_source_snapshot_and_linux_library(self):
        self.stub_audit()
        result = self.invoke(self.build_args() + ["-ApplySecurityFixes"])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        audit = json.loads((self.root / "audit-arguments.json").read_text(encoding="utf-8-sig"))
        build = json.loads((self.root / "build-arguments.json").read_text(encoding="utf-8-sig"))
        self.assertTrue(audit["ApplyFixes"])
        self.assertTrue(audit["RunBuildTests"])
        self.assertEqual(build["Version"], "9.8.7")
        self.assertEqual(build["SigningKeyPath"], str(self.root / "key.fixture"))
        self.assertEqual(build["PublicKey"], "A" * 43)
        self.assertTrue(Path(build["LinuxLibrary"]).is_file())
        self.assertTrue(Path(build["SourceRoot"]).is_relative_to(self.root / "update"))
        native = Path(build["WindowsNativeExecutable"])
        self.assertEqual(native.name, "rustdesk.exe")
        self.assertTrue((native.parent / "sciter.dll").exists())
        self.assertTrue((native.parent / "dylib_virtual_display.dll").exists())
        self.assertEqual(list((self.root / "relaisdesk/downloads").iterdir()), [self.root / "relaisdesk/downloads/untouched.txt"])

    def test_audit_failure_blocks_build(self):
        self.stub_audit()
        result = self.invoke(self.build_args(), extra_env={"RELAISDESK_TEST_AUDIT_EXIT": "2"})
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "build-arguments.json").exists())

    def test_native_rebuild_uses_snapshot_and_exact_receipt(self):
        self.stub_audit()
        self.write("scripts/build-rustdesk-windows.ps1", ENGINE_STUB)
        result = self.invoke([a for a in self.build_args() if a != "-SkipEngineBuild"])
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        engine = json.loads((self.root / "engine-arguments.json").read_text(encoding="utf-8-sig"))
        build = json.loads((self.root / "build-arguments.json").read_text(encoding="utf-8-sig"))
        self.assertEqual(engine["ProjectRoot"], build["SourceRoot"])
        self.assertTrue(Path(engine["ProjectRoot"]).is_relative_to(self.root / "update"))
        self.assertTrue(engine["SkipLaunchers"])
        self.assertTrue(engine["SkipNativeDependencies"])
        self.assertEqual(build["WindowsPortable"], engine["DestinationPath"])

    def test_report_can_block_even_zero_exit_audit(self):
        self.stub_audit()
        result = self.invoke(self.build_args(), extra_env={"RELAISDESK_TEST_BLOCK": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "build-arguments.json").exists())

    def test_stale_engine_dependency_pin_blocks_build(self):
        self.stub_audit()
        self.manifest["rustdesk_cargo_lock_sha256"] = "0" * 64
        self.save_manifest()
        result = self.invoke(self.build_args())
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "build-arguments.json").exists())

    def test_tampered_engine_blocks_build(self):
        self.stub_audit()
        self.write("inputs/windows_portable", "tampered")
        result = self.invoke(self.build_args())
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "build-arguments.json").exists())

    def test_missing_dll_blocks_build(self):
        self.stub_audit()
        del self.manifest["artifacts"]["windows_sciter"]
        self.save_manifest()
        result = self.invoke(self.build_args())
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "build-arguments.json").exists())

    def test_build_failure_is_not_reported_as_ready(self):
        self.stub_audit()
        result = self.invoke(self.build_args(), extra_env={"RELAISDESK_TEST_BUILD_FAIL": "1"})
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(list((self.root / "update").glob("*/build-result.json")))

    def test_skipped_audit_is_recorded_not_hidden(self):
        self.stub_audit()
        result = self.invoke(self.build_args() + ["-SkipSecurityAudit"])
        self.assertEqual(result.returncode, 0, result.stderr)
        records = list((self.root / "update").glob("*/build-result.json"))
        self.assertEqual(len(records), 1)
        self.assertTrue(json.loads(records[0].read_text())["audit_skipped"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
