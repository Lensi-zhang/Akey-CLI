#!/bin/sh
set -eu

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

usage="Usage: install.sh [--uninstall] [--purge] [version]"
uninstall=
purge=
version=latest
version_set=

for argument in "$@"; do
    case "$argument" in
        --uninstall) uninstall=1 ;;
        --purge) purge=1 ;;
        --help|-h)
            printf '%s\n' "$usage"
            exit 0
            ;;
        -*)
            fail "Unknown option: $argument. $usage"
            ;;
        *)
            [ -z "$version_set" ] || fail "Unexpected argument: $argument. $usage"
            version=$argument
            version_set=1
            ;;
    esac
done

if [ -n "$purge" ] && [ -z "$uninstall" ]; then
    fail "--purge must be used together with --uninstall."
fi

[ -n "${HOME:-}" ] || fail "HOME is not set."
install_dir="$HOME/.local/bin"

if [ -n "$uninstall" ]; then
    if [ -n "$purge" ]; then
        akey_binary=
        if [ -x "$install_dir/akey" ]; then
            akey_binary="$install_dir/akey"
        elif command -v akey >/dev/null 2>&1; then
            akey_binary=$(command -v akey)
        fi

        if [ -n "$akey_binary" ]; then
            profiles=$("$akey_binary" profile list --output json 2>/dev/null || true)
            for profile_name in $(printf '%s\n' "$profiles" | sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([a-z0-9][a-z0-9_-]*\)".*/\1/p'); do
                "$akey_binary" profile delete "$profile_name" --yes >/dev/null 2>&1 ||
                    printf 'Warning: could not delete profile %s.\n' "$profile_name" >&2
            done
        else
            printf 'Warning: akey binary not found; API keys in the system keyring were not removed.\n' >&2
        fi

        if [ -d "$HOME/.akey" ]; then
            rm -rf "$HOME/.akey"
            printf 'Removed profile data at %s\n' "$HOME/.akey"
        fi
    fi

    if [ -f "$install_dir/akey" ]; then
        rm -f "$install_dir/akey"
        printf 'Removed %s\n' "$install_dir/akey"
    else
        printf 'akey is not installed at %s\n' "$install_dir/akey"
    fi

    if [ -z "$purge" ]; then
        printf 'Profile data and API keys were kept. Re-run with --purge to remove them.\n'
    fi
    exit 0
fi

if [ "$version" != "latest" ]; then
    case "$version" in
        v*) ;;
        *) version="v$version" ;;
    esac
    case "$version" in
        v[0-9A-Za-z]*) ;;
        *) fail "Invalid version. Use latest, v1.0.0, or 1.0.0." ;;
    esac
    case "$version" in
        *[!0-9A-Za-z._-]*) fail "Invalid version. Use latest, v1.0.0, or 1.0.0." ;;
    esac
fi

case "$(uname -s)" in
    Darwin) asset_os=Darwin ;;
    Linux) asset_os=Linux ;;
    *) fail "This installer supports macOS and Linux only." ;;
esac

machine=$(uname -m)
case "$machine" in
    x86_64|amd64) asset_arch=x86_64 ;;
    aarch64|arm64) asset_arch=arm64 ;;
    armv7l|armv7) asset_arch=armv7 ;;
    mips*) fail "MIPS/OpenWrt releases require manual installation." ;;
    *) fail "Unsupported architecture: $machine" ;;
esac

for required_command in curl tar mktemp; do
    command -v "$required_command" >/dev/null 2>&1 || fail "Required command not found: $required_command"
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    fail "Either sha256sum or shasum is required."
fi

archive_name="akey_${asset_os}_${asset_arch}.tar.gz"
release_base="https://github.com/Lensi-zhang/Akey-CLI/releases"
if [ "$version" = "latest" ]; then
    download_base="$release_base/latest/download"
else
    download_base="$release_base/download/$version"
fi

tmp_dir=
stage_path=
cleanup() {
    if [ -n "${stage_path:-}" ] && [ -e "$stage_path" ]; then
        rm -f "$stage_path"
    fi
    if [ -n "${tmp_dir:-}" ] && [ -d "$tmp_dir" ]; then
        rm -rf "$tmp_dir"
    fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/akey-install.XXXXXX") || fail "Could not create a temporary directory."
archive_path="$tmp_dir/$archive_name"
checksum_path="$tmp_dir/checksums.txt"
curl -fsSL --retry 3 "$download_base/$archive_name" -o "$archive_path" || fail "Failed to download $archive_name."
curl -fsSL --retry 3 "$download_base/checksums.txt" -o "$checksum_path" || fail "Failed to download checksums.txt."

expected_hash=
while read -r hash filename remainder; do
    if [ "$filename" = "$archive_name" ]; then
        expected_hash=$hash
        break
    fi
done < "$checksum_path"
case "$expected_hash" in
    "") fail "No SHA256 checksum found for $archive_name." ;;
    *[!0-9a-fA-F]*) fail "Invalid SHA256 checksum for $archive_name." ;;
esac
[ "${#expected_hash}" -eq 64 ] || fail "Invalid SHA256 checksum for $archive_name."

if command -v sha256sum >/dev/null 2>&1; then
    actual_hash=$(sha256sum "$archive_path")
else
    actual_hash=$(shasum -a 256 "$archive_path")
fi
actual_hash=${actual_hash%% *}
[ "$actual_hash" = "$expected_hash" ] || fail "SHA256 verification failed for $archive_name."

mkdir "$tmp_dir/extract"
tar -xzf "$archive_path" -C "$tmp_dir/extract"
[ -f "$tmp_dir/extract/akey" ] || fail "The archive does not contain akey."

mkdir -p "$install_dir"
stage_path=$(mktemp "$install_dir/.akey.XXXXXX") || fail "Could not create a staging file."
cp "$tmp_dir/extract/akey" "$stage_path"
chmod 755 "$stage_path"
mv -f "$stage_path" "$install_dir/akey"
stage_path=

case ":${PATH:-}:" in
    *":$install_dir:"*) ;;
    *) printf 'Note: %s is not in PATH; add it manually to use akey directly.\n' "$install_dir" ;;
esac
printf 'Installed akey to %s\n' "$install_dir/akey"
