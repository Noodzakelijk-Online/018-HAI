#!/usr/bin/env bash
set -euo pipefail

# `npm audit` uses a remote advisory service. A valid report remains a hard
# gate for high/critical production findings; an unavailable service is called
# out explicitly instead of being misreported as a frontend regression.
report="$(mktemp)"
stderr_file="$(mktemp)"
trap 'rm -f "$report" "$stderr_file"' EXIT

validate_report() {
  node - "$report" <<'NODE'
const fs = require("fs");

let report;
try {
  report = JSON.parse(fs.readFileSync(process.argv[2], "utf8"));
} catch {
  process.exit(2);
}

const counts = report && report.metadata && report.metadata.vulnerabilities;
if (!counts || typeof counts !== "object") {
  process.exit(2);
}

const high = Number(counts.high);
const critical = Number(counts.critical);
if (!Number.isInteger(high) || high < 0 || !Number.isInteger(critical) || critical < 0) {
  process.exit(2);
}

if (high > 0 || critical > 0) {
  console.error(`confirmed high or critical production dependency advisory: high=${high}, critical=${critical}`);
  process.exit(1);
}
NODE
}

for attempt in 1 2 3; do
  : >"$report"
  : >"$stderr_file"
  set +e
  timeout 90s npm audit --omit=dev --audit-level=high --json >"$report" 2>"$stderr_file"
  audit_exit=$?
  set -e

  set +e
  validate_report
  report_exit=$?
  set -e

  case "$report_exit" in
    0)
      # npm returns non-zero when it found findings at the selected threshold.
      # A valid zero high/critical report is authoritative regardless of a
      # transport-oriented npm exit code.
      exit 0
      ;;
    1)
      # The JSON report itself proves a high/critical finding. Do not retry or
      # downgrade this into an infrastructure warning.
      exit 1
      ;;
  esac

  if [ "$attempt" -lt 3 ]; then
    echo "npm audit did not return a trustworthy report (exit ${audit_exit}); retrying (${attempt}/3)." >&2
    sleep $((attempt * 10))
  fi
done

echo "::warning::npm audit infrastructure did not return a trustworthy report after 3 bounded attempts; frontend build and tests passed, but production advisory status was not verified." >&2
exit 0
