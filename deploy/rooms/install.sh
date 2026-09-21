#!/bin/sh
set -eu
if [ "$(id -u)" -ne 0 ]; then printf '%s\n' 'Run as root.' >&2; exit 1; fi
release=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
listen="127.0.0.1:18081"
allow="127.0.0.0/8"
if [ "$#" -ge 1 ]; then listen="$1"; fi
if [ "$#" -ge 2 ]; then allow="$2"; fi
tls_cert="${3:-/etc/xchat-rooms/tls.crt}"
tls_key="${4:-/etc/xchat-rooms/tls.key}"
if [ "$#" -eq 3 ] || [ "$#" -gt 4 ]; then printf '%s\n' 'Usage: install.sh [listen] [CIDRs] [TLS certificate PEM] [TLS private key PEM]' >&2; exit 1; fi
if [ ! -f "$tls_cert" ] || [ ! -r "$tls_cert" ] || [ ! -f "$tls_key" ] || [ ! -r "$tls_key" ]; then
 printf '%s\n' 'TLS certificate and private key are required. Supply both paths or provision /etc/xchat-rooms/tls.crt and tls.key before installing.' >&2; exit 1
fi
case "$listen" in *[!0-9.:]*) printf '%s\n' 'Invalid listen address' >&2; exit 1 ;; esac
case "$allow" in *[!0-9a-fA-F:.,/]*) printf '%s\n' 'Invalid CIDRs' >&2; exit 1 ;; esac
for command in systemctl python3 install cp find grep; do command -v "$command" >/dev/null; done
if [ ! -f "$release/kaomoji.json" ] || [ -L "$release/kaomoji.json" ] || [ ! -r "$release/kaomoji.json" ]; then
 printf '%s\n' 'Release is missing a safe kaomoji.json.' >&2; exit 1
fi
if [ ! -d "$release/updates" ] || [ -L "$release/updates" ] || [ ! -d "$release/installers" ] || [ -L "$release/installers" ] || [ ! -f "$release/update-public.key" ] || [ -L "$release/update-public.key" ]; then
 printf '%s\n' 'Release is missing a safe signed update/installer catalog or public key.' >&2; exit 1
fi
for catalog in "$release/updates" "$release/installers"; do
 if find "$catalog" -type l -print -quit | grep -q .; then printf 'Catalog must not contain symlinks: %s\n' "$catalog" >&2; exit 1; fi
done
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
if [ -L /etc/xchat-rooms/encryption.key ] || [ -L /etc/xchat-rooms/server.env ] || [ -L /etc/xchat-rooms/tls.crt ] || [ -L /etc/xchat-rooms/tls.key ] || [ -L /etc/xchat-rooms/kaomoji.json ]; then printf '%s\n' 'Refusing symlink configuration.' >&2; exit 1; fi
if [ ! -e /etc/xchat-rooms/kaomoji.json ]; then
 install -m 0640 -o root -g xchat-rooms "$release/kaomoji.json" /etc/xchat-rooms/kaomoji.json
fi
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
if [ "$tls_cert" != /etc/xchat-rooms/tls.crt ]; then install -m 0640 -o root -g xchat-rooms "$tls_cert" /etc/xchat-rooms/tls.crt; fi
if [ "$tls_key" != /etc/xchat-rooms/tls.key ]; then install -m 0640 -o root -g xchat-rooms "$tls_key" /etc/xchat-rooms/tls.key; fi
chown root:xchat-rooms /etc/xchat-rooms/tls.crt /etc/xchat-rooms/tls.key
chmod 0640 /etc/xchat-rooms/tls.crt /etc/xchat-rooms/tls.key
python3 - /etc/xchat-rooms/server.env <<'PY'
import pathlib
import sys
path = pathlib.Path(sys.argv[1])
lines = [line for line in path.read_text().splitlines() if not line.startswith(("XCHAT_TLS_CERT=", "XCHAT_TLS_KEY="))]
lines.extend(["XCHAT_TLS_CERT=/etc/xchat-rooms/tls.crt", "XCHAT_TLS_KEY=/etc/xchat-rooms/tls.key"])
if not any(line.startswith("XCHAT_KAOMOJI=") for line in lines):
    lines.append("XCHAT_KAOMOJI=/etc/xchat-rooms/kaomoji.json")
path.write_text("\n".join(lines) + "\n")
PY
for source in "$release/xchat-rooms-server-linux-amd64" "$release/clear_history.py" "$release/update-public.key" "$release/xchat-rooms.service" "$release/xchat-rooms-cleanup.service" "$release/xchat-rooms-cleanup.timer"; do
 if [ ! -f "$source" ] || [ -L "$source" ]; then printf 'Invalid release file: %s\n' "$source" >&2; exit 1; fi
