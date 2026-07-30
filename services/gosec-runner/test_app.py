import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import app


class GosecRunnerTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.inputs = self.root / "inputs"
        self.workspace = self.inputs / "review-snapshot"
        self.workspace.mkdir(parents=True)
        (self.workspace / "go.mod").write_text("module example.test/review\n", encoding="utf-8")
        (self.workspace / "vendor").mkdir()
        (self.workspace / "vendor" / "modules.txt").write_text("# example.test/review\n", encoding="utf-8")
        self.environment = {
            "HAI_GOSEC_RUNNER_TOKEN": "runner-token-1234",
            "HAI_GOSEC_WORKSPACES": "review-snapshot",
            "HAI_GOSEC_INPUT_ROOT": str(self.inputs),
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
            if command[-1] == "-version":
                return subprocess.CompletedProcess(command, 0, stdout="Version: 2.28.0\nGit tag: v2.28.0\n", stderr="")
            report_path = Path(command[command.index("-out") + 1])
            report_path.write_text(json.dumps({"Issues": [
                {"severity": "HIGH", "confidence": "HIGH", "rule_id": "G204", "file": "cmd/run.go", "details": "unsafe command"},
                {"severity": "LOW", "confidence": "MEDIUM", "rule_id": "G101", "file": "internal/secret.go", "details": "hard-coded"},
            ]}), encoding="utf-8")
            return subprocess.CompletedProcess(command, 0, stdout=b"", stderr=b"")

        with patch.object(app.subprocess, "run", side_effect=fake_run):
            result = app.scan("review-snapshot")

        self.assertEqual(result["status"], "completed")
        self.assertEqual(result["engine"], "gosec 2.28.0")
        self.assertEqual(result["findingCount"], 2)
        self.assertEqual(result["severities"], [{"severity": "high", "count": 1}, {"severity": "low", "count": 1}])
        self.assertEqual(result["confidences"], [{"confidence": "high", "count": 1}, {"confidence": "medium", "count": 1}])
        serialized = json.dumps(result)
        self.assertNotIn("cmd/run.go", serialized)
        self.assertNotIn("G204", serialized)
        self.assertNotIn("unsafe command", serialized)

    def test_workspace_requires_a_vendored_go_module(self):
        (self.workspace / "vendor" / "modules.txt").unlink()
        with self.assertRaises(app.RequestError):
            app.workspace_directory("review-snapshot")

    def test_snapshot_entry_limit_is_bounded_before_scan(self):
        with patch.object(app, "MAX_SNAPSHOT_ENTRIES", 2):
            (self.workspace / "first").touch()
            (self.workspace / "second").touch()
            with self.assertRaises(app.RequestError):
                app.workspace_directory("review-snapshot")

    def test_version_parser_rejects_unexpected_output(self):
        with patch.object(app.subprocess, "run", return_value=subprocess.CompletedProcess(["gosec"], 0, stdout="unexpected", stderr="")):
            with self.assertRaises(app.RequestError):
                app.runner_engine()


if __name__ == "__main__":
    unittest.main()
