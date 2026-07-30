import asyncio
import os
import unittest
from unittest.mock import patch

from app import ConfigurationError, Config, StaticTokenVerifier, build_server, load_config, local_api_base_url


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

    def test_registers_only_four_read_only_hai_tools(self):
        server = build_server(Config("http://backend:8080/api/v1/mcp-agent", "a" * 32, "b" * 32))
        tools = asyncio.run(server.list_tools())
        names = {tool.name for tool in tools}
        self.assertEqual(names, {"hai_operating_overview", "hai_actionable_workflows", "hai_github_repository_context", "hai_model_maintenance_readiness"})


if __name__ == "__main__":
    unittest.main()
