"""Offline safety/regression tests: python -B scripts/test_security_maintenance.py."""
import argparse
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import security_maintenance as sm


class MaintenanceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="relaisdesk-maintenance-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.write("database/go.mod", "module example.test/database\n\ngo 1.26.6\n")
        self.write("rustdesk/Cargo.toml", '[package]\nname = "fixture"\nversion = "1.0.0"\n')
        self.args = argparse.Namespace(project_root=str(self.root), audit_only=False,
                                       prepare_only=False, run_build_tests=False,
                                       install_audit_tools=False, timeout=30)
        # No network or discovery of real audit tools is necessary for these tests.
        self.discovery = patch.object(sm, "command_prefix", return_value=None)
        self.discovery.start()
        self.addCleanup(self.discovery.stop)

    def write(self, relative, content):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content if isinstance(content, bytes) else content.encode())
        return path

    def maintenance(self):
        runner = sm.Maintenance(self.args)
        runner.inventory, _ = sm.snapshot(self.root, runner.project)
        runner.snapshot_complete = True
        return runner

    def quiet(self):
        return contextlib.redirect_stdout(io.StringIO())

    def test_snapshot_excludes_credentials_private_data_and_binary_outputs(self):
        excluded = ["api/.env", "api/.env.production", "api/production.env", "api/a.ppk",
                    "api/a.pem", "api/a.key", "api/a.db", "api/a.db-wal", "api/.npmrc",
                    "rustdesk/.git/config", "api/data/x.txt", "api/api", "api/api.exe",
                    "installer/build/launcher.exe", "installer/build/nested/x.txt"]
        for name in excluded:
            self.write(name, b"MZdummy" if name == "api/api" else "excluded")
        # Construct the marker to avoid a literal private-key marker in this test source.
        excluded.append("api/not-a-key.txt")
        self.write(excluded[-1], "-----BEGIN " + "PRIVATE KEY-----\ndummy")
        self.write("api/main.go", "package main\n")
        self.write("installer/build/license.txt", "legal notice")
        self.write("installer/viewer/embedded/rustdesk.exe", b"MZembedded")
        self.write("update/old/projet/api/main.go", "old output")
        self.write("oracle private.ppk", "root private key")
        self.write(".secrets/oracle private.ppk", "private key after cleanup")
        self.write("archives/livraisons/old/api/main.go", "archived source")
        runner = self.maintenance()
        for name in excluded:
            self.assertFalse((runner.project / name).exists(), name)
        self.assertTrue((runner.project / "api/main.go").is_file())
        self.assertTrue((runner.project / "installer/build/license.txt").is_file())
        self.assertTrue((runner.project / "installer/viewer/embedded/rustdesk.exe").is_file())
        self.assertFalse((runner.project / "update").exists())
        self.assertFalse((runner.project / "oracle private.ppk").exists())
        self.assertFalse((runner.project / ".secrets").exists())
        self.assertFalse((runner.project / "archives").exists())

    def test_update_must_be_a_real_directory(self):
        self.write("update", "not a directory")
        with self.assertRaises(ValueError):
            sm.Maintenance(self.args)

    def test_symlink_root_is_rejected(self):
        elsewhere = self.root / "elsewhere"
        elsewhere.mkdir()
        try:
            (self.root / "update").symlink_to(elsewhere, target_is_directory=True)
        except OSError:
            self.skipTest("Creating symlinks is not permitted on this Windows account")
        with self.assertRaises(ValueError):
            sm.Maintenance(self.args)

    def test_each_run_is_unique_and_keeps_old_results(self):
        first, second = self.maintenance(), self.maintenance()
        self.assertNotEqual(first.run, second.run)
        self.assertTrue(first.run.is_dir())
        self.assertTrue(second.run.is_dir())

    def test_explicit_run_id_is_exact_and_cannot_be_reused(self):
        self.args.run_id = "20260919-120000-1234abcd"
        runner = self.maintenance()
        self.assertEqual(runner.run.name, self.args.run_id)
        with self.assertRaises(FileExistsError):
            sm.Maintenance(self.args)
        self.args.run_id = "../escape"
        with self.assertRaises(ValueError):
            sm.Maintenance(self.args)

    def test_native_fleet_payload_is_copied_for_tests_but_not_exported(self):
        for name in ("rustdesk.exe", "sciter.dll", "dylib_virtual_display.dll"):
            self.write("installer/viewer/embedded/fleet/" + name, b"MZtest-input")
        runner = self.maintenance()
        for name in ("rustdesk.exe", "sciter.dll", "dylib_virtual_display.dll"):
            self.assertTrue((runner.project / "installer/viewer/embedded/fleet" / name).is_file())
        runner.export()
        self.assertFalse((runner.run / "fichiers-corriges/installer/viewer/embedded/fleet").exists())

    def test_no_build_tests_cannot_authorize_application(self):
        runner = self.maintenance()
        with self.quiet():
            self.assertEqual(runner.finish(), 0)
        self.assertTrue(runner.report["do_not_apply_batch"])

    def test_only_whitelisted_changes_are_exported(self):
        self.write("api/main.go", "package main\n")
        runner = self.maintenance()
        before = (self.root / "database/go.mod").read_bytes()
        (runner.project / "database/go.mod").write_text("module upgraded\n")
        (runner.project / "api/main.go").write_text("unexpected edit\n")
        runner.export()
        self.assertEqual((self.root / "database/go.mod").read_bytes(), before)
        self.assertEqual([c["path"] for c in runner.report["changed_files"]], ["database/go.mod"])
        self.assertEqual(runner.report["unexpected_source_edits"], ["api/main.go"])
        self.assertIn("module upgraded", (runner.run / "modifications.patch").read_text())
        self.assertTrue(runner.report["errors"])

    def test_changed_original_is_not_overwritten_or_exported(self):
        runner = self.maintenance()
        self.write("database/go.mod", "user edit\n")
        (runner.project / "database/go.mod").write_text("candidate\n")
        runner.export()
        self.assertEqual((self.root / "database/go.mod").read_text(), "user edit\n")
        self.assertFalse(runner.report["changed_files"])
        self.assertEqual(runner.report["original_source_changed_during_run"], ["database/go.mod"])

    def test_new_go_sum_does_not_overwrite_an_external_edit(self):
        runner = self.maintenance()
        (runner.project / "database/go.sum").write_text("candidate\n")
        self.write("database/go.sum", "user edit\n")
        runner.export()
        self.assertFalse((runner.run / "fichiers-corriges/database/go.sum").exists())
        self.assertIn("database/go.sum", runner.report["original_source_changed_during_run"])

    def test_transaction_rolls_back_failure_and_preserves_rejected_candidate(self):
        runner = self.maintenance()
        manifest, new_sum = runner.project / "database/go.mod", runner.project / "database/go.sum"
        old = manifest.read_bytes()
        def fail():
            manifest.write_text("broken\n")
            new_sum.write_text("temporary\n")
            raise ValueError("resolver failure")
        with self.assertRaisesRegex(ValueError, "resolver failure"):
            runner.transactional("test", [manifest, new_sum], fail)
        self.assertEqual(manifest.read_bytes(), old)
        self.assertFalse(new_sum.exists())
        self.assertEqual(len(list((runner.run / "tentatives-rejetees").glob("*/go.mod"))), 1)

    def test_transaction_refuses_path_outside_snapshot(self):
        runner = self.maintenance()
        with self.assertRaises(ValueError):
            runner.transactional("bad", [self.root / "database/go.mod"], lambda: None)

    def test_missing_tool_yields_unknown_not_clean(self):
        runner = self.maintenance()
        with self.quiet():
            runner.go_module("database")
            code = runner.finish()
        self.assertEqual(code, 2)
        result = runner.report["components"][0]
        self.assertFalse(result["after"]["ok"])
        self.assertIn("ECHEC", result["update"])
        self.assertTrue(runner.report["do_not_apply_batch"])

    def test_audit_only_never_calls_updater(self):
        self.args.audit_only = True
        runner = self.maintenance()
        with patch.object(runner, "checked") as checked:
            runner.component("fixture", lambda: [], lambda _: self.fail("updater must not run"))
        checked.assert_not_called()
        self.assertTrue(runner.report["components"][0]["after"]["ok"])
        self.assertFalse(runner.report["changed_files"] if "changed_files" in runner.report else [])

    def test_prepare_only_never_runs_tools_or_network(self):
        self.args.prepare_only = True
        runner = sm.Maintenance(self.args)
        with patch.object(runner, "call", side_effect=AssertionError("unexpected process")), \
             patch.object(runner, "http_json", side_effect=AssertionError("unexpected network")), self.quiet():
            self.assertEqual(runner.execute(), 0)
        self.assertEqual(runner.report["status"], "PREPARATION_SANS_AUDIT")
        self.assertFalse(runner.report["changed_files"])

    def test_copy_error_is_not_a_successful_preparation(self):
        self.args.prepare_only = True
        runner = sm.Maintenance(self.args)
        with patch.object(sm, "snapshot", side_effect=OSError("copy denied")), self.quiet():
            self.assertEqual(runner.execute(), 2)
        self.assertIn("copy denied", (runner.run / "rapport.md").read_text(encoding="utf-8"))

    def test_interrupted_audit_still_produces_report(self):
        runner = self.maintenance()
        def interrupt():
            raise KeyboardInterrupt()
        with self.assertRaises(KeyboardInterrupt):
            runner.component("interrupted", interrupt, lambda _: "unused")
        runner.report["errors"].append("interrupted")
        with self.quiet():
            self.assertEqual(runner.finish(), 2)
        self.assertTrue((runner.run / "rapport.json").is_file())

    def test_failed_validator_blocks_applying_batch(self):
        runner = self.maintenance()
        def fail():
            raise RuntimeError("build failed")
        runner.component("fixture", lambda: [], lambda _: "candidate", fail)
        with self.quiet():
            self.assertEqual(runner.finish(), 2)
        self.assertTrue(runner.report["do_not_apply_batch"])

    def test_disappeared_alerts_are_computed_by_identifier_and_package(self):
        runner = self.maintenance()
        before = [sm.finding("CVE-test", "example", "1.0")]
        with patch.object(runner, "audit", side_effect=[{"ok": True, "findings": before}, {"ok": True, "findings": []}]):
            runner.component("fixture", lambda: None, lambda _: "updated")
        self.assertEqual(runner.report["components"][0]["no_longer_reported"], [("CVE-test", "example")])

    def test_osv_pagination_is_never_silently_truncated(self):
        runner = self.maintenance()
        with patch.object(runner, "http_json", return_value={"results": [{"next_page_token": "more"}]}):
            with self.assertRaisesRegex(ValueError, "pagine"):
                runner.osv("Pub", [("package", "1.0.0")])

    def test_osv_failed_or_incomplete_response_is_unknown(self):
        runner = self.maintenance()
        for response in ({}, {"results": []}, {"results": [{"vulns": "invalid"}]}):
            with self.subTest(response=response), patch.object(runner, "http_json", return_value=response):
                report = runner.audit("test", "before", lambda: runner.osv("Pub", [("package", "1.0")]))
                self.assertFalse(report["ok"])

    def test_osv_preserves_identifiers_and_fix_versions(self):
        runner = self.maintenance()
        responses = [{"results": [{"vulns": [{"id": "GHSA-test"}]}]},
                     {"affected": [{"package": {"name": "brotli"}, "ranges": [{"events": [{"fixed": "1.2.0"}]}]}]}]
        with patch.object(runner, "http_json", side_effect=responses):
            findings = runner.osv("PyPI", [("brotli", "1.0.0")])
        self.assertEqual(findings[0]["id"], "GHSA-test")
        self.assertEqual(findings[0]["fixed_versions"], ["1.2.0"])

    def test_child_environment_omits_common_secrets_and_injection_options(self):
        with patch.dict(os.environ, {"ADMIN_TOKEN": "dummy", "SMTP_PASS": "dummy", "NODE_OPTIONS": "dummy", "PYTHONPATH": "dummy", "GIT_SSH_COMMAND": "dummy"}):
            runner = self.maintenance()
        for key in ("ADMIN_TOKEN", "SMTP_PASS", "NODE_OPTIONS", "PYTHONPATH", "GIT_SSH_COMMAND"):
            self.assertNotIn(key, runner.environment)
        self.assertEqual(runner.environment["npm_config_ignore_scripts"], "true")

    def test_command_runner_never_uses_shell_interpolation(self):
        runner = self.maintenance()
        argument = "literal;&$() spaces"
        with self.quiet():
            code, output = runner.call("literal", [sys.executable, "-c", "import sys; print(sys.argv[1])", argument], runner.project)
        self.assertEqual(code, 0)
        self.assertEqual(output.strip(), argument)

    def test_command_timeout_is_reported_and_process_is_stopped(self):
        self.args.timeout = 0.2
        runner = self.maintenance()
        with self.quiet():
            code, _ = runner.call("timeout", [sys.executable, "-c", "import time; time.sleep(30)"], runner.project)
        self.assertEqual(code, -124)
        self.assertTrue(runner.report["commands"][0]["timeout"])

    def test_rust_git_revision_change_is_rolled_back(self):
        initial = 'version = 4\n[[package]]\nname = "vulnerable"\nversion = "1.0.0"\nsource = "registry+https://registry.test"\n[[package]]\nname = "fork"\nversion = "1.0.0"\nsource = "git+https://example.test/repo#initial"\n'
        self.write("rustdesk/Cargo.lock", initial)
        runner = self.maintenance()
        runner.tools["cargo"] = ["fake-cargo"]
        runner.tools["cargo-audit"] = ["fake-audit"]
        audit = {"vulnerabilities": {"list": [{"advisory": {"id": "RUSTSEC-test"}, "package": {"name": "vulnerable", "version": "1.0.0"}}]}, "warnings": {}}
        def call(label, command, cwd, extra_env=None):
            if "-cargo-update-" in label:
                (runner.project / "rustdesk/Cargo.lock").write_text(initial.replace("#initial", "#different"))
            return (1, json.dumps(audit)) if "cargo-audit" in label else (0, "")
        with patch.object(runner, "call", side_effect=call):
            runner.rust_module("rustdesk")
        self.assertEqual((runner.project / "rustdesk/Cargo.lock").read_text(), initial)
        self.assertIn("revision Git", runner.report["components"][0]["update"])

    def test_npm_changed_manifest_is_rejected_without_force_or_install_scripts(self):
        self.write("rustdesk-server/ui/html/package.json", '{"name":"fixture"}\n')
        self.write("rustdesk-server/ui/html/package-lock.json", '{"lockfileVersion":3}\n')
        runner = self.maintenance()
        runner.tools["npm"] = ["fake-node", "npm-cli.js"]
        def call(label, command, cwd, extra_env=None):
            self.assertIn("--ignore-scripts", command)
            self.assertNotIn("--force", command)
            if "fix" in command:
                (cwd / "package.json").write_text('{"changed":true}')
                return 0, '{}'
            return 0, '{"vulnerabilities":{}}'
        with patch.object(runner, "call", side_effect=call):
            runner.npm_module()
        self.assertIn("ECHEC", runner.report["components"][0]["update"])
        self.assertEqual((runner.project / "rustdesk-server/ui/html/package.json").read_text(), '{"name":"fixture"}\n')

    def test_go_external_replace_is_rejected_before_audit(self):
        runner = self.maintenance()
        runner.tools["go"] = ["fake-go"]
        runner.tools["govulncheck"] = ["fake-govulncheck"]
        outside = self.root / "outside"
        module = {"Replace": [{"Old": {"Path": "local"}, "New": {"Path": str(outside)}}]}
        with patch.object(runner, "call", return_value=(0, json.dumps(module))) as call:
            runner.go_module("database")
        self.assertTrue(all("govulncheck" not in args.args[0] for args in call.call_args_list))
        self.assertFalse(runner.report["components"][0]["after"]["ok"])


