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


class CIWorkflowContractTest(unittest.TestCase):
    def test_canonical_service_runtime_images_do_not_float_on_latest(
        self,
    ) -> None:
        backend = (ROOT / "backend" / "Dockerfile").read_text(encoding="utf-8")
        self.assertNotRegex(backend, r"(?m)^FROM\s+\S+:latest(?:\s|$)")
        self.assertRegex(
            backend,
            r"(?m)^FROM alpine:3\.22@sha256:[0-9a-f]{64}$",
        )

        idp = (ROOT / "idp" / "Dockerfile").read_text(encoding="utf-8")
        self.assertNotRegex(idp, r"(?m)^FROM\s+\S+:latest(?:\s|$)")
        self.assertRegex(
            idp,
            r"(?m)^FROM alpine:3\.22@sha256:[0-9a-f]{64}$",
        )

        manager = (ROOT / "nginx-config-manager" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        self.assertNotRegex(manager, r"(?m)^FROM\s+\S+:latest(?:\s|$)")
        self.assertRegex(
            manager,
            r"(?m)^FROM alpine:3\.22@sha256:[0-9a-f]{64}$",
        )

    def test_backend_runtime_is_static_non_root_and_resource_bounded(self) -> None:
        dockerfile = (ROOT / "backend" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        compose = (ROOT / "docker-compose.local.yml").read_text(
            encoding="utf-8"
        )
        backend_start = compose.index("  backend:\n")
        backend_end = compose.index("\n  frontend:\n", backend_start)
        backend = compose[backend_start:backend_end]

        for required in (
            "CGO_ENABLED=0",
            "-trimpath",
            '-ldflags "-s -w"',
            "USER 10001:10001",
            'ENTRYPOINT ["/app/hai-backend"]',
        ):
            with self.subTest(required=required):
                self.assertIn(required, dockerfile)
        self.assertNotIn("apt-get", dockerfile)
        self.assertNotIn("curl", dockerfile)

        for required in (
            'user: "10001:10001"',
            "init: true",
            "read_only: true",
            "/tmp:rw,noexec,nosuid,nodev,size=${BACKEND_TMPFS_SIZE:-128m}",
            "mem_limit: ${BACKEND_MEMORY_LIMIT:-512m}",
            "mem_reservation: ${BACKEND_MEMORY_RESERVATION:-96m}",
            "cpus: ${BACKEND_CPU_LIMIT:-1.5}",
            "pids_limit: ${BACKEND_PIDS_LIMIT:-256}",
            "no-new-privileges:true",
            "cap_drop:",
            "- ALL",
            "wget -q -O /dev/null",
        ):
            with self.subTest(required=required):
                self.assertIn(required, backend)

    def test_idp_runtime_is_static_non_root_and_resource_bounded(self) -> None:
        dockerfile = (ROOT / "idp" / "Dockerfile").read_text(encoding="utf-8")
        compose = (ROOT / "docker-compose.local.yml").read_text(
            encoding="utf-8"
        )
        idp_start = compose.index("  idp:\n")
        idp_end = compose.index("\n  backend:\n", idp_start)
        idp = compose[idp_start:idp_end]
        backend_start = idp_end + 1
        backend_end = compose.index("\n  frontend:\n", backend_start)
        backend = compose[backend_start:backend_end]
        nginx_start = compose.index("  nginx:\n")
        nginx_end = compose.index("\n  ngrok:\n", nginx_start)
        nginx = compose[nginx_start:nginx_end]

        for required in (
            "CGO_ENABLED=0",
            "-trimpath",
            '-ldflags "-s -w"',
            "USER 10001:10001",
            'ENTRYPOINT ["/app/idp"]',
        ):
            with self.subTest(required=required):
                self.assertIn(required, dockerfile)
        self.assertNotIn("apt-get", dockerfile)
        self.assertNotIn("curl", dockerfile)
        self.assertNotIn("bash", dockerfile)
        self.assertTrue((ROOT / "idp" / ".dockerignore").is_file())

        for required in (
            'user: "10001:10001"',
            "init: true",
            "read_only: true",
            "/tmp:rw,noexec,nosuid,nodev,size=${IDP_TMPFS_SIZE:-32m}",
            "mem_limit: ${IDP_MEMORY_LIMIT:-256m}",
            "mem_reservation: ${IDP_MEMORY_RESERVATION:-48m}",
            "cpus: ${IDP_CPU_LIMIT:-1.0}",
            "pids_limit: ${IDP_PIDS_LIMIT:-128}",
            "no-new-privileges:true",
            "cap_drop:",
            "- ALL",
            "wget -q -O /dev/null http://127.0.0.1:${WEB_SERVER_PORT}/healthz",
        ):
            with self.subTest(required=required):
                self.assertIn(required, idp)

        self.assertRegex(
            backend,
            r"(?s)idp:\s+condition: service_healthy",
        )
        self.assertRegex(
            nginx,
            r"(?s)backend:\s+condition: service_healthy.*idp:\s+condition: service_healthy",
        )

    def test_frontend_runtime_is_non_root_single_worker_and_resource_bounded(
        self,
    ) -> None:
        dockerfile = (ROOT / "frontend" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        main_config = (ROOT / "frontend" / "nginx-main.conf").read_text(
            encoding="utf-8"
        )
        site_config = (ROOT / "frontend" / "nginx-custom.conf").read_text(
            encoding="utf-8"
        )
        compose = (ROOT / "docker-compose.local.yml").read_text(
            encoding="utf-8"
        )
        frontend_start = compose.index("  frontend:\n")
        frontend_end = compose.index("\n  browser-verifier:\n", frontend_start)
        frontend = compose[frontend_start:frontend_end]

        self.assertRegex(
            dockerfile,
            r"(?m)^FROM node:22\.22\.3-alpine@sha256:[0-9a-f]{64} AS build$",
        )
        self.assertRegex(
            dockerfile,
            r"(?m)^FROM nginx:stable-alpine-slim@sha256:[0-9a-f]{64}$",
        )
        self.assertIn("USER 101:101", dockerfile)
        self.assertIn('ENTRYPOINT ["nginx", "-g", "daemon off;"]', dockerfile)
        self.assertIn("worker_processes 1;", main_config)
        self.assertIn("pid /tmp/nginx.pid;", main_config)
        self.assertIn("client_body_temp_path /tmp/client_temp;", main_config)
        self.assertIn("listen 8080;", site_config)
        self.assertIn("location = /healthz", site_config)

        for required in (
            'user: "101:101"',
            "init: true",
            "read_only: true",
            "/tmp:rw,noexec,nosuid,nodev,size=${FRONTEND_TMPFS_SIZE:-16m}",
            "mem_limit: ${FRONTEND_MEMORY_LIMIT:-64m}",
            "mem_reservation: ${FRONTEND_MEMORY_RESERVATION:-8m}",
            "cpus: ${FRONTEND_CPU_LIMIT:-0.25}",
            "pids_limit: ${FRONTEND_PIDS_LIMIT:-32}",
            "no-new-privileges:true",
            "cap_drop:",
            "- ALL",
            "condition: service_healthy",
            "http://127.0.0.1:8080/healthz",
        ):
            with self.subTest(required=required):
                self.assertIn(required, frontend)

        for config_path in (
            ROOT / "nginx-config" / "nginx.conf",
            ROOT / "nginx-config" / "nginx.conf.template",
        ):
            with self.subTest(config_path=config_path):
                self.assertIn(
                    "set $frontend_upstream frontend:8080;",
                    config_path.read_text(encoding="utf-8"),
                )

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

    def test_nginx_manager_is_observable_non_root_and_resource_bounded(
        self,
    ) -> None:
        dockerfile = (ROOT / "nginx-config-manager" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        dockerignore = ROOT / "nginx-config-manager" / ".dockerignore"
        compose = (ROOT / "docker-compose.local.yml").read_text(
            encoding="utf-8"
        )
        manager_start = compose.index("  nginxconfigmanager:\n")
        manager_end = compose.index("\n  ortools-solver:\n", manager_start)
        manager = compose[manager_start:manager_end]
        consumer = (
            ROOT
            / "nginx-config-manager"
            / "internal"
            / "app"
            / "autoconfig"
            / "consumer.go"
        ).read_text(encoding="utf-8")
        config_writer = (
            ROOT
            / "nginx-config-manager"
            / "internal"
            / "app"
            / "autoconfig"
            / "auto_config_service.go"
        ).read_text(encoding="utf-8")

        for required in (
            "CGO_ENABLED=0",
            "-trimpath",
            '-ldflags "-s -w"',
            "USER 10001:10001",
            'ENTRYPOINT ["/app/nginx-config-manager"]',
        ):
            with self.subTest(required=required):
                self.assertIn(required, dockerfile)
        self.assertNotIn("apt-get", dockerfile)
        self.assertTrue(dockerignore.is_file())

        for required in (
            'user: "10001:10001"',
            "init: true",
            "read_only: true",
            "/tmp:rw,noexec,nosuid,nodev,size=${NGINX_CONFIG_MANAGER_TMPFS_SIZE:-16m}",
            "mem_limit: ${NGINX_CONFIG_MANAGER_MEMORY_LIMIT:-128m}",
            "mem_reservation: ${NGINX_CONFIG_MANAGER_MEMORY_RESERVATION:-16m}",
            "cpus: ${NGINX_CONFIG_MANAGER_CPU_LIMIT:-0.5}",
            "pids_limit: ${NGINX_CONFIG_MANAGER_PIDS_LIMIT:-64}",
            "no-new-privileges:true",
            "cap_drop:",
            "- ALL",
            "condition: service_healthy",
            "/healthz",
        ):
            with self.subTest(required=required):
                self.assertIn(required, manager)

        self.assertIn("Consumer.Return.Errors = true", consumer)
        self.assertIn("ready.Store(true)", consumer)
        self.assertIn("Kafka message channel closed", consumer)
        self.assertIn("os.CreateTemp", config_writer)
        self.assertIn("file.Sync()", config_writer)
        self.assertIn("os.Rename", config_writer)

    def test_frontend_toolchain_and_security_gate_are_pinned(self) -> None:
        package = (ROOT / "frontend" / "package.json").read_text(
            encoding="utf-8"
        )
        angular = (ROOT / "frontend" / "angular.json").read_text(
            encoding="utf-8"
        )
        dockerfile = (ROOT / "frontend" / "Dockerfile").read_text(
            encoding="utf-8"
        )
        frontend = job_block("frontend")

        self.assertIn('"packageManager": "npm@10.9.8"', package)
        self.assertIn('"@angular/core": "22.1.1"', package)
        self.assertIn('"@angular/build": "22.1.3"', package)
        self.assertIn('"builder": "@angular/build:application"', angular)
        self.assertRegex(
            dockerfile,
            r"(?m)^FROM node:22\.22\.3-alpine@sha256:[0-9a-f]{64} AS build$",
        )
        self.assertIn('node-version: "22.22.3"', frontend)
        self.assertIn("npm ci --no-audit --no-fund", frontend)
        self.assertIn("npm audit --audit-level=high", frontend)
        self.assertNotIn("continue-on-error", frontend)
        self.assertFalse((ROOT / "frontend" / "pnpm-lock.yaml").exists())
        self.assertFalse((ROOT / "frontend" / "pnpm-workspace.yaml").exists())

    def test_ngrok_profile_is_opt_in_pinned_and_preflight_gated(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        workflow = (ROOT / ".github" / "workflows" / "ci.yml").read_text(
            encoding="utf-8"
        )
        preflight = (ROOT / "scripts" / "start-ngrok.ps1").read_text(
            encoding="utf-8"
        )
        config = (ROOT / "deploy" / "ngrok" / "ngrok.yml").read_text(
            encoding="utf-8"
        )
        entrypoint = (ROOT / "deploy" / "ngrok" / "start-ngrok.sh").read_text(
            encoding="utf-8"
        )
        ngrok_start = compose.index("  ngrok:\n")
        ngrok_end = compose.index("\n  nginxconfigmanager:\n", ngrok_start)
        ngrok_service = compose[ngrok_start:ngrok_end]

        self.assertIn('profiles: ["cloud-tunnel"]', ngrok_service)
        self.assertRegex(
            ngrok_service,
            r"image: ngrok/ngrok:alpine@sha256:[0-9a-f]{64}",
        )
        self.assertNotIn("\n    ports:", ngrok_service)
        self.assertIn("read_only: true", ngrok_service)
        self.assertIn("no-new-privileges:true", ngrok_service)
        self.assertIn("cap_drop:", ngrok_service)
        self.assertIn('entrypoint: ["/bin/sh", "/etc/hai/start-ngrok.sh"]', ngrok_service)
        for required in (
            "LOCAL_LOGIN_BYPASS_ENABLED",
            "IDP_COOKIE_SECURE",
            "GATEWAY_HOST_BIND",
            "NGROK_AUTHTOKEN",
            "HAI_NGROK_URL",
            "GOOGLE_LOGIN_REDIRECT_URL",
            "GOOGLE_OAUTH_REDIRECT_URL",
            "docker compose",
        ):
            with self.subTest(required=required):
                self.assertIn(required, preflight)
        secured_up = "up -d --no-build idp backend frontend nginx"
        tunnel_up = "up -d --no-build ngrok"
        self.assertIn(secured_up, preflight)
        self.assertIn(tunnel_up, preflight)
        self.assertLess(preflight.index(secured_up), preflight.index(tunnel_up))
        self.assertIn("remote_management: false", config)
        self.assertIn("update_check: false", config)
        self.assertIn("inspect_db_size: -1", config)
        self.assertIn("http://nginx:80", entrypoint)
        self.assertIn("Test-NgrokHostname", preflight)
        self.assertIn("HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED", preflight)
        self.assertIn("HAI_A2A_BRIDGE_TOKEN", preflight)
        self.assertIn("/api/v1/a2a", preflight)
        self.assertIn("Ngrok container fail-closed gate", workflow)
        self.assertIn("sh -n deploy/ngrok/start-ngrok.sh", workflow)
        self.assertIn("mismatched public A2A origin unexpectedly passed", workflow)
        for required in (
            "HAI_A2A_BRIDGE_ENABLED",
            "HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED",
            "HAI_A2A_BRIDGE_TOKEN",
            "HAI_A2A_BRIDGE_OWNER_ID",
            "HAI_A2A_BRIDGE_URL",
        ):
            with self.subTest(ngrok_environment=required):
                self.assertIn(required, ngrok_service)
        for required in (
            'RUN_MODE must be production',
            'local login bypass must be false',
            'secure IDP cookies are required',
            'gateway host bind must remain loopback-only',
            'a dedicated ngrok authtoken is required',
            'HAI_NGROK_VALIDATE_ONLY',
            'public A2A requires HAI_A2A_BRIDGE_ENABLED=true',
            'public A2A requires a dedicated 32+ character bridge token',
            'public A2A requires one named owner',
            'public A2A URL must exactly match the fixed ngrok origin',
            '/bin/ngrok http http://nginx:80',
        ):
            with self.subTest(entrypoint_required=required):
                self.assertIn(required, entrypoint)

    def test_windows_initializer_generates_every_required_production_secret(self) -> None:
        initializer = (ROOT / "scripts" / "initialize-windows.ps1").read_text(
            encoding="utf-8"
        )
        unix_generator = (ROOT / "scripts" / "generate-secrets.sh").read_text(
            encoding="utf-8"
        )
        for required in (
            "BACKEND_API_SHARED_KEY",
            "HAI_MEMORY_ENCRYPTION_KEY",
            "JWT_SECRET",
            "HAI_APPROVAL_PROOF_SIGNING_KEY",
        ):
            with self.subTest(required=required):
                self.assertIn(required, initializer)
                self.assertIn(required, unix_generator)
        self.assertIn("RandomNumberGenerator", initializer)
        self.assertIn('LOCAL_LOGIN_BYPASS_ENABLED\" \"false', initializer)
        self.assertIn('GATEWAY_HOST_BIND\" \"127.0.0.1', initializer)
        self.assertIn('RUN_MODE\" \"production', initializer)
        self.assertIn("were not printed", initializer)

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
            r".\scripts\start-ngrok.ps1 -ValidateOnly",
            r".\scripts\initialize-windows.ps1",
            "Generated Windows environment still contains a shipped placeholder",
            "Insecure example environment unexpectedly passed ngrok preflight",
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
        self.assertNotIn("actions/upload-artifact", WORKFLOW)
        self.assertNotRegex(WORKFLOW, r"(?i)\bupload[\w -]*(?:log|env|secret)")

    def test_every_job_has_an_explicit_timeout(self) -> None:
        for job_id in (
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
