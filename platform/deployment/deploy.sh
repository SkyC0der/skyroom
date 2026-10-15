#!/bin/sh
# Builds and starts the API stack from the currently checked-out commit, then
# waits for the public health endpoint through Caddy. Invoked by the VPS
# forced-command wrapper (platform/deployment/vps-deploy-wrapper.sh) after it
# has checked out the requested commit.
set -eu

cd "$(dirname "$0")/../.."
compose="docker compose -f platform/deployment/compose.yaml"
api_host="${API_ADDRESS:-api.rooms.studioeco.co}"

if [ ! -f platform/deployment/.env ]; then
	echo "platform/deployment/.env is missing; create it from production.env.example" >&2
	exit 1
fi

echo "deploying $(git rev-parse --short HEAD)"
$compose build --pull api
$compose up -d --remove-orphans

# Caddy redirects plain HTTP, so check over TLS against the local listener.
attempt=0
until curl -fsS --max-time 5 --resolve "$api_host:443:127.0.0.1" "https://$api_host/api/v1/health"; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 30 ]; then
		echo "health check failed after $attempt attempts" >&2
		$compose ps >&2
		$compose logs --tail=100 api caddy >&2
		exit 1
	fi
	sleep 3
done
echo
$compose ps
docker image prune -f >/dev/null
