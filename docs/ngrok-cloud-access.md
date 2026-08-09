# Governed ngrok Cloud Access

HAI remains loopback-only by default. Public access is an explicit Compose
profile and must be started through the Windows preflight script. The tunnel
forwards only to the nginx gateway on the private `service-hub` network; it does
not publish Postgres, Redis, Kafka, the backend, the IDP, or ngrok's inspector.

## Security boundary

The preflight fails closed unless:

- `RUN_MODE=production`;
- `LOCAL_LOGIN_BYPASS_ENABLED=false`;
- `IDP_COOKIE_SECURE=true`;
- `GATEWAY_HOST_BIND=127.0.0.1`;
- the ngrok token and HAI signing/encryption secrets are non-placeholder values;
- `HAI_NGROK_URL` is a fixed HTTPS origin; and
- configured Google login/source callbacks exactly match that public origin.

The container entrypoint independently rechecks production mode, disabled
local-login bypass, secure cookies, loopback gateway binding, a dedicated token,
and a fixed ngrok HTTPS origin. Calling Compose directly therefore cannot bypass
the core exposure gate; the PowerShell preflight adds the stronger secret and
OAuth consistency checks.

Use a dedicated ngrok authtoken with an ACL restricted to the reserved HAI
domain. Do not reuse a general account token. The agent image is digest-pinned,
remote management and request inspection are disabled, and the container is
read-only with dropped capabilities and bounded CPU, memory, and process count.

## Start on Windows 11

Set these values in the uncommitted `.env.local` file:

```text
NGROK_AUTHTOKEN=<dedicated ACL-restricted token>
HAI_NGROK_URL=https://your-hai-domain.ngrok.app
RUN_MODE=production
LOCAL_LOGIN_BYPASS_ENABLED=false
IDP_COOKIE_SECURE=true
GATEWAY_HOST_BIND=127.0.0.1
```

If Google login or Google connected sources are configured, register and set:

```text
GOOGLE_LOGIN_REDIRECT_URL=https://your-hai-domain.ngrok.app/api/v1/auth/google/callback
GOOGLE_OAUTH_REDIRECT_URL=https://your-hai-domain.ngrok.app/api/v1/sources/oauth/google/callback
```

Validate without opening a tunnel:

```powershell
.\scripts\start-ngrok.ps1 -ValidateOnly
```

Start and stop the profile:

```powershell
.\scripts\start-ngrok.ps1
.\scripts\start-ngrok.ps1 -Stop
```

Build the ordinary HAI stack before the first tunnel start. On every start, the
script reconciles and health-checks the IDP, backend, frontend, and gateway with
the hardened environment before it creates the public endpoint. This prevents
an IDP that was previously started with insecure cookie settings from being
exposed merely because the file was edited afterward. A healthy container and
reachable login page prove only transport and authentication routing. Each
external connector, model, or runtime still requires its own bounded
authorization and acceptance evidence.

## Recovery

If the tunnel is unhealthy, inspect only the bounded service log:

```powershell
docker compose --env-file .env.local --profile cloud-tunnel -f docker-compose.local.yml ps ngrok
docker compose --env-file .env.local --profile cloud-tunnel -f docker-compose.local.yml logs --tail 100 ngrok
```

Stopping the tunnel does not stop HAI. Rotating the dedicated ngrok authtoken
does not change HAI sessions, but existing public sessions should be revoked if
the endpoint or token was exposed.
