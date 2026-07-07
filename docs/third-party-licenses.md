# License & Third-Party Review

## Project license

The repository ships a `LICENSE` file at the root and in each service directory
(`backend/`, `frontend/`). Confirm the intended license is consistent across all
copies before distribution.

## Key third-party licenses (backend)

Permissive licenses compatible with typical commercial/self-hosted use:

| Dependency | License |
| --- | --- |
| `gin-gonic/gin` | MIT |
| `gorm.io/gorm`, `driver/postgres` | MIT |
| `IBM/sarama` | MIT |
| `google/uuid` | BSD-3-Clause |
| `swaggo/*` | MIT |
| `golang.org/x/*` | BSD-3-Clause |

## Frontend

Angular (MIT) and ng-zorro-antd (MIT).

## Process

- Generate a full license inventory in CI, e.g.:

  ```bash
  go install github.com/google/go-licenses@latest
  go-licenses report ./... > docs/generated-licenses.txt
  ```

- Fail the build on any license outside an allowlist (MIT / BSD / Apache-2.0)
  pending explicit legal review.
- Re-run on every dependency change.

## Status

No copyleft (GPL/AGPL) dependency identified in the reviewed set. A generated,
authoritative inventory via `go-licenses` is the recommended next step to make
this continuously verifiable rather than manually maintained.
