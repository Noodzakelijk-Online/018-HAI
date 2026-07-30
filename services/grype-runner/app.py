"""Internal-only, aggregate Grype vulnerability runner for HAI.

The runner accepts an approved snapshot name rather than a path. It uses an
operator-supplied read-only advisory database, disables all update checks and
network proxies, and returns severity totals only. No CVE, package, version,
path, advisory, or raw result is returned or persisted.
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
MAX_VULNERABILITIES = 100_000
MAX_DURATION_SECONDS = 300
MAX_SNAPSHOT_ENTRIES = 100_000
WORKSPACE_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
VERSION_PATTERN = re.compile(r"^\d+(?:\.\d+){1,3}(?:[-+][A-Za-z0-9.-]+)?$")
SEVERITIES = {"critical", "high", "medium", "low", "negligible", "unknown"}
RUN_LOCK = Lock()


class RequestError(ValueError):
    """The caller or local configuration crossed a fixed runner boundary."""


def configured() -> tuple[str, Path, Path, set[str]]:
    token = os.environ.get("HAI_GRYPE_RUNNER_TOKEN", "").strip()
    root = Path(os.environ.get("HAI_GRYPE_INPUT_ROOT", "/inputs")).resolve()
    advisory_root = Path(os.environ.get("HAI_GRYPE_DB_ROOT", "/grype-db")).resolve()
    workspaces = {value.strip() for value in os.environ.get("HAI_GRYPE_WORKSPACES", "").split(",") if value.strip()}
    if len(token) < 16:
        raise RequestError("runner token must have at least 16 characters")
    if not root.is_dir() or not advisory_root.is_dir():
        raise RequestError("source snapshot input root or local advisory database is unavailable")
    if not workspaces or len(workspaces) > MAX_WORKSPACES or any(not WORKSPACE_PATTERN.fullmatch(value) for value in workspaces):
        raise RequestError("one to eight valid snapshot names are required")
    return token, root, advisory_root, workspaces


def environment(advisory_root: Path) -> dict[str, str]:
    return {
        "PATH": "/usr/local/bin:/usr/bin:/bin",
        "HOME": "/tmp",
        "XDG_CACHE_HOME": "/tmp/cache",
        "GRYPE_DB_AUTO_UPDATE": "false",
        "GRYPE_CHECK_FOR_APP_UPDATE": "false",
        "GRYPE_DB_CACHE_DIR": str(advisory_root),
        "HTTP_PROXY": "",
        "HTTPS_PROXY": "",
        "ALL_PROXY": "",
        "NO_PROXY": "*",
    }


def runner_engine() -> str:
    try:
        _, _, advisory_root, _ = configured()
        response = subprocess.run(["/usr/local/bin/grype", "version"], check=True, capture_output=True, text=True, timeout=5, env=environment(advisory_root))
    except (OSError, subprocess.SubprocessError, RequestError) as exc:
        raise RequestError("Grype binary or local advisory database is unavailable") from exc
    match = re.search(r"^Version:\s*([^\s]+)", response.stdout, re.MULTILINE)
    if not match or not VERSION_PATTERN.fullmatch(match.group(1).lstrip("v")):
        raise RequestError("Grype returned an invalid version")
    return "grype " + match.group(1).lstrip("v")


def require_token(headers) -> None:
    expected, _, _, _ = configured()
    if not compare_digest(expected, headers.get("X-HAI-Grype-Token", "")):
        raise PermissionError("invalid runner token")


def workspace_directory(workspace_id: str) -> Path:
    _, root, _, approved = configured()
    if workspace_id not in approved:
        raise RequestError("workspace is not approved for vulnerability scanning")
    requested = root / workspace_id
    if requested.is_symlink():
        raise RequestError("approved snapshot must not be a symlink")
    candidate = requested.resolve()
    if candidate.parent != root or not candidate.is_dir():
        raise RequestError("approved snapshot is unavailable")
    entries = 0
    for directory, directories, files in os.walk(candidate, followlinks=False):
        for name in directories + files:
            entries += 1
            if entries > MAX_SNAPSHOT_ENTRIES:
                raise RequestError("approved snapshot exceeds the fixed entry limit")
            if (Path(directory) / name).is_symlink():
                raise RequestError("approved snapshot must not contain symlinks")
    return candidate


def request_workspace(raw: bytes) -> str:
    try:
        value = json.loads(raw)["workspaceId"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("workspaceId is required") from exc
    if not isinstance(value, str) or not WORKSPACE_PATTERN.fullmatch(value):
        raise RequestError("workspaceId is invalid")
    return value


def scan(workspace_id: str) -> dict:
    workspace = workspace_directory(workspace_id)
    _, _, advisory_root, _ = configured()
    started = time.monotonic()
    try:
        completed = subprocess.run(["/usr/local/bin/grype", str(workspace), "-o", "json"], check=True, capture_output=True, timeout=MAX_DURATION_SECONDS, env=environment(advisory_root))
    except (OSError, subprocess.SubprocessError) as exc:
        raise RequestError("Grype vulnerability scan failed") from exc
    if len(completed.stdout) > MAX_RESPONSE_BYTES:
        raise RequestError("Grype result exceeded the fixed output limit")
    try:
        matches = json.loads(completed.stdout)["matches"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("Grype returned an invalid vulnerability payload") from exc
    if not isinstance(matches, list) or len(matches) > MAX_VULNERABILITIES:
        raise RequestError("Grype vulnerability count exceeds the fixed scan limit")
    severities: Counter[str] = Counter()
    fix_available = 0
    for match in matches:
        if not isinstance(match, dict) or not isinstance(match.get("vulnerability"), dict):
            raise RequestError("Grype returned an invalid vulnerability match")
        vulnerability = match["vulnerability"]
        severity = vulnerability.get("severity", "unknown")
        severity = severity.lower() if isinstance(severity, str) else "unknown"
        severities[severity if severity in SEVERITIES else "unknown"] += 1
        fix = vulnerability.get("fix", {})
        if isinstance(fix, dict) and isinstance(fix.get("versions"), list) and len(fix["versions"]) > 0:
            fix_available += 1
    result = {
        "status": "completed",
        "engine": runner_engine(),
        "workspaceId": workspace_id,
        "vulnerabilityCount": len(matches),
        "fixAvailableCount": fix_available,
        "severities": [{"severity": severity, "count": count} for severity, count in sorted(severities.items())],
        "durationMs": int((time.monotonic() - started) * 1000),
    }
    result["resultDigest"] = hashlib.sha256(json.dumps(result, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest()
    return result


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-Grype/1.0"

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
        if self.path != "/v1/scan":
            self.respond(HTTPStatus.NOT_FOUND, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > MAX_REQUEST_BYTES:
                raise RequestError("request is invalid")
            require_token(self.headers)
            workspace_id = request_workspace(self.rfile.read(length))
            if not RUN_LOCK.acquire(blocking=False):
                self.respond(HTTPStatus.CONFLICT, {"error": "a vulnerability scan is already running"})
                return
            try:
                self.respond(HTTPStatus.OK, scan(workspace_id))
            finally:
                RUN_LOCK.release()
        except PermissionError:
            self.respond(HTTPStatus.UNAUTHORIZED, {"error": "unauthorized"})
        except (RequestError, ValueError):
            self.respond(HTTPStatus.BAD_REQUEST, {"error": "invalid vulnerability scan request or runner configuration"})


def main() -> None:
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()


if __name__ == "__main__":
    main()
