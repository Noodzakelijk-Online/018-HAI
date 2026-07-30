"""Local FastMCP server exposing HAI read models to a reviewed MCP client.

This process deliberately contains no HAI write endpoint, tool executor,
filesystem capability, source connector, model client, or secret-returning
tool. Both its caller and the HAI backend must present separate explicit
tokens, and Docker publishes it to loopback only.
"""

from __future__ import annotations

from dataclasses import dataclass
import hmac
import ipaddress
import os
from urllib.parse import urlparse

import httpx
from fastmcp import FastMCP
from fastmcp.server.auth import AccessToken, TokenVerifier


MAX_TOKEN_LENGTH = 512
ALLOWED_API_HOSTS = {"localhost", "host.docker.internal", "backend"}
READ_SCOPE = "hai:read"


class ConfigurationError(ValueError):
    """Raised before the bridge binds a port when its local trust setup is incomplete."""


@dataclass(frozen=True)
class Config:
    api_base_url: str
    bridge_token: str
    client_token: str


def configured_token(name: str) -> str:
    value = os.environ.get(name, "").strip()
    if len(value) < 32 or len(value) > MAX_TOKEN_LENGTH or "\n" in value or "\r" in value:
        raise ConfigurationError(f"{name} must contain 32-512 single-line characters")
    return value


def local_api_base_url() -> str:
    raw = os.environ.get("HAI_FASTMCP_BRIDGE_API_BASE_URL", "http://backend:8080/api/v1/mcp-agent").strip().rstrip("/")
    parsed = urlparse(raw)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ConfigurationError("HAI_FASTMCP_BRIDGE_API_BASE_URL must be a plain local HTTP(S) URL")
    host = parsed.hostname.lower()
    try:
        address = ipaddress.ip_address(host)
        if not (address.is_loopback or address.is_private):
            raise ConfigurationError("HAI_FASTMCP_BRIDGE_API_BASE_URL must use a loopback or private-network IP")
    except ValueError:
        if host not in ALLOWED_API_HOSTS:
            raise ConfigurationError("HAI_FASTMCP_BRIDGE_API_BASE_URL host is not allowlisted")
    return raw


def load_config() -> Config:
    if os.environ.get("HAI_FASTMCP_BRIDGE_ENABLED", "").strip().lower() != "true":
        raise ConfigurationError("HAI_FASTMCP_BRIDGE_ENABLED=true is required")
    return Config(
        api_base_url=local_api_base_url(),
        bridge_token=configured_token("HAI_FASTMCP_BRIDGE_TOKEN"),
        client_token=configured_token("HAI_FASTMCP_CLIENT_TOKEN"),
    )


class StaticTokenVerifier(TokenVerifier):
    """A local bearer verifier with one narrow read scope and no OAuth server."""

    def __init__(self, token: str):
        super().__init__(required_scopes=[READ_SCOPE])
        self._token = token

    async def verify_token(self, token: str) -> AccessToken | None:
        if not isinstance(token, str) or not hmac.compare_digest(token, self._token):
            return None
        return AccessToken(token=token, client_id="hai-local-mcp-client", subject="hai-local-mcp-client", scopes=[READ_SCOPE])


async def fetch_read_model(config: Config, path: str, params: dict[str, int] | None = None) -> dict:
    url = config.api_base_url + path
    headers = {
        "Accept": "application/json",
        "X-HAI-MCP-Bridge-Token": config.bridge_token,
        "User-Agent": "HAI-FastMCP-ReadBridge/1.0",
    }
    async with httpx.AsyncClient(timeout=5.0, follow_redirects=False, trust_env=False) as client:
        response = await client.get(url, headers=headers, params=params)
    if response.status_code != 200:
        raise RuntimeError("HAI local read bridge is unavailable")
    payload = response.json()
    if not isinstance(payload, dict):
        raise RuntimeError("HAI local read bridge returned an invalid response")
    return payload


def build_server(config: Config) -> FastMCP:
    server = FastMCP(
        name="HAI local read-only bridge",
        instructions=(
            "Read only the bounded HAI workflow summaries exposed by these tools. "
            "Do not claim authority to approve, execute, create, update, or retrieve sources. "
            "Route every consequential decision back to HAI's owner-facing approval and workflow screens."
        ),
        auth=StaticTokenVerifier(config.client_token),
    )

    @server.tool(tags={"hai", "read-only", "operations"})
    async def hai_operating_overview() -> dict:
        """Return aggregate workflow counts for the configured HAI owner."""
        return await fetch_read_model(config, "/overview")

    @server.tool(tags={"hai", "read-only", "operations"})
    async def hai_actionable_workflows(limit: int = 5) -> dict:
        """Return up to eight sanitized workflow summaries that need attention."""
        if not isinstance(limit, int) or limit < 1 or limit > 8:
            raise ValueError("limit must be between 1 and 8")
        return await fetch_read_model(config, "/actionable", {"limit": limit})

    return server


if __name__ == "__main__":
    server = build_server(load_config())
    server.run(transport="streamable-http", host="0.0.0.0", port=8080, path="/mcp")
