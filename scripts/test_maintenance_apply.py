"""Offline integration tests for guarded dependency application and key policy."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import maintenance_apply as ma


class ApplyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="relaisdesk-apply-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.run_id = "20260919-120000-1234abcd"
        self.run = self.root / "update" / self.run_id
        self.run.mkdir(parents=True)
        self.change = {"path": "database/go.mod", "before_sha256": ma.sha(b"old"), "after_sha256": ma.sha(b"new")}
        self.report = {"schema_version": 1, "mode": "update", "do_not_apply_batch": False,
                       "status": "CONTROLES_AUTOMATIQUES_TERMINES_RECETTE_REQUISE", "run_build_tests": True,
                       "source": str(self.root), "output": str(self.run), "errors": [],
                       "components": [{"after": {"ok": True, "findings": []}, "validation": "OK", "update": "OK"}],
                       "changed_files": [self.change]}
        self.write("database/go.mod", b"old")
        self.write("update/" + self.run_id + "/fichiers-corriges/database/go.mod", b"new")
        self.save_report()

    def write(self, name, data):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return path

    def save_report(self):
        (self.run / "rapport.json").write_text(json.dumps(self.report), encoding="utf-8")

    def run_apply(self, **kwargs):
        with contextlib.redirect_stdout(io.StringIO()):
            ma.execute(self.root, self.run_id, **kwargs)

    def test_apply_and_rollback_preserve_exact_bytes(self):
        self.run_apply()
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"new")
        self.assertEqual((self.run / "application/before/database/go.mod").read_bytes(), b"old")
        self.run_apply(rollback=True)
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"old")

    def test_new_go_sum_is_removed_only_by_explicit_rollback(self):
        self.report["changed_files"].append({"path": "database/go.sum", "before_sha256": None, "after_sha256": ma.sha(b"sum")})
        self.write("update/" + self.run_id + "/fichiers-corriges/database/go.sum", b"sum")
        self.save_report()
        self.run_apply()
        self.assertEqual((self.root / "database/go.sum").read_bytes(), b"sum")
        self.run_apply(rollback=True)
        self.assertFalse((self.root / "database/go.sum").exists())

    def test_dry_run_creates_no_lock_backup_or_source_change(self):
        before = sorted(str(p) for p in self.root.rglob("*"))
        self.run_apply(dry_run=True)
        self.assertEqual(sorted(str(p) for p in self.root.rglob("*")), before)
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"old")

    def test_dry_run_rollback_keeps_applied_state(self):
        self.run_apply()
        receipt = (self.run / "application/receipt.json").read_bytes()
        self.run_apply(rollback=True, dry_run=True)
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"new")
        self.assertEqual((self.run / "application/receipt.json").read_bytes(), receipt)

    def test_invalid_or_unvalidated_report_never_applies(self):
        for field, value in (("do_not_apply_batch", True), ("mode", "prepare"), ("mode", "audit"),
                             ("run_build_tests", False), ("source", str(self.root / "other")),
                             ("components", []), ("errors", ["failed"]), ("schema_version", 2)):
            with self.subTest(field=field, value=value):
                original = self.report[field]
                self.report[field] = value
                self.save_report()
                with self.assertRaises(ValueError):
                    self.run_apply()
                self.report[field] = original
                self.assertEqual((self.root / "database/go.mod").read_bytes(), b"old")

    def test_alerts_and_failed_validators_block_batch(self):
        for component in ({"after": {"ok": False, "findings": []}},
                          {"after": {"ok": True, "findings": ["warning"]}},
                          {"after": {"ok": True, "findings": []}, "validation": "ECHEC"}):
            self.report["components"] = [component]
            self.save_report()
            with self.assertRaises(ValueError):
                self.run_apply()

    def test_source_edit_is_never_overwritten(self):
        self.write("database/go.mod", b"user edit")
        with self.assertRaisesRegex(ValueError, "Source modifiee"):
            self.run_apply()
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"user edit")

    def test_candidate_tampering_is_rejected(self):
        (self.run / "fichiers-corriges/database/go.mod").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "altere"):
            self.run_apply()

    def test_files_not_in_report_are_not_copied(self):
        self.write("update/" + self.run_id + "/fichiers-corriges/api/main.go", b"bad")
        self.write("update/" + self.run_id + "/fichiers-corriges/LIRE_AVANT_APPLICATION.txt", b"notice")
        self.run_apply()
        self.assertFalse((self.root / "api/main.go").exists())
        self.assertFalse((self.root / "LIRE_AVANT_APPLICATION.txt").exists())

    def test_unlisted_paths_duplicates_and_invalid_hashes_are_rejected(self):
        for change in (dict(self.change, path="api/main.go"), dict(self.change, path="../outside"),
                       dict(self.change, after_sha256="invalid"), dict(self.change, before_sha256=None)):
            with self.assertRaises(ValueError):
                ma.checked_changes([change])
        with self.assertRaises(ValueError):
            ma.checked_changes([self.change, self.change])

    def test_newer_directory_is_not_selected(self):
        (self.root / "update/20990101-120000-1234abcd").mkdir()
        self.run_apply()
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"new")

    def test_lock_prevents_concurrent_application(self):
        self.write("update/.apply.lock", b"other process")
        with self.assertRaisesRegex(ValueError, "active ou interrompue"):
            self.run_apply()
        self.assertEqual((self.root / "update/.apply.lock").read_bytes(), b"other process")

    def test_rollback_refuses_later_user_edits(self):
        self.run_apply()
        self.write("database/go.mod", b"later edit")
        with self.assertRaises(ValueError):
            self.run_apply(rollback=True)
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"later edit")

    def test_rollback_rejects_modified_backup(self):
        self.run_apply()
        (self.run / "application/before/database/go.mod").write_bytes(b"bad backup")
        with self.assertRaises(ValueError):
            self.run_apply(rollback=True)

    def test_second_file_failure_restores_first_file(self):
        self.write("database/go.sum", b"old sum")
        self.write("update/" + self.run_id + "/fichiers-corriges/database/go.sum", b"new sum")
        self.report["changed_files"].append({"path": "database/go.sum", "before_sha256": ma.sha(b"old sum"), "after_sha256": ma.sha(b"new sum")})
        self.save_report()
        real_write = ma.atomic_write
        def failing_write(path, data):
            if path == self.root / "database/go.sum":
                raise OSError("simulated disk error")
            return real_write(path, data)
        with patch.object(ma, "atomic_write", side_effect=failing_write), self.assertRaises(OSError):
            self.run_apply()
        self.assertEqual((self.root / "database/go.mod").read_bytes(), b"old")
        self.assertEqual((self.root / "database/go.sum").read_bytes(), b"old sum")
        self.assertEqual(ma.read_json(self.run / "application/receipt.json")["state"], "failed_restored")

    def test_link_or_junction_is_rejected(self):
        real_linked = ma.linked
        with patch.object(ma, "linked", side_effect=lambda p: p == self.root / "database" or real_linked(p)):
            with self.assertRaisesRegex(ValueError, "Lien/jonction"):
                self.run_apply()

    def test_path_escape_and_windows_stream_are_rejected(self):
        for name in ("../x", "/x", "database/../x", "database\\go.mod", "database/go.mod:stream", "database/./go.mod"):
            with self.assertRaises(ValueError):
                ma.safe_path(self.root, name)

    def test_repeat_application_is_rejected(self):
        self.run_apply()
        with self.assertRaises(ValueError):
            self.run_apply()


class SecretPolicyTests(unittest.TestCase):
    def test_regeneration_is_blocked_before_any_access(self):
        spec = importlib.util.spec_from_file_location("manage_secrets", Path(__file__).with_name("manage-secrets.py"))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        for target in (*module.SUPPORTED_TARGETS, "all"):
            with patch.object(Path, "read_text", side_effect=AssertionError("secret read")), \
                 patch.object(Path, "write_text", side_effect=AssertionError("secret write")), \
                 self.assertRaisesRegex(ValueError, "desactivee"):
                module.main(["--action", "regenerate", "--target", target])


if __name__ == "__main__":
    unittest.main(verbosity=2)
