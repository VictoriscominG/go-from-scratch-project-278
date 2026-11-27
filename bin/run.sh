#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Running DB migrations"
goose -dir ./db/migrations postgres "${DATABASE_URL}" up

echo "[run.sh] Starting Go app on :8080"
PORT=8080 /app/bin/app &

echo "[run.sh] Waiting for the app"
for _ in $(seq 1 30); do
  (exec 3<>/dev/tcp/127.0.0.1/8080) 2>/dev/null && break
  sleep 1
done

echo "[run.sh] Starting Caddy"
exec caddy run --config /etc/caddy/Caddyfile
