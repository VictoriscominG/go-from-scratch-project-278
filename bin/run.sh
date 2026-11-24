#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Starting service"

echo "[run.sh] Running DB migrations"
goose -dir ./db/migrations postgres "${DATABASE_URL}" up

# Go-приложение слушает 8081 внутри, наружу через Caddy — 8080
PORT=8081 /app/bin/app &
APP_PID=$!

caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
CADDY_PID=$!

trap 'kill $APP_PID $CADDY_PID 2>/dev/null; wait' TERM INT

wait -n $APP_PID $CADDY_PID
EXIT_CODE=$?
echo "[run.sh] a process exited with code $EXIT_CODE, shutting down"
kill $APP_PID $CADDY_PID 2>/dev/null
exit $EXIT_CODE
