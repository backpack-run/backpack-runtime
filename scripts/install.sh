#!/bin/sh
set -eu

version=${1:-}
if ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  echo "usage: install.sh vVERSION [INSTALL_DIRECTORY]" >&2
  exit 2
fi

system=$(uname -s)
machine=$(uname -m)
case "$system/$machine" in
  Linux/x86_64|Linux/amd64) os=linux; arch=amd64 ;;
  Darwin/arm64|Darwin/aarch64) os=darwin; arch=arm64 ;;
  *) echo "unsupported platform: $system/$machine" >&2; exit 1 ;;
esac

install_directory=${2:-"${HOME}/.local/bin"}
release_version=${version#v}
archive="backpack_${release_version}_${os}_${arch}.tar.gz"
repository=https://github.com/backpack-run/backpack-runtime
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/backpack-install.XXXXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

curl --fail --location --proto '=https' --tlsv1.2 --output "$temporary_directory/$archive" "$repository/releases/download/$version/$archive"
curl --fail --location --proto '=https' --tlsv1.2 --output "$temporary_directory/checksums.txt" "$repository/releases/download/$version/checksums.txt"
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print tolower($1); exit }' "$temporary_directory/checksums.txt")
[ -n "$expected" ] || { echo "release checksum for $archive was not published" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary_directory/$archive" | awk '{print tolower($1)}')
else
  actual=$(shasum -a 256 "$temporary_directory/$archive" | awk '{print tolower($1)}')
fi
[ "$actual" = "$expected" ] || { echo "SHA-256 mismatch for $archive" >&2; exit 1; }
tar -xzf "$temporary_directory/$archive" -C "$temporary_directory"
mkdir -p "$install_directory"
install -m 0755 "$temporary_directory/backpack" "$install_directory/backpack"
echo "Installed verified Backpack Runtime $version to $install_directory"
