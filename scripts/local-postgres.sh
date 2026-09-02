#!/usr/bin/env bash
#
# local-postgres.sh — start a throwaway Postgres container for
# `make test-pg`. Prints the POSTGRES_URL to export.
#
# Usage:
#   scripts/local-postgres.sh [--port N] [--name NAME]
#   POSTGRES_URL="$(scripts/local-postgres.sh --port 55488 | tail -n1)" make test-pg
#
# Stop it with: docker rm -f <name>
set -euo pipefail

port=55488
name=mwanachama-git-pg
user=gituser
pass=gitpass
db=mwanachama_git_test

while [ $# -gt 0 ]; do
  case "$1" in
    --port) port="$2"; shift 2 ;;
    --name) name="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if docker inspect "$name" >/dev/null 2>&1; then
  echo "container $name already exists — reusing it (docker rm -f $name to reset)" >&2
else
  docker run -d --name "$name" \
    -e POSTGRES_USER="$user" \
    -e POSTGRES_PASSWORD="$pass" \
    -e POSTGRES_DB="$db" \
    -p "${port}:5432" \
    postgres:16-alpine >/dev/null
fi

echo "waiting for Postgres to accept connections..." >&2
for _ in $(seq 1 30); do
  if docker exec "$name" pg_isready -U "$user" -d "$db" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "ready. export this, then run \`make test-pg\`:" >&2
echo "postgres://${user}:${pass}@localhost:${port}/${db}?sslmode=disable"
