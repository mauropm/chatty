#!/usr/bin/env bash
# Integration tests: spin up an ephemeral PostgreSQL, run the DB-backed tests
# from a Go container on the same network, then tear everything down.
#
#   ./tests/integration.sh
set -euo pipefail

cd "$(dirname "$0")/.."
NET="chatty-int-test-net"
PG="chatty-int-test-pg"
DBPASS="int_test_pass_$$"
GOMOD_VOL="chatty-go-modcache"

cleanup() {
  echo "── cleanup ──"
  sudo docker rm -f "$PG" >/dev/null 2>&1 || true
  sudo docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "── network + database ──"
sudo docker network create "$NET" >/dev/null 2>&1 || true
sudo docker rm -f "$PG" >/dev/null 2>&1 || true
sudo docker run -d --name "$PG" --network "$NET" \
  -e POSTGRES_DB=chatty -e POSTGRES_USER=chatty -e POSTGRES_PASSWORD="$DBPASS" \
  postgres:17-alpine >/dev/null

echo -n "  waiting for postgres "
for _ in $(seq 1 60); do
  if sudo docker exec "$PG" pg_isready -U chatty -d chatty >/dev/null 2>&1; then echo " ready"; break; fi
  echo -n "."; sleep 1
done

echo "── running integration tests (go test ./tests) ──"
sudo docker run --rm --network "$NET" \
  -v "$PWD":/src -w /src \
  -e CGO_ENABLED=0 -e GOFLAGS=-mod=mod \
  -e CHATTY_TEST_DATABASE=1 \
  -e CHATTY_TEST_DB_HOST="$PG" -e CHATTY_TEST_DB_PORT=5432 \
  -e CHATTY_TEST_DB_NAME=chatty -e CHATTY_TEST_DB_USER=chatty -e CHATTY_TEST_DB_PASSWORD="$DBPASS" \
  -v "$GOMOD_VOL":/go/pkg/mod -v chatty-go-buildcache:/root/.cache/go-build \
  golang:1.27-alpine go test ./tests/ -v -count=1
