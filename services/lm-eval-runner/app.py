"""Internal-only fixed-suite LM Evaluation Harness runner.

It executes no caller-provided command, task, endpoint, model id, prompt, or
dataset. The only suite is a six-case synthetic fixture shipped in this image.
"""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import hashlib
from importlib.metadata import version
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time
from urllib.parse import urlparse


MAX_REQUEST_BYTES = 256
MAX_RUN_SECONDS = 120
MODEL_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/-]{0,95}$")
SUITE = "hai_synthetic_v1"
CASE_COUNT = 6
ENGINE = f"lm-eval {version('lm-eval')}"
TASKS_PATH = "/app/tasks"


class RequestError(ValueError):
    """Raised when the fixed evaluation boundary is not respected."""


def validate_local_url(value: str) -> str:
    parsed = urlparse(value.strip())
    if parsed.scheme not in {"http", "https"} or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise RequestError("model base URL must be a plain local HTTP(S) URL")
    host = parsed.hostname.lower()
    if host not in {"localhost", "host.docker.internal", "ollama", "mistralrs"} and host not in {"127.0.0.1", "::1"}:
        raise RequestError("model base URL must use a local runner hostname")
    return value.rstrip("/")


def configuration() -> tuple[str, str]:
    model_id = os.environ.get("HAI_LM_EVAL_MODEL_ID", "").strip()
    if not MODEL_ID_PATTERN.fullmatch(model_id):
        raise RequestError("a bounded HAI_LM_EVAL_MODEL_ID is required")
    return model_id, validate_local_url(os.environ.get("HAI_LM_EVAL_MODEL_BASE_URL", ""))


def run_evaluation() -> dict:
    model_id, base_url = configuration()
    output_dir = Path(tempfile.mkdtemp(prefix="hai-lm-eval-", dir="/tmp"))
    started = time.monotonic()
    try:
        command = [
            "lm-eval", "run",
            "--model", "local-chat-completions",
            "--model_args", f"model={model_id},base_url={base_url}/chat/completions,num_concurrent=1,max_retries=0,tokenized_requests=False",
            "--include_path", TASKS_PATH,
            "--tasks", SUITE,
            "--batch_size", "1",
            "--apply_chat_template",
            "--output_path", str(output_dir),
            "--verbosity", "ERROR",
        ]
        completed = subprocess.run(
            command,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
            timeout=MAX_RUN_SECONDS,
            env={**os.environ, "HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1", "DATASETS_OFFLINE": "1"},
        )
        if completed.returncode != 0:
            raise RequestError("fixed synthetic evaluation did not complete")
        exact_match = result_metric(output_dir)
        duration_ms = int((time.monotonic() - started) * 1000)
        metadata = {"suite": SUITE, "modelId": model_id, "caseCount": CASE_COUNT, "exactMatch": exact_match}
        digest = hashlib.sha256(json.dumps(metadata, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest()
        return {
            "status": "completed",
            "engine": ENGINE,
            **metadata,
            "durationMs": duration_ms,
            "resultDigest": digest,
            "scope": "Aggregate result from HAI's six-case synthetic local suite. Raw generations, task rows, model requests, and result files are not returned, retained, or exported. Review required before any separate model-profile decision.",
        }
    except subprocess.TimeoutExpired as error:
        raise RequestError("fixed synthetic evaluation exceeded its time limit") from error
    finally:
        shutil.rmtree(output_dir, ignore_errors=True)


def result_metric(output_dir: Path) -> float:
    for candidate in output_dir.rglob("*.json"):
        try:
            payload = json.loads(candidate.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        results = payload.get("results") if isinstance(payload, dict) else None
        task = results.get(SUITE) if isinstance(results, dict) else None
        if not isinstance(task, dict):
            continue
        for key, value in task.items():
            if key.startswith("exact_match") and isinstance(value, (int, float)) and 0 <= float(value) <= 1:
                return float(value)
    raise RequestError("fixed synthetic evaluation did not return the required exact-match metric")


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-LMEval/1.0"

    def do_GET(self):
        if self.path != "/healthz":
            self.respond(404, {"error": "not found"})
            return
        try:
            model_id, base_url = configuration()
            self.respond(200, {"status": "ok", "engine": ENGINE, "configured": True, "modelId": model_id, "modelEndpoint": base_url, "suite": SUITE})
        except RequestError:
            self.respond(200, {"status": "ok", "engine": ENGINE, "configured": False, "suite": SUITE})

    def do_POST(self):
        if self.path != "/v1/run":
            self.respond(404, {"error": "not found"})
            return
        try:
            self.require_empty_request()
            self.respond(200, run_evaluation())
        except RequestError as error:
            self.respond(400, {"error": str(error)})

    def require_empty_request(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length < 0 or length > MAX_REQUEST_BYTES:
            raise RequestError("request is too large")
        raw = self.rfile.read(length) if length else b"{}"
        try:
            payload = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as error:
            raise RequestError("request must be empty JSON") from error
        if payload != {}:
            raise RequestError("runner accepts no caller-provided task, model, endpoint, or data")

    def respond(self, status: int, body: dict):
        data = json.dumps(body, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, _format, *_args):
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
