#!/usr/bin/env sh
set -eu

REPO="ota-takeru/run-weaver"
TARGET="wsl"
REPO_URL=""
GH_REPO=""
BIN_DIR="${HOME}/.local/bin"
POLL_INTERVAL="1m"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      TARGET="$2"
      shift 2
      ;;
    --repo-url)
      REPO_URL="$2"
      shift 2
      ;;
    --repo)
      GH_REPO="$2"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="$2"
      shift 2
      ;;
    --poll-interval)
      POLL_INTERVAL="$2"
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 64
      ;;
  esac
done

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ASSET="run-weaver_linux_${GOARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
mkdir -p "$BIN_DIR"
curl -fsSL "$URL" -o "$TMP_DIR/$ASSET"
tar -xzf "$TMP_DIR/$ASSET" -C "$TMP_DIR"
install -m 0755 "$TMP_DIR/run-weaver" "$BIN_DIR/run-weaver"

INSTALL_ARGS="--target $TARGET --poll-interval $POLL_INTERVAL"
if [ -n "$REPO_URL" ]; then
  INSTALL_ARGS="$INSTALL_ARGS --repo-url $REPO_URL"
fi
if [ -n "$GH_REPO" ]; then
  INSTALL_ARGS="$INSTALL_ARGS --repo $GH_REPO"
fi

"$BIN_DIR/run-weaver" install $INSTALL_ARGS
echo "run-weaver installed at $BIN_DIR/run-weaver"
