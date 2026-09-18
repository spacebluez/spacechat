#!/bin/sh
set -eu
printf '%s\n' 'This branch uses the isolated rooms deployment. Build scripts/build.ps1 and run dist/rooms-server/install.sh. Do not upgrade the legacy service with this binary.' >&2
exit 1