done
for target in /opt/xchat-rooms/updates /opt/xchat-rooms/updates.new /opt/xchat-rooms/installers /opt/xchat-rooms/installers.new /etc/xchat-rooms/update-public.key /etc/xchat-rooms/update-public.key.new /opt/xchat-rooms/xchat-server.new /opt/xchat-rooms/clear_history.py.new /etc/systemd/system/xchat-rooms.service.new /etc/systemd/system/xchat-rooms-cleanup.service.new /etc/systemd/system/xchat-rooms-cleanup.timer.new; do
 if [ -L "$target" ]; then printf 'Refusing symlink: %s\n' "$target" >&2; exit 1; fi
done
if [ -e /opt/xchat-rooms/updates.new ]; then rm -rf /opt/xchat-rooms/updates.new; fi
if [ -e /opt/xchat-rooms/installers.new ]; then rm -rf /opt/xchat-rooms/installers.new; fi
rm -f /etc/xchat-rooms/update-public.key.new /opt/xchat-rooms/xchat-server.new /opt/xchat-rooms/clear_history.py.new /etc/systemd/system/xchat-rooms.service.new /etc/systemd/system/xchat-rooms-cleanup.service.new /etc/systemd/system/xchat-rooms-cleanup.timer.new
install -d -m 0755 /opt/xchat-rooms/updates.new
cp -R "$release/updates/." /opt/xchat-rooms/updates.new/
find /opt/xchat-rooms/updates.new -type d -exec chmod 0755 {} \;
find /opt/xchat-rooms/updates.new -type f -exec chmod 0644 {} \;
install -d -m 0755 /opt/xchat-rooms/installers.new
cp -R "$release/installers/." /opt/xchat-rooms/installers.new/
find /opt/xchat-rooms/installers.new -type d -exec chmod 0755 {} \;
find /opt/xchat-rooms/installers.new -type f -exec chmod 0644 {} \;
install -m 0644 "$release/update-public.key" /etc/xchat-rooms/update-public.key.new
install -m 0755 "$release/xchat-rooms-server-linux-amd64" /opt/xchat-rooms/xchat-server.new
install -m 0755 "$release/clear_history.py" /opt/xchat-rooms/clear_history.py.new
install -m 0644 "$release/xchat-rooms.service" /etc/systemd/system/xchat-rooms.service.new
install -m 0644 "$release/xchat-rooms-cleanup.service" /etc/systemd/system/xchat-rooms-cleanup.service.new
install -m 0644 "$release/xchat-rooms-cleanup.timer" /etc/systemd/system/xchat-rooms-cleanup.timer.new
/opt/xchat-rooms/xchat-server.new -validate-updates -update-dir /opt/xchat-rooms/updates.new -installer-dir /opt/xchat-rooms/installers.new -update-public-key-file /etc/xchat-rooms/update-public.key.new

if [ -e /opt/xchat-rooms/updates.previous ]; then rm -rf /opt/xchat-rooms/updates.previous; fi
if [ -e /opt/xchat-rooms/installers.previous ]; then rm -rf /opt/xchat-rooms/installers.previous; fi
rm -f /etc/xchat-rooms/update-public.key.previous /opt/xchat-rooms/xchat-server.previous /opt/xchat-rooms/clear_history.py.previous /etc/systemd/system/xchat-rooms.service.previous /etc/systemd/system/xchat-rooms-cleanup.service.previous /etc/systemd/system/xchat-rooms-cleanup.timer.previous

