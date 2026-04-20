#!/usr/bin/env bash
# 一键打包 macOS DMG
#
# 依赖: Homebrew 安装的 create-dmg
#   brew install create-dmg
#
# 用法: 在项目根目录执行
#   bash build/darwin/build-dmg.sh [arm64|amd64|universal]

set -euo pipefail

ARCH="${1:-arm64}"
VERSION="1.0.0"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN_DIR="$ROOT/build/bin"
APP_NAME="windsurf-tools-wails.app"
APP_PATH="$BIN_DIR/$APP_NAME"

cd "$ROOT"

# Setup PATH for Homebrew + Go
if [[ -x /opt/homebrew/bin/brew ]]; then
  eval "$(/opt/homebrew/bin/brew shellenv)"
fi
export PATH="$HOME/go/bin:$PATH"

echo "==> 编译 $ARCH"
case "$ARCH" in
  arm64)     PLATFORM="darwin/arm64" ;;
  amd64)     PLATFORM="darwin/amd64" ;;
  universal) PLATFORM="darwin/universal" ;;
  *) echo "unknown arch: $ARCH"; exit 1 ;;
esac

wails build -platform "$PLATFORM" -clean

echo "==> 生成 DMG"
cd "$BIN_DIR"
DMG_NAME="WindsurfTools-${VERSION}-${ARCH}.dmg"
rm -f "$DMG_NAME"

create-dmg \
  --volname "Windsurf Tools" \
  --volicon "$APP_PATH/Contents/Resources/iconfile.icns" \
  --window-pos 200 120 \
  --window-size 640 400 \
  --icon-size 110 \
  --icon "$APP_NAME" 160 200 \
  --hide-extension "$APP_NAME" \
  --app-drop-link 480 200 \
  --no-internet-enable \
  "$DMG_NAME" \
  "$APP_NAME"

echo ""
echo "==> 完成: $BIN_DIR/$DMG_NAME"
ls -lh "$BIN_DIR/$DMG_NAME"
