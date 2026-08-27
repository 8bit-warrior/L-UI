#!/bin/sh
set -eu

REPO="8bit-warrior/L-UI"
VERSION="${LUI_VERSION:-latest}"

say() { printf '%s\n' "$*"; }
die() { printf '错误: %s\n' "$*" >&2; exit 1; }

case "$(uname -s 2>/dev/null || true)" in
  Linux) ;;
  *) die "当前仅支持 Linux" ;;
esac

case "$(uname -m 2>/dev/null || true)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l|armv7|armhf) ARCH="armv7" ;;
  i386|i486|i586|i686|x86) ARCH="386" ;;
  *) die "暂不支持的 CPU 架构: $(uname -m 2>/dev/null || echo unknown)" ;;
esac

if [ "$(id -u)" -eq 0 ]; then
  DEST="${LUI_INSTALL_PATH:-/usr/local/bin/l-ui}"
else
  DEST="${LUI_INSTALL_PATH:-$HOME/.local/bin/l-ui}"
  mkdir -p "$(dirname "$DEST")"
  say "非 root 安装：程序将安装到 $DEST；服务管理和 /etc/l-ui 模式需要 root。"
fi

ASSET="l-ui-linux-$ARCH"
if [ "$VERSION" = "latest" ]; then
  BASE="https://github.com/$REPO/releases/latest/download"
else
  case "$VERSION" in v*) : ;; *) VERSION="v$VERSION" ;; esac
  BASE="https://github.com/$REPO/releases/download/$VERSION"
fi
URL="$BASE/$ASSET"
SUMURL="$BASE/sha256sums.txt"
TMPDIR="${TMPDIR:-/tmp}/l-ui-install-$$"
mkdir -p "$TMPDIR"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM
BIN="$TMPDIR/$ASSET"
SUMS="$TMPDIR/sha256sums.txt"

download() {
  url="$1"; out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL --connect-timeout 15 --retry 2 -o "$out" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
  elif command -v busybox >/dev/null 2>&1 && busybox wget --help >/dev/null 2>&1; then
    busybox wget -O "$out" "$url"
  else
    die "缺少下载工具：需要 curl、wget 或带 wget applet 的 BusyBox 之一。L-UI 本体安装后不依赖这些工具。"
  fi
}

say "下载 $ASSET ..."
download "$URL" "$BIN"
chmod 0755 "$BIN"

if download "$SUMURL" "$SUMS" 2>/dev/null; then
  EXPECTED="$(awk -v f="$ASSET" '$2==f || $2=="*"f {print $1; exit}' "$SUMS" 2>/dev/null || true)"
  if [ -n "$EXPECTED" ]; then
    ACTUAL=""
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL="$(sha256sum "$BIN" | awk '{print $1}')"
    elif command -v busybox >/dev/null 2>&1; then
      ACTUAL="$(busybox sha256sum "$BIN" 2>/dev/null | awk '{print $1}')"
    fi
    if [ -n "$ACTUAL" ] && [ "$ACTUAL" != "$EXPECTED" ]; then
      die "SHA256 校验失败"
    fi
    [ -n "$ACTUAL" ] && say "SHA256 校验通过。"
  fi
fi

"$BIN" version >/dev/null 2>&1 || die "下载的 L-UI 二进制无法在当前系统运行"
mkdir -p "$(dirname "$DEST")"
OLD="$DEST.old"
if [ -f "$DEST" ]; then cp "$DEST" "$OLD" 2>/dev/null || true; fi
if ! cp "$BIN" "$DEST"; then
  [ -f "$OLD" ] && mv "$OLD" "$DEST"
  die "写入 $DEST 失败"
fi
chmod 0755 "$DEST"
rm -f "$OLD"

say "L-UI 已安装: $DEST"
"$DEST" version
say "运行: $DEST"
