#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    printf '%s\n' 'Run as root.' >&2
    exit 1
fi
release=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
listen="${1:-192.168.33.216:18080}"
allow="${2:-192.168.0.0/16,127.0.0.0/8}"
case "$listen" in *[!0-9.:]*) printf '%s\n' 'Invalid listen address' >&2; exit 1 ;; esac
case "$allow" in *[!0-9a-fA-F:.,/]*) printf '%s\n' 'Invalid CIDR list' >&2; exit 1 ;; esac
command -v systemctl >/dev/null
command -v curl >/dev/null
if [ ! -s /etc/xchat/access.key ]; then
    printf '%s\n' 'Create /etc/xchat/access.key with the shared key before installation; never pass the key on the command line.' >&2
    exit 1
fi
if [ -L /etc/xchat/access.key ]; then printf '%s\n' 'Refusing a symlink key file.' >&2; exit 1; fi
for target in /opt/xchat /var/lib/xchat /etc/xchat /etc/systemd/system/xchat.service; do
    if [ -L "$target" ]; then printf '%s\n' "Refusing symlink: $target" >&2; exit 1; fi
done
if [ -e /etc/systemd/system/xchat.service ] && ! grep -q '^Description=XChat internal chat server$' /etc/systemd/system/xchat.service; then
    printf '%s\n' 'Refusing to replace an unrelated xchat service.' >&2
    exit 1
fi
if ! getent group xchat >/dev/null; then groupadd --system xchat; fi
if ! id xchat >/dev/null 2>&1; then
    useradd --system --gid xchat --home-dir /var/lib/xchat --no-create-home --shell /sbin/nologin xchat
fi
install -d -m 0755 /opt/xchat
install -d -m 0750 -o xchat -g xchat /var/lib/xchat
install -d -m 0750 -o root -g xchat /etc/xchat
chown root:xchat /etc/xchat/access.key
chmod 0640 /etc/xchat/access.key
if [ -f /opt/xchat/xchat-server ]; then cp -p /opt/xchat/xchat-server /opt/xchat/xchat-server.previous; fi
install -m 0755 "$release/xchat-server-linux-amd64" /opt/xchat/xchat-server.new
mv /opt/xchat/xchat-server.new /opt/xchat/xchat-server
if [ ! -f /etc/xchat/server.env ]; then
    printf 'XCHAT_LISTEN=%s\nXCHAT_ALLOW_CIDR=%s\n' "$listen" "$allow" > /etc/xchat/server.env
    chmod 0640 /etc/xchat/server.env
    chown root:xchat /etc/xchat/server.env
fi
install -m 0644 "$release/xchat.service" /etc/systemd/system/xchat.service
install -m 0755 "$release/cleanup.sh" /opt/xchat/cleanup.sh
install -m 0644 "$release/xchat-cleanup.service" /etc/systemd/system/xchat-cleanup.service
install -m 0644 "$release/xchat-cleanup.timer" /etc/systemd/system/xchat-cleanup.timer
systemctl daemon-reload
systemctl enable xchat.service
systemctl restart xchat.service
systemctl enable xchat-cleanup.timer
systemctl restart xchat-cleanup.timer
systemctl --no-pager --full status xchat.service
systemctl list-timers xchat-cleanup.timer --no-pager
