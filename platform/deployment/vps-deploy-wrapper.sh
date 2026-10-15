#!/bin/sh
# Forced command for the CI deploy key. Install on the VPS as
# /home/skyroom/bin/skyroom-deploy and reference it from authorized_keys:
#
#   command="/home/skyroom/bin/skyroom-deploy",restrict ssh-ed25519 AAAA... skyroom-ci
#
# The key can only ask for "deploy <sha>", and the commit must be reachable
# from origin/staging.
set -eu

repo=/home/skyroom/apps/skyroom
branch=staging

case "${SSH_ORIGINAL_COMMAND:-}" in
	"deploy "*) sha="${SSH_ORIGINAL_COMMAND#deploy }" ;;
	*) echo "usage: deploy <commit-sha>" >&2; exit 2 ;;
esac
if ! printf '%s' "$sha" | grep -Eq '^[0-9a-f]{40}$'; then
	echo "expected a full 40-character commit SHA" >&2
	exit 2
fi

exec 9>/home/skyroom/.skyroom-deploy.lock
flock 9

cd "$repo"
git fetch --quiet origin "$branch"
if ! git merge-base --is-ancestor "$sha" "origin/$branch"; then
	echo "commit $sha is not on origin/$branch" >&2
	exit 3
fi
git checkout --quiet --detach "$sha"
exec ./platform/deployment/deploy.sh
