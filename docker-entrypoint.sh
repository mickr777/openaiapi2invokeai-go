#!/bin/sh
set -eu

PUID="\${PUID:-99}"
PGID="\${PGID:-100}"

mkdir -p /config
chown "\${PUID}:\${PGID}" /config 2>/dev/null || true

exec su-exec "\${PUID}:\${PGID}" "$@"
