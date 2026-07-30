import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import app


class SyftRunnerTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        (self.root / "review-snapshot").mkdir()
        self.environment = {
            "HAI_SYFT_RUNNER_TOKEN": "runner-token-1234",
            "HAI_SYFT_WORKSPACES": "review-snapshot",
            "HAI_SYFT_INPUT_ROOT": str(self.root),
        }
        self.previous = {key: os.environ.get(key) for key in self.environment}
        os.environ.update(self.environment)

    def tearDown(self):
        for key, value in self.previous.items():
            if value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = value
        self.tempdir.cleanup()

    def test_inventory_returns_aggregate_without_package_metadata(self):
        def fake_run(command, **_kwargs):
            if command[1] == "version":
                return subprocess.CompletedProcess(command, 0, stdout="Application: syft\nVersion: 1.48.0\n", stderr="")
            payload = {"artifacts": [{"type": "npm", "name": "private-package", "version": "1.2.3"}, {"type": "go-module", "name": "private.module", "licenses": ["Apache-2.0"]}]}
            return subprocess.CompletedProcess(command, 0, stdout=json.dumps(payload).encode("utf-8"), stderr=b"")

        with patch.object(app.subprocess, "run", side_effect=fake_run):
            result = app.inventory("review-snapshot")

        self.assertEqual(result["status"], "completed")
        self.assertEqual(result["engine"], "syft 1.48.0")
        self.assertEqual(result["packageCount"], 2)
        self.assertEqual(result["ecosystems"], [{"id": "go-module", "count": 1}, {"id": "npm", "count": 1}])
        self.assertNotIn("private-package", json.dumps(result))
        self.assertNotIn("1.2.3", json.dumps(result))
        self.assertEqual(len(result["resultDigest"]), 64)

    def test_workspace_must_be_a_configured_direct_child(self):
        with self.assertRaises(app.RequestError):
            app.workspace_directory("unapproved")

    def test_snapshot_entry_limit_is_bounded_before_inventory(self):
        with patch.object(app, "MAX_SNAPSHOT_ENTRIES", 1):
            (self.root / "review-snapshot" / "first").touch()
            (self.root / "review-snapshot" / "second").touch()
            with self.assertRaises(app.RequestError):
                app.workspace_directory("review-snapshot")

    def test_version_parser_rejects_unexpected_output(self):
        with patch.object(app.subprocess, "run", return_value=subprocess.CompletedProcess(["syft"], 0, stdout="unexpected", stderr="")):
            with self.assertRaises(app.RequestError):
                app.runner_engine()


if __name__ == "__main__":
    unittest.main()