swapping=1
updates_had_current=0
installers_had_current=0
key_had_current=0
server_had_current=0
clear_had_current=0
service_had_current=0
cleanup_service_had_current=0
cleanup_timer_had_current=0
if [ -e /opt/xchat-rooms/updates ]; then updates_had_current=1; fi
if [ -e /opt/xchat-rooms/installers ]; then installers_had_current=1; fi
if [ -e /etc/xchat-rooms/update-public.key ]; then key_had_current=1; fi
if [ -e /opt/xchat-rooms/xchat-server ]; then server_had_current=1; fi
if [ -e /opt/xchat-rooms/clear_history.py ]; then clear_had_current=1; fi
if [ -e /etc/systemd/system/xchat-rooms.service ]; then service_had_current=1; fi
if [ -e /etc/systemd/system/xchat-rooms-cleanup.service ]; then cleanup_service_had_current=1; fi
if [ -e /etc/systemd/system/xchat-rooms-cleanup.timer ]; then cleanup_timer_had_current=1; fi
restore_directory() {
 current=$1
 previous=$2
 staged=$3
 had_current=$4
 if [ -e "$previous" ]; then
  if [ -e "$current" ]; then rm -rf "$current"; fi
  mv "$previous" "$current"
 elif [ "$had_current" -eq 0 ] && [ ! -e "$staged" ]; then
  if [ -e "$current" ]; then rm -rf "$current"; fi
 fi
}
restore_file() {
 current=$1
 previous=$2
 staged=$3
 had_current=$4
 if [ -e "$previous" ]; then
  if [ -e "$current" ]; then rm -f "$current"; fi
  mv "$previous" "$current"
 elif [ "$had_current" -eq 0 ] && [ ! -e "$staged" ]; then
  if [ -e "$current" ]; then rm -f "$current"; fi
 fi
}
rollback() {
 status=$?
 trap - EXIT HUP INT TERM
 if [ "$swapping" -eq 1 ] && [ "$status" -ne 0 ]; then
  set +e
  restore_directory /opt/xchat-rooms/updates /opt/xchat-rooms/updates.previous /opt/xchat-rooms/updates.new "$updates_had_current"
  restore_directory /opt/xchat-rooms/installers /opt/xchat-rooms/installers.previous /opt/xchat-rooms/installers.new "$installers_had_current"
  restore_file /etc/xchat-rooms/update-public.key /etc/xchat-rooms/update-public.key.previous /etc/xchat-rooms/update-public.key.new "$key_had_current"
  restore_file /opt/xchat-rooms/xchat-server /opt/xchat-rooms/xchat-server.previous /opt/xchat-rooms/xchat-server.new "$server_had_current"
  restore_file /opt/xchat-rooms/clear_history.py /opt/xchat-rooms/clear_history.py.previous /opt/xchat-rooms/clear_history.py.new "$clear_had_current"
  restore_file /etc/systemd/system/xchat-rooms.service /etc/systemd/system/xchat-rooms.service.previous /etc/systemd/system/xchat-rooms.service.new "$service_had_current"
  restore_file /etc/systemd/system/xchat-rooms-cleanup.service /etc/systemd/system/xchat-rooms-cleanup.service.previous /etc/systemd/system/xchat-rooms-cleanup.service.new "$cleanup_service_had_current"
  restore_file /etc/systemd/system/xchat-rooms-cleanup.timer /etc/systemd/system/xchat-rooms-cleanup.timer.previous /etc/systemd/system/xchat-rooms-cleanup.timer.new "$cleanup_timer_had_current"
  systemctl daemon-reload || true
  systemctl restart xchat-rooms.service || true
  systemctl restart xchat-rooms-cleanup.timer || true
 fi
 exit "$status"
}
trap rollback EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -e /opt/xchat-rooms/updates ]; then mv /opt/xchat-rooms/updates /opt/xchat-rooms/updates.previous; fi
mv /opt/xchat-rooms/updates.new /opt/xchat-rooms/updates
if [ -e /opt/xchat-rooms/installers ]; then mv /opt/xchat-rooms/installers /opt/xchat-rooms/installers.previous; fi
mv /opt/xchat-rooms/installers.new /opt/xchat-rooms/installers
if [ -e /etc/xchat-rooms/update-public.key ]; then mv /etc/xchat-rooms/update-public.key /etc/xchat-rooms/update-public.key.previous; fi
mv /etc/xchat-rooms/update-public.key.new /etc/xchat-rooms/update-public.key
if [ -e /opt/xchat-rooms/xchat-server ]; then mv /opt/xchat-rooms/xchat-server /opt/xchat-rooms/xchat-server.previous; fi
mv /opt/xchat-rooms/xchat-server.new /opt/xchat-rooms/xchat-server
if [ -e /opt/xchat-rooms/clear_history.py ]; then mv /opt/xchat-rooms/clear_history.py /opt/xchat-rooms/clear_history.py.previous; fi
mv /opt/xchat-rooms/clear_history.py.new /opt/xchat-rooms/clear_history.py
if [ -e /etc/systemd/system/xchat-rooms.service ]; then mv /etc/systemd/system/xchat-rooms.service /etc/systemd/system/xchat-rooms.service.previous; fi
mv /etc/systemd/system/xchat-rooms.service.new /etc/systemd/system/xchat-rooms.service
if [ -e /etc/systemd/system/xchat-rooms-cleanup.service ]; then mv /etc/systemd/system/xchat-rooms-cleanup.service /etc/systemd/system/xchat-rooms-cleanup.service.previous; fi
mv /etc/systemd/system/xchat-rooms-cleanup.service.new /etc/systemd/system/xchat-rooms-cleanup.service
if [ -e /etc/systemd/system/xchat-rooms-cleanup.timer ]; then mv /etc/systemd/system/xchat-rooms-cleanup.timer /etc/systemd/system/xchat-rooms-cleanup.timer.previous; fi
mv /etc/systemd/system/xchat-rooms-cleanup.timer.new /etc/systemd/system/xchat-rooms-cleanup.timer
systemctl daemon-reload
systemctl enable xchat-rooms.service
systemctl restart xchat-rooms.service
systemctl enable xchat-rooms-cleanup.timer
systemctl restart xchat-rooms-cleanup.timer
systemctl --no-pager --full status xchat-rooms.service
systemctl list-timers xchat-rooms-cleanup.timer --no-pager
swapping=0
trap - EXIT HUP INT TERM
rm -rf /opt/xchat-rooms/updates.previous
rm -rf /opt/xchat-rooms/installers.previous
rm -f /etc/xchat-rooms/update-public.key.previous /opt/xchat-rooms/xchat-server.previous /opt/xchat-rooms/clear_history.py.previous /etc/systemd/system/xchat-rooms.service.previous /etc/systemd/system/xchat-rooms-cleanup.service.previous /etc/systemd/system/xchat-rooms-cleanup.timer.previous
