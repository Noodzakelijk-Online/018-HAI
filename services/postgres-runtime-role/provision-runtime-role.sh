#!/bin/sh
set -eu

for name in PGHOST PGUSER PGPASSWORD PGDATABASE HAI_RUNTIME_DB_USER HAI_RUNTIME_DB_PASSWORD; do
  eval "value=\${$name:-}"
  if [ -z "$value" ]; then
    echo "missing required runtime-role setting: $name" >&2
    exit 64
  fi
done

if ! printf '%s\n' "$PGUSER" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]{0,62}$'; then
  echo "PGUSER must be a PostgreSQL identifier" >&2
  exit 64
fi
if ! printf '%s\n' "$HAI_RUNTIME_DB_USER" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]{0,62}$'; then
  echo "HAI_RUNTIME_DB_USER must be a PostgreSQL identifier" >&2
  exit 64
fi

if [ "$PGUSER" = "$HAI_RUNTIME_DB_USER" ]; then
  echo "runtime database user must differ from the schema owner" >&2
  exit 64
fi

# psql's quoted variable forms and format(%I/%L) keep role names and passwords
# out of shell interpolation. This service is deliberately idempotent because
# it runs on every controlled Compose startup and rotates the runtime password
# to the value held in the ignored local environment file.
psql -v ON_ERROR_STOP=1 \
  --set=runtime_user="$HAI_RUNTIME_DB_USER" \
  --set=runtime_password="$HAI_RUNTIME_DB_PASSWORD" \
  --set=owner_user="$PGUSER" \
  --set=database_name="$PGDATABASE" <<'SQL'
SELECT format(
  'CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT',
  :'runtime_user', :'runtime_password'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'runtime_user')
\gexec

SELECT format(
  'ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT',
  :'runtime_user', :'runtime_password'
)
\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO %I', :'database_name', :'runtime_user')
\gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'runtime_user')
\gexec
SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %I', :'runtime_user')
\gexec
SELECT format('GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO %I', :'runtime_user')
\gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I', :'owner_user', :'runtime_user')
\gexec
SELECT format('ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO %I', :'owner_user', :'runtime_user')
\gexec
SQL

echo "runtime database role provisioned"
