# Dependency Vulnerabilities — Status & Next Action

Records the actual `govulncheck` result, the CI scanning posture, and the exact
remediation, so nothing is hand-waved.

## Backend remediation completed (40 to 0)

The 2026-07-30 scan first found 40 code-affecting vulnerabilities under Go
1.21.13. The backend was then moved as one coordinated change to Go 1.25.12 and
the affected module family was upgraded:

- Go toolchain and Docker/CI builder: **1.25.12**
- `golang.org/x/net`: **v0.56.0**
- `golang.org/x/text`: **v0.39.0**
- `github.com/jackc/pgx/v5`: **v5.9.2**
- `github.com/redis/go-redis/v9`: **v9.6.3**

Go 1.25 vet identified one dynamic `fmt.Errorf` call; it was changed to
`errors.New`. Full vet, tests, and build then passed.

## Backend (`govulncheck ./...`)

`govulncheck` v1.6.0 using the 2026-07-27 vulnerability database and Go
1.25.12 reports:

- **0** vulnerabilities affecting code
- **0** vulnerabilities in imported packages
- 3 advisories in required modules whose vulnerable symbols are not called

The backend scan is pinned and blocking in CI.

## IDP (`govulncheck ./...`)

The same scanner, database snapshot, and Go 1.25.12 toolchain report 0
vulnerabilities affecting IDP code and 0 in imported packages. Two advisories
remain in required modules whose vulnerable symbols are not called. The IDP
scan is also pinned and blocking in CI.

## Nginx configuration manager (`govulncheck ./...`)

The first 2026-07-30 scan found 31 code-affecting vulnerabilities under Go
1.21.13, including findings in the obsolete Docker SDK reload path. HAI removed
the Docker SDK and all Compose Docker-socket mounts, upgraded Sarama, and moved
the service to Go 1.25.12. Enabling automatic reload now fails closed and tells
the operator to use an approved deployment operation.

Tests, vet, and build pass. The refreshed scan reports 0 vulnerabilities
affecting code and 0 in imported packages. One advisory remains in a required
module whose vulnerable symbol is not called. The scan is pinned and blocking
in CI.

## Frontend (`npm audit --audit-level=high --omit=dev`)

The 2026-07-30 audit reports **13 production dependency advisories: 12 high and
1 moderate**. The high findings are in the Angular 16 runtime and its compatible
CDK/ng-zorro dependency chain. The automated remediation proposes Angular
21.2.19, which is a breaking platform upgrade rather than a safe lockfile patch.

Do not expose this frontend as an untrusted multi-user Internet application
until the Angular migration is completed and the audit is rerun. The coordinated
migration must cover Angular core/runtime/compiler, Angular CLI/build tooling,
CDK, ng-zorro, TypeScript, Zone.js, and `angular-mixed-cdk-drag-drop`; it must
also retain the authenticated route, drawer, workflow, and 126-test contracts.
Applying `npm audit fix --force` without that migration and regression pass is
not an accepted remediation.

## Why the CI gate is advisory (for now)

The backend scan is no longer advisory. The frontend audit remains advisory
because its safe remediation is a coordinated Angular platform migration, not a
lockfile-only patch. CI still surfaces every frontend audit result.

## Exact next action (to reach zero and make the gate blocking)

1. Migrate the frontend dependency family to a mutually compatible, supported
   Angular release; run the full unit/build/browser suite and
   `npm audit --audit-level=high --omit=dev`; make the frontend scan a hard gate
   only when the remaining findings are zero or explicitly accepted with an
   owner, scope, and expiry.

## Current CI posture

- Backend: vet, build, test, race checks, and pinned `govulncheck` are **hard
  gates**.
- IDP: vet, build, test, and pinned `govulncheck` are **hard gates**.
- Nginx configuration manager: vet, build, test, and pinned `govulncheck` are
  **hard gates**; Docker-socket control is forbidden by an executable contract.
- Frontend: build + **unit tests (headless Chrome)** are hard gates; `npm audit
  --audit-level=high` is advisory.
