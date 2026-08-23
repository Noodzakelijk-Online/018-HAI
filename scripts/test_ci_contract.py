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
    def test_frontend_uses_one_direct_angular_cdk_dependency(self) -> None:
        frontend = ROOT / "frontend"
        package = json.loads((frontend / "package.json").read_text(encoding="utf-8"))
        package_lock = (frontend / "package-lock.json").read_text(encoding="utf-8")
        pnpm_lock = (frontend / "pnpm-lock.yaml").read_text(encoding="utf-8")

        self.assertIn("@angular/cdk", package["dependencies"])
        self.assertNotIn("angular-mixed-cdk-drag-drop", package["dependencies"])
        self.assertNotIn("angular-mixed-cdk-drag-drop", package_lock)
        self.assertNotIn("angular-mixed-cdk-drag-drop", pnpm_lock)

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
        self.assertIn("app-control-center{--bg", control_center_styles)
        self.assertIn(".operations-shell", control_center_styles)
        self.assertIn("ViewEncapsulation.None", control_center_component)

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
            self.assertIn("NGROK_AUTHTOKEN", content)
            self.assertIn("HAI_NGROK_URL", content)

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
            "AMBIENT_SCHEDULER_DURABLE",
            "AMBIENT_WORKER_POLL_SECONDS",
            "WHATSAPP_EXPORT_CHUNK_MESSAGES",
            "HAI_CATALOG_REVALIDATION_ENABLED",
            "HAI_CATALOG_REVALIDATION_INTERVAL_HOURS",
            "HAI_CATALOG_REVALIDATION_BATCH_SIZE",
            "HAI_CATALOG_REVALIDATION_SCHEDULER_ENABLED",
            "HAI_CATALOG_REVALIDATION_SCHEDULER_INTERVAL_MINUTES",
            "OPENCLAW_ECOSYSTEM_ALLOWED_ROOTS",
        ):
            with self.subTest(setting=setting):
                self.assertIn(f"{setting}=", defaults)
                self.assertIn(f"{setting}: ${{{setting}", backend)

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
            "go test -count=1 -race ./internal/automation ./internal/task",
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
