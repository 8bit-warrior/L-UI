#!/bin/sh
set -eu
REPO="8bit-warrior/L-UI"
BASE="https://raw.githubusercontent.com/${REPO}/main"

if [ "$(id -u)" -ne 0 ]; then
  echo "请使用 root 运行安装脚本" >&2
  exit 1
fi

need_cmd() { command -v "$1" >/dev/null 2>&1; }
install_deps() {
  if need_cmd apt-get; then apt-get update; DEBIAN_FRONTEND=noninteractive apt-get install -y python3 curl unzip ca-certificates
  elif need_cmd dnf; then dnf install -y python3 curl unzip ca-certificates
  elif need_cmd yum; then yum install -y python3 curl unzip ca-certificates
  elif need_cmd apk; then apk add --no-cache python3 curl unzip ca-certificates
  elif need_cmd pacman; then pacman -Sy --noconfirm python curl unzip ca-certificates
  elif need_cmd zypper; then zypper --non-interactive install python3 curl unzip ca-certificates
  else echo "不支持的包管理器，请手动安装 python3 curl unzip ca-certificates" >&2; exit 1
  fi
}
install_deps
python3 - <<'PYVER'
import sys
if sys.version_info < (3, 9):
    raise SystemExit("L-UI requires Python 3.9 or newer")
PYVER
mkdir -p /usr/local/lib/l-ui/src /etc/l-ui /var/log/l-ui
curl -fL "${BASE}/lui.py" -o /usr/local/lib/l-ui/lui.py
for part in part_001.py part_002.py part_003.py part_004.py part_005.py part_006.py part_007.py part_008.py part_009.py part_010.py part_011.py part_012.py part_013.py part_014.py part_015.py part_016.py part_017.py; do
  curl -fL "${BASE}/src/${part}" -o "/usr/local/lib/l-ui/src/${part}"
done
chmod 0755 /usr/local/lib/l-ui/lui.py
cat >/usr/local/bin/l-ui <<'EOF'
#!/bin/sh
exec python3 /usr/local/lib/l-ui/lui.py "$@"
EOF
chmod 0755 /usr/local/bin/l-ui
echo "L-UI 已安装。运行: l-ui"
if [ -t 0 ]; then exec /usr/local/bin/l-ui; fi
