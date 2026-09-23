#!/bin/sh
set -eu
umask 077

mode=${1:-ws}
case "$mode" in
    ws|--tls) ;;
    *) printf '%s\n' 'Usage: sh deploy/docker/smoke-test.sh [--tls]' >&2; exit 2 ;;
esac
[ "$#" -le 1 ] || exit 2
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$root"
docker compose version >/dev/null
docker info >/dev/null
temporary=$(mktemp -d)
project="spacechat-smoke-$$-$(basename "$temporary" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9')"

# Override every deployment input and bypass the user's .env and override files.
export SPACECHAT_PORT=0 SPACECHAT_VERSION=0.4.0 SPACECHAT_MINIMUM_VERSION=0.0.0
export SPACECHAT_ALLOW_CIDR=127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
export SPACECHAT_PUBLIC_URL=
export SPACECHAT_SIGNING_KEY_FILE="$root/deploy/docker/default-update-signing.seed"
export SPACECHAT_CLIENT_CA_FILE="$root/deploy/docker/empty-client-ca.pem"
export SPACECHAT_TLS_DIR="$temporary/tls"
mkdir "$SPACECHAT_TLS_DIR"
touch "$temporary/empty.env"
compose() {
    docker compose --env-file "$temporary/empty.env" --project-directory "$root" \
        -f "$root/compose.yaml" -p "$project" "$@"
}
cleanup() {
    status=$?
    trap - EXIT HUP INT TERM
    if [ "$status" -ne 0 ]; then
        compose logs --no-color --tail 80 >&2 || true
    fi
    if ! compose down -v --remove-orphans >/dev/null; then
        printf 'Failed to remove test project %s; temporary inputs remain in %s\n' "$project" "$temporary" >&2
        exit 1
    fi
    rm -rf "$temporary"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

if [ "$mode" = --tls ]; then
    openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=localhost \
        -addext subjectAltName=DNS:localhost,IP:127.0.0.1 \
        -keyout "$temporary/tls/tls.key" -out "$temporary/tls/tls.crt" 2>/dev/null
    openssl rand -base64 32 > "$temporary/update-signing.key"
    export SPACECHAT_SIGNING_KEY_FILE="$temporary/update-signing.key"
    export SPACECHAT_CLIENT_CA_FILE="$temporary/tls/tls.crt"
fi

compose up -d --build
binding=$(compose port --index 1 server 18081 | head -n 1)
port=${binding##*:}
case "$port" in ''|*[!0-9]*) printf '%s\n' 'Missing published server port' >&2; exit 1 ;; esac
# Persist the assigned port before restart, which can reallocate an ephemeral port.
export SPACECHAT_PORT="$port"
compose up -d --no-deps server
scheme=http
websocket=ws
if [ "$mode" = --tls ]; then scheme=https; websocket=wss; fi
origin="$scheme://127.0.0.1:$port"
expected_server="$websocket://127.0.0.1:$port/ws"
fetch() {
    if [ "$mode" = --tls ]; then
        curl --fail --silent --show-error --max-time 15 --cacert "$temporary/tls/tls.crt" "$@"
    else
        curl --fail --silent --show-error --max-time 15 "$@"
    fi
}
wait_healthy() {
    attempt=0
    until fetch "$origin/healthz" > /dev/null 2>&1; do
        attempt=$((attempt + 1))
        if [ "$attempt" -ge 60 ]; then
            printf '%s\n' 'Server did not become healthy' >&2
            return 1
        fi
        sleep 2
    done
    compose exec -T server /usr/local/bin/healthcheck.sh
}
check_installers() {
    fetch "$origin/install/linux" > "$temporary/install-linux"
    fetch "$origin/install/windows" > "$temporary/install-windows"
    grep -Fq "$expected_server" "$temporary/install-linux"
    grep -Fq "$expected_server" "$temporary/install-windows"
}

wait_healthy
check_installers
for endpoint in /api/kaomoji /updates/v1/manifest /updates/v1/manifest.sig /install/v1/manifest /install/v1/manifest.sig; do
    fetch "$origin$endpoint" > /dev/null
done
docker top "$(compose ps -q server)" -eo uid,pid,comm > "$temporary/processes"
awk '$1 == 10001 && $3 == "xchat-server" { found=1 } END { exit !found }' "$temporary/processes"
compose exec -T server sh -ec '
    test -s /data/rooms.db
    test "$(stat -c %a /data/encryption.key)" = 600
    test "$(stat -c %a /run/spacechat)" = 750
    test ! -e /run/secrets/update-signing.key
    sha256sum /data/encryption.key
' > "$temporary/key-before"
compose exec -T cleanup sh -ec '
    test "$(id -u)" = 10001
    test ! -e /data/encryption.key
    test ! -e /run/spacechat-tls/tls.key
    test ! -e /run/secrets/update-signing.key
'
compose restart server
wait_healthy
compose exec -T server sha256sum /data/encryption.key > "$temporary/key-after"
cmp "$temporary/key-before" "$temporary/key-after"

if [ "$mode" = --tls ]; then
    export SPACECHAT_PUBLIC_URL="wss://localhost:$port/ws"
    compose up -d --no-deps server
    wait_healthy
    expected_server=$SPACECHAT_PUBLIC_URL
    check_installers
fi

if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ]; then
    mkdir "$temporary/client-home"
    if [ "$mode" = --tls ]; then export CURL_CA_BUNDLE="$temporary/tls/tls.crt"; fi
    HOME="$temporary/client-home" XDG_CONFIG_HOME="$temporary/client-home/.config" \
        XDG_DATA_HOME="$temporary/client-home/.local/share" sh "$temporary/install-linux"
    HOME="$temporary/client-home" XDG_CONFIG_HOME="$temporary/client-home/.config" \
        XDG_DATA_HOME="$temporary/client-home/.local/share" \
        "$temporary/client-home/.local/bin/spacechat" --version > "$temporary/client-version"
    grep -Fxq 'spacechat 0.4.0' "$temporary/client-version"
else
    printf '%s\n' 'Linux client execution skipped: requires a Linux amd64 test host.'
fi

compose run --rm --no-deps cleanup --yes --socket /run/spacechat/admin.sock

if [ "$mode" = --tls ]; then
    mv "$temporary/tls/tls.key" "$temporary/saved-tls.key"
    if compose run --rm --no-deps server > "$temporary/incomplete-tls" 2>&1; then
        printf '%s\n' 'Server accepted an incomplete TLS pair' >&2
        exit 1
    fi
    grep -q 'TLS certificate and private key must be mounted together' "$temporary/incomplete-tls"
    mv "$temporary/saved-tls.key" "$temporary/tls/tls.key"
fi
printf 'Compose %s smoke test passed (%s).\n' "$mode" "$project"
