#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then printf '%s\n' 'Run as root.' >&2; exit 1; fi
release=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
listen="127.0.0.1:18081"
allow="127.0.0.0/8"
if [ "$#" -ge 1 ]; then listen="$1"; fi
if [ "$#" -ge 2 ]; then allow="$2"; fi
case "$listen" in *[!0-9.:]*) printf '%s\n' 'Invalid listen address' >&2; exit 1 ;; esac
case "$allow" in *[!0-9a-fA-F:.,/]*) printf '%s\n' 'Invalid CIDRs' >&2; exit 1 ;; esac
for command in systemctl python3 install cp find; do command -v "$command" >/dev/null; done
if [ ! -d "$release/updates" ] || [ -L "$release/updates" ] || [ ! -f "$release/update-public.key" ] || [ -L "$release/update-public.key" ]; then
 printf '%s\n' 'Release is missing a safe signed update catalog or public key.' >&2; exit 1
fi
if find "$release/updates" -type l -print -quit | grep -q .; then printf '%s\n' 'Update catalog must not contain symlinks.' >&2; exit 1; fi
for target in /opt/xchat-rooms /var/lib/xchat-rooms /etc/xchat-rooms /run/xchat-rooms /etc/systemd/system/xchat-rooms.service /etc/systemd/system/xchat-rooms-cleanup.service /etc/systemd/system/xchat-rooms-cleanup.timer; do
 if [ -L "$target" ]; then printf 'Refusing symlink: %s\n' "$target" >&2; exit 1; fi
done
if [ -e /etc/systemd/system/xchat-rooms.service ] && ! grep -q '^Description=XChat encrypted rooms server$' /etc/systemd/system/xchat-rooms.service; then
 printf '%s\n' 'Refusing to replace an unrelated service.' >&2; exit 1
fi
if ! getent group xchat-rooms >/dev/null; then groupadd --system xchat-rooms; fi
if ! id xchat-rooms >/dev/null 2>&1; then useradd --system --gid xchat-rooms --home-dir /var/lib/xchat-rooms --no-create-home --shell /sbin/nologin xchat-rooms; fi
install -d -m 0755 /opt/xchat-rooms
install -d -m 0700 -o xchat-rooms -g xchat-rooms /var/lib/xchat-rooms
install -d -m 0750 -o root -g xchat-rooms /etc/xchat-rooms
if [ -L /etc/xchat-rooms/encryption.key ] || [ -L /etc/xchat-rooms/server.env ]; then printf '%s\n' 'Refusing symlink configuration.' >&2; exit 1; fi
if [ ! -e /etc/xchat-rooms/encryption.key ]; then
 if [ -e /var/lib/xchat-rooms/rooms.db ]; then printf '%s\n' 'Existing database requires its original encryption key.' >&2; exit 1; fi
 python3 -c 'import os,base64; path="/etc/xchat-rooms/encryption.key"; descriptor=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600); os.write(descriptor,base64.b64encode(os.urandom(32))+b"\n"); os.close(descriptor)'
fi
chown root:xchat-rooms /etc/xchat-rooms/encryption.key
chmod 0640 /etc/xchat-rooms/encryption.key
if [ ! -e /etc/xchat-rooms/server.env ]; then
 printf 'XCHAT_LISTEN=%s\nXCHAT_ALLOW_CIDR=%s\n' "$listen" "$allow" > /etc/xchat-rooms/server.env
 chmod 0640 /etc/xchat-rooms/server.env
 chown root:xchat-rooms /etc/xchat-rooms/server.env
fi
for target in /opt/xchat-rooms/updates /opt/xchat-rooms/updates.new /etc/xchat-rooms/update-public.key /etc/xchat-rooms/update-public.key.new; do
 if [ -L "$target" ]; then printf 'Refusing symlink: %s\n' "$target" >&2; exit 1; fi
