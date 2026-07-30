import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import app


class GrypeRunnerTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.inputs = self.root / "inputs"
        self.advisories = self.root / "advisories"
        (self.inputs / "review-snapshot").mkdir(parents=True)
        self.advisories.mkdir()
        self.environment = {
            "HAI_GRYPE_RUNNER_TOKEN": "runner-token-1234",
            "HAI_GRYPE_WORKSPACES": "review-snapshot",
            "HAI_GRYPE_INPUT_ROOT": str(self.inputs),
            "HAI_GRYPE_DB_ROOT": str(self.advisories),
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

    def test_scan_returns_aggregate_without_vulnerability_details(self):
        def fake_run(command, **_kwargs):
            if command[1] == "version":
                return subprocess.CompletedProcess(command, 0, stdout="Application: grype\nVersion: 0.116.0\n", stderr="")
            payload = {
                "matches": [
                    {"vulnerability": {"id": "CVE-2026-9999", "severity": "high", "fix": {"versions": ["2.0.0"]}}},
                    {"vulnerability": {"id": "CVE-2026-0001", "severity": "low", "fix": {"versions": []}}},
                ]
            }
            return subprocess.CompletedProcess(command, 0, stdout=json.dumps(payload).encode("utf-8"), stderr=b"")

        with patch.object(app.subprocess, "run", side_effect=fake_run):
            result = app.scan("review-snapshot")

        self.assertEqual(result["status"], "completed")
        self.assertEqual(result["engine"], "grype 0.116.0")
        self.assertEqual(result["vulnerabilityCount"], 2)
        self.assertEqual(result["fixAvailableCount"], 1)
        self.assertEqual(result["severities"], [{"severity": "high", "count": 1}, {"severity": "low", "count": 1}])
        serialized = json.dumps(result)
        self.assertNotIn("CVE-2026-9999", serialized)
        self.assertNotIn("2.0.0", serialized)
        self.assertEqual(len(result["resultDigest"]), 64)

    def test_workspace_must_be_a_configured_direct_child(self):
        with self.assertRaises(app.RequestError):
            app.workspace_directory("unapproved")

    def test_snapshot_entry_limit_is_bounded_before_scan(self):
        with patch.object(app, "MAX_SNAPSHOT_ENTRIES", 1):
            (self.inputs / "review-snapshot" / "first").touch()
            (self.inputs / "review-snapshot" / "second").touch()
            with self.assertRaises(app.RequestError):
                app.workspace_directory("review-snapshot")

    def test_version_parser_rejects_unexpected_output(self):
        with patch.object(app.subprocess, "run", return_value=subprocess.CompletedProcess(["grype"], 0, stdout="unexpected", stderr="")):
            with self.assertRaises(app.RequestError):
                app.runner_engine()


if __name__ == "__main__":
    unittest.main()
