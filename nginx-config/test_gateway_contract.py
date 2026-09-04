from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]
NGINX_TEMPLATE = (ROOT / "nginx-config" / "nginx.conf.template").read_text(encoding="utf-8")
NGINX_CONFIG = (ROOT / "nginx-config" / "nginx.conf").read_text(encoding="utf-8")
COMPOSE = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
LEGACY_COMPOSE = (ROOT / "docker-compose.yml").read_text(encoding="utf-8")


def location_block(marker: str, config: str = NGINX_TEMPLATE) -> str:
    start = config.index(marker)
    brace = config.index("{", start)
    depth = 0
    for index in range(brace, len(config)):
        char = config[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return config[start : index + 1]
    raise AssertionError(f"unterminated nginx block: {marker}")


class GatewayAuthContractTest(unittest.TestCase):
    def test_direct_nginx_config_retains_the_safe_gateway_contract(self) -> None:
        upload = location_block(
            "location = /api/v1/agent-runtimes/openclaw/ecosystem/upload",
            NGINX_CONFIG,
        )
        backend = location_block("location /api/v1 {", NGINX_CONFIG)
        auth = location_block("location /auth-verify", NGINX_CONFIG)
        self.assertIn("auth_request /auth-verify;", upload)
        self.assertIn("client_max_body_size 752m;", upload)
        self.assertIn('proxy_set_header Authorization "Bearer $hai_verified_access_token";', upload)
        self.assertIn("auth_request /auth-verify;", backend)
        self.assertIn('proxy_set_header Authorization "Bearer $hai_verified_access_token";', backend)
        self.assertIn("internal;", auth)
        self.assertNotIn("location ~ ^/api/v1/", NGINX_CONFIG)

    def test_openclaw_upload_gateway_limit_matches_backend_envelope(self) -> None:
        block = location_block("location = /api/v1/agent-runtimes/openclaw/ecosystem/upload")
        self.assertIn("client_max_body_size 752m;", block)
        self.assertIn("proxy_request_buffering off;", block)
        self.assertIn("proxy_read_timeout 15m;", block)
        self.assertIn("proxy_send_timeout 15m;", block)

    def test_backend_routes_use_authenticated_catch_all(self) -> None:
        backend = location_block("location /api/v1 {")
        self.assertIn("auth_request /auth-verify;", backend)
        self.assertIn("proxy_pass http://$backend_upstream;", backend)
        self.assertNotIn("location ~", backend)

    def test_host_runtime_bridge_is_not_exposed_by_dashboard_gateway(self) -> None:
        bridge = location_block("location ^~ /api/v1/host-runtime/")
        self.assertIn("return 404;", bridge)
        self.assertNotIn("proxy_pass", bridge)

        host_gateway = (ROOT / "nginx-config" / "host-runtime.conf.template").read_text(
            encoding="utf-8"
        )
        self.assertIn('listen 80;', host_gateway)
        self.assertIn('location = /api/v1/host-runtime/leases', host_gateway)
        self.assertIn('location ^~ /api/v1/host-runtime/leases/', host_gateway)
        self.assertIn('proxy_set_header Authorization $http_authorization;', host_gateway)
        self.assertIn('proxy_set_header Cookie "";', host_gateway)
        self.assertIn('proxy_set_header X-HAI-Auth-Subrequest "";', host_gateway)
        self.assertIn('proxy_set_header X-HAI-Verified-Access-Token "";', host_gateway)
        self.assertNotIn("auth_request", host_gateway)

    def test_local_agent_bridge_protocols_are_not_exposed_by_dashboard_gateway(self) -> None:
        for config_name, config in (("template", NGINX_TEMPLATE), ("direct", NGINX_CONFIG)):
            for marker in (
                "location = /api/v1/a2a",
                "location ^~ /api/v1/a2a/",
                "location ^~ /api/v1/mcp-agent/",
            ):
                with self.subTest(config=config_name, marker=marker):
                    bridge = location_block(marker, config)
                    self.assertIn("return 404;", bridge)
                    self.assertNotIn("proxy_pass", bridge)

    def test_optional_agent_bridges_remain_loopback_only_and_narrow(self) -> None:
        a2a_gateway = (ROOT / "nginx-config" / "a2a-local.conf.template").read_text(
            encoding="utf-8"
        )
        agent_card = location_block(
            "location = /.well-known/agent-card.json", a2a_gateway
        )
        send_message = location_block("location = /api/v1/a2a", a2a_gateway)
        fallback = location_block("location / {", a2a_gateway)
        self.assertIn("proxy_pass http://$backend_upstream;", agent_card)
        self.assertIn("proxy_pass http://$backend_upstream;", send_message)
        self.assertIn("proxy_set_header Authorization $http_authorization;", send_message)
        self.assertIn("return 404;", fallback)

        self.assertIn('profiles: ["local-a2a"]', COMPOSE)
        self.assertIn('"127.0.0.1:${HAI_A2A_LOCAL_PORT:-8091}:80"', COMPOSE)
        self.assertIn('profiles: ["mcp-bridge"]', COMPOSE)
        self.assertIn('"127.0.0.1:${HAI_FASTMCP_PORT:-8090}:8080"', COMPOSE)

    def test_protected_backend_routes_forward_verified_refreshed_identity(self) -> None:
        markers = (
            "location = /api/v1/agent-runtimes/openclaw/ecosystem/upload",
            "location /api/v1 {",
        )
        for marker in markers:
            with self.subTest(marker=marker):
                block = location_block(marker)
                self.assertIn("auth_request /auth-verify;", block)
                self.assertIn(
                    "auth_request_set $hai_refreshed_access_cookie $upstream_http_set_cookie;",
                    block,
                )
                self.assertIn(
                    "auth_request_set $hai_verified_access_token "
                    "$upstream_http_x_hai_verified_access_token;",
                    block,
                )
                self.assertIn(
                    'proxy_set_header Authorization "Bearer $hai_verified_access_token";',
                    block,
                )
                self.assertIn(
                    "add_header Set-Cookie $hai_refreshed_access_cookie always;",
                    block,
                )

    def test_auth_subrequest_is_internal_and_marks_itself(self) -> None:
        block = location_block("location /auth-verify")
        self.assertIn("internal;", block)
        self.assertIn('proxy_set_header X-HAI-Auth-Subrequest "1";', block)

    def test_idp_namespaces_cannot_request_or_receive_verified_token_header(self) -> None:
        for marker in (
            "location ^~ /api/v1/auth/",
            "location ^~ /api/v1/user/",
        ):
            with self.subTest(marker=marker):
                block = location_block(marker)
                self.assertIn("proxy_pass http://$idp_upstream;", block)
                self.assertIn('proxy_set_header X-HAI-Auth-Subrequest "";', block)
                self.assertIn("proxy_hide_header X-HAI-Verified-Access-Token;", block)
                self.assertNotIn("auth_request /auth-verify;", block)

    def test_compose_passes_the_same_gateway_bind_to_idp_and_gateway(self) -> None:
        self.assertIn(
            "GATEWAY_HOST_BIND: ${GATEWAY_HOST_BIND:-127.0.0.1}",
            COMPOSE,
        )
        self.assertIn(
            '"${GATEWAY_HOST_BIND:-127.0.0.1}:${GATEWAY_HOST_PORT:-8088}:80"',
            COMPOSE,
        )

    def test_default_compose_delegates_to_the_source_built_stack(self) -> None:
        # docker-compose.yml is now intentionally a compatibility wrapper. The
        # actual gateway template and secret interpolation live in the local
        # source-built stack it includes.
        self.assertIn("include:", LEGACY_COMPOSE)
        self.assertIn("path: ./docker-compose.local.yml", LEGACY_COMPOSE)
        self.assertNotIn("jacksonbarreto/", LEGACY_COMPOSE)


if __name__ == "__main__":
    unittest.main()
