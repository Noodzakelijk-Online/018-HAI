# Remote ChatGPT Logs MCP over ngrok

Status: proposed design; not implemented by the current ChatGPT Logs MCP pull
request.

## Purpose

Allow HAI on a laptop to use the ChatGPT/Codex history corpus hosted on a home
machine. The laptop should not run its own MCP server or database. The home
machine remains the single history host, while ngrok provides the private HTTPS
route preferred by the deployment owner.

This is a different traffic direction from HAI's existing governed ngrok
profile:

- the existing profile publishes HAI's nginx gateway so a user can reach HAI;
- this proposal lets HAI act as an MCP client of a separately published
  `hist-mcp` endpoint.

The two endpoints must not share credentials, domains, or security assumptions.

## Current state

The history daemon repository now has two explicit MCP transports:

```text
hist mcp                                      # stdio, the default
hist mcp --transport streamable-http \
  --bind 127.0.0.1:8099                      # local HTTP endpoint
```

The HTTP transport intentionally accepts only loopback binds and validates the
HTTP `Host`, optional `Origin`, content type, MCP protocol version, request size,
concurrency, and timeout. It serves the same reviewed nine read-only tools as
stdio.

HAI's ChatGPT Logs adapter currently accepts only `localhost`,
`host.docker.internal`, and literal loopback/private IP endpoints. It does not
send credentials to MCP. Consequently, substituting a public ngrok hostname in
`HAI_CHATGPT_LOGS_MCP_URL` is intentionally rejected today.

The existing HAI ngrok profile does not remove either restriction. It exposes
only HAI's nginx gateway and does not publish Postgres, the backend, or an MCP
server. See [Governed ngrok Cloud Access](ngrok-cloud-access.md).

## Target architecture

```text
HAI backend on laptop
        |
        | HTTPS + Authorization: Bearer <dedicated MCP token>
        v
dedicated reserved ngrok MCP domain
        |
        | tunnel to home-machine loopback
        v
hist-mcp on 127.0.0.1:8099
        |
        v
home-machine Postgres history corpus
```

The `hist-mcp` listener remains bound to `127.0.0.1`. The ngrok agent runs on
the home machine and is the only process that forwards external traffic to that
listener. No database port is exposed.

## Required changes

### History daemon

Add an authenticated reverse-proxy mode without weakening the safe defaults:

1. Keep stdio as the default transport and loopback as the default and only
   supported HTTP bind.
2. Add a bearer-token file option for Streamable HTTP, for example
   `--bearer-token-file <path>`. Reading the token from a file avoids putting it
   in process arguments, committed environment files, or logs.
3. Authenticate before parsing or dispatching a JSON-RPC body. Use a
   constant-time token comparison and return a generic `401` response.
4. Permit an exact configured proxy host, or require the ngrok route to rewrite
   `Host` to the local upstream value. Do not allow arbitrary suffix matching.
5. Continue validating `Origin` when it is present. A machine-to-machine HAI
   request normally has no browser origin.
6. Never include the token or authorization header in errors, tracing, tool
   results, provenance, or health output.
7. Preserve all existing body, concurrency, timeout, tool, and result limits.

The daemon should not gain a general `0.0.0.0` unauthenticated mode. The MCP
transport specification recommends localhost binding for local servers and
authentication for HTTP connections.

### HAI

Extend only the reviewed ChatGPT Logs client rather than relaxing HAI's generic
URL validation:

1. Permit one exact configured HTTPS ngrok MCP origin when remote mode is
   explicitly enabled. Arbitrary public HTTP(S) endpoints must remain invalid.
2. Load a dedicated bearer token from a mounted secret file and send it only to
   that exact origin.
3. Use the same authenticated client for preflight and runtime tool calls so a
   successful `initialize`/`tools/list` proves the path used during generation.
4. Reject redirects to a different origin and never forward authorization
   across redirects.
5. Keep the static nine-tool allowlist, model-call budget, round limit,
   per-result cap, aggregate-context cap, and untrusted-context labeling.
6. Redact the token from configuration status, errors, audit records, model
   context, and provenance. Status may report only whether a credential is
   configured.
7. Fail closed when the URL is not HTTPS, the hostname differs from the
   configured reserved domain, or the token file is absent or weak.

A possible configuration contract is:

```text
HAI_CHATGPT_LOGS_MCP_ENABLED=true
HAI_CHATGPT_LOGS_MCP_URL=https://history-mcp.example.ngrok.app/mcp
HAI_CHATGPT_LOGS_MCP_TOKEN_FILE=/run/secrets/chatgpt_logs_mcp_token
```

Names are illustrative until the implementation is reviewed.

### ngrok

Create a dedicated MCP tunnel instead of reusing HAI's public application
endpoint:

1. Use a reserved HTTPS domain dedicated to history MCP traffic.
2. Use a dedicated ngrok authtoken whose ACL permits only that endpoint.
3. Forward only to `http://127.0.0.1:8099` on the home machine.
4. Disable request inspection and remote management, matching the existing HAI
   ngrok hardening.
5. Keep MCP authentication at the daemon even if ngrok also enforces an edge
   policy. Ownership of an ngrok URL alone is not client authentication.
6. Never expose Postgres or reuse the HAI UI domain for MCP.

## Data ownership and synchronization

Remote MCP changes where queries execute, not what is indexed. The laptop sees
only the corpus stored in the home machine's Postgres database. Codex or ChatGPT
sessions created only on the laptop will not appear automatically.

If laptop sessions must be included, design a separate authenticated ingestion
path from laptop to home. Do not give MCP write tools or database credentials to
the laptop as a shortcut; MCP remains read-only.

## Acceptance tests

The feature is ready only after the following checks pass:

- local stdio behavior remains unchanged;
- local Streamable HTTP behavior remains unchanged without remote settings;
- missing, incorrect, expired, and rotated tokens fail closed;
- HAI rejects plain HTTP, arbitrary public hosts, cross-origin redirects, and
  endpoints whose TLS identity does not match the configured domain;
- ngrok reaches only the loopback MCP port and cannot reach Postgres;
- `initialize`, `notifications/initialized`, `tools/list`, and a bounded
  `tools/call` succeed from HAI on the laptop to the home machine;
- the model can answer the reviewed history questions using retained tool and
  endpoint provenance;
- authorization values never appear in logs, errors, status APIs, audit rows,
  prompts, or tool results;
- disabling either the HAI remote flag, ngrok profile, or home MCP process
  removes remote access without affecting local HAI operation;
- Windows home host and macOS laptop are tested as the primary deployment pair.

## Rollout recommendation

Implement this as two small, independently reviewable changes:

1. authenticated proxy support in `chatgpt-codex-mcp-daemon`;
2. exact-origin remote MCP credentials in HAI, plus the dedicated governed
   ngrok service.

Do not merge a configuration-only workaround that broadly allows public MCP
URLs or relies on obscurity of the ngrok hostname. The remote path carries
private conversation history and therefore requires explicit authentication,
TLS, bounded access, and revocation.

## References

- [HAI governed ngrok profile](ngrok-cloud-access.md)
- [HAI MCP integration boundary](agent-tool-catalog.md#chatgptcodex-conversation-history-context)
- [MCP Streamable HTTP transport security](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)
- [ngrok documentation](https://ngrok.com/docs/)
