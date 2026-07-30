#!/bin/sh
set -eu

address="${TEMPORAL_ADDRESS:-temporal:7233}"
namespace="${DEFAULT_NAMESPACE:-default}"
attempt=1
while ! temporal operator cluster health --address "${address}"; do
  if [ "${attempt}" -ge 30 ]; then
    echo "Temporal did not become healthy in time" >&2
    exit 1
  fi
  attempt=$((attempt + 1))
  sleep 2
done

if temporal operator namespace describe -n "${namespace}" --address "${address}" >/dev/null 2>&1; then
  echo "Temporal namespace ${namespace} already exists."
  exit 0
fi
temporal operator namespace create -n "${namespace}" --address "${address}"
