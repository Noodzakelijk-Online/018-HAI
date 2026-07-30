import os
import unittest
from unittest.mock import patch

import app


class FakeJudge(app.DeepEvalBaseLLM):
    def load_model(self):
        return None

    def generate(self, _prompt, schema=None, **_kwargs):
        if schema.__name__ == "Truths":
            return '{"truths":["synthetic source fact"]}'
        if schema.__name__ == "Claims":
            return '{"claims":["synthetic answer claim"]}'
        if schema.__name__ == "Verdicts":
            return '{"verdicts":[{"verdict":"yes","reason":"synthetic"}]}'
        raise AssertionError(f"unexpected schema {schema}")

    async def a_generate(self, *args, **kwargs):
        return self.generate(*args, **kwargs)

    def get_model_name(self):
        return "qwen2.5:7b"


class DeepEvalRunnerTests(unittest.TestCase):
    @patch.dict(
        os.environ,
        {
            "HAI_DEEPEVAL_MODEL_BASE_URL": "http://host.docker.internal:11434/v1",
            "HAI_DEEPEVAL_MODEL_ID": "qwen2.5:7b",
        },
        clear=False,
    )
    def test_accepts_one_bounded_local_model_configuration(self):
        self.assertEqual(app.configuration()[:2], ("http://host.docker.internal:11434/v1", "qwen2.5:7b"))

    @patch.dict(
        os.environ,
        {
            "HAI_DEEPEVAL_MODEL_BASE_URL": "https://example.com/v1",
            "HAI_DEEPEVAL_MODEL_ID": "remote",
        },
        clear=False,
    )
    def test_rejects_external_model_endpoint(self):
        with self.assertRaises(app.RequestError):
            app.configuration()

    @patch.dict(
        os.environ,
        {
            "HAI_DEEPEVAL_MODEL_BASE_URL": "http://localhost:11434/v1",
            "HAI_DEEPEVAL_MODEL_ID": "qwen2.5:7b",
        },
        clear=False,
    )
    @patch.object(app, "LocalOpenAIJudge", FakeJudge)
    def test_fixed_suite_returns_only_aggregate_metadata(self):
        with patch.object(app, "engine_name", return_value="deepeval 4.1.1"):
            result = app.run_evaluation()
        self.assertEqual(result["caseCount"], app.MAX_CASES)
        self.assertEqual(result["passedCount"] + result["failedCount"], app.MAX_CASES)
        self.assertNotIn("filing deadline", str(result).lower())
        self.assertNotIn("legal email", str(result).lower())


if __name__ == "__main__":
    unittest.main()
