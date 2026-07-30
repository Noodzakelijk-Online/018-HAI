import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import app


class TrivyRunnerTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.inputs = self.root / "inputs"
        self.workspace = self.inputs / "review-snapshot"
        self.workspace.mkdir(parents=True)
        (self.workspace / "compose.yaml").write_text("services: {}\n", encoding="utf-8")
        self.environment = {
            "HAI_TRIVY_RUNNER_TOKEN": "runner-token-1234",
            "HAI_TRIVY_WORKSPACES": "review-snapshot",
            "HAI_TRIVY_INPUT_ROOT": str(self.inputs),
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

    def test_scan_returns_aggregate_without_findings_or_source_details(self):
        def fake_run(command, **kwargs):
            if command[-1] == "--version":
                return subprocess.CompletedProcess(command, 0, stdout="Version: 0.72.0\n", stderr="")
            report_path = Path(command[command.index("--output") + 1])
            report_path.write_text(json.dumps({"Results": [{"Misconfigurations": [
                {"Status": "FAIL", "Severity": "HIGH", "ID": "AVD-DS-0001", "Target": "compose.yaml", "Title": "unsafe setting"},
                {"Status": "FAIL", "Severity": "LOW", "ID": "AVD-DS-0002", "Target": "infra/main.tf", "Title": "weak setting"},
            ]}]}), encoding="utf-8")
            return subprocess.CompletedProcess(command, 0, stdout=b"", stderr=b"")

        with patch.object(app.subprocess, "run", side_effect=fake_run):
            result = app.scan("review-snapshot")

        self.assertEqual(result["status"], "completed")
        self.assertEqual(result["engine"], "trivy 0.72.0")
        self.assertEqual(result["findingCount"], 2)
        self.assertEqual(result["severities"], [{"severity": "high", "count": 1}, {"severity": "low", "count": 1}])
        serialized = json.dumps(result)
        self.assertNotIn("compose.yaml", serialized)
        self.assertNotIn("AVD-DS-0001", serialized)
        self.assertNotIn("unsafe setting", serialized)

    def test_workspace_requires_a_supported_configuration_file(self):
        (self.workspace / "compose.yaml").unlink()
        with self.assertRaises(app.RequestError):
            app.workspace_directory("review-snapshot")

    def test_snapshot_entry_limit_is_bounded_before_scan(self):
        with patch.object(app, "MAX_SNAPSHOT_ENTRIES", 2):
            (self.workspace / "first").touch()
            (self.workspace / "second").touch()
            with self.assertRaises(app.RequestError):
                app.workspace_directory("review-snapshot")

    def test_version_parser_rejects_unexpected_output(self):
        with patch.object(app.subprocess, "run", return_value=subprocess.CompletedProcess(["trivy"], 0, stdout="unexpected", stderr="")):
            with self.assertRaises(app.RequestError):
                app.runner_engine()


if __name__ == "__main__":
    unittest.main()
