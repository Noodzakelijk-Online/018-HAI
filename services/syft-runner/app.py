"""Internal-only, aggregate Syft SBOM inventory runner for HAI.

The runner accepts a configured snapshot name, not a filesystem path. It reads
only that read-only child under /inputs and returns package totals and package
ecosystem counts. It never return an SBOM or package-level metadata.
"""

from __future__ import annotations

from collections import Counter
from hmac import compare_digest
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
from threading import Lock


MAX_REQUEST_BYTES = 256
MAX_WORKSPACES = 8
MAX_RESPONSE_BYTES = 8 * 1024 * 1024
MAX_PACKAGES = 100_000
MAX_ECOSYSTEMS = 64
MAX_DURATION_SECONDS = 300
MAX_SNAPSHOT_ENTRIES = 100_000
WORKSPACE_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
ECOSYSTEM_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$")
VERSION_PATTERN = re.compile(r"^\d+(?:\.\d+){1,3}(?:[-+][A-Za-z0-9.-]+)?$")
RUN_LOCK = Lock()


class RequestError(ValueError):
    """The caller or local configuration crossed a fixed runner boundary."""


def configured() -> tuple[str, Path, set[str]]:
    token = os.environ.get("HAI_SYFT_RUNNER_TOKEN", "").strip()
    root = Path(os.environ.get("HAI_SYFT_INPUT_ROOT", "/inputs")).resolve()
    workspaces = {value.strip() for value in os.environ.get("HAI_SYFT_WORKSPACES", "").split(",") if value.strip()}
    if len(token) < 16:
        raise RequestError("runner token must have at least 16 characters")
    if not root.is_dir():
        raise RequestError("source snapshot input root is unavailable")
    if not workspaces or len(workspaces) > MAX_WORKSPACES or any(not WORKSPACE_PATTERN.fullmatch(value) for value in workspaces):
        raise RequestError("one to eight valid snapshot names are required")
    return token, root, workspaces


def runner_engine() -> str:
    try:
        response = subprocess.run(
            ["/usr/local/bin/syft", "version"],
            check=True,
            capture_output=True,
            text=True,
            timeout=5,
            env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/tmp", "XDG_CACHE_HOME": "/tmp/cache", "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": ""},
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise RequestError("Syft binary is unavailable") from exc
    match = re.search(r"^Version:\s*([^\s]+)", response.stdout, re.MULTILINE)
    if not match or not VERSION_PATTERN.fullmatch(match.group(1).lstrip("v")):
        raise RequestError("Syft returned an invalid version")
    return "syft " + match.group(1).lstrip("v")


def require_token(headers) -> None:
    expected, _, _ = configured()
    if not compare_digest(expected, headers.get("X-HAI-Syft-Token", "")):
        raise PermissionError("invalid runner token")


def workspace_directory(workspace_id: str) -> Path:
    _, root, approved = configured()
    if workspace_id not in approved:
        raise RequestError("workspace is not approved for SBOM inventory")
    requested = root / workspace_id
    if requested.is_symlink():
        raise RequestError("approved snapshot must not be a symlink")
    candidate = requested.resolve()
    if candidate.parent != root or not candidate.is_dir():
        raise RequestError("approved snapshot is unavailable")
    ensure_snapshot_has_no_symlinks(candidate)
    return candidate


def ensure_snapshot_has_no_symlinks(snapshot: Path) -> None:
    entries = 0
    for directory, directories, files in os.walk(snapshot, followlinks=False):
        for name in directories + files:
            entries += 1
            if entries > MAX_SNAPSHOT_ENTRIES:
                raise RequestError("approved snapshot exceeds the fixed entry limit")
            if (Path(directory) / name).is_symlink():
                raise RequestError("approved snapshot must not contain symlinks")


def request_workspace(raw: bytes) -> str:
    try:
        payload = json.loads(raw)
        value = payload["workspaceId"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("workspaceId is required") from exc
    if not isinstance(value, str) or not WORKSPACE_PATTERN.fullmatch(value):
        raise RequestError("workspaceId is invalid")
    return value


def inventory(workspace_id: str) -> dict:
    workspace = workspace_directory(workspace_id)
    started = time.monotonic()
    command = ["/usr/local/bin/syft", str(workspace), "-o", "syft-json"]
    environment = {"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/tmp", "XDG_CACHE_HOME": "/tmp/cache", "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": ""}
    try:
        completed = subprocess.run(command, check=True, capture_output=True, timeout=MAX_DURATION_SECONDS, env=environment)
    except (OSError, subprocess.SubprocessError) as exc:
        raise RequestError("Syft inventory failed") from exc
    if len(completed.stdout) > MAX_RESPONSE_BYTES:
        raise RequestError("Syft inventory exceeded the fixed output limit")
    try:
        payload = json.loads(completed.stdout)
        artifacts = payload["artifacts"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("Syft returned an invalid SBOM payload") from exc
    if not isinstance(artifacts, list) or len(artifacts) > MAX_PACKAGES:
        raise RequestError("Syft package count exceeds the fixed inventory limit")
    ecosystems: Counter[str] = Counter()
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            raise RequestError("Syft returned an invalid artifact")
        ecosystem = artifact.get("type", "unknown")
        if not isinstance(ecosystem, str) or not ECOSYSTEM_PATTERN.fullmatch(ecosystem):
            ecosystem = "unknown"
        ecosystems[ecosystem] += 1
    ranked = sorted(ecosystems.items())
    if len(ranked) > MAX_ECOSYSTEMS:
        leading = ranked[: MAX_ECOSYSTEMS - 1]
        leading.append(("other", sum(count for _, count in ranked[MAX_ECOSYSTEMS - 1 :])))
        ranked = leading
    duration_ms = int((time.monotonic() - started) * 1000)
    result = {
        "status": "completed",
        "engine": runner_engine(),
        "workspaceId": workspace_id,
        "packageCount": len(artifacts),
        "ecosystems": [{"id": ecosystem, "count": count} for ecosystem, count in ranked],
        "durationMs": duration_ms,
    }
    result["resultDigest"] = hashlib.sha256(json.dumps(result, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest()
    return result


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-Syft/1.0"

    def log_message(self, _format: str, *_args) -> None:
        return

    def respond(self, status: HTTPStatus, payload: dict) -> None:
        encoded = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self) -> None:
        if self.path != "/healthz":
            self.respond(HTTPStatus.NOT_FOUND, {"error": "not found"})
            return
        try:
            self.respond(HTTPStatus.OK, {"status": "ok", "engine": runner_engine(), "configured": True})
        except RequestError:
            self.respond(HTTPStatus.SERVICE_UNAVAILABLE, {"status": "unavailable", "configured": False})

    def do_POST(self) -> None:
        if self.path != "/v1/inventory":
            self.respond(HTTPStatus.NOT_FOUND, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > MAX_REQUEST_BYTES:
                raise RequestError("request is invalid")
            require_token(self.headers)
            workspace_id = request_workspace(self.rfile.read(length))
            if not RUN_LOCK.acquire(blocking=False):
                self.respond(HTTPStatus.CONFLICT, {"error": "an inventory is already running"})
                return
            try:
                self.respond(HTTPStatus.OK, inventory(workspace_id))
            finally:
                RUN_LOCK.release()
        except PermissionError:
            self.respond(HTTPStatus.UNAUTHORIZED, {"error": "unauthorized"})
        except (RequestError, ValueError):
            self.respond(HTTPStatus.BAD_REQUEST, {"error": "invalid inventory request or runner configuration"})


def main() -> None:
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()


if __name__ == "__main__":
    main()
