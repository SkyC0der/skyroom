#!/bin/sh
# Verifies the public, unauthenticated API surface after deployment without
# creating Users, sessions, or durable product data.
set -eu

api_base="${SKYROOM_API_BASE_URL:-https://api.rooms.studioeco.co/api/v1}"
web_origin="${SKYROOM_WEB_ORIGIN:-https://rooms.studioeco.co}"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT HUP INT TERM

curl -fsS --retry 5 --retry-delay 2 --retry-all-errors --max-time 10 \
	"$api_base/health" >"$scratch/health.json"
grep -q '"status":"ok"' "$scratch/health.json"

curl -fsS --max-time 10 "$api_base/health/live" >"$scratch/live.json"
grep -q '"status":"alive"' "$scratch/live.json"

curl -fsS --max-time 10 "$api_base/metrics" >"$scratch/metrics.json"
grep -q '"requests_total"' "$scratch/metrics.json"
grep -q '"server_errors_total"' "$scratch/metrics.json"

curl -fsS --max-time 10 -D "$scratch/cors.headers" -o /dev/null \
	-H "Origin: $web_origin" "$api_base/health"
grep -qi "^Access-Control-Allow-Origin: $web_origin" "$scratch/cors.headers"
grep -qi '^X-Request-ID:' "$scratch/cors.headers"

blocked_status="$(curl -sS --max-time 10 -o "$scratch/origin.json" -w '%{http_code}' \
	-H 'Content-Type: application/json' -H 'Origin: https://attacker.invalid' \
	--data '{"email":"smoke-test@example.invalid"}' "$api_base/auth/codes")"
if [ "$blocked_status" != "403" ]; then
	echo "cross-origin mutation returned $blocked_status, expected 403" >&2
	exit 1
fi

echo "Skyroom smoke checks passed for $api_base"
