import hashlib
import json
import os
from pathlib import Path
from tempfile import TemporaryDirectory
import unittest
from unittest.mock import patch

import app


class GarakRunnerTests(unittest.TestCase):
    def test_configuration_rejects_external_endpoint(self):
        with self.assertRaises(app.RequestError):
            app.configuration({"HAI_GARAK_MODEL_BASE_URL": "https://example.com/v1", "HAI_GARAK_MODEL_ID": "model"})

    def test_runner_environment_does_not_inherit_cloud_credentials(self):
        previous = os.environ.get("OPENAI_API_KEY")
        os.environ["OPENAI_API_KEY"] = "must-not-leak"
        try:
            environment = app.garak_environment("local-key")
        finally:
            if previous is None:
                os.environ.pop("OPENAI_API_KEY", None)
            else:
                os.environ["OPENAI_API_KEY"] = previous
        self.assertEqual(environment["OPENAICOMPATIBLE_API_KEY"], "local-key")
        self.assertNotIn("OPENAI_API_KEY", environment)
        self.assertEqual(environment["NO_PROXY"], "*")

    def test_fixed_config_uses_the_minimum_valid_bootstrap_setting(self):
        config = app.run_config("http://ollama:11434/v1")
        self.assertEqual(config["run"]["soft_probe_prompt_cap"], app.MAX_CASES)
        self.assertEqual(config["reporting"]["bootstrap_num_iterations"], 1)

    def test_parse_report_returns_aggregate_only(self):
        with TemporaryDirectory() as directory:
            report = Path(directory) / "scan.report.jsonl"
            report.write_text("\n".join([
                json.dumps({"entry_type": "attempt", "outputs": ["must-not-be-returned"]}),
                json.dumps({"entry_type": "eval", "probe": app.PROBE, "detector": app.DETECTOR, "passed": 3, "fails": 1, "total_evaluated": 4}),
            ]), encoding="utf-8")
            with patch.object(app, "engine_name", return_value="garak 0.15.1"):
                result = app.parse_report(report, "qwen2.5:7b", 42)
        self.assertEqual(result["caseCount"], 4)
        self.assertEqual(result["score"], 0.75)
        self.assertNotIn("outputs", result)
        self.assertEqual(result["resultDigest"], hashlib.sha256(json.dumps({"suite": app.SUITE, "modelId": "qwen2.5:7b", "probe": app.PROBE, "detector": app.DETECTOR, "caseCount": 4, "passedCount": 3, "failedCount": 1, "score": 0.75}, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest())

if __name__ == "__main__":
    unittest.main()
