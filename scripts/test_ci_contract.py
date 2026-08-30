import json
from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / ".github" / "workflows" / "ci.yml").read_text(
    encoding="utf-8"
)


def job_block(job_id: str) -> str:
    marker = f"  {job_id}:\n"
    start = WORKFLOW.index(marker)
    remaining = WORKFLOW[start + len(marker) :]
    next_job_offsets = [
        offset
        for offset, line in enumerate(remaining.splitlines(keepends=True))
        if line.startswith("  ")
        and not line.startswith("    ")
        and line.rstrip().endswith(":")
    ]
    if not next_job_offsets:
        return WORKFLOW[start:]
    lines = remaining.splitlines(keepends=True)
    return WORKFLOW[start : start + len(marker) + sum(
        len(line) for line in lines[: next_job_offsets[0]]
    )]


def compose_service_block(compose: str, service: str) -> str:
    pattern = rf"(?ms)^  {re.escape(service)}:\n(.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)"
    match = re.search(pattern, compose)
    if not match:
        raise AssertionError(f"missing Compose service {service!r}")
    return match.group(1)


class CIWorkflowContractTest(unittest.TestCase):
    def test_compose_validation_runs_the_fail_closed_truthfulness_audit(self) -> None:
        compose = job_block("compose")
        audit = (ROOT / "scripts" / "no-fake-claims-audit.sh").read_text(
            encoding="utf-8"
        )

        self.assertIn("No-fake claims and tracked-artifact audit", compose)
        self.assertIn("bash scripts/no-fake-claims-audit.sh", compose)
        self.assertIn("command -v git", audit)
        self.assertIn("refusing to report an incomplete audit as passing", audit)

    def test_bootstrap_document_matches_the_dark_first_theme_contract(self) -> None:
        index = (ROOT / "frontend" / "src" / "index.html").read_text(
            encoding="utf-8"
        )

        # Angular loads after the browser has already painted index.html. The
        # bootstrap shell must apply the same persisted dark-first preference as
        # ThemeService so a slow local Windows startup does not flash the light
        # theme or present an unbranded document title.
        self.assertIn("HAI Automation Hub", index)
        self.assertIn("hai-theme-mode", index)
        self.assertIn("document.documentElement.classList.add(themeClass)", index)
        self.assertIn("background:#08111f", index)

    def test_operational_shell_avoids_decorative_compositing_effects(self) -> None:
        styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        self.assertNotIn("backdrop-filter:", styles)
        self.assertNotIn("linear-gradient(180deg", styles)

    def test_control_center_keeps_its_route_styles_isolated(self) -> None:
        component = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "control-center"
            / "control-center.component.ts"
        ).read_text(encoding="utf-8")
        self.assertIn("encapsulation: ViewEncapsulation.Emulated", component)
        self.assertNotIn("encapsulation: ViewEncapsulation.None", component)

    def test_frontend_uses_one_direct_angular_cdk_dependency(self) -> None:
        frontend = ROOT / "frontend"
        package = json.loads((frontend / "package.json").read_text(encoding="utf-8"))
        package_lock = (frontend / "package-lock.json").read_text(encoding="utf-8")

        self.assertIn("@angular/cdk", package["dependencies"])
        self.assertNotIn("angular-mixed-cdk-drag-drop", package["dependencies"])
        self.assertNotIn("angular-mixed-cdk-drag-drop", package_lock)

    def test_frontend_theme_excludes_unused_ng_zorro_components(self) -> None:
        theme = (ROOT / "frontend" / "src" / "theme.less").read_text(
            encoding="utf-8"
        )

        # Importing ng-zorro-antd.less expands the complete component catalog
        # into HAI's initial CSS chunk. Keep this explicit list aligned with
        # application module usage so a new UI dependency has a deliberate
        # styling and payload review.
        self.assertNotIn("ng-zorro-antd/ng-zorro-antd.less", theme)
        expected_components = {
            "icon", "alert", "button", "card", "checkbox", "drawer",
            "dropdown", "empty", "form", "input", "input-number",
            "layout", "list", "modal", "radio", "select", "spin",
            "steps", "table", "tag", "timeline", "tooltip", "upload",
        }
        imported_components = set(
            re.findall(r'ng-zorro-antd/([^/]+)/style/entry\.less', theme)
        )
        self.assertEqual(imported_components, expected_components)
        self.assertIn('ng-zorro-antd/style/default.less', theme)
        self.assertIn('ng-zorro-antd/style/patch.less', theme)

    def test_home_menu_uses_the_ng_zorro_menu_module(self) -> None:
        home_module = (ROOT / "frontend" / "src" / "app" / "pages" / "home" / "home.module.ts").read_text(
            encoding="utf-8"
        )
        home_styles = (ROOT / "frontend" / "src" / "app" / "pages" / "home" / "home.component.scss").read_text(
            encoding="utf-8"
        )

        self.assertIn('from "ng-zorro-antd/menu"', home_module)
        self.assertIn("NzMenuModule", home_module)
        self.assertIn(".dropdown-menu {", home_styles)
        self.assertIn(".ant-menu {", home_styles)

    def test_home_automation_images_defer_offscreen_work(self) -> None:
        home_template = (ROOT / "frontend" / "src" / "app" / "pages" / "home" / "home.component.html").read_text(
            encoding="utf-8"
        )

        self.assertIn('loading="lazy"', home_template)
        self.assertIn('decoding="async"', home_template)

    def test_pursuit_reservation_styles_load_with_the_lazy_module(self) -> None:
        global_styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        pursuit_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "pursuits"
            / "pursuits.component.scss"
        ).read_text(encoding="utf-8")

        self.assertNotIn(".resource-reservations {", global_styles)
        self.assertIn(".resource-reservations {", pursuit_styles)
        self.assertIn(".resource-reservation--stale", pursuit_styles)

    def test_control_center_styles_load_with_the_lazy_module(self) -> None:
        global_styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        control_center_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "control-center"
            / "control-center.component.scss"
        ).read_text(encoding="utf-8")
        control_center_component = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "control-center"
            / "control-center.component.ts"
        ).read_text(encoding="utf-8")

        self.assertNotIn("/* HAI Control Center design system */", global_styles)
        self.assertNotIn("app-control-center{--bg", global_styles)
        # The Control Center now consumes the shared app-shell tokens from its
        # lazy component host. The legacy app-control-center wrapper was
        # deliberately removed during the shell consolidation.
        normalized_styles = control_center_styles.replace(" ", "").replace("\n", "")
        self.assertIn(":host{--hai-primary:var(--hai-blue);", normalized_styles)
        self.assertNotIn("app-control-center", control_center_styles)
        self.assertIn(".command-board{display:grid;", normalized_styles)
        self.assertIn(".page-content", control_center_styles)
        self.assertIn("ViewEncapsulation.Emulated", control_center_component)
        self.assertNotIn("ViewEncapsulation.None", control_center_component)

    def test_workflow_engine_styles_load_with_the_lazy_module(self) -> None:
        global_styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        workflow_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "workflow-engine"
            / "workflow-engine.component.scss"
        ).read_text(encoding="utf-8")
        workflow_component = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "workflow-engine"
            / "workflow-engine.component.ts"
        ).read_text(encoding="utf-8")

        self.assertNotIn(
            "/* Workflow Engine follows the command-center rule:", global_styles
        )
        self.assertIn("app-workflow-engine .workflow-more-tools {", workflow_styles)
        self.assertIn("ViewEncapsulation.Emulated", workflow_component)
        self.assertNotIn("ViewEncapsulation.None", workflow_component)

    def test_pursuits_disclosure_styles_load_with_the_lazy_authenticated_shell(self) -> None:
        global_styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        shell_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "control-room"
            / "app-shell.component.scss"
        ).read_text(encoding="utf-8")
        pursuit_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "pursuits"
            / "pursuits.component.scss"
        ).read_text(encoding="utf-8")
        pursuit_component = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "pursuits"
            / "pursuits.component.ts"
        ).read_text(encoding="utf-8")
        shell_component = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "control-room"
            / "app-shell.component.ts"
        ).read_text(encoding="utf-8")

        self.assertNotIn(
            "/* Pursuits uses the same calm, progressive-disclosure pattern", global_styles
        )
        self.assertNotIn("app-pursuits .pursuit-health", pursuit_styles)
        self.assertIn("details.pursuit-health", shell_styles)
        self.assertIn("details.route-intake-panel", shell_styles)
        self.assertIn("encapsulation: ViewEncapsulation.None", shell_component)
        self.assertIn("ViewEncapsulation.Emulated", pursuit_component)
        self.assertNotIn("ViewEncapsulation.None", pursuit_component)

    def test_pursuit_portfolio_planner_styles_load_with_the_lazy_module(self) -> None:
        global_styles = (ROOT / "frontend" / "src" / "styles.scss").read_text(
            encoding="utf-8"
        )
        pursuit_styles = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "pursuits"
            / "pursuits.component.scss"
        ).read_text(encoding="utf-8")

        # The planner opens only from the lazy Pursuits route. Angular retains
        # the component's scoped attributes when the content is rendered in the
        # NG-Zorro overlay, so it remains styled without leaking route rules.
        self.assertNotIn(".portfolio-planner {", global_styles)
        self.assertIn(".portfolio-planner {", pursuit_styles)
        self.assertIn(".portfolio-workflow-settlement__usage", pursuit_styles)

    def test_component_style_budget_allows_a_deferred_complex_workspace(self) -> None:
        angular = json.loads(
            (ROOT / "frontend" / "angular.json").read_text(encoding="utf-8")
        )
        budgets = angular["projects"]["app"]["architect"]["build"][
            "configurations"
        ]["production"]["budgets"]
        component_budget = next(
            budget for budget in budgets if budget["type"] == "anyComponentStyle"
        )

        # Route styles are loaded on demand. The portfolio workspace is an
        # intentional advanced surface, not a reason to ship its CSS in the
        # initial bundle. Keep a ceiling to catch accidental style growth.
        self.assertEqual(component_budget["maximumWarning"], "20kb")
        self.assertEqual(component_budget["maximumError"], "48kb")

    def test_frontend_copies_only_runtime_icon_assets(self) -> None:
        angular = json.loads(
            (ROOT / "frontend" / "angular.json").read_text(encoding="utf-8")
        )
        assets = angular["projects"]["app"]["architect"]["build"]["options"][
            "assets"
        ]
        icon_asset = next(
            asset
            for asset in assets
            if isinstance(asset, dict)
            and asset.get("input")
            == "./node_modules/@ant-design/icons-angular/src/inline-svg/"
        )

        # The package directory also contains source JavaScript modules. They
        # are not browser assets and publishing them inflates every Windows
        # installer and container image without helping nz-icon resolve SVGs.
        self.assertEqual(icon_asset["glob"], "**/*.svg")

    def test_canonical_service_runtime_images_do_not_float_on_latest(
        self,
    ) -> None:
        for relative_path in (
            "backend/Dockerfile",
            "idp/Dockerfile",
            "nginx-config-manager/Dockerfile",
        ):
            with self.subTest(path=relative_path):
                dockerfile = (ROOT / relative_path).read_text(encoding="utf-8")
                self.assertNotRegex(
                    dockerfile,
                    r"(?m)^FROM\s+\S+:latest(?:\s|$)",
                )
                self.assertIn("FROM ubuntu:24.04", dockerfile)

    def test_local_compose_uses_one_lightweight_kafka_protocol_broker(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        self.assertIn("docker.redpanda.com/redpandadata/redpanda:", compose)
        self.assertNotIn("confluentinc/cp-zookeeper", compose)
        self.assertNotIn("confluentinc/cp-kafka", compose)
        self.assertNotIn("  zookeeper:\n", compose)
        self.assertNotIn("  kafka-network:\n", compose)
        self.assertIn("mem_limit: ${KAFKA_MEMORY_LIMIT:-256m}", compose)
        self.assertIn("cpus: ${KAFKA_CPU_LIMIT:-0.5}", compose)
        self.assertIn("--overprovisioned=true", compose)

    def test_default_compose_starts_only_the_core_local_stack(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        services_section = compose.split("services:\n", 1)[1].split(
            "\nnetworks:\n", 1
        )[0]
        services = re.findall(r"(?m)^  ([A-Za-z0-9_-]+):\n", services_section)
        core_services = {
            "idp",
            "backend",
            "backend-migrate",
            "backend-runtime-role",
            "frontend",
            "nginx",
            "postgres-idp",
            "postgres-automation",
            "redis",
        }

        self.assertEqual(set(services) & core_services, core_services)
        optional_services = set(services) - core_services
        self.assertTrue(optional_services)
        for service in optional_services:
            with self.subTest(service=service):
                self.assertIn(
                    "profiles:", compose_service_block(compose, service),
                    "optional integrations must never join the default local startup",
                )

    def test_local_compose_bounds_the_always_on_desktop_services(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")

        for service, prefix in {
            "idp": "IDP",
            "backend": "BACKEND",
            "frontend": "FRONTEND",
            "nginx": "GATEWAY",
            "nginxconfigmanager": "NGINX_CONFIG_MANAGER",
            "postgres-idp": "POSTGRES_IDP",
            "postgres-automation": "POSTGRES_AUTOMATION",
            "redis": "REDIS",
        }.items():
            with self.subTest(service=service):
                service_block = compose_service_block(compose, service)
                self.assertIn(f"mem_limit: ${{{prefix}_MEMORY_LIMIT:-", service_block)
                self.assertIn(f"cpus: ${{{prefix}_CPU_LIMIT:-", service_block)
                self.assertIn(f"pids_limit: ${{{prefix}_PIDS_LIMIT:-", service_block)
                self.assertIn(f"{prefix}_MEMORY_LIMIT=", defaults)
                self.assertIn(f"{prefix}_CPU_LIMIT=", defaults)
                self.assertIn(f"{prefix}_PIDS_LIMIT=", defaults)

    def test_backend_runtime_is_immutable_and_uses_separate_runtime_db_settings(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        backend = compose_service_block(compose, "backend")

        # The API must never regain broad host or kernel privileges merely
        # because an execution adapter is configured. Its writable locations
        # are explicit mounts, while the image root remains immutable.
        self.assertIn("read_only: true", backend)
        self.assertIn("/tmp:rw,noexec,nosuid,size=64m", backend)
        self.assertIn("no-new-privileges:true", backend)
        self.assertIn("cap_drop:\n      - ALL", backend)

        # Existing local environments fall back to DB_USER/DB_PASSWORD, but a
        # hardened deployment can configure a DML-only API account and disable
        # startup migrations after the owner has applied them.
        self.assertIn("DB_USER: ${BACKEND_DB_USER:-hai_runtime}", backend)
        self.assertIn("DB_PASSWORD: ${BACKEND_DB_PASSWORD:-${DB_PASSWORD}}", backend)
        self.assertIn("DB_MIGRATIONS_ENABLED: ${DB_MIGRATIONS_ENABLED:-true}", backend)
        self.assertIn("backend-migrate:\n        condition: service_completed_successfully", backend)
        self.assertIn("backend-runtime-role:\n        condition: service_completed_successfully", backend)
        for setting in ("BACKEND_DB_USER=", "BACKEND_DB_PASSWORD=", "DB_MIGRATIONS_ENABLED="):
            self.assertIn(setting, defaults)

        migration = compose_service_block(compose, "backend-migrate")
        runtime_role = compose_service_block(compose, "backend-runtime-role")
        role_script = (ROOT / "services" / "postgres-runtime-role" / "provision-runtime-role.sh").read_text(encoding="utf-8")
        self.assertIn('command: ["/root/app", "migrate", "up"]', migration)
        self.assertIn("DB_USER: ${DB_USER}", migration)
        self.assertIn("DB_MIGRATIONS_ENABLED: \"false\"", migration)
        self.assertIn("backend-migrate:\n        condition: service_completed_successfully", runtime_role)
        self.assertIn("HAI_RUNTIME_DB_USER: ${BACKEND_DB_USER:-hai_runtime}", runtime_role)
        self.assertIn("HAI_RUNTIME_DB_PASSWORD: ${BACKEND_DB_PASSWORD:-${DB_PASSWORD}}", runtime_role)
        for service, prefix in ((migration, "BACKEND_MIGRATE"), (runtime_role, "BACKEND_RUNTIME_ROLE")):
            self.assertIn(f"mem_limit: ${{{prefix}_MEMORY_LIMIT:-", service)
            self.assertIn(f"cpus: ${{{prefix}_CPU_LIMIT:-", service)
            self.assertIn(f"pids_limit: ${{{prefix}_PIDS_LIMIT:-", service)
            self.assertIn(f"{prefix}_MEMORY_LIMIT=", defaults)
            self.assertIn(f"{prefix}_CPU_LIMIT=", defaults)
            self.assertIn(f"{prefix}_PIDS_LIMIT=", defaults)
        for required in ("NOSUPERUSER", "NOCREATEDB", "NOCREATEROLE", "ALTER DEFAULT PRIVILEGES", "GRANT SELECT, INSERT, UPDATE, DELETE"):
            self.assertIn(required, role_script)

    def test_gateway_waits_for_ready_control_plane_dependencies(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        gateway = compose_service_block(compose, "nginx")

        # The browser gateway is the first endpoint a local or ngrok user sees.
        # Starting it before the IDP or API is healthy creates an intermittent
        # blank/login failure window even though Compose eventually recovers.
        self.assertIn(
            "backend:\n        condition: service_healthy",
            gateway,
        )
        self.assertIn(
            "idp:\n        condition: service_healthy",
            gateway,
        )

        a2a_gateway = compose_service_block(compose, "a2a-gateway")
        self.assertIn(
            "backend:\n        condition: service_healthy",
            a2a_gateway,
        )

    def test_optional_ngrok_tunnel_is_profiled_and_fail_closed(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        launcher = (ROOT / "scripts" / "start-ngrok.ps1")
        entrypoint = ROOT / "deploy" / "ngrok" / "start-ngrok.sh"

        service = compose_service_block(compose, "ngrok")
        self.assertIn('profiles: ["cloud-tunnel"]', service)
        self.assertNotIn("ports:", service)
        self.assertIn("read_only: true", service)
        self.assertIn("no-new-privileges:true", service)
        self.assertIn("cap_drop:", service)
        self.assertIn("HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED", service)
        self.assertIn("RATE_LIMIT_PER_MINUTE", service)
        self.assertIn("NGROK_AUTHTOKEN=", defaults)
        self.assertIn("HAI_NGROK_URL=", defaults)
        self.assertIn("HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED=false", defaults)
        self.assertTrue(launcher.is_file())
        self.assertTrue(entrypoint.is_file())

        launcher_text = launcher.read_text(encoding="utf-8")
        entrypoint_text = entrypoint.read_text(encoding="utf-8")
        for content in (launcher_text, entrypoint_text):
            self.assertIn("RUN_MODE", content)
            self.assertIn("LOCAL_LOGIN_BYPASS_ENABLED", content)
            self.assertIn("IDP_COOKIE_SECURE", content)
            self.assertIn("GATEWAY_HOST_BIND", content)
            self.assertIn("HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED", content)
            self.assertIn("RATE_LIMIT_PER_MINUTE", content)
            self.assertIn("NGROK_AUTHTOKEN", content)
            self.assertIn("HAI_NGROK_URL", content)
            self.assertIn("GOOGLE_LOGIN_REDIRECT_URL", content)
            self.assertIn("GOOGLE_OAUTH_REDIRECT_URL", content)

        self.assertIn("GOOGLE_LOGIN_REDIRECT_URL: ${GOOGLE_LOGIN_REDIRECT_URL:-}", service)
        self.assertIn("GOOGLE_OAUTH_REDIRECT_URL: ${GOOGLE_OAUTH_REDIRECT_URL:-}", service)
        self.assertIn("/api/v1/auth/google/callback", launcher_text)
        self.assertIn("/api/v1/sources/oauth/google/callback", launcher_text)
        self.assertIn("/api/v1/auth/google/callback", entrypoint_text)
        self.assertIn("/api/v1/sources/oauth/google/callback", entrypoint_text)

    def test_optional_event_bus_does_not_expand_the_default_local_stack(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")

        kafka = compose_service_block(compose, "kafka")
        config_manager = compose_service_block(compose, "nginxconfigmanager")
        backend = compose_service_block(compose, "backend")
        idp = compose_service_block(compose, "idp")

        self.assertIn('profiles: ["event-bus"]', kafka)
        self.assertIn('profiles: ["event-bus"]', config_manager)
        self.assertNotIn("      kafka:\n", backend)
        self.assertNotIn("      kafka:\n", idp)
        self.assertIn("HAI_EVENT_BUS_ENABLED=false", defaults)

    def test_local_a2a_connector_is_loopback_only_and_not_on_the_cloud_gateway(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        connector_template = ROOT / "nginx-config" / "a2a-local.conf.template"

        service = compose_service_block(compose, "a2a-gateway")
        self.assertIn('profiles: ["local-a2a"]', service)
        self.assertIn('"127.0.0.1:${HAI_A2A_LOCAL_PORT:-8091}:80"', service)
        self.assertIn("- a2a-local", service)
        self.assertNotIn("- service-hub", service)
        self.assertIn("a2a-local:", compose)
        self.assertIn("internal: true", compose)
        self.assertIn("HAI_A2A_LOCAL_PORT=8091", defaults)
        backend_service = compose_service_block(compose, "backend")
        self.assertIn(
            "HAI_A2A_BRIDGE_URL: ${HAI_A2A_BRIDGE_URL:-http://127.0.0.1:8091/api/v1/a2a}",
            backend_service,
        )
        self.assertTrue(connector_template.is_file())

        template = connector_template.read_text(encoding="utf-8")
        self.assertIn("location = /.well-known/agent-card.json", template)
        self.assertIn("location = /api/v1/a2a", template)
        self.assertIn("X-HAI-Backend-Key", template)
        self.assertIn("return 404", template)
        self.assertNotIn("location /api/v1", template)

    def test_default_connected_source_allowlist_enables_live_trello(self) -> None:
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        match = re.search(
            r"^CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS=(.+)$",
            defaults,
            re.MULTILINE,
        )
        self.assertIsNotNone(match)
        hosts = {host.strip().lower() for host in match.group(1).split(",")}
        self.assertIn("api.trello.com", hosts)
        for setting in ("TRELLO_API_KEY", "TRELLO_READ_TOKEN", "TRELLO_API_BASE_URL"):
            with self.subTest(setting=setting):
                self.assertIn(f"{setting}=", defaults)
                self.assertIn(f"{setting}: ${{{setting}:-}}", compose)

    def test_google_oauth_callback_uses_signed_state_not_session_rbac(self) -> None:
        routes = (ROOT / "backend" / "internal" / "router" / "routes.go").read_text(
            encoding="utf-8"
        )

        # The provider returns through a browser navigation that may not carry
        # HAI's session cookie. Start remains permission-gated; the callback
        # must reach the source service so its signed, short-lived OAuth state
        # can be verified before any code is exchanged.
        self.assertIn(
            'sourceOAuth.GET("/oauth/google/start", requirePermission(rbac.PermWrite), sourceHandler.StartGoogleOAuth)',
            routes,
        )
        self.assertIn(
            'sourceOAuth.GET("/oauth/google/callback", sourceHandler.GoogleOAuthCallback)',
            routes,
        )
        self.assertNotIn(
            'sourceOAuth.GET("/oauth/google/callback", requirePermission(', routes
        )

    def test_command_dashboard_names_all_registered_controlled_runtimes(self) -> None:
        template = (
            ROOT
            / "frontend"
            / "src"
            / "app"
            / "pages"
            / "command-dashboard"
            / "command-dashboard.component.html"
        ).read_text(encoding="utf-8")

        # The registry exposes four controlled runtime families. Do not leave
        # DeepSeek Harness invisible in the operator-facing summary merely
        # because OpenClaw has additional ecosystem-inspection controls.
        self.assertIn("Hermes, DeepSeek Harness, Odysseus, and OpenClaw", template)

    def test_documented_provider_and_local_observability_settings_reach_backend(self) -> None:
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        backend = compose_service_block(compose, "backend")

        for setting in (
            "NOUS_PORTAL_BASE_URL",
            "NOUS_PORTAL_API_KEY",
            "MIXTURE_OF_AGENTS_BASE_URL",
            "MIXTURE_OF_AGENTS_API_KEY",
            "OPENAI_CODEX_BASE_URL",
            "OPENAI_CODEX_API_KEY",
            "HAI_LANGFUSE_ENABLED",
            "HAI_LANGFUSE_BASE_URL",
            "HAI_LANGFUSE_PUBLIC_KEY",
            "HAI_LANGFUSE_SECRET_KEY",
            "HAI_LANGFUSE_TIMEOUT_SECONDS",
            "DB_AUTOMIGRATE",
            "LM_STUDIO_MODEL_ID",
            "SGLANG_BASE_URL",
            "SGLANG_MODEL_ID",
            "DSPARK_ENABLED",
            "DSPARK_BASE_URL",
            "DSPARK_PROBE_PATH",
            "DSPARK_GENERATION_PATH",
            "DSPARK_MODEL_ID",
            "SOURCE_SCHEDULER_DURABLE",
            "SOURCE_WORKER_POLL_SECONDS",
            "WORKFLOW_SCHEDULER_DURABLE",
            "WORKFLOW_WORKER_POLL_SECONDS",
            "WORKFLOW_REMINDER_DELIVERY_ENABLED",
            "AMBIENT_SCHEDULER_DURABLE",
            "AMBIENT_WORKER_POLL_SECONDS",
            "WHATSAPP_EXPORT_CHUNK_MESSAGES",
            "HAI_CATALOG_REVALIDATION_ENABLED",
            "HAI_CATALOG_COLLECTION_REVALIDATION_ENABLED",
            "HAI_CATALOG_REPOSITORY_DISCOVERY_REVALIDATION_ENABLED",
            "HAI_CATALOG_REVALIDATION_INTERVAL_HOURS",
            "HAI_CATALOG_REVALIDATION_BATCH_SIZE",
            "HAI_CATALOG_REVALIDATION_SCHEDULER_ENABLED",
            "HAI_CATALOG_REVALIDATION_SCHEDULER_INTERVAL_MINUTES",
            "LLM_PROVIDER_PROBE_TIMEOUT_SECONDS",
            "OPENCLAW_ECOSYSTEM_ALLOWED_ROOTS",
            "DEEPSEEK_HARNESS_ENABLED",
            "DEEPSEEK_HARNESS_EXECUTION_ENABLED",
            "DEEPSEEK_HARNESS_EXECUTABLE",
            "DEEPSEEK_HARNESS_WORKSPACE",
            "DEEPSEEK_HARNESS_STATE_DIR",
            "DEEPSEEK_HARNESS_TIMEOUT_SECONDS",
            "DEEPSEEK_HARNESS_ENV_ALLOWLIST",
        ):
            with self.subTest(setting=setting):
                self.assertIn(f"{setting}=", defaults)
                self.assertIn(f"{setting}: ${{{setting}", backend)

    def test_provider_fixture_is_opt_in_and_has_no_host_power(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        fixture = compose_service_block(compose, "provider-fixture")

        self.assertIn('profiles: ["provider-fixture"]', fixture)
        self.assertIn("target: provider-fixture", fixture)
        self.assertIn("read_only: true", fixture)
        self.assertIn("mem_limit: 32m", fixture)
        self.assertIn("cpus: 0.10", fixture)
        self.assertIn("pids_limit: 32", fixture)
        self.assertIn("- ALL", fixture)
        self.assertIn("no-new-privileges:true", fixture)
        self.assertIn('"11434"', fixture)
        self.assertNotIn("ports:", fixture)
        self.assertIn("--healthcheck", fixture)

    def test_ci_runs_provider_fixture_http_contract(self) -> None:
        fixture_job = job_block("provider-fixture")
        smoke = ROOT / "scripts" / "smoke-provider-fixture.sh"

        self.assertTrue(smoke.is_file())
        smoke_text = smoke.read_text(encoding="utf-8")
        self.assertIn("bash scripts/smoke-provider-fixture.sh", fixture_job)
        self.assertIn("--target provider-fixture", smoke_text)
        self.assertIn("docker network create --internal", smoke_text)

    def test_phase2_control_state_and_safe_worker_paths_are_durable(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        backend = compose_service_block(compose, "backend")

        # The background control service persists emergency-stop and autonomy
        # decisions. Its safe-worker output and opt-in feed imports must use
        # distinct paths so a restart neither resets control state nor lets the
        # worker modify its read-only intake folder.
        for setting, value in (
            ("HAI_PHASE2_WORKSPACE_DIR", "/root/agent-workspaces/phase2"),
            ("HAI_PHASE2_FEEDS_DIR", "/root/phase2-feeds"),
            ("HAI_PHASE2_STATE_DIR", "/root/phase2-control-state"),
            ("HAI_PHASE2_FEED_FILES", ""),
        ):
            with self.subTest(setting=setting):
                self.assertIn(f"{setting}=", defaults)
                self.assertIn(f"{setting}: ${{{setting}:-{value}}}", backend)

        self.assertIn(
            "- phase2-control-state:/root/phase2-control-state",
            backend,
        )
        self.assertIn("- ./phase2-feeds:/root/phase2-feeds:ro", backend)
        self.assertIn("phase2-control-state:\n    name: 018-hai-phase2-control-state", compose)

    def test_quick_start_and_google_local_callback_match_gateway_port(self) -> None:
        defaults = (ROOT / ".env.example").read_text(encoding="utf-8")
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        self.assertIn("GATEWAY_HOST_PORT=8088", defaults)
        self.assertIn("http://localhost:8088/api/v1/sources/oauth/google/callback", defaults)
        self.assertIn("http://localhost:8088", readme)

    def test_directly_invoked_contract_and_smoke_files_exist(self) -> None:
        for relative_path in (
            "nginx-config/test_gateway_contract.py",
            "scripts/test_ci_contract.py",
            "scripts/test_smoke_auth_contract.py",
            "scripts/smoke-all.sh",
            "scripts/two-account-isolation-test.sh",
        ):
            with self.subTest(path=relative_path):
                self.assertTrue((ROOT / relative_path).is_file())

    def test_execution_boundary_race_tests_are_not_served_from_test_cache(
        self,
    ) -> None:
        backend = job_block("backend")
        self.assertIn(
            "go test -count=1 -race ./internal/automation ./internal/task ./internal/source ./internal/llm ./internal/agentruntime ./internal/durablejob",
            backend,
        )

    def test_backend_vulnerability_scan_is_pinned_and_blocking(self) -> None:
        backend = job_block("backend")
        self.assertIn(
            "go install golang.org/x/vuln/cmd/govulncheck@v1.6.0",
            backend,
        )
        self.assertIn("govulncheck ./...", backend)
        self.assertNotIn("continue-on-error", backend)

    def test_idp_vulnerability_scan_is_pinned_and_blocking(self) -> None:
        idp = job_block("idp")
        self.assertIn(
            "go install golang.org/x/vuln/cmd/govulncheck@v1.6.0",
            idp,
        )
        self.assertIn("govulncheck ./...", idp)
        self.assertNotIn("continue-on-error", idp)

    def test_nginx_manager_toolchain_and_scan_match_container(self) -> None:
        go_mod = (ROOT / "nginx-config-manager" / "go.mod").read_text(
            encoding="utf-8"
        )
        dockerfile = (ROOT / "nginx-config-manager" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        job = job_block("nginx-config-manager")

        recommended = re.search(
            r"^toolchain\s+go(\d+\.\d+\.\d+)$",
            go_mod,
            re.MULTILINE,
        )
        container = re.search(
            r"^FROM\s+golang:(\d+\.\d+\.\d+)\s+AS\s+builder$",
            dockerfile,
            re.MULTILINE,
        )
        self.assertIsNotNone(recommended)
        self.assertIsNotNone(container)
        self.assertEqual(recommended.group(1), container.group(1))
        self.assertIn(f'go-version: "{recommended.group(1)}"', job)
        self.assertIn("- run: go vet ./...", job)
        self.assertIn(
            "go install golang.org/x/vuln/cmd/govulncheck@v1.6.0",
            job,
        )
        self.assertIn("govulncheck ./...", job)
        self.assertNotIn("continue-on-error", job)

    def test_nginx_manager_has_no_docker_socket_control_path(self) -> None:
        go_mod = (ROOT / "nginx-config-manager" / "go.mod").read_text(
            encoding="utf-8"
        )
        service = (
            ROOT
            / "nginx-config-manager"
            / "internal"
            / "app"
            / "autoconfig"
            / "auto_config_service.go"
        ).read_text(encoding="utf-8")
        self.assertNotIn("github.com/docker/docker", go_mod)
        self.assertNotIn("github.com/docker/docker", service)
        self.assertIn("Docker socket control is disabled", service)
        for compose_file in ("docker-compose.local.yml", "docker-compose.yml"):
            with self.subTest(compose_file=compose_file):
                content = (ROOT / compose_file).read_text(encoding="utf-8")
                self.assertNotIn("/var/run/docker.sock", content)

    def test_all_compose_entry_points_delegate_to_the_source_built_stack(self) -> None:
        expected = {
            "docker-compose.yml": "./docker-compose.local.yml",
            "backend/docker-compose.yml": "../docker-compose.local.yml",
            "idp/docker-compose.yml": "../docker-compose.local.yml",
            "gate/docker-compose.yml": "../docker-compose.local.yml",
        }
        for compose_file, local_stack_path in expected.items():
            with self.subTest(compose_file=compose_file):
                content = (ROOT / compose_file).read_text(encoding="utf-8")
                self.assertIn("include:", content)
                self.assertIn(local_stack_path, content)
                self.assertNotIn("jacksonbarreto/", content)

    def test_cross_platform_secret_generator_covers_every_required_backend_key(self) -> None:
        generator = (ROOT / "scripts" / "generate-secrets.sh").read_text(encoding="utf-8")
        for key in (
            "BACKEND_API_SHARED_KEY",
            "HAI_MEMORY_ENCRYPTION_KEY",
            "JWT_SECRET",
            "HAI_APPROVAL_PROOF_SIGNING_KEY",
            "DB_PASSWORD",
            "FIRST_RUN_ADMIN_PASSWORD",
        ):
            with self.subTest(key=key):
                self.assertIn(f"{key}=$(secret)", generator)

    def test_idp_toolchain_matches_ci_and_container(self) -> None:
        go_mod = (ROOT / "idp" / "go.mod").read_text(encoding="utf-8")
        dockerfile = (ROOT / "idp" / "Dockerfile").read_text(encoding="utf-8")
        idp = job_block("idp")

        language = re.search(r"^go\s+(\d+\.\d+)(?:\.\d+)?$", go_mod, re.MULTILINE)
        recommended = re.search(
            r"^toolchain\s+go(\d+\.\d+\.\d+)$",
            go_mod,
            re.MULTILINE,
        )
        container = re.search(
            r"^FROM\s+golang:(\d+\.\d+\.\d+)\s+AS\s+builder$",
            dockerfile,
            re.MULTILINE,
        )
        ci = re.search(r'go-version:\s+"(\d+\.\d+\.\d+)"', idp)

        self.assertIsNotNone(language)
        self.assertIsNotNone(recommended)
        self.assertIsNotNone(container)
        self.assertIsNotNone(ci)
        self.assertEqual(recommended.group(1), container.group(1))
        self.assertEqual(recommended.group(1), ci.group(1))
        self.assertEqual(
            ".".join(recommended.group(1).split(".")[:2]),
            language.group(1),
        )
        self.assertIn("- run: go vet ./...", idp)

    def test_backend_toolchain_matches_ci_and_container(self) -> None:
        go_mod = (ROOT / "backend" / "go.mod").read_text(encoding="utf-8")
        dockerfile = (ROOT / "backend" / "Dockerfile").read_text(
            encoding="utf-8"
        )

        language = re.search(r"^go\s+(\d+\.\d+)(?:\.\d+)?$", go_mod, re.MULTILINE)
        recommended = re.search(
            r"^toolchain\s+go(\d+\.\d+\.\d+)$",
            go_mod,
            re.MULTILINE,
        )
        container = re.search(
            r"^FROM\s+golang:(\d+\.\d+\.\d+)\s+AS\s+builder$",
            dockerfile,
            re.MULTILINE,
        )

        self.assertIsNotNone(language)
        self.assertIsNotNone(recommended)
        self.assertIsNotNone(container)
        self.assertEqual(recommended.group(1), container.group(1))
        self.assertEqual(
            ".".join(recommended.group(1).split(".")[:2]),
            language.group(1),
        )
        for job_id in (
            "backend",
            "authenticated-smoke",
            "migrations-integration",
            "isolation-acceptance",
        ):
            with self.subTest(job=job_id):
                self.assertIn(
                    f'go-version: "{recommended.group(1)}"',
                    job_block(job_id),
                )

    def test_authenticated_smoke_requires_each_suite_result(self) -> None:
        smoke = job_block("authenticated-smoke")
        self.assertIn(
            'smoke_log="$RUNNER_TEMP/authenticated-smoke.log"',
            smoke,
        )
        for suite in (
            "smoke-background-operations",
            "smoke-model-intelligence",
            "smoke-runtime-lab",
            "smoke-account-bridges",
            "smoke-windows-runtime",
        ):
            with self.subTest(suite=suite):
                self.assertIn(suite, smoke)
        self.assertIn(
            r'grep -Eq "^  PASS  ${suite}  \(Result: [1-9][0-9]* passed, 0 failed\)$"',
            smoke,
        )
        self.assertIn(
            "grep -qx '==> ALL PHASE 2 SMOKE SUITES PASSED'",
            smoke,
        )

    def test_browser_acceptance_uses_an_isolated_real_compose_stack(self) -> None:
        acceptance = job_block("browser-acceptance")
        for contract in (
            'name: Browser acceptance (real Compose stack)',
            'COMPOSE_PROJECT_NAME": "hai-browser-acceptance"',
            'RUN_MODE": "production"',
            'LOCAL_LOGIN_BYPASS_ENABLED": "false"',
            'docker compose --env-file .env.browser-acceptance -f docker-compose.local.yml up -d --build --wait --wait-timeout 240',
            'http://127.0.0.1:8080/readyz',
            'if status != "ready"',
            'E2E_BASE_URL: http://127.0.0.1:8080',
            'E2E_OPERATOR_EMAIL: e2e-owner@example.test',
            'E2E_ALLOW_MUTATION: "true"',
            'npx playwright install --with-deps chromium',
            'run: npm test',
            'down -v --remove-orphans',
        ):
            with self.subTest(contract=contract):
                self.assertIn(contract, acceptance)
        self.assertNotIn("LOCAL_LOGIN_BYPASS_ENABLED\": \"true\"", acceptance)
        self.assertNotIn('status not in {"ready", "degraded"}', acceptance)

    def test_browser_acceptance_environment_file_is_not_trackable(self) -> None:
        gitignore = (ROOT / ".gitignore").read_text(encoding="utf-8")
        self.assertIn(
            ".env.browser-acceptance",
            gitignore,
            "a local reproduction of browser acceptance must not expose its generated credentials to git status",
        )

    def test_browser_acceptance_requires_an_explicit_mutation_opt_in(self) -> None:
        acceptance_test = (
            ROOT / "frontend" / "e2e" / "tests" / "acceptance.spec.ts"
        ).read_text(encoding="utf-8")
        self.assertIn(
            "process.env.E2E_ALLOW_MUTATION === 'true'", acceptance_test
        )
        self.assertIn("!password || !allowMutation", acceptance_test)
        self.assertIn("only for an isolated acceptance stack", acceptance_test)

    def test_postgres_jobs_cannot_silently_skip_or_match_no_tests(self) -> None:
        migrations = job_block("migrations-integration")
        for contract in (
            "hai_migration_runner_test",
            "hai_framework_registry_test",
            "hai_task_state_test",
            "hai_agent_registry_test",
            "createdb",
            'HAI_ALLOW_DESTRUCTIVE_DATABASE_TESTS: "true"',
            'HAI_REQUIRE_POSTGRES_INTEGRATION: "true"',
            'HAI_TEST_DATABASE_DSN="$migration_dsn" go test -count=1 -tags integration',
            'HAI_TEST_DATABASE_DSN="$registry_dsn" go test -count=1 -tags integration',
            'HAI_TEST_DATABASE_DSN="$task_dsn" go test -count=1 -tags integration',
            "^--- PASS: TestRunMigrationsAppliesAndIsIdempotent",
            "^--- PASS: TestRollbackMigrationReversesPostMigration",
            "^--- PASS: TestConcurrentMigrationRunnersSerializeAndRecheck",
            "^--- PASS: TestLegacyBaselineRejectsDifferentExistingPrimaryKey",
            "^--- PASS: TestFrameworkRegistryPostgresIntegrationRequiredEnvironment",
            "^--- PASS: TestFrameworkRegistryPostgresMigrationApplyRollbackAndRerun",
            "^--- PASS: TestFrameworkRegistryPostgresConstraintsAndImmutability",
            "^--- PASS: TestPostgresTaskStateRepositoryDurabilityOwnerScopeAndImmutability",
            "^--- PASS: TestPostgresRepositoryRoundTripOwnerIsolationCASAndImmutableLedgers",
            "^--- PASS: TestPostgresAgentRegistryMigrationCanReplayAgainstExistingSchema",
        ):
            with self.subTest(contract=contract):
                self.assertIn(contract, migrations)
        self.assertIn(
            "-run '^TestPostgresTaskStateRepositoryDurabilityOwnerScopeAndImmutability$'",
            migrations,
        )
        database_assignments = dict(
            re.findall(
                r'^\s*(migration|registry|task|agent_registry)_dsn="[^"]*dbname=([^ "\n]+)',
                migrations,
                re.MULTILINE,
            )
        )
        self.assertEqual(
            database_assignments,
            {
                "migration": "hai_migration_runner_test",
                "registry": "hai_framework_registry_test",
                "task": "hai_task_state_test",
                "agent_registry": "hai_agent_registry_test",
            },
        )
        self.assertEqual(len(set(database_assignments.values())), 4)

    def test_running_stack_must_be_live_before_acceptance_test(self) -> None:
        isolation = job_block("isolation-acceptance")
        for contract in (
            'backend_live=""',
            'if ! kill -0 "$(cat backend.pid)" 2>/dev/null; then',
            '[ -n "${backend_live}" ] || {',
            'readyz_http="$(curl -sS -o readyz.json',
            '[ "${readyz_http}" = "200" ] || {',
            'status not in {"ready", "degraded"}',
        ):
            with self.subTest(contract=contract):
                self.assertIn(contract, isolation)
        self.assertNotIn(
            "curl -s http://localhost:8080/readyz || true",
            isolation,
        )

    def test_windows_contract_executes_from_a_space_containing_path(self) -> None:
        windows = job_block("windows-contract")
        for contract in (
            "runs-on: windows-latest",
            r'C:\Program Files\Git\bin\bash.exe',
            '"HAI smoke path with spaces"',
            "cygpath -u",
            'python3() { python "$@"; }',
            "hai_smoke_mint_jwt owner ci-secret windows-owner",
            "python scripts/test_ci_contract.py",
            "python scripts/test_smoke_auth_contract.py",
        ):
            with self.subTest(contract=contract):
                self.assertIn(contract, windows)

    def test_smoke_aggregator_rejects_zero_or_missing_assertions(self) -> None:
        aggregator = (ROOT / "scripts" / "smoke-all.sh").read_text(
            encoding="utf-8"
        )
        self.assertIn(
            r"grep -E '^==> Result: [1-9][0-9]* passed, 0 failed$'",
            aggregator,
        )
        self.assertIn(
            'if [ "${code}" -eq 0 ] && [ -n "${valid_line}" ]; then',
            aggregator,
        )
        self.assertIn(
            'line="${reported_line:-==> Result: missing or invalid}"',
            aggregator,
        )

    def test_ci_never_uploads_generated_runtime_or_secret_artifacts(self) -> None:
        self.assertIn("actions/upload-artifact@v4", WORKFLOW)
        self.assertIn("name: hai-windows-installer", WORKFLOW)
        self.assertIn("path: installer/release/HAI-Setup-*.exe", WORKFLOW)
        self.assertNotIn("installer/release/payload", WORKFLOW)
        self.assertNotIn("payload-manifest.json", WORKFLOW)
        self.assertNotRegex(WORKFLOW, r"(?i)\bupload[\w -]*(?:log|env|secret)")

    def test_windows_installer_ci_compiles_the_distributable(self) -> None:
        installer = job_block("windows-installer")
        for contract in (
            "runs-on: windows-latest",
            "choco install innosetup --yes --no-progress",
            "build-windows-installer.ps1 -Version",
            "actions/upload-artifact@v4",
            "retention-days: 14",
        ):
            with self.subTest(contract=contract):
                self.assertIn(contract, installer)

    def test_every_job_has_an_explicit_timeout(self) -> None:
        for job_id in (
            "windows-installer",
            "backend",
            "idp",
            "nginx-config-manager",
            "frontend",
            "browser-acceptance",
            "compose",
            "authenticated-smoke",
            "migrations-integration",
            "isolation-acceptance",
            "windows-contract",
        ):
            with self.subTest(job=job_id):
                self.assertIn("timeout-minutes:", job_block(job_id))


if __name__ == "__main__":
    unittest.main()
