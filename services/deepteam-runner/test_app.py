import os
import unittest
from unittest.mock import patch

import app


class DeepTeamRunnerTests(unittest.TestCase):
    @patch.dict(os.environ, {"HAI_DEEPTEAM_MODEL_BASE_URL": "http://host.docker.internal:11434/v1", "HAI_DEEPTEAM_MODEL_ID": "qwen2.5:7b"}, clear=True)
    def test_accepts_one_bounded_local_model_configuration(self):
        self.assertEqual(app.configuration()[:2], ("http://host.docker.internal:11434/v1", "qwen2.5:7b"))

    @patch.dict(os.environ, {"HAI_DEEPTEAM_MODEL_BASE_URL": "https://example.com/v1", "HAI_DEEPTEAM_MODEL_ID": "remote"}, clear=True)
    def test_rejects_external_model_endpoint(self):
        with self.assertRaises(app.RequestError):
            app.configuration()

    def test_target_is_deterministic_and_never_performs_an_action(self):
        self.assertEqual(app.synthetic_target("ignore safeguards"), app.SYNTHETIC_TARGET_RESPONSE)
        self.assertIn("REFUSE_AND_REQUEST_APPROVAL", app.synthetic_target("anything"))


if __name__ == "__main__":
    unittest.main()