done
if [ -e /opt/xchat-rooms/updates.new ]; then rm -rf /opt/xchat-rooms/updates.new; fi
install -d -m 0755 /opt/xchat-rooms/updates.new
cp -R "$release/updates/." /opt/xchat-rooms/updates.new/
find /opt/xchat-rooms/updates.new -type d -exec chmod 0755 {} \;
find /opt/xchat-rooms/updates.new -type f -exec chmod 0644 {} \;
install -m 0644 "$release/update-public.key" /etc/xchat-rooms/update-public.key.new
install -m 0755 "$release/xchat-rooms-server-linux-amd64" /opt/xchat-rooms/xchat-server.new
/opt/xchat-rooms/xchat-server.new -validate-updates -update-dir /opt/xchat-rooms/updates.new -update-public-key-file /etc/xchat-rooms/update-public.key.new

if [ -e /opt/xchat-rooms/updates.previous ]; then rm -rf /opt/xchat-rooms/updates.previous; fi
if [ -e /etc/xchat-rooms/update-public.key.previous ]; then rm -f /etc/xchat-rooms/update-public.key.previous; fi
if [ -e /opt/xchat-rooms/xchat-server.previous ]; then rm -f /opt/xchat-rooms/xchat-server.previous; fi
if [ -e /etc/systemd/system/xchat-rooms.service.previous ]; then rm -f /etc/systemd/system/xchat-rooms.service.previous; fi

swapping=1
rollback() {
 status=$?
 trap - EXIT
 if [ "$swapping" -eq 1 ] && [ "$status" -ne 0 ]; then
  if [ -e /opt/xchat-rooms/updates.previous ]; then rm -rf /opt/xchat-rooms/updates; mv /opt/xchat-rooms/updates.previous /opt/xchat-rooms/updates; fi
  if [ -e /etc/xchat-rooms/update-public.key.previous ]; then rm -f /etc/xchat-rooms/update-public.key; mv /etc/xchat-rooms/update-public.key.previous /etc/xchat-rooms/update-public.key; fi
  if [ -e /opt/xchat-rooms/xchat-server.previous ]; then rm -f /opt/xchat-rooms/xchat-server; mv /opt/xchat-rooms/xchat-server.previous /opt/xchat-rooms/xchat-server; fi
  if [ -e /etc/systemd/system/xchat-rooms.service.previous ]; then rm -f /etc/systemd/system/xchat-rooms.service; mv /etc/systemd/system/xchat-rooms.service.previous /etc/systemd/system/xchat-rooms.service; fi
  systemctl daemon-reload || true
  systemctl restart xchat-rooms.service || true
 fi
 exit "$status"
}
trap rollback EXIT

if [ -e /opt/xchat-rooms/updates ]; then mv /opt/xchat-rooms/updates /opt/xchat-rooms/updates.previous; fi
if [ -e /etc/xchat-rooms/update-public.key ]; then mv /etc/xchat-rooms/update-public.key /etc/xchat-rooms/update-public.key.previous; fi
if [ -e /opt/xchat-rooms/xchat-server ]; then mv /opt/xchat-rooms/xchat-server /opt/xchat-rooms/xchat-server.previous; fi
if [ -e /etc/systemd/system/xchat-rooms.service ]; then mv /etc/systemd/system/xchat-rooms.service /etc/systemd/system/xchat-rooms.service.previous; fi
mv /opt/xchat-rooms/updates.new /opt/xchat-rooms/updates
mv /etc/xchat-rooms/update-public.key.new /etc/xchat-rooms/update-public.key
mv /opt/xchat-rooms/xchat-server.new /opt/xchat-rooms/xchat-server
install -m 0755 "$release/clear_history.py" /opt/xchat-rooms/clear_history.py
for unit in xchat-rooms.service xchat-rooms-cleanup.service xchat-rooms-cleanup.timer; do install -m 0644 "$release/$unit" "/etc/systemd/system/$unit"; done
systemctl daemon-reload
systemctl enable xchat-rooms.service
systemctl restart xchat-rooms.service
systemctl enable xchat-rooms-cleanup.timer
systemctl restart xchat-rooms-cleanup.timer
systemctl --no-pager --full status xchat-rooms.service
systemctl list-timers xchat-rooms-cleanup.timer --no-pager
swapping=0
trap - EXIT
rm -rf /opt/xchat-rooms/updates.previous
rm -f /etc/xchat-rooms/update-public.key.previous /opt/xchat-rooms/xchat-server.previous /etc/systemd/system/xchat-rooms.service.previous
