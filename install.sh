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

tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' || true)"

if [[ -n "$tag" ]]; then
  url="https://github.com/$REPO/releases/download/$tag/$asset"
  tmp="$BIN.tmp"
  curl -fsSL "$url" -o "$tmp"
  chmod +x "$tmp"
  mv -f "$tmp" "$BIN"
  echo "installed $BIN ($tag, $asset)"
else
  echo "no release found; building from source with go install"
  if ! command -v go >/dev/null 2>&1; then
    echo "go is required to build from source: https://go.dev/dl/" >&2
    exit 1
  fi
  GOBIN="$DEST" go install -ldflags "-X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" "github.com/$REPO/cmd/lserve@latest"
  echo "installed $BIN (built from source)"
fi

# Prove which build landed: version includes the build timestamp.
"$BIN" version || true

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "note: $DEST is not on your PATH" ;;
esac
