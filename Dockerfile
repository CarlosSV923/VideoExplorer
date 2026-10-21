# Production image: React SPA embedded in the Go server, with pinned 7zz and
# jellyfin-ffmpeg ffprobe. Published to GHCR from v* tags.

# --- 1. Web app ---
FROM node:24-alpine AS web
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile --filter @videoexplorer/web
COPY apps/web apps/web
COPY contracts contracts
ENV VITE_DATA_SOURCE=http
RUN pnpm --filter @videoexplorer/web build

# --- 2. Go binary with the SPA embedded ---
FROM golang:1.27.3 AS api
WORKDIR /src/apps/api
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api .
COPY --from=web /src/apps/web/dist internal/platform/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/videoexplorer ./cmd/videoexplorer

# --- 3. Runtime ---
FROM debian:13-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
 && rm -rf /var/lib/apt/lists/*
COPY scripts/install-7zip.sh /tmp/install-7zip.sh
RUN sh /tmp/install-7zip.sh && rm /tmp/install-7zip.sh
COPY scripts/install-ffprobe.sh /tmp/install-ffprobe.sh
RUN sh /tmp/install-ffprobe.sh && rm /tmp/install-ffprobe.sh \
 && install -D -m 0644 /usr/share/common-licenses/GPL-3 /usr/share/doc/videoexplorer/ffprobe/LICENSE \
 && apt-get purge -y xz-utils && apt-get autoremove -y \
 && rm -rf /var/lib/apt/lists/*
COPY licenses/ffprobe-NOTICE.txt /usr/share/doc/videoexplorer/ffprobe/NOTICE
COPY --from=api /out/videoexplorer /usr/local/bin/videoexplorer
ENV PORT=8080 LIBRARY_PATH=/library DATA_PATH=/data
EXPOSE 8080
VOLUME ["/library", "/data"]
HEALTHCHECK --interval=30s --timeout=5s CMD curl -fsS http://localhost:8080/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/videoexplorer"]
