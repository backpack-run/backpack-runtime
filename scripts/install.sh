#!/bin/sh
set -eu
umask 077

usage() {
  echo "usage: install.sh [vVERSION] [INSTALL_DIRECTORY]" >&2
  echo "       BACKPACK_VERSION=vVERSION BACKPACK_CHANNEL=latest|stable sh install.sh" >&2
  exit 2
}

[ "$#" -le 2 ] || usage
version=${1:-${BACKPACK_VERSION:-}}
channel=${BACKPACK_CHANNEL:-latest}
case "$channel" in latest|stable) ;; *) echo "BACKPACK_CHANNEL must be latest or stable" >&2; exit 2 ;; esac

valid_version() {
  printf '%s\n' "$1" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
}
[ -z "$version" ] || valid_version "$version" || usage

system=$(uname -s)
machine=$(uname -m)
case "$system/$machine" in
  Linux/x86_64|Linux/amd64) os=linux; arch=amd64 ;;
  Darwin/arm64|Darwin/aarch64) os=darwin; arch=arm64 ;;
  *) echo "unsupported platform: $system/$machine" >&2; exit 1 ;;
esac

if [ "$#" -ge 2 ]; then
  install_directory=$2
else
  [ -n "${HOME:-}" ] || { echo "HOME is not set; pass an installation directory" >&2; exit 1; }
  install_directory=$HOME/.local/bin
fi
[ -n "$install_directory" ] || { echo "installation directory must not be empty" >&2; exit 2; }

repository=https://github.com/backpack-run/backpack-runtime
api_repository=https://api.github.com/repos/backpack-run/backpack-runtime
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/backpack-install.XXXXXXXX")
staged_binary=
cleanup() {
  [ -z "$staged_binary" ] || rm -f "$staged_binary"
  rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

for command_name in curl tar awk grep sed install; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 1; }
done
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "required command not found: sha256sum or shasum" >&2
  exit 1
fi

download() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 "$@"
}

metadata=$temporary_directory/release.json
if [ -n "$version" ]; then
  download --header 'Accept: application/vnd.github+json' --output "$metadata" "$api_repository/releases/tags/$version"
else
  if [ "$channel" = stable ]; then
    metadata_url=$api_repository/releases/latest
  else
    metadata_url=$api_repository/releases?per_page=1
  fi
  download --header 'Accept: application/vnd.github+json' --output "$metadata" "$metadata_url"
fi

metadata_value() {
  sed -n "s/^[[:space:]]*\"$1\":[[:space:]]*\(\"[^\"]*\"\|true\|false\).*/\1/p" "$metadata" | awk 'NR == 1 { gsub(/^"|"$/, ""); print; exit }'
}
published_version=$(metadata_value tag_name)
draft=$(metadata_value draft)
prerelease=$(metadata_value prerelease)
[ -n "$published_version" ] && valid_version "$published_version" || { echo "GitHub returned invalid release metadata" >&2; exit 1; }
[ "$draft" = false ] || { echo "refusing to install a draft release" >&2; exit 1; }
if [ -n "$version" ] && [ "$published_version" != "$version" ]; then
  echo "GitHub release metadata did not match requested version $version" >&2
  exit 1
fi
version=$published_version
if [ "$channel" = stable ] && [ "$prerelease" != false ]; then
  echo "stable channel resolved to a prerelease; refusing installation" >&2
  exit 1
fi
if [ "$prerelease" = true ]; then
  echo "Installing prerelease $version (interfaces and behavior may change)." >&2
fi

release_version=${version#v}
archive="backpack_${release_version}_${os}_${arch}.tar.gz"
download --output "$temporary_directory/$archive" "$repository/releases/download/$version/$archive"
download --output "$temporary_directory/checksums.txt" "$repository/releases/download/$version/checksums.txt"
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name { print tolower($1); exit }' "$temporary_directory/checksums.txt")
[ -n "$expected" ] || { echo "release checksum for $archive was not published" >&2; exit 1; }
case "$expected" in *[!0-9a-f]*|'') echo "release checksum for $archive is malformed" >&2; exit 1 ;; esac
[ "${#expected}" -eq 64 ] || { echo "release checksum for $archive is malformed" >&2; exit 1; }
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
  "backpack $release_version"|"backpack $release_version "*) ;;
  *) echo "downloaded binary did not report expected version $release_version" >&2; exit 1 ;;
esac
mkdir -p "$install_directory"
staged_binary="$install_directory/.backpack-install-$$"
install -m 0755 "$temporary_directory/backpack" "$staged_binary"
mv -f "$staged_binary" "$install_directory/backpack"
staged_binary=
echo "Installed verified Backpack Runtime $version to $install_directory"
