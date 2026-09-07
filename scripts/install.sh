#!/bin/sh
set -eu
umask 077

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
staged_binary=
cleanup() {
  [ -z "$staged_binary" ] || rm -f "$staged_binary"
  rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

for command_name in curl tar awk grep install; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 1; }
done

curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 --output "$temporary_directory/$archive" "$repository/releases/download/$version/$archive"
curl --fail --location --proto '=https' --proto-redir '=https' --tlsv1.2 --output "$temporary_directory/checksums.txt" "$repository/releases/download/$version/checksums.txt"
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print tolower($1); exit }' "$temporary_directory/checksums.txt")
[ -n "$expected" ] || { echo "release checksum for $archive was not published" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$temporary_directory/$archive" | awk '{print tolower($1)}')
else
  actual=$(shasum -a 256 "$temporary_directory/$archive" | awk '{print tolower($1)}')
fi
[ "$actual" = "$expected" ] || { echo "SHA-256 mismatch for $archive" >&2; exit 1; }

tar -tzf "$temporary_directory/$archive" | awk '
  BEGIN { allowed["LICENSE"]=1; allowed["README.md"]=1; allowed["THIRD_PARTY_NOTICES.md"]=1; allowed["backpack"]=1 }
  !($0 in allowed) || seen[$0]++ { exit 1 }
  END { if (NR != 4) exit 1 }
' || { echo "release archive contains unsafe, duplicate, or unexpected entries" >&2; exit 1; }
tar -tvzf "$temporary_directory/$archive" | awk 'substr($0,1,1) != "-" { exit 1 }' || { echo "release archive contains non-regular entries" >&2; exit 1; }
tar -xzf "$temporary_directory/$archive" -C "$temporary_directory" backpack
reported_version=$("$temporary_directory/backpack" version)
case "$reported_version" in
  *"$release_version"*) ;;
  *) echo "downloaded binary did not report expected version $release_version" >&2; exit 1 ;;
esac
mkdir -p "$install_directory"
staged_binary="$install_directory/.backpack-install-$$"
install -m 0755 "$temporary_directory/backpack" "$staged_binary"
mv -f "$staged_binary" "$install_directory/backpack"
staged_binary=
echo "Installed verified Backpack Runtime $version to $install_directory"
