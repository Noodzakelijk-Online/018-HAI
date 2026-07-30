import asyncio
import os
import unittest
from unittest.mock import patch

from app import ConfigurationError, StaticTokenVerifier, load_config, local_api_base_url


class FastMCPBridgeTests(unittest.TestCase):
    def test_requires_explicit_enablement_and_both_tokens(self):
        with patch.dict(os.environ, {}, clear=True):
            with self.assertRaises(ConfigurationError):
                load_config()
        with patch.dict(os.environ, {
            "HAI_FASTMCP_BRIDGE_ENABLED": "true",
            "HAI_FASTMCP_BRIDGE_TOKEN": "a" * 32,
            "HAI_FASTMCP_CLIENT_TOKEN": "b" * 31,
        }, clear=True):
            with self.assertRaises(ConfigurationError):
                load_config()

    def test_rejects_external_backend_url(self):
        with patch.dict(os.environ, {"HAI_FASTMCP_BRIDGE_API_BASE_URL": "https://example.com/api"}, clear=True):
            with self.assertRaises(ConfigurationError):
                local_api_base_url()

    def test_accepts_only_the_configured_bearer_token(self):
        verifier = StaticTokenVerifier("t" * 32)
        accepted = asyncio.run(verifier.verify_token("t" * 32))
        rejected = asyncio.run(verifier.verify_token("x" * 32))
        self.assertIsNotNone(accepted)
        self.assertEqual(accepted.scopes, ["hai:read"])
        self.assertIsNone(rejected)


if __name__ == "__main__":
    unittest.main()
