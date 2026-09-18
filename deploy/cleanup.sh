#!/bin/sh
set -eu
socket="${XCHAT_ROOMS_ADMIN_SOCKET:-/run/xchat-rooms/admin.sock}"
if [ ! -S "$socket" ]; then
    printf '%s\n' 'XChat admin socket is unavailable; history was not cleared.' >&2
    exit 1
fi
exec curl --fail --silent --show-error --max-time 30 --unix-socket "$socket" --request POST http://localhost/clear-history
