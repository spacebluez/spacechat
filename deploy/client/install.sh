#!/bin/sh
set -eu
umask 077

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
    printf '%s\n' 'Usage: install.sh <ws[s]://server/path> <version> [package-directory]' >&2
    exit 2
fi

server=$1
version=$2
source_dir=${3:-$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)}

case "$server" in
    wss://*|ws://*) ;;
    *) printf '%s\n' 'Server must use ws:// or wss://' >&2; exit 2 ;;
esac
case "$server" in
    *[[:space:]\"\\]*|*'@'*|*'#'*) printf '%s\n' 'Server URL contains unsupported credentials, fragment, or characters' >&2; exit 2 ;;
esac
authority=${server#*://}
authority=${authority%%/*}
authority=${authority%%\?*}
if [ -z "$authority" ]; then printf '%s\n' 'Server URL has no host' >&2; exit 2; fi
case "$version" in
    0.0.0) ;;
    0|*[!0-9.]*|.*|*.|*..*) printf '%s\n' 'Version must use MAJOR.MINOR.PATCH' >&2; exit 2 ;;
    *)
        old_ifs=$IFS; IFS=.; set -- $version; IFS=$old_ifs
        if [ "$#" -ne 3 ]; then printf '%s\n' 'Version must use MAJOR.MINOR.PATCH' >&2; exit 2; fi
        for component in "$@"; do
            case "$component" in 0|[1-9]|[1-9][0-9]*) ;; *) printf '%s\n' 'Invalid version component' >&2; exit 2 ;; esac
        done
        ;;
esac

if [ -z "${HOME:-}" ]; then printf '%s\n' 'HOME is required' >&2; exit 1; fi
launcher_source=$source_dir/spacechat
client_source=$source_dir/spacechat-client
for path in "$launcher_source" "$client_source"; do
    if [ ! -f "$path" ] || [ -L "$path" ]; then printf 'Invalid installer input: %s\n' "$path" >&2; exit 1; fi
done

config_root=${XDG_CONFIG_HOME:-$HOME/.config}
data_root=${XDG_DATA_HOME:-$HOME/.local/share}
config_dir=$config_root/spacechat
install_root=$data_root/spacechat
bin_dir=$HOME/.local/bin
versions=$install_root/versions
target=$versions/$version
target_client=$target/spacechat-client
staging=$versions/.$version.new.$$
profile=$HOME/.profile
bash_profile=$HOME/.bash_profile
bash_login=$HOME/.bash_login

for path in "$config_dir" "$install_root" "$bin_dir" "$versions"; do
    if [ -L "$path" ]; then printf 'Refusing symlink: %s\n' "$path" >&2; exit 1; fi
done
for path in "$profile" "$bash_profile" "$bash_login"; do
    if [ -L "$path" ]; then printf 'Refusing symlinked shell profile: %s\n' "$path" >&2; exit 1; fi
done
if [ -e "$target" ]; then printf 'Version already installed: %s\n' "$version" >&2; exit 1; fi

mkdir -p -m 0700 "$config_dir" "$install_root" "$versions"
mkdir -p -m 0755 "$bin_dir"
trap 'rm -rf "$staging"' EXIT HUP INT TERM
mkdir -m 0700 "$staging"
install -m 0755 "$client_source" "$staging/spacechat-client"
embedded_version=$("$staging/spacechat-client" --version)
if [ "$embedded_version" != "spacechat $version" ]; then
    printf 'Package version mismatch: expected spacechat %s, got %s\n' "$version" "$embedded_version" >&2
    exit 1
fi
"$staging/spacechat-client" --self-check
mv "$staging" "$target"

launcher_temporary=$bin_dir/.spacechat.new.$$
install -m 0755 "$launcher_source" "$launcher_temporary"
mv -f "$launcher_temporary" "$bin_dir/spacechat"

config_temporary=$config_dir/.config.json.new.$$
allow_insecure=false
case "$server" in
    ws://*) allow_insecure=true ;;
esac
printf '{"server":"%s","allow_insecure":%s}\n' "$server" "$allow_insecure" > "$config_temporary"
chmod 0600 "$config_temporary"
mv -f "$config_temporary" "$config_dir/config.json"

current_temporary=$install_root/.current.new.$$
printf '%s\n' "$version" > "$current_temporary"
chmod 0600 "$current_temporary"
mv -f "$current_temporary" "$install_root/current"

append_spacechat_path() {
    path_profile=$1
    if ! grep -F 'export PATH="$HOME/.local/bin:$PATH"' "$path_profile" >/dev/null 2>&1; then
        printf '\n%s\n%s\n' '# Added by SpaceChat installer' 'export PATH="$HOME/.local/bin:$PATH"' >> "$path_profile"
    fi
}
if [ ! -e "$profile" ]; then : > "$profile"; chmod 0600 "$profile"; fi
append_spacechat_path "$profile"
for path in "$bash_profile" "$bash_login"; do
    if [ -e "$path" ]; then append_spacechat_path "$path"; fi
done

trap - EXIT HUP INT TERM
printf 'SpaceChat %s installed. Open a new terminal and run: spacechat\n' "$version"
