#!/bin/sh
set -eu

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

if [ "$#" -gt 1 ]; then
    fail "Usage: install.sh [version]"
fi
version=${1:-latest}
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

[ -n "${HOME:-}" ] || fail "HOME is not set."
install_dir="$HOME/.local/bin"
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
