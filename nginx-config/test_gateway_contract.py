from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]
NGINX_TEMPLATE = (ROOT / "nginx-config" / "nginx.conf.template").read_text(encoding="utf-8")
COMPOSE = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")


def location_block(marker: str) -> str:
    start = NGINX_TEMPLATE.index(marker)
    brace = NGINX_TEMPLATE.index("{", start)
    depth = 0
    for index in range(brace, len(NGINX_TEMPLATE)):
        char = NGINX_TEMPLATE[index]
        if char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return NGINX_TEMPLATE[start : index + 1]
    raise AssertionError(f"unterminated nginx block: {marker}")


class GatewayAuthContractTest(unittest.TestCase):
    def test_local_a2a_peer_route_preserves_its_token_and_rejects_tunnel_traffic(self) -> None:
        for marker in (
            "location = /api/v1/a2a",
            "location = /.well-known/agent-card.json",
        ):
            with self.subTest(marker=marker):
                block = location_block(marker)
                self.assertNotIn("auth_request /auth-verify;", block)
                self.assertIn("proxy_set_header X-HAI-Backend-Key ${BACKEND_API_SHARED_KEY};", block)
                self.assertIn("if ($hai_a2a_ngrok_request) { return 404; }", block)

        a2a = location_block("location = /api/v1/a2a")
        self.assertIn("proxy_set_header Authorization $http_authorization;", a2a)

        self.assertIn(
            'map $http_x_forwarded_for $hai_a2a_ngrok_request',
            NGINX_TEMPLATE,
        )
        self.assertIn('default 1;', NGINX_TEMPLATE)
        self.assertIn('""      0;', NGINX_TEMPLATE)

    def test_backend_routes_use_authenticated_catch_all(self) -> None:
        backend = location_block("location /api/v1 {")
        self.assertIn("auth_request /auth-verify;", backend)
        self.assertIn("proxy_pass http://$backend_upstream;", backend)
        self.assertNotIn("location ~", backend)

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

    def test_automation_uploads_match_the_documented_backend_image_limit(self) -> None:
        block = location_block("location ^~ /api/v1/automation/")
        self.assertIn("client_max_body_size 6m;", block)
        self.assertIn("auth_request /auth-verify;", block)
        self.assertIn("proxy_pass http://$backend_upstream;", block)
        self.assertIn(
            'proxy_set_header Authorization "Bearer $hai_verified_access_token";',
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


if __name__ == "__main__":
    unittest.main()
