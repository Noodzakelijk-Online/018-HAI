"""Internal-only, redacted Gitleaks aggregate scanner for HAI.

The runner accepts a configured snapshot name, not a filesystem path. It scans
only that read-only child under /inputs and returns aggregate rule counts. The
temporary report is redacted and deleted before the HTTP response is sent.
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
MAX_RULES = 100
MAX_FINDINGS = 100_000
MAX_DURATION_SECONDS = 300
WORKSPACE_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")
RULE_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$")
RUN_LOCK = Lock()


class RequestError(ValueError):
    """The caller or local configuration crossed a fixed runner boundary."""


def configured() -> tuple[str, Path, set[str]]:
    token = os.environ.get("HAI_GITLEAKS_RUNNER_TOKEN", "").strip()
    root = Path(os.environ.get("HAI_GITLEAKS_INPUT_ROOT", "/inputs")).resolve()
    workspaces = {value.strip() for value in os.environ.get("HAI_GITLEAKS_WORKSPACES", "").split(",") if value.strip()}
    if len(token) < 16:
        raise RequestError("runner token must have at least 16 characters")
    if not root.is_dir():
        raise RequestError("source snapshot input root is unavailable")
    if not workspaces or len(workspaces) > MAX_WORKSPACES or any(not WORKSPACE_PATTERN.fullmatch(value) for value in workspaces):
        raise RequestError("one to eight valid snapshot names are required")
    return token, root, workspaces


def runner_engine() -> str:
    try:
        response = subprocess.run(["/usr/local/bin/gitleaks", "version"], check=True, capture_output=True, text=True, timeout=5, env={"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/tmp"})
    except (OSError, subprocess.SubprocessError) as exc:
        raise RequestError("Gitleaks binary is unavailable") from exc
    version = response.stdout.strip().lstrip("v")
    if not re.fullmatch(r"\d+(?:\.\d+){1,3}(?:[-+][A-Za-z0-9.-]+)?", version):
        raise RequestError("Gitleaks returned an invalid version")
    return "gitleaks " + version


def require_token(headers) -> None:
    expected, _, _ = configured()
    if not compare_digest(expected, headers.get("X-HAI-Gitleaks-Token", "")):
        raise PermissionError("invalid runner token")


def workspace_directory(workspace_id: str) -> Path:
    _, root, approved = configured()
    if workspace_id not in approved:
        raise RequestError("workspace is not approved for secret scanning")
    requested = root / workspace_id
    if requested.is_symlink():
        raise RequestError("approved snapshot must not be a symlink")
    candidate = requested.resolve()
    if candidate.parent != root or not candidate.is_dir():
        raise RequestError("approved snapshot is unavailable")
    return candidate


def request_workspace(raw: bytes) -> str:
    try:
        payload = json.loads(raw)
        value = payload["workspaceId"]
    except (KeyError, TypeError, ValueError) as exc:
        raise RequestError("workspaceId is required") from exc
    if not isinstance(value, str) or not WORKSPACE_PATTERN.fullmatch(value):
        raise RequestError("workspaceId is invalid")
    return value


def scan(workspace_id: str) -> dict:
    workspace = workspace_directory(workspace_id)
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="hai-gitleaks-", dir="/tmp") as temporary:
        report_path = Path(temporary) / "report.json"
        command = [
            "/usr/local/bin/gitleaks", "dir", "--no-banner", "--no-color", "--redact=100",
            "--report-format", "json", "--report-path", str(report_path), "--timeout", "90",
            "--max-target-megabytes", "2", "--exit-code", "1", str(workspace),
        ]
        environment = {"PATH": "/usr/local/bin:/usr/bin:/bin", "HOME": "/tmp", "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "", "http_proxy": "", "https_proxy": "", "all_proxy": "", "NO_PROXY": "*", "no_proxy": "*"}
        try:
            result = subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=MAX_DURATION_SECONDS, env=environment, check=False)
        except (OSError, subprocess.SubprocessError) as exc:
            raise RequestError("Gitleaks scan could not complete") from exc
        if result.returncode not in {0, 1}:
            raise RequestError("Gitleaks scan could not complete")
        try:
            findings = json.loads(report_path.read_text(encoding="utf-8")) if report_path.exists() else []
        except (OSError, ValueError) as exc:
            raise RequestError("Gitleaks returned an invalid redacted report") from exc
    if not isinstance(findings, list) or len(findings) > MAX_FINDINGS:
        raise RequestError("Gitleaks returned an invalid finding count")
    rules: Counter[str] = Counter()
    files: set[str] = set()
    for finding in findings:
        if not isinstance(finding, dict):
            raise RequestError("Gitleaks returned invalid finding metadata")
        rule = finding.get("RuleID")
        file_name = finding.get("File")
        if not isinstance(rule, str) or not RULE_PATTERN.fullmatch(rule) or not isinstance(file_name, str) or not file_name or len(file_name) > 1024:
            raise RequestError("Gitleaks returned invalid finding metadata")
        rules[rule] += 1
        files.add(file_name)
    if len(rules) > MAX_RULES:
        raise RequestError("Gitleaks returned too many rule categories")
    rule_counts = [{"id": rule, "count": rules[rule]} for rule in sorted(rules)]
    metadata = {"workspaceId": workspace_id, "findingCount": len(findings), "affectedFiles": len(files), "rules": rule_counts}
    return {
        "status": "completed",
        "engine": runner_engine(),
        **metadata,
        "durationMs": round((time.monotonic() - started) * 1000),
        "resultDigest": hashlib.sha256(json.dumps(metadata, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest(),
        "scope": "Aggregate redacted metadata only. Matched content, secret values, paths, lines, commits, authors, raw reports, and source files are never returned or retained.",
    }


def response(handler: BaseHTTPRequestHandler, status: int, payload: dict) -> None:
    data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Cache-Control", "no-store")
    handler.send_header("Content-Length", str(len(data)))
    handler.end_headers()
    handler.wfile.write(data)


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-Gitleaks/1.0"

    def log_message(self, _format: str, *_args) -> None:
        return

    def do_GET(self) -> None:
        if self.path != "/healthz":
            response(self, HTTPStatus.NOT_FOUND, {"error": "not found"})
            return
        try:
            configured()
            response(self, HTTPStatus.OK, {"status": "ok", "engine": runner_engine(), "configured": True})
        except RequestError:
            response(self, HTTPStatus.OK, {"status": "ok", "engine": "gitleaks unavailable", "configured": False})

    def do_POST(self) -> None:
        if self.path != "/v1/scan":
            response(self, HTTPStatus.NOT_FOUND, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            response(self, HTTPStatus.BAD_REQUEST, {"error": "invalid content length"})
            return
        if length <= 0 or length > MAX_REQUEST_BYTES:
            response(self, HTTPStatus.REQUEST_ENTITY_TOO_LARGE, {"error": "request must contain one bounded workspaceId"})
            return
        try:
            require_token(self.headers)
            workspace_id = request_workspace(self.rfile.read(length))
        except PermissionError:
            response(self, HTTPStatus.UNAUTHORIZED, {"error": "unauthorized"})
            return
        except RequestError as exc:
            response(self, HTTPStatus.BAD_REQUEST, {"error": str(exc)})
            return
        if not RUN_LOCK.acquire(blocking=False):
            response(self, HTTPStatus.CONFLICT, {"error": "a local secret scan is already running"})
            return
        try:
            response(self, HTTPStatus.OK, scan(workspace_id))
        except RequestError:
            response(self, HTTPStatus.BAD_GATEWAY, {"error": "local Gitleaks scan could not complete"})
        finally:
            RUN_LOCK.release()


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
