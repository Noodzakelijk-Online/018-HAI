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
