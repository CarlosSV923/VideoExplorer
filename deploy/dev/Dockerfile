# Development/test image: the Go toolchain plus the official 7-Zip (with RAR)
# and ffprobe. Built by `task dev:image`; used by compose.dev.yaml and the
# Taskfile's Go tasks.
FROM golang:1.27.2

RUN apt-get update \
 && apt-get install -y --no-install-recommends xz-utils \
 && rm -rf /var/lib/apt/lists/*

COPY scripts/install-7zip.sh /tmp/install-7zip.sh
RUN sh /tmp/install-7zip.sh && rm /tmp/install-7zip.sh

COPY scripts/install-ffprobe.sh /tmp/install-ffprobe.sh
RUN sh /tmp/install-ffprobe.sh && rm /tmp/install-ffprobe.sh

# The image only builds if both binaries start here.
RUN 7zz i > /dev/null && ffprobe -version > /dev/null

# Tests that need 7zz or ffprobe fail instead of being skipped inside this image.
ENV REQUIRE_7ZZ=1
ENV REQUIRE_FFPROBE=1