class ParserTests(unittest.TestCase):
    def test_json_stream_rejects_empty_truncated_and_non_objects(self):
        for text in ("", "[]", '{}\n{"truncated":', "not JSON"):
            with self.subTest(text=text), self.assertRaises(ValueError):
                sm.json_stream(text)
        self.assertEqual(sm.json_stream('{}\n{"config":{}}'), [{}, {"config": {}}])

    def test_go_findings_deduplicate_and_do_not_claim_exploitability(self):
        entry = {"finding": {"osv": "GO-test", "fixed_version": "v1.2.0", "trace": [{"module": "example.test/pkg", "version": "v1.0.0"}]}}
        result = sm.go_findings([{"config": {}}, entry, entry])
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["kind"], "go-module")
        with self.assertRaises(ValueError):
            sm.go_findings([entry])
        with self.assertRaises(ValueError):
            sm.go_findings([{"config": {}}, {"error": "partial"}])

    def test_go_symbol_scan_preserves_strongest_evidence_not_module_order(self):
        module = {"finding": {"osv": "GO-test", "trace": [{"module": "pkg", "version": "v1"}]}}
        symbol = {"finding": {"osv": "GO-test", "trace": [{"module": "pkg", "version": "v1", "package": "pkg/sub", "function": "Function"}]}}
        result = sm.go_findings([{"config": {}}, symbol, module])
        self.assertEqual(result[0]["kind"], "go-symbol")

    def test_rust_preserves_maintenance_warnings(self):
        result = sm.rust_findings({"vulnerabilities": {"list": []}, "warnings": {"unmaintained": [{"advisory": {"id": "RUSTSEC-test"}, "package": {"name": "gtk", "version": "0.1"}}]}})
        self.assertEqual(result[0]["kind"], "unmaintained")
        with self.assertRaises(ValueError):
            sm.rust_findings({"error": "offline"})

    def test_rust_yanked_entries_allow_null_advisory_and_versions(self):
        report = {"vulnerabilities": {"list": []}, "warnings": {"yanked": [{"advisory": None,
                  "versions": None, "package": {"name": "spin", "version": "0.9.8"}}]}}
        result = sm.rust_findings(report)
        self.assertEqual(result[0]["id"], "YANKED-spin-0.9.8")
        self.assertEqual(result[0]["fixed_versions"], [])

    def test_npm_never_suppresses_incomplete_graphs(self):
        for report in ({"error": "network"}, {"vulnerabilities": {"example": {"via": ["another-package"]}}}):
            with self.subTest(report=report), self.assertRaises(ValueError):
                sm.npm_findings(report)
        self.assertEqual(sm.npm_findings({"vulnerabilities": {}}), [])

    def test_real_dart_lock_has_hosted_and_git_dependencies(self):
        path = Path(__file__).resolve().parents[1] / "rustdesk/flutter/pubspec.lock"
        packages, git = sm.pub_packages(path)
        self.assertGreater(len(packages), 50)
        self.assertGreater(len(git), 0)


if __name__ == "__main__":
    unittest.main(verbosity=2)
