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
        self.assertIn("FROM node:22.22.3-alpine AS build", dockerfile)
        self.assertIn('node-version: "22.22.3"', frontend)
        self.assertIn("npm ci --no-audit --no-fund", frontend)
        self.assertIn("npm audit --audit-level=high", frontend)
        self.assertNotIn("continue-on-error", frontend)
        self.assertFalse((ROOT / "frontend" / "pnpm-lock.yaml").exists())
        self.assertFalse((ROOT / "frontend" / "pnpm-workspace.yaml").exists())

    def test_ngrok_profile_is_opt_in_pinned_and_preflight_gated(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
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
            "HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED",
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
        self.assertIn(
            "HAI_A2A_BRIDGE_PUBLIC_NGROK_ENABLED must remain false",
            preflight,
        )
        self.assertIn("remote_management: false", config)
        self.assertIn("update_check: false", config)
        self.assertIn("inspect_db_size: -1", config)
        self.assertIn("http://nginx:80", entrypoint)
        for required in (
            'RUN_MODE must be production',
            'local login bypass must be false',
            'secure IDP cookies are required',
            'gateway host bind must remain loopback-only',
            'a dedicated ngrok authtoken is required',
            'HAI_NGROK_VALIDATE_ONLY',
            '/bin/ngrok http http://nginx:80',
        ):
            with self.subTest(entrypoint_required=required):
                self.assertIn(required, entrypoint)

    def test_legacy_generic_auto_is_not_part_of_the_default_local_stack(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        start = compose.index("  generic-auto:\n")
        end = compose.index("\nnetworks:\n", start)
        service = compose[start:end]

        self.assertIn('profiles: ["legacy-compatibility"]', service)
        self.assertIn("Legacy compatibility server", service)

    def test_core_local_services_have_explicit_resource_ceilings(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        for name in (
            "postgres-idp",
            "postgres-automation",
            "redis",
            "backend-migrate",
            "idp",
            "backend",
            "frontend",
            "nginx",
        ):
            with self.subTest(service=name):
                match = re.search(
                    rf"(?ms)^  {re.escape(name)}:\n(.*?)(?=^  [A-Za-z0-9_-]+:\n|^networks:)",
                    compose,
                )
                self.assertIsNotNone(match)
                self.assertRegex(match.group(1), r"(?m)^    mem_limit: \S+")
                self.assertRegex(match.group(1), r"(?m)^    cpus: \S+")

    def test_redis_has_a_bounded_fail_closed_memory_budget(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        env_template = (ROOT / ".env.example").read_text(encoding="utf-8")
        start = compose.index("  redis:\n")
        end = compose.index("\n  zookeeper:\n", start)
        redis = compose[start:end]

        self.assertIn("REDIS_MAXMEMORY=128mb", env_template)
        self.assertIn("REDIS_MAXMEMORY_POLICY=noeviction", env_template)
        self.assertIn('"--maxmemory", "${REDIS_MAXMEMORY}"', redis)
        self.assertIn('"--maxmemory-policy", "${REDIS_MAXMEMORY_POLICY}"', redis)

    def test_default_compose_entrypoint_uses_the_canonical_source_stack(self) -> None:
        compose = (ROOT / "docker-compose.yml").read_text(encoding="utf-8")
        self.assertIn("include:", compose)
        self.assertIn("./docker-compose.local.yml", compose)
        self.assertNotIn("jacksonbarreto/", compose)
        self.assertNotIn(":latest", compose)

    def test_trello_read_only_connector_is_wired_to_the_runtime(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        backend_start = compose.index("  backend:\n")
        backend_end = compose.index("\n  frontend:\n", backend_start)
        backend = compose[backend_start:backend_end]
        env_template = (ROOT / ".env.example").read_text(encoding="utf-8")

        self.assertIn("TRELLO_API_KEY: ${TRELLO_API_KEY:-}", backend)
        self.assertIn("TRELLO_READ_TOKEN: ${TRELLO_READ_TOKEN:-}", backend)
        self.assertIn("TRELLO_API_BASE_URL: ${TRELLO_API_BASE_URL:-https://api.trello.com}", backend)
        self.assertIn("TRELLO_API_KEY=", env_template)
        self.assertIn("TRELLO_READ_TOKEN=", env_template)
        self.assertIn("TRELLO_LIVE_BOARD=", env_template)
        self.assertIn(
            "CONNECTED_SOURCE_HTTP_ALLOWED_HOSTS=localhost,127.0.0.1,::1,host.docker.internal,api.github.com,api.trello.com",
            env_template,
        )

    def test_durable_scheduler_controls_reach_the_backend_runtime(self) -> None:
        compose = (ROOT / "docker-compose.local.yml").read_text(encoding="utf-8")
        backend_start = compose.index("  backend:\n")
        backend_end = compose.index("\n  frontend:\n", backend_start)
        backend = compose[backend_start:backend_end]
        env_template = (ROOT / ".env.example").read_text(encoding="utf-8")

        for name, default in (
            ("SOURCE_SCHEDULER_DURABLE", "true"),
            ("SOURCE_WORKER_POLL_SECONDS", "15"),
            ("WORKFLOW_SCHEDULER_DURABLE", "true"),
            ("WORKFLOW_WORKER_POLL_SECONDS", "15"),
            ("WORKFLOW_REMINDER_DELIVERY_ENABLED", "true"),
            ("AMBIENT_SCHEDULER_DURABLE", "true"),
            ("AMBIENT_WORKER_POLL_SECONDS", "15"),
        ):
            with self.subTest(name=name):
                self.assertIn(f"{name}={default}", env_template)
                self.assertIn(f"{name}: ${{{name}:-{default}}}", backend)

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
            "^--- PASS: TestRuntimeRoleCanUseDataButCannotAlterSchema",
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
        self.assertIn("actions/upload-artifact@v6", WORKFLOW)
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
            "actions/upload-artifact@v6",
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
