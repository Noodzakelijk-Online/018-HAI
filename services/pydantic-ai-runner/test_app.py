import os
import unittest
from unittest.mock import patch

import app


class PydanticAIRunnerTests(unittest.TestCase):
    def setUp(self):
        self.environment = {
            "HAI_PYDANTIC_AI_LOCAL_BASE_URL": "http://host.docker.internal:11434/v1",
            "HAI_PYDANTIC_AI_LOCAL_MODEL_ID": "qwen-local",
        }

    @patch.dict(os.environ, {"HAI_PYDANTIC_AI_LOCAL_BASE_URL": "http://host.docker.internal:11434/v1", "HAI_PYDANTIC_AI_LOCAL_MODEL_ID": "qwen-local"}, clear=True)
    def test_accepts_bounded_local_configuration_and_input(self):
        self.assertEqual(app.configured()[:2], ("http://host.docker.internal:11434/v1", "qwen-local"))
        request, criteria = app.validate_payload({"request": "Draft an evidence plan", "successCriteria": ["Use source links"]})
        self.assertEqual(request, "Draft an evidence plan")
        self.assertEqual(criteria, ["Use source links"])

    @patch.dict(os.environ, {"HAI_PYDANTIC_AI_LOCAL_BASE_URL": "https://example.com/v1", "HAI_PYDANTIC_AI_LOCAL_MODEL_ID": "remote"}, clear=True)
    def test_rejects_external_model_endpoints(self):
        with self.assertRaises(app.RequestError):
            app.configured()

    def test_rejects_multiline_or_unbounded_input(self):
        with self.assertRaises(app.RequestError):
            app.validate_payload({"request": "one\ntwo"})
        with self.assertRaises(app.RequestError):
            app.validate_payload({"request": "x" * (app.MAX_REQUEST_CHARS + 1)})

    def test_rejects_invalid_model_output(self):
        with self.assertRaises(app.RequestError):
            app.validate_proposal(app.PlanProposal(
                goal="Review",
                successCriteria=["Criterion"],
                nextSteps=["Inspect"],
                risk="unexpected",
                requiresApproval=False,
                reasons=["Reason"],
            ))


if __name__ == "__main__":
    unittest.main()
