# Install VideoExplorer on TrueNAS SCALE

1. **Create datasets.** Mount the existing media dataset at `/library`; it must contain `Movies/`, `TV Shows/`, and `_unassigned/`. VideoExplorer also creates `.videoexplorer/` there for staging, trash and operation scratch, so these folders stay on the same dataset for atomic moves (RNF-04). Create a separate small app-state dataset for `/data` (SQLite and image cache).
2. **Set permissions.** The compose example runs as `3000:3000` via `user:`. Replace it with the UID:GID you choose and grant that user an inherited **Modify** ACL on both datasets. Do not set `PUID` or `PGID` (RNF-03). If `/data` is not writable, the app stops before serving; a non-writable library is reported by `/api/health` (RF-63).
3. **Prepare credentials and paths.** Edit the host paths and the example UID:GID in [`compose.yaml`](compose.yaml). Generate an argon2id hash with `task hash-password`, then paste it into `APP_PASSWORD_HASH`, doubling each `$` as `$$` so Compose preserves the hash literally. Set a random `SESSION_SECRET` of at least 32 bytes. Keep either `APP_PASSWORD_HASH` or `APP_PASSWORD`, not both. Set `COOKIE_SECURE=true` only behind HTTPS.
4. **Install the app.** In TrueNAS SCALE, open **Apps → Discover Apps → Custom App → Install via YAML**. Use `videoexplorer` as the app name, paste the compose YAML, and install. The image is pinned to `0.1.0`; keep Jellyfin settings commented unless configuring Jellyfin.
5. **Check first start.** Open `http://<nas-ip>:8080/api/health`. It should return HTTP 200 with `status: "ok"`. If the library ACL is wrong, health reports a degraded state; if `/data` is not writable, check the app log for the UID/GID and missing ACL.

## Jellyfin paths

Jellyfin should see the same media dataset that VideoExplorer mounts at `/library`. If both containers expose it at different paths, VideoExplorer detects the Jellyfin paths for `Movies/` and `TV Shows/` automatically. If detection cannot find them, set `JELLYFIN_LIBRARY_PATH` to the path Jellyfin uses for the root mounted at `/library`. Configure `JELLYFIN_URL` and `JELLYFIN_API_KEY` together; all three values are optional. Jellyfin scans the shared files normally, and VideoExplorer requests a refresh after changes.

## Update

Change the image tag in `compose.yaml` to the desired published version, save/update the Custom App, and let TrueNAS pull it. Releases are published from `v*` tags; the compose pins `0.1.0` so upgrades are explicit. Keep the datasets and credentials unchanged.
