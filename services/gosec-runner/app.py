"""Internal-only, aggregate Gosec runner for HAI.

The runner accepts a configured snapshot name rather than a path. It requires a
self-contained Go module with vendored dependencies and turns Go module network
resolution off. It returns only aggregate severity/confidence counts; source,
paths, rules, CWEs, findings, and raw reports never leave the runner.
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
import tempfile
import time
from threading import Lock


MAX_REQUEST_BYTES = 256
MAX_WORKSPACES = 8
MAX_REPORT_BYTES = 8 * 1024 * 1024
MAX_FINDINGS = 100_000
MAX_DURATION_SECONDS = 300
MAX_SNAPSHOT_ENTRIES = 100_000
WORKSPACE_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
VERSION_PATTERN = re.compile(r"^\d+(?:\.\d+){1,3}(?:[-+][A-Za-z0-9.-]+)?$")
LEVELS = {"high", "medium", "low"}
RUN_LOCK = Lock()


class RequestError(ValueError):
    """The caller or local configuration crossed a fixed runner boundary."""


def configured() -> tuple[str, Path, set[str]]:
    token = os.environ.get("HAI_GOSEC_RUNNER_TOKEN", "").strip()
    root = Path(os.environ.get("HAI_GOSEC_INPUT_ROOT", "/inputs")).resolve()
    workspaces = {value.strip() for value in os.environ.get("HAI_GOSEC_WORKSPACES", "").split(",") if value.strip()}
    if len(token) < 16:
        raise RequestError("runner token must have at least 16 characters")
    if not root.is_dir():
        raise RequestError("source snapshot input root is unavailable")
    if not workspaces or len(workspaces) > MAX_WORKSPACES or any(not WORKSPACE_PATTERN.fullmatch(value) for value in workspaces):
        raise RequestError("one to eight valid snapshot names are required")
    return token, root, workspaces


def environment() -> dict[str, str]:
    return {
        "PATH": "/usr/local/bin:/usr/local/go/bin:/usr/bin:/bin",
        "HOME": "/tmp",
        "GOPROXY": "off",
        "GOSUMDB": "off",
        "GONOSUMDB": "*",
        "GOFLAGS": "-mod=vendor",
        "GOTOOLCHAIN": "local",
        "HTTP_PROXY": "",
        "HTTPS_PROXY": "",
        "ALL_PROXY": "",
        "NO_PROXY": "*",
    }


def runner_engine() -> str:
    try:
        response = subprocess.run(["/usr/local/bin/gosec", "-version"], check=True, capture_output=True, text=True, timeout=5, env=environment())
    except (OSError, subprocess.SubprocessError) as exc:
        raise RequestError("Gosec binary is unavailable") from exc
    match = re.search(r"^Version:\s*([^\s]+)", response.stdout, re.MULTILINE)
    version = match.group(1).lstrip("v") if match else ""
    if not VERSION_PATTERN.fullmatch(version):
        raise RequestError("Gosec returned an invalid version")
    return "gosec " + version


def require_token(headers) -> None:
    expected, _, _ = configured()
    if not compare_digest(expected, headers.get("X-HAI-Gosec-Token", "")):
        raise PermissionError("invalid runner token")


def workspace_directory(workspace_id: str) -> Path:
    _, root, approved = configured()
    if workspace_id not in approved:
        raise RequestError("workspace is not approved for Go security scanning")
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
    if not (candidate / "go.mod").is_file() or not (candidate / "vendor" / "modules.txt").is_file():
        raise RequestError("approved Go snapshot must include go.mod and vendored dependencies")
    return candidate


def request_workspace(raw: bytes) -> str:
    try:
        value = json.loads(raw)["workspaceId"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("workspaceId is required") from exc
    if not isinstance(value, str) or not WORKSPACE_PATTERN.fullmatch(value):
        raise RequestError("workspaceId is invalid")
    return value


def aggregate(report: dict) -> tuple[int, list[dict], list[dict]]:
    issues = report.get("Issues")
    if not isinstance(issues, list) or len(issues) > MAX_FINDINGS:
        raise RequestError("Gosec returned an invalid or oversized report")
    severities: Counter[str] = Counter()
    confidences: Counter[str] = Counter()
    for issue in issues:
        if not isinstance(issue, dict):
            raise RequestError("Gosec returned an invalid finding")
        severity = str(issue.get("severity", "")).lower()
        confidence = str(issue.get("confidence", "")).lower()
        if severity not in LEVELS or confidence not in LEVELS:
            raise RequestError("Gosec returned an unsupported aggregate level")
        severities[severity] += 1
        confidences[confidence] += 1
    return len(issues), [{"severity": level, "count": count} for level, count in sorted(severities.items())], [{"confidence": level, "count": count} for level, count in sorted(confidences.items())]


def scan(workspace_id: str) -> dict:
    workspace = workspace_directory(workspace_id)
    started = time.monotonic()
    # The Compose service provides only a tmpfs-backed /tmp. Leaving directory
    # selection to tempfile keeps host-side runner tests portable without
    # widening the container's writable surface.
    with tempfile.NamedTemporaryFile(prefix="gosec-", suffix=".json", delete=False) as report_file:
        report_path = Path(report_file.name)
    try:
        subprocess.run(["/usr/local/bin/gosec", "-no-fail", "-fmt=json", "-out", str(report_path), "./..."], cwd=workspace, check=True, capture_output=True, timeout=MAX_DURATION_SECONDS, env=environment())
        if report_path.stat().st_size > MAX_REPORT_BYTES:
            raise RequestError("Gosec report exceeded the fixed output limit")
        with report_path.open("rb") as handle:
            finding_count, severities, confidences = aggregate(json.load(handle))
    except (OSError, subprocess.SubprocessError, ValueError, json.JSONDecodeError) as exc:
        if isinstance(exc, RequestError):
            raise
        raise RequestError("Gosec Go security scan failed") from exc
    finally:
        report_path.unlink(missing_ok=True)
    result = {
        "status": "completed",
        "engine": runner_engine(),
        "workspaceId": workspace_id,
        "findingCount": finding_count,
        "severities": severities,
        "confidences": confidences,
        "durationMs": int((time.monotonic() - started) * 1000),
    }
    result["resultDigest"] = hashlib.sha256(json.dumps(result, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest()
    return result


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-Gosec/1.0"

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
                self.respond(HTTPStatus.CONFLICT, {"error": "a Go security scan is already running"})
                return
            try:
                self.respond(HTTPStatus.OK, scan(workspace_id))
            finally:
                RUN_LOCK.release()
        except PermissionError:
            self.respond(HTTPStatus.UNAUTHORIZED, {"error": "unauthorized"})
        except (RequestError, ValueError):
            self.respond(HTTPStatus.BAD_REQUEST, {"error": "invalid Go security scan request or runner configuration"})


def main() -> None:
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()


if __name__ == "__main__":
    main()
