"""Offline tests: python -B scripts/test_backup_google_drive.py."""
import argparse
import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
import urllib.error
from unittest.mock import MagicMock, patch

import backup_google_drive as backup


class GoogleDriveBackupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.directory = self.root / "archives"
        self.directory.mkdir(mode=0o700)
        self.config = self.root / "rclone.conf"
        self.config.write_text('[relaisdesk-drive]\ntype = drive\nscope = drive.file\n'
                               'client_id = test-only\nclient_secret = test-only\n'
                               'token = {"refresh_token":"test-only"}\n', encoding="utf-8")
        self.config.chmod(0o600)
        self.invoices = self.root / "invoices"
        self.invoices.mkdir(mode=0o700)
        (self.invoices / "2026-0001.pdf").write_bytes(b"%PDF-fake")
        (self.invoices / "2026-0001.html").write_bytes(b"<html>fake</html>")
        self.args = argparse.Namespace(directory=self.directory, database=self.root / "live.db",
                                       key=self.root / "backup.key", backup_binary="backup-tool",
                                       config=self.config, rclone="rclone", tar="tar",
                                       invoices_dir=self.invoices, heartbeat_url="",
                                       retention_days=30)
        self.latest = "relaisdesk-20260910T180000Z.db.aesgcm"
        self.invoices_latest = "relaisdesk-20260910T180000Z.invoices.aesgcm"
        self.old = self.directory / "relaisdesk-20260101T000000Z.db.aesgcm"
        self.old.write_bytes(b"RDBKAES2old encrypted fixture")
        os.utime(self.old, (time.time() - 90 * 86400,) * 2)
        self.remote = {}
        self.calls = []

    def fake_run(self, command, stage):
        self.calls.append(command)
        if "create" in command:
            output = Path(command[command.index("-out-dir") + 1])
            output.mkdir()
            (output / self.latest).write_bytes(b"RDBKAES2new encrypted fixture")
        elif "-czf" in command:
            Path(command[command.index("-czf") + 1]).write_bytes(b"fake-tarball-bytes")
        elif "-tzf" in command:
            self.assertTrue(Path(command[command.index("-tzf") + 1]).exists())
        elif "seal" in command:
            content = Path(command[command.index("-in") + 1]).read_bytes()
            Path(command[command.index("-out") + 1]).write_bytes(b"RDBKAES2" + content)
        elif "open" in command:
            content = Path(command[command.index("-file") + 1]).read_bytes()
            self.assertTrue(content.startswith(b"RDBKAES2"))
            Path(command[command.index("-out") + 1]).write_bytes(content[8:])
        elif "copy" in command:
            self.assertIn("--immutable", command)
            self.assertIn("--checksum", command)
            selected = Path(command[command.index("--files-from-raw") + 1]).read_text().splitlines()
            for name in selected:
                self.remote[name] = (self.directory / name).read_bytes()
        elif "copyto" in command:
            name = str(command[-2]).rsplit("/", 1)[-1]
            Path(command[-1]).write_bytes(self.remote[name])
        elif "verify" not in command:
            self.fail("Unexpected command")

    def perform(self, side_effect=None):
        with patch.object(backup, "run_checked", side_effect=side_effect or self.fake_run), \
                contextlib.redirect_stdout(io.StringIO()):
            backup.perform_backup(self.args)

    def test_valid_restricted_config(self):
        backup.check_config(self.config)

    def test_missing_auth_rejected(self):
        with self.assertRaisesRegex(ValueError, "Connexion Google absente"):
            backup.check_config(self.root / "missing.conf")

    def test_full_drive_permission_rejected(self):
        self.config.write_text(self.config.read_text().replace("drive.file", "drive"))
        with self.assertRaisesRegex(ValueError, "scope limité"):
            backup.check_config(self.config)

    def test_missing_refresh_token_rejected(self):
        self.config.write_text(self.config.read_text().replace("refresh_token", "access_token"))
        with self.assertRaisesRegex(ValueError, "renouvellement absent"):
            backup.check_config(self.config)

    def test_parse_error_does_not_leak_secret(self):
        self.config.write_text("sensitive-value-without-section")
        with self.assertRaises(ValueError) as caught:
            backup.check_config(self.config)
        self.assertNotIn("sensitive-value", str(caught.exception))

    def test_only_encrypted_files_uploaded_and_success_verified(self):
        for name in ("live.db", "backup.key", ".hidden.tmp", "api.env", "notes.txt"):
            (self.directory / name).write_text("must not leave server")
        self.perform()
        self.assertEqual(set(self.remote), {self.old.name, self.latest, self.invoices_latest})
        self.assertFalse(self.old.exists())
        self.assertTrue((self.directory / "backup.key").exists())
        marker = json.loads((self.directory / "google-drive-last-success.json").read_text())
        self.assertEqual(marker["archive"], self.latest)
        self.assertEqual(len(marker["sha256"]), 64)
        self.assertEqual(marker["invoices_archive"], self.invoices_latest)
        self.assertEqual(len(marker["invoices_sha256"]), 64)
        self.assertTrue(any("verify" in call for call in self.calls))
        self.assertTrue(any("seal" in call for call in self.calls))
        self.assertTrue(any("open" in call for call in self.calls))
        self.assertTrue(any("-tzf" in call for call in self.calls))
        self.assertFalse(any(p.name.startswith(".drive-") for p in self.directory.iterdir()))

    def test_upload_failure_keeps_local_backups_and_no_success(self):
        def fail(command, stage):
            if "copy" in command:
                raise RuntimeError("upload failed")
            self.fake_run(command, stage)
        with self.assertRaises(RuntimeError):
            self.perform(fail)
        self.assertTrue(self.old.exists())
        self.assertTrue((self.directory / self.latest).exists())
        self.assertFalse((self.directory / "google-drive-last-success.json").exists())

    def test_changed_download_does_not_purge_or_claim_success(self):
        def corrupt(command, stage):
            self.fake_run(command, stage)
            if "copyto" in command:
                Path(command[-1]).write_bytes(b"corruption")
        with self.assertRaisesRegex(RuntimeError, "diffère"):
            self.perform(corrupt)
        self.assertTrue(self.old.exists())
        self.assertFalse((self.directory / "google-drive-last-success.json").exists())

    def test_decrypt_failure_does_not_purge(self):
        def fail(command, stage):
            if "verify" in command:
                raise RuntimeError("verify failed")
            self.fake_run(command, stage)
        with self.assertRaises(RuntimeError):
            self.perform(fail)
        self.assertTrue(self.old.exists())

    def test_plaintext_named_like_archive_is_refused(self):
        self.old.write_bytes(b"SQLite format 3\x00")
        with self.assertRaisesRegex(ValueError, "non chiffrée"):
            backup.archives(self.directory)

    def test_collision_never_overwrites_previous_backup(self):
        target = self.directory / self.latest
        target.write_bytes(b"RDBKAES2existing")
        with self.assertRaises(FileExistsError):
            self.perform()
        self.assertEqual(target.read_bytes(), b"RDBKAES2existing")
        self.assertEqual(self.remote, {})

    def test_symlink_archive_is_refused(self):
        link = self.directory / "relaisdesk-20260909T010000Z.db.aesgcm"
        try:
            link.symlink_to(self.old)
        except OSError:
            self.skipTest("Symlink privileges unavailable")
        with self.assertRaisesRegex(ValueError, "non régulière"):
            backup.archives(self.directory)

    def test_changed_invoices_download_does_not_purge_or_claim_success(self):
        def corrupt(command, stage):
            self.fake_run(command, stage)
            if "copyto" in command and str(command[-1]).endswith("downloaded.invoices.aesgcm"):
                Path(command[-1]).write_bytes(b"corruption")
        with self.assertRaisesRegex(RuntimeError, "diffère"):
            self.perform(corrupt)
        self.assertTrue(self.old.exists())
        self.assertFalse((self.directory / "google-drive-last-success.json").exists())

    def test_heartbeat_sent_after_verified_backup(self):
        self.args.heartbeat_url = "https://uptime.example.com/api/v1/heartbeat/secret"
        with patch.object(backup, "ping_heartbeat") as ping:
            self.perform()
        ping.assert_called_once_with("https://uptime.example.com/api/v1/heartbeat/secret")

    def test_heartbeat_disabled_by_default(self):
        with patch.object(backup, "ping_heartbeat") as ping:
            self.perform()
        ping.assert_not_called()

    def test_heartbeat_failure_blocks_success_marker(self):
        self.args.heartbeat_url = "https://uptime.example.com/api/v1/heartbeat/secret"
        with patch.object(backup, "ping_heartbeat", side_effect=RuntimeError("no ping")):
            with self.assertRaises(RuntimeError):
                self.perform()
        self.assertFalse((self.directory / "google-drive-last-success.json").exists())
        self.assertTrue((self.directory / self.latest).exists())
        self.assertTrue((self.directory / self.invoices_latest).exists())

    def test_heartbeat_url_validation(self):
        self.assertEqual(backup.check_heartbeat_url(""), "")
        self.assertEqual(backup.check_heartbeat_url(None), "")
        self.assertEqual(backup.check_heartbeat_url("https://uptime.example.com/ping"),
                         "https://uptime.example.com/ping")
        with self.assertRaisesRegex(ValueError, "https"):
            backup.check_heartbeat_url("http://uptime.example.com/ping")
        with self.assertRaisesRegex(ValueError, "https"):
            backup.check_heartbeat_url("not-a-url")

    def test_heartbeat_success_accepts_2xx(self):
        response = MagicMock()
        response.status = 200
        context = MagicMock()
        context.__enter__.return_value = response
        context.__exit__.return_value = False
        with patch.object(backup.urllib.request, "urlopen", return_value=context) as opened:
            backup.ping_heartbeat("https://uptime.example.com/ping")
        self.assertEqual(opened.call_args.kwargs.get("timeout"), 30)

    def test_heartbeat_bad_status_rejected(self):
        response = MagicMock()
        response.status = 500
        context = MagicMock()
        context.__enter__.return_value = response
        context.__exit__.return_value = False
        with patch.object(backup.urllib.request, "urlopen", return_value=context):
            with self.assertRaisesRegex(RuntimeError, "inattendue"):
                backup.ping_heartbeat("https://uptime.example.com/ping")

    def test_heartbeat_network_error_hides_url(self):
        secret = "https://uptime.example.com/api/v1/heartbeat/s3cr3t"
        with patch.object(backup.urllib.request, "urlopen",
                          side_effect=urllib.error.URLError("down")):
            with self.assertRaises(RuntimeError) as caught:
                backup.ping_heartbeat(secret)
        self.assertNotIn("s3cr3t", str(caught.exception))

    def _backup_fixtures(self):
        live = self.root / "live.db"
        live.write_bytes(b"SQLite format 3\x00" + b"x" * 100)
        key = self.root / "backup.key"
        key.write_bytes(b"k" * 32)
        for name in ("backup-tool", "rclone", "tar"):
            (self.root / name).write_bytes(b"#!/bin/sh\n")
        self.args.database = live
        self.args.key = key
        self.args.backup_binary = self.root / "backup-tool"
        self.args.rclone = self.root / "rclone"
        self.args.tar = self.root / "tar"

    def test_backup_rejects_missing_invoices_dir(self):
        self._backup_fixtures()
        self.args.invoices_dir = self.root / "no-such-dir"
        with self.assertRaisesRegex(ValueError, "factures"):
            backup.backup(self.args)

    def test_backup_rejects_bad_heartbeat_url(self):
        self._backup_fixtures()
        self.args.heartbeat_url = "http://plain.example.com/ping"
        with self.assertRaisesRegex(ValueError, "https"):
            backup.backup(self.args)

    def test_rclone_environment_and_errors_do_not_expose_credentials(self):
        with patch.dict(os.environ, {"RCLONE_DUMP": "auth", "RCLONE_CONFIG_PASS": "secret"}), \
                patch.object(subprocess, "run", return_value=subprocess.CompletedProcess([], 9)) as run:
            with self.assertRaisesRegex(RuntimeError, "code 9"):
                backup.run_checked(["rclone", "copy"], "upload")
            self.assertFalse(any(k.startswith("RCLONE_") for k in run.call_args.kwargs["env"]))
            self.assertEqual(run.call_args.kwargs["stderr"], subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
