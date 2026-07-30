import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import app


class LMEvalRunnerTests(unittest.TestCase):
    def setUp(self):
        self.environ = {
            "HAI_LM_EVAL_MODEL_ID": "qwen2.5:7b",
            "HAI_LM_EVAL_MODEL_BASE_URL": "http://host.docker.internal:11434/v1",
        }

    def test_uses_a_fixed_local_command_and_returns_aggregate_only(self):
        seen = {}

        def fake_run(command, **kwargs):
            seen["command"] = command
            output_path = Path(command[command.index("--output_path") + 1])
            output_path.mkdir(parents=True, exist_ok=True)
            (output_path / "results.json").write_text(json.dumps({"results": {app.SUITE: {"exact_match,none": 5 / 6}}}), encoding="utf-8")
            return subprocess.CompletedProcess(command, 0)

        with patch.dict(os.environ, self.environ, clear=False), patch("app.subprocess.run", side_effect=fake_run):
            result = app.run_evaluation()
        self.assertEqual(result["status"], "completed")
        self.assertEqual(result["caseCount"], 6)
        self.assertAlmostEqual(result["exactMatch"], 5 / 6)
        self.assertIn("local-chat-completions", seen["command"])
        self.assertIn("--tasks", seen["command"])
        self.assertIn("--apply_chat_template", seen["command"])
        self.assertNotIn("--log_samples", seen["command"])
        self.assertNotIn("host.docker.internal", str(result))
        self.assertNotIn("Task: What is 2 plus 2?", str(result))

    def test_rejects_non_local_endpoint_and_caller_data(self):
        with patch.dict(os.environ, {"HAI_LM_EVAL_MODEL_ID": "qwen2.5:7b", "HAI_LM_EVAL_MODEL_BASE_URL": "https://example.com/v1"}, clear=False):
            with self.assertRaises(app.RequestError):
                app.configuration()
        handler = app.Handler
        self.assertEqual(app.SUITE, "hai_synthetic_v1")


if __name__ == "__main__":
    unittest.main()
