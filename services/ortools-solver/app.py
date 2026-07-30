"""Bounded local scheduling proposals backed by Google OR-Tools CP-SAT.

This service is intentionally not an automation executor. It models one work
lane, solves an explicitly supplied set of opaque job IDs, and returns a
proposal. It has no source connectors, credentials, filesystem access, or
endpoint that can apply a result to a calendar or workflow.
"""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os

import ortools
from ortools.sat.python import cp_model


MAX_REQUEST_BYTES = 64 * 1024
MAX_JOBS = 100
MAX_DURATION = 24 * 60
MAX_SOLVE_SECONDS = min(max(int(os.environ.get("MAX_SOLVE_SECONDS", "2")), 1), 5)


class RequestError(ValueError):
    """Raised when a scheduling scenario would exceed the bounded contract."""


def solve(payload: dict) -> dict:
    if not isinstance(payload, dict):
        raise RequestError("request must be an object")
    day_start = integer(payload.get("dayStartMinute", 540), "dayStartMinute", 0, 24 * 60)
    day_end = integer(payload.get("dayEndMinute", 1020), "dayEndMinute", 1, 24 * 60)
    if day_end <= day_start:
        raise RequestError("dayEndMinute must be after dayStartMinute")
    raw_jobs = payload.get("jobs")
    if not isinstance(raw_jobs, list) or not raw_jobs:
        raise RequestError("jobs must contain at least one item")
    if len(raw_jobs) > MAX_JOBS:
        raise RequestError(f"jobs may contain at most {MAX_JOBS} items")

    jobs = []
    ids = set()
    for raw in raw_jobs:
        if not isinstance(raw, dict):
            raise RequestError("every job must be an object")
        job_id = raw.get("id")
        if not isinstance(job_id, str) or not safe_job_id(job_id):
            raise RequestError("job id must be a short opaque identifier")
        if job_id in ids:
            raise RequestError("job ids must be unique")
        ids.add(job_id)
        duration = integer(raw.get("durationMinutes"), "durationMinutes", 1, MAX_DURATION)
        priority = integer(raw.get("priority", 50), "priority", 1, 100)
        earliest = integer(raw.get("earliestMinute", day_start), "earliestMinute", day_start, day_end)
        latest_end = integer(raw.get("latestEndMinute", day_end), "latestEndMinute", day_start, day_end)
        if earliest + duration > latest_end:
            raise RequestError(f"job {job_id} cannot fit within its time window")
        fixed_start = raw.get("fixedStartMinute")
        if fixed_start is not None:
            fixed_start = integer(fixed_start, "fixedStartMinute", earliest, latest_end - duration)
        jobs.append({
            "id": job_id,
            "duration": duration,
            "priority": priority,
            "earliest": earliest,
            "latest_end": latest_end,
            "fixed_start": fixed_start,
        })

    model = cp_model.CpModel()
    intervals = []
    active = []
    starts = []
    for index, job in enumerate(jobs):
        latest_start = job["latest_end"] - job["duration"]
        start = model.new_int_var(job["earliest"], latest_start, f"start_{index}")
        end = model.new_int_var(job["earliest"] + job["duration"], job["latest_end"], f"end_{index}")
        present = model.new_bool_var(f"present_{index}")
        interval = model.new_optional_interval_var(start, job["duration"], end, present, f"job_{index}")
        if job["fixed_start"] is not None:
            model.add(start == job["fixed_start"]).only_enforce_if(present)
        intervals.append(interval)
        active.append(present)
        starts.append(start)
    model.add_no_overlap(intervals)

    # Prefer satisfying higher-priority work. The small start-time term is only
    # a deterministic tie-breaker; it cannot outweigh one priority point.
    model.maximize(sum(active[i] * jobs[i]["priority"] * 10_000 - starts[i] for i in range(len(jobs))))
    solver = cp_model.CpSolver()
    solver.parameters.max_time_in_seconds = MAX_SOLVE_SECONDS
    solver.parameters.num_search_workers = 1
    status = solver.solve(model)
    status_name = solver.status_name(status).lower()

    scheduled = []
    deferred = []
    if status in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        for index, job in enumerate(jobs):
            if solver.value(active[index]):
                start = solver.value(starts[index])
                scheduled.append({
                    "id": job["id"],
                    "startMinute": start,
                    "endMinute": start + job["duration"],
                    "priority": job["priority"],
                })
            else:
                deferred.append(job["id"])
        scheduled.sort(key=lambda item: (item["startMinute"], item["id"]))
        deferred.sort()
    else:
        # A non-feasible solve must still account for every supplied job so
        # callers can distinguish an infeasible plan from missing data.
        deferred = sorted(job["id"] for job in jobs)

    return {
        "status": status_name,
        "solver": f"or-tools-cp-sat {ortools.__version__}",
        "scheduled": scheduled,
        "deferred": deferred,
        "objectiveValue": int(solver.objective_value) if status in (cp_model.OPTIMAL, cp_model.FEASIBLE) else None,
        "assumptions": [
            "One local work lane with no overlapping jobs.",
            "Only supplied integer minute windows and priorities were considered.",
            "The result is a proposal; HAI does not apply it to workflows or calendars.",
        ],
    }


def integer(value, field: str, minimum: int, maximum: int) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value < minimum or value > maximum:
        raise RequestError(f"{field} must be an integer from {minimum} to {maximum}")
    return value


def safe_job_id(value: str) -> bool:
    return bool(value) and len(value) <= 96 and all(
        ("a" <= char <= "z") or ("A" <= char <= "Z") or ("0" <= char <= "9") or char in "_-"
        for char in value
    )


class Handler(BaseHTTPRequestHandler):
    server_version = "HAI-ORTools/1.0"

    def do_GET(self):
        if self.path != "/healthz":
            self.respond(404, {"error": "not found"})
            return
        self.respond(200, {"status": "ok", "solver": f"or-tools-cp-sat {ortools.__version__}"})

    def do_POST(self):
        if self.path != "/v1/schedule":
            self.respond(404, {"error": "not found"})
            return
        if self.headers.get("Content-Type", "").split(";", 1)[0].strip().lower() != "application/json":
            self.respond(415, {"error": "application/json required"})
            return
        length = self.headers.get("Content-Length")
        try:
            length = int(length)
        except (TypeError, ValueError):
            self.respond(411, {"error": "content length required"})
            return
        if length < 1 or length > MAX_REQUEST_BYTES:
            self.respond(413, {"error": "request size outside bounded limit"})
            return
        try:
            payload = json.loads(self.rfile.read(length))
            self.respond(200, solve(payload))
        except (UnicodeDecodeError, json.JSONDecodeError, RequestError) as exc:
            self.respond(400, {"error": str(exc)})
        except Exception:
            self.respond(500, {"error": "solver failed"})

    def respond(self, status: int, payload: dict):
        data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, _format, *_args):
        # Avoid request-body or identifier logging. Compose captures only server
        # lifecycle/errors from Python itself.
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
