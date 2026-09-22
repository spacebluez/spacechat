#!/bin/sh
set -eu
umask 077

version=${SPACECHAT_VERSION:-0.4.0}
minimum=${SPACECHAT_MINIMUM_VERSION:-0.0.0}
output=${SPACECHAT_OUTPUT_DIR:-/artifacts}
signing_key=${SPACECHAT_SIGNING_KEY_PATH:-/run/secrets/update-signing.key}
client_ca=${SPACECHAT_CLIENT_CA_PATH:-/run/config/client-ca.pem}
terminal_dir=${SPACECHAT_TERMINAL_DIR:-/opt/windows-terminal}
release_bin=${SPACECHAT_RELEASE_BIN:-/usr/local/bin/spacechat-release}
server_bin=${SPACECHAT_SERVER_BIN:-/usr/local/bin/xchat-server}

valid_version() {
    case $1 in
        ''|*[!0-9.]*|.*|*.|*..*) return 1 ;;
    esac
    saved_ifs=$IFS
    IFS=.
    set -- $1
    IFS=$saved_ifs
    [ "$#" -eq 3 ] || return 1
    for component do
        case $component in
            0|[1-9]|[1-9][0-9]*) ;;
            *) return 1 ;;
        esac
    done
}

if ! valid_version "$version" || ! valid_version "$minimum"; then
    printf '%s\n' 'Versions must use MAJOR.MINOR.PATCH without leading zeroes' >&2
    exit 2
fi
if [ -e "$output" ] && [ ! -d "$output" ]; then
    printf '%s\n' 'Artifact output must be a directory' >&2
    exit 1
fi
install -d -m 0755 "$output"
output=$(CDPATH= cd -- "$output" && pwd)

private_key=$(mktemp)
staging=
cleanup() {
    rm -f "$private_key"
    if [ -n "$staging" ]; then
        rm -rf "$staging"
    fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
staging=$(mktemp -d "$output/.release.XXXXXX")

install -m 0600 "$signing_key" "$private_key"
public_key=$("$release_bin" public-key -private-key "$private_key")

client_ldflags="-s -w -X main.defaultServer=ws://127.0.0.1:18081/ws -X main.version=$version -X main.updatePublicKey=$public_key"
if [ -s "$client_ca" ]; then
    if grep -q -- '-----BEGIN .*PRIVATE KEY-----' "$client_ca" ||
        ! grep -q -- '-----BEGIN CERTIFICATE-----' "$client_ca" ||
        ! openssl crl2pkcs7 -nocrl -certfile "$client_ca" -out /dev/null
    then
        printf '%s\n' 'Client CA must contain valid public PEM certificates only' >&2
        exit 1
    fi
    encoded_ca=$(base64 "$client_ca")
    encoded_ca=$(printf '%s' "$encoded_ca" | tr -d '\n')
    client_ldflags="$client_ldflags -X main.defaultTLSCA=$encoded_ca"
fi

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$staging/spacechat.exe" ./cmd/spacechat
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$client_ldflags" -o "$staging/spacechat-client.exe" ./cmd/xchat
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$staging/spacechat" ./cmd/spacechat
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$client_ldflags" -o "$staging/spacechat-client" ./cmd/xchat

windows_package=$staging/spacechat-windows-amd64-$version
linux_package=$staging/spacechat-linux-amd64-$version
install -d -m 0700 "$windows_package" "$linux_package"
install -m 0700 "$staging/spacechat.exe" "$windows_package/spacechat.exe"
install -m 0700 "$staging/spacechat-client.exe" "$windows_package/spacechat-client.exe"
install -m 0600 deploy/client/install.ps1 "$windows_package/install.ps1"
install -m 0600 README.md "$windows_package/README.md"
cp -R "$terminal_dir" "$windows_package/terminal"
install -d -m 0700 "$windows_package/terminal/settings"
install -m 0600 scripts/windows-terminal/.portable "$windows_package/terminal/.portable"
install -m 0600 scripts/windows-terminal/settings.json "$windows_package/terminal/settings/settings.json"

install -m 0700 "$staging/spacechat" "$linux_package/spacechat"
install -m 0700 "$staging/spacechat-client" "$linux_package/spacechat-client"
install -m 0600 deploy/client/install.sh "$linux_package/install.sh"
install -m 0600 README.md "$linux_package/README.md"

windows_archive=$staging/spacechat-windows-amd64-$version.zip
linux_archive=$staging/spacechat-linux-amd64-$version.zip
(
    CDPATH= cd -- "$windows_package"
    zip -qr "$windows_archive" spacechat.exe spacechat-client.exe install.ps1 README.md terminal
)
(
    CDPATH= cd -- "$linux_package"
    zip -qr "$linux_archive" spacechat spacechat-client install.sh README.md
)

install -d -m 0700 "$staging/release"
"$release_bin" manifest \
    -version "$version" -minimum "$minimum" -private-key "$private_key" \
    -windows "$staging/spacechat-client.exe" -linux "$staging/spacechat-client" \
    -out "$staging/release/updates"
"$release_bin" installers \
    -version "$version" -server-mode request -private-key "$private_key" \
    -windows-package "$windows_archive" -linux-package "$linux_archive" \
    -out "$staging/release/installers"
printf '%s\n' "$public_key" > "$staging/release/update-public.key"

"$server_bin" -validate-updates \
    -update-dir "$staging/release/updates" \
    -installer-dir "$staging/release/installers" \
    -update-public-key-file "$staging/release/update-public.key"

find "$staging/release" -type d -exec chmod 0755 {} \;
find "$staging/release" -type f -exec chmod 0644 {} \;

current=$output/release
previous=$output/release.previous
rm -rf "$previous"
had_current=0
if [ -d "$current" ]; then
    mv "$current" "$previous"
    had_current=1
fi
if ! mv "$staging/release" "$current"; then
    if [ "$had_current" -eq 1 ]; then
        mv "$previous" "$current"
    fi
    exit 1
fi
rm -rf "$previous"
