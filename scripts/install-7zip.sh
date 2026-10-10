#!/bin/sh
# Installs the OFFICIAL 7-Zip build as /usr/local/bin/7zz.
#
# Distribution packages (Alpine, Debian/Ubuntu main) ship 7zz without the RAR
# codec for licensing reasons, and most game archives are RAR5 (phase 0.5
# finding, docs/spike-results.md). Used by the dev image, CI and production.
set -eu

VERSION=2601
case "$(uname -m)" in
  x86_64 | amd64)
    ARCH=x64
    SHA256=8ea0fc8a135e7b848e80a4116fe22dff56c8c4518dde1f43cce67f4e340b437a
    ;;
  aarch64 | arm64)
    ARCH=arm64
    SHA256=39f8c9070c300a63c7484d9a983119ef3edf841e1ddf69f1affae29fdec5f612
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

URL="https://www.7-zip.org/a/7z${VERSION}-linux-${ARCH}.tar.xz"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$TMP/7z.tar.xz" "$URL"
else
  wget -qO "$TMP/7z.tar.xz" "$URL"
fi
echo "$SHA256  $TMP/7z.tar.xz" | sha256sum -c -

# 7zzs is the statically linked build: works on glibc and musl alike.
xz -dc "$TMP/7z.tar.xz" | tar -x -C "$TMP" 7zzs License.txt
install -m 0755 "$TMP/7zzs" /usr/local/bin/7zz
mkdir -p /usr/share/licenses/7zip
cp "$TMP/License.txt" /usr/share/licenses/7zip/License.txt
7zz | head -n 2
