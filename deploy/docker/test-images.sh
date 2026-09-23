#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$root"
server_image=${SPACECHAT_SERVER_IMAGE:-spacechat-server:test}
cleanup_image=${SPACECHAT_CLEANUP_IMAGE:-spacechat-cleanup:test}
artifacts_image=${SPACECHAT_ARTIFACTS_IMAGE:-spacechat-artifacts:test}
temporary=$(mktemp -d)
secret=$(mktemp "$root/.docker-secret.XXXXXX.key")
cleanup() {
    status=$?
    trap - EXIT HUP INT TERM
    rm -f "$secret"
    rm -rf "$temporary"
    exit "$status"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
printf '%s\n' 'build-context exclusion fixture, not a real key' > "$secret"

docker build --build-arg GOPROXY --target server -t "$server_image" .
docker build --target cleanup -t "$cleanup_image" .
docker build --build-arg GOPROXY --build-arg WINDOWS_TERMINAL_URL --target artifacts -t "$artifacts_image" .

docker run --rm --entrypoint sh "$server_image" -ec '
    test ! -e /usr/local/go
    test ! -e /run/secrets/update-signing.key
    test ! -e /src
    test "$(su-exec spacechat id -u)" = 10001
    test "$(stat -c %a /run/spacechat)" = 750
    test "$(stat -c %u /data)" = 10001
'
docker run --rm --entrypoint sh "$cleanup_image" -ec '
    test "$(id -u)" = 10001
    test ! -e /usr/local/go
    test ! -e /run/secrets/update-signing.key
'
docker run --rm "$cleanup_image" --help > "$temporary/help"
grep -q -- '--schedule-daily' "$temporary/help"

for side in tls.crt tls.key; do
    mkdir "$temporary/$side"
    printf '%s\n' fixture > "$temporary/$side/$side"
    if docker run --rm --mount "type=bind,src=$temporary/$side,dst=/tls,readonly" \
        "$server_image" > "$temporary/tls-error" 2>&1; then
        printf '%s\n' 'Incomplete TLS pair was accepted' >&2
        exit 1
    fi
    grep -q 'TLS certificate and private key must be mounted together' "$temporary/tls-error"
done

docker run --rm --network none --entrypoint sh "$artifacts_image" -ec '
    test ! -e "/src/$1"
    test ! -e /run/secrets/update-signing.key
    for platform in linux windows; do
        CGO_ENABLED=0 GOOS=$platform GOARCH=amd64 go list -deps ./cmd/xchat ./cmd/spacechat >/dev/null
    done
' sh "$(basename "$secret")"
printf '%s\n' 'Container image checks passed.'
