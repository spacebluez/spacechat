#!/bin/sh
set -eu

# Probe the mode selected at startup; mounted certificates may change later.
if [ -f /run/spacechat-tls/tls.crt ] && [ -f /run/spacechat-tls/tls.key ]; then
    exec wget -q -T 2 --no-check-certificate -O /dev/null https://127.0.0.1:18081/healthz
fi
exec wget -q -T 2 -O /dev/null http://127.0.0.1:18081/healthz
