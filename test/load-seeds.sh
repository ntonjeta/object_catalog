#!/usr/bin/env bash
# Load the two seed catalogs: alpha <- test/seed-alpha.xlsx, beta <- test/seed-beta.xlsx.
# Requires the compose stack up (see docker-compose.yml) and curl on the host.
# Idempotent: import updates same-named objects instead of duplicating.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

load() { # $1=name $2=port $3=email $4=password $5=file
  local jar
  jar="$(mktemp)"
  echo ">> $1 ($2): logging in"
  curl -fsS -c "$jar" -X POST "http://127.0.0.1:$2/api/auth/login" \
    -H 'content-type: application/json' \
    -d "{\"email\":\"$3\",\"password\":\"$4\"}" >/dev/null
  echo ">> $1 ($2): importing $5"
  curl -fsS -b "$jar" -F "file=@${root}/$5" "http://127.0.0.1:$2/api/objects/import"
  echo
  rm -f "$jar"
}

load alpha 8081 admin@alpha.example alpha12345 test/seed-alpha.xlsx
load beta  8082 admin@beta.example  beta12345  test/seed-beta.xlsx
