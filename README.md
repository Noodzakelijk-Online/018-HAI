# Automation Hub - Unified Repository

This repository contains the complete automation hub system with all components consolidated into a single repository structure.

## Project Structure

```
automation-hub-unified/
├── backend/                    # Go backend service
├── frontend/                   # Angular frontend application
├── gate/                      # API gateway configuration
├── idp/                       # Identity provider service
├── kafka/                     # Kafka configuration
├── nginx-config-manager/      # Nginx configuration manager
├── docs/                      # Documentation
├── generic-auto/              # Generic automation scripts
├── nginx-config/              # Nginx configurations
├── docker-compose.yml         # Main docker compose file
├── makefile                   # Build automation
└── init.sql                   # Database initialization

```

## Components

### Backend (`/backend`)
Go-based backend service providing the core API functionality.

### Frontend (`/frontend`) 
Angular-based web application providing the user interface.

### Gate (`/gate`)
API gateway configuration for routing and load balancing.

### Identity Provider (`/idp`)
Authentication and authorization service.

### Kafka (`/kafka`)
Message broker configuration for event-driven communication.

### Nginx Config Manager (`/nginx-config-manager`)
Service for managing nginx configurations dynamically.

## Getting Started

1. Clone this repository
2. Run `make` to see available build commands
3. Use `docker-compose up` to start all services
4. Access the application through the configured gateway

## Development

Each component maintains its own build configuration and can be developed independently while being part of the unified repository structure.

## Automation Control Center

The Control Center adds operational monitoring on top of the existing automation
list: health status per automation, manual health checks, a health summary, and
a diagnostics view with recent check history.

### What was added

Backend (`/backend`):

- `internal/infra/database.go` — the operational models (health events,
  dependencies, route checks, alerts, incidents, SLOs) are now part of
  `AutoMigrate`, and the `uuid-ossp` extension is ensured on startup.
- `internal/router/routes.go` — three routes registered:
  - `GET  /api/v1/automation/health/summary`
  - `POST /api/v1/automation/:id/health-check`
  - `GET  /api/v1/automation/:id/diagnostics`
- `internal/automation/automation_service.go` — each health check now persists
  an `AutomationHealthEvent` (history) and the diagnostics response returns the
  last 10 events plus the last checked/success/failure timestamps. The health
  summary now reports `total`, `healthy`, `warning`, `degraded`, `broken` and
  `unknown` counts.
- `internal/automation/automation_repository.go` — `SaveHealthEvent` and
  `FindHealthEvents` for persisting and reading health history.

Frontend (`/frontend`):

- `services/automations/automations.service.ts` — `getHealthSummary`,
  `runHealthCheck` and `getDiagnostics` methods.
- `pages/control-center/` — the Control Center page (summary bar, automation
  table with status badges, last checked/success/failure, failure reason, a
  manual **Run Check** button, an **Open** link for browser-safe URLs, and a
  **Diagnostics** modal with configuration checks and recent health events).
- A **Control Center** entry was added to the home menu, routed at
  `/control-center`.

### Health check behaviour

- `http` (default): GETs the health check URL (or `http://host:port`) and
  compares against the expected status code.
- `tcp`: dials `host:port`.
- `manual` / `disabled`: reported as `unknown`, no automatic probe.

Repeated failures escalate the status: `warning` → `degraded` → `broken`.

### Safety

No arbitrary command execution is performed. Health checks are HTTP/TCP probes
only, and opening a target uses a browser `window.open` with `noopener`. Local
runner / service control remains out of scope (see the blueprint, step 10).

### How to test

Backend compiles:

```bash
cd backend
go build ./...
```

Frontend compiles:

```bash
cd frontend
npm install
npm run build
```

Full stack (optional, end-to-end):

```bash
docker-compose up --build
```

Then open the gateway, log in, and choose **Control Center** from the home
menu. Use **Run Check** on a row to probe an automation; the status badge,
timestamps and summary bar update, and **Diagnostics** shows the recorded
history.

API smoke test (with the backend running):

```bash
# health summary
curl http://localhost/api/v1/automation/health/summary

# run a health check for one automation
curl -X POST http://localhost/api/v1/automation/<automation-id>/health-check

# diagnostics for one automation
curl http://localhost/api/v1/automation/<automation-id>/diagnostics
```

## License

See LICENSE file for details.

