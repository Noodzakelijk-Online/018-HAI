#!/bin/sh
set -eu

: "${POSTGRES_SEEDS:?POSTGRES_SEEDS is required}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"

echo "Waiting for local Temporal PostgreSQL..."
nc -z -w 10 "${POSTGRES_SEEDS}" "${DB_PORT:-5432}"

# This is intentionally the official Temporal PostgreSQL schema sequence. The
# setup job is one-shot and runs only inside the optional local durability profile.
for database in temporal temporal_visibility; do
  temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT:-5432}" --db "${database}" create
  temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT:-5432}" --db "${database}" setup-schema -v 0.0
done
temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT:-5432}" --db temporal update-schema -d /etc/temporal/schema/postgresql/v12/temporal/versioned
temporal-sql-tool --plugin postgres12 --ep "${POSTGRES_SEEDS}" -u "${POSTGRES_USER}" -p "${DB_PORT:-5432}" --db temporal_visibility update-schema -d /etc/temporal/schema/postgresql/v12/visibility/versioned

echo "Local Temporal schemas are ready."
