#!/bin/sh
set -eu
umask 077

cert=/tls/tls.crt
key=/tls/tls.key
if [ -f "$cert" ] && [ -f "$key" ]; then
    mkdir -p /run/spacechat-tls
    chown spacechat:spacechat /run/spacechat-tls
    chmod 0700 /run/spacechat-tls
    cp "$cert" /run/spacechat-tls/tls.crt
    cp "$key" /run/spacechat-tls/tls.key
    chown spacechat:spacechat /run/spacechat-tls/tls.crt /run/spacechat-tls/tls.key
    chmod 0600 /run/spacechat-tls/tls.crt /run/spacechat-tls/tls.key
    set -- "$@" -tls-cert /run/spacechat-tls/tls.crt -tls-key /run/spacechat-tls/tls.key
elif [ -e "$cert" ] || [ -L "$cert" ] || [ -e "$key" ] || [ -L "$key" ]; then
    printf '%s\n' 'TLS certificate and private key must be mounted together.' >&2
    exit 1
else
    rm -f /run/spacechat-tls/tls.crt /run/spacechat-tls/tls.key
    set -- "$@" -allow-insecure
fi

if [ -n "${SPACECHAT_PUBLIC_URL:-}" ]; then
    set -- "$@" -public-url "$SPACECHAT_PUBLIC_URL"
fi
allowed=${SPACECHAT_ALLOW_CIDR:-0.0.0.0/0,::/0}
# The health probe uses loopback even when the external allowlist is narrower.
allowed="127.0.0.1/32,::1/128,$allowed"
mkdir -p /data /run/spacechat
chown spacechat:spacechat /data /run/spacechat
chmod 0750 /data /run/spacechat
exec su-exec spacechat /usr/local/bin/xchat-server \
    -listen 0.0.0.0:18081 -db /data/rooms.db \
    -allow-cidr "$allowed" \
    -encryption-key-file /data/encryption.key -init-encryption-key \
    -admin-socket /run/spacechat/admin.sock \
    -kaomoji /app/kaomoji.json \
    -update-dir /artifacts/release/updates \
    -installer-dir /artifacts/release/installers \
    -update-public-key-file /artifacts/release/update-public.key "$@"
