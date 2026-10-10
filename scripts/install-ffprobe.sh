#!/bin/sh
# Installs ffprobe as /usr/local/bin/ffprobe, from a pinned release of the
# portable jellyfin-ffmpeg builds (https://github.com/jellyfin/jellyfin-ffmpeg),
# the FFmpeg build Jellyfin ships. Used by the dev image, CI and production.
#
# Linkage: every codec library is linked statically into the binary; only the
# C library (glibc) and libm/libdl are dynamic, and every Debian/Ubuntu base
# image has them. No distribution package of ffprobe is used.
#
# Licence: the build is GPL-3.0 (--enable-gpl --enable-version3); its source
# code is in the jellyfin/jellyfin-ffmpeg repository.
#
# Updating:
#   1. Pick a tag from https://github.com/jellyfin/jellyfin-ffmpeg/releases
#      (not a draft or pre-release) and set VERSION to it, without the leading v.
#   2. Copy the `digest` (the value after `sha256:`) of the linux64-gpl and
#      linuxarm64-gpl assets from the GitHub API:
#      https://api.github.com/repos/jellyfin/jellyfin-ffmpeg/releases/tags/v<VERSION>
#      into SHA256 below.
#   3. Run the script again: it fails if the downloaded file does not match.
set -eu

VERSION=8.1.3-1
case "$(uname -m)" in
  x86_64 | amd64)
    ARCH=linux64
    SHA256=b86dc023c64a5a9a410e6e3a938a970460214fae78f31c1553c9f88f1c289b6a
    ;;
  aarch64 | arm64)
    ARCH=linuxarm64
    SHA256=7bc8e8d0986f7f4693f63e9728570f671f07b6e21c05d5465dd7ce461d1631dc
    ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

ASSET="jellyfin-ffmpeg_${VERSION}_portable_${ARCH}-gpl.tar.xz"
URL="https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v${VERSION}/${ASSET}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$TMP/ffmpeg.tar.xz" "$URL"
else
  wget -qO "$TMP/ffmpeg.tar.xz" "$URL"
fi
echo "$SHA256  $TMP/ffmpeg.tar.xz" | sha256sum -c -

# The archive holds ffmpeg and ffprobe at its top level; only ffprobe is kept.
tar -xJf "$TMP/ffmpeg.tar.xz" -C "$TMP" ffprobe
install -m 0755 "$TMP/ffprobe" /usr/local/bin/ffprobe

# Smoke test: the binary starts and reports the pinned build.
ffprobe -version | head -n 1
