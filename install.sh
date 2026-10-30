#!/usr/bin/env bash
# Installs or updates lserve into ~/.local/bin (override with INSTALL_DIR).
set -euo pipefail

REPO="EstebanForge/lite-serve"
DEST="${INSTALL_DIR:-$HOME/.local/bin}"
BIN="$DEST/lserve"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os-$arch" in
  linux-x86_64)  asset="lserve-linux-amd64" ;;
  darwin-arm64)  asset="lserve-darwin-arm64" ;;
  *)
    echo "unsupported platform: $os-$arch (linux-amd64 and darwin-arm64 only)" >&2
    exit 1
    ;;
esac

mkdir -p "$DEST"

# latest/download resolves the release without an API request, so
# unauthenticated API rate limits cannot break installation.
url="https://github.com/$REPO/releases/latest/download/$asset"
tmp="$BIN.tmp"
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"
mv -f "$tmp" "$BIN"
echo "installed $BIN (latest, $asset)"

# Prove which build landed: version includes the build timestamp.
"$BIN" version || true

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "note: $DEST is not on your PATH" ;;
esac
