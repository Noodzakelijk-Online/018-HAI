# Supply Chain & Dependency Review

## Backend (Go)

Module `automation-hub-backend`, declared Go 1.21 (toolchain in use 1.25.6).
Direct dependencies of note:

| Dependency | Purpose | Notes |
| --- | --- | --- |
| `gin-gonic/gin` | HTTP framework | widely used, active |
| `gorm.io/gorm` + `driver/postgres` | ORM / Postgres | active |
| `IBM/sarama` | Kafka client | active |
| `google/uuid` | IDs | stable |
| `swaggo/*` | Swagger docs | dev/doc only |

All third-party code is pinned via `go.sum` (checksums enforced by the Go
toolchain).

## Frontend (Angular)

Angular 16 + ng-zorro-antd, pinned via `package-lock.json`.

## Gaps & actions

| Item | Action |
| --- | --- |
| No automated vulnerability scan | Add `govulncheck ./...` (Go) and `npm audit --production` (frontend) as CI gates. |
| Go version unpinned in CI | Pin the toolchain in `.github/workflows/ci.yml`. |
| Committed binary `hai-engine-control.zip` | Remove from VCS; move to release assets. |
| Dependency freshness | Adopt a scheduled `go list -m -u all` / `npm outdated` review (register #95). |

## Policy

- No new direct dependency without a stated purpose and a license check
  (see `docs/third-party-licenses.md`).
- Prefer the standard library; several internal utilities (rate limiting,
  idempotency, path safety, feature flags) are dependency-free by design.
