#!/usr/bin/env python3
"""L-UI - lightweight, panel-less Xray terminal manager.

The design intentionally keeps Xray as the only proxy core. 3x-ui compatibility
is implemented as data import, not as a runtime dependency.
"""
from __future__ import annotations

import base64
import contextlib
import copy
import datetime as dt
import hashlib
import ipaddress
import json
import os
import platform
import re
import shutil
import socket
import ssl
import sqlite3
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from pathlib import Path
from typing import Any, Iterable, Optional, Union

APP_NAME = "L-UI"
SCHEMA_VERSION = 1
COMPAT_XRAY = "v26.6.27"
XRAY_REPO = "XTLS/Xray-core"
GITHUB_API = f"https://api.github.com/repos/{XRAY_REPO}"
OUTBOUND_PROTOCOLS = [
    "freedom",
    "blackhole",
    "dns",
    "vmess",
    "vless",
    "trojan",
    "shadowsocks",
    "wireguard",
    "hysteria",
    "socks",
    "http",
    "loopback",
]
INBOUND_PROTOCOLS = [
    "vless",
    "vmess",
    "trojan",
    "shadowsocks",
    "wireguard",
    "hysteria",
    "http",
    "mixed",
    "tunnel",
    "amneziawg",
]
NETWORKS = ["tcp", "kcp", "ws", "grpc", "httpupgrade", "xhttp", "hysteria"]
SECURITIES = ["none", "tls", "reality"]
DEFAULT_TEST_URL = "https://www.google.com/generate_204"


def _default_home() -> Path:
    env = os.environ.get("LUI_HOME")
    if env:
        return Path(env).expanduser().resolve()
    return Path("/etc/l-ui") if os.geteuid() == 0 else Path.home() / ".config" / "l-ui"


HOME = _default_home()
STATE_FILE = HOME / "state.json"
CONFIG_FILE = HOME / "config.json"
BACKUP_DIR = HOME / "backups"
LOG_DIR = Path(os.environ.get("LUI_LOG_DIR", "/var/log/l-ui" if os.geteuid() == 0 else str(HOME / "logs")))
ACCESS_LOG = LOG_DIR / "access.log"
ERROR_LOG = LOG_DIR / "error.log"
BIN_DIR = Path(os.environ.get("LUI_BIN_DIR", "/usr/local/lib/l-ui" if os.geteuid() == 0 else str(HOME / "bin")))
XRAY_BIN = Path(os.environ.get("LUI_XRAY_BIN", str(BIN_DIR / "xray")))
SERVICE_NAME = "l-ui-xray.service"
SYSTEMD_UNIT = Path("/etc/systemd/system") / SERVICE_NAME


def now_iso() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def default_state() -> dict[str, Any]:
    return {
        "schema": SCHEMA_VERSION,
        "created_at": now_iso(),
        "xray": {"version": "", "installed_at": ""},
        "log": {"loglevel": "warning", "access": str(ACCESS_LOG), "error": str(ERROR_LOG)},
        "inbounds": [],
        "outbounds": [
            {"tag": "direct", "protocol": "freedom", "settings": {"domainStrategy": "AsIs"}},
            {"tag": "blocked", "protocol": "blackhole", "settings": {}},
        ],
        "routing": {"domainStrategy": "AsIs", "rules": [], "balancers": []},
        "clients": [],
        "client_groups": [],
        "external_links": [],
        "outbound_subscriptions": [],
        "meta": {"next_client_id": 1, "next_subscription_id": 1, "last_import_report": None},
    }


def ensure_dirs() -> None:
    for p in (HOME, BACKUP_DIR, LOG_DIR, BIN_DIR):
        p.mkdir(parents=True, exist_ok=True)


def atomic_json_write(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=path.name + ".", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
            f.write("\n")
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp, path)
    finally:
        with contextlib.suppress(FileNotFoundError):
            os.unlink(tmp)


def load_state() -> dict[str, Any]:
    ensure_dirs()
    if not STATE_FILE.exists():
        st = default_state()
        atomic_json_write(STATE_FILE, st)
        write_config(st)
        return st
    with STATE_FILE.open("r", encoding="utf-8") as f:
        st = json.load(f)
    if not isinstance(st, dict) or st.get("schema") != SCHEMA_VERSION:
        raise RuntimeError(f"不支持的 L-UI 数据格式: schema={st.get('schema') if isinstance(st, dict) else '?'}")
    st.setdefault("outbound_subscriptions", [])
    st.setdefault("meta", {}).setdefault("next_subscription_id", 1)
    return st


def save_state(st: dict[str, Any], apply: bool = True) -> None:
    """Validate a prospective state before replacing the live files."""
    validate_state(st)
    ensure_dirs()
    old_state = None
    old_config = None
    if STATE_FILE.exists():
        try:
            old_state = json.loads(STATE_FILE.read_text(encoding="utf-8"))
        except Exception:
            old_state = None
    if CONFIG_FILE.exists():
        old_config = CONFIG_FILE.read_text(encoding="utf-8", errors="replace")
    cfg = build_config(st)
    if XRAY_BIN.exists():
        with tempfile.TemporaryDirectory(prefix="lui-validate-") as td:
            candidate = Path(td) / "config.json"
            atomic_json_write(candidate, cfg)
            ok, msg = validate_specific_xray_config(candidate)
        if not ok:
            if isinstance(old_state, dict):
                st.clear(); st.update(old_state)
            raise RuntimeError("Xray 配置校验失败，未修改正式配置：\n" + msg)
    atomic_json_write(STATE_FILE, st)
    atomic_json_write(CONFIG_FILE, cfg)
    if apply and XRAY_BIN.exists():
        if not restart_service(best_effort=True):
            if isinstance(old_state, dict):
                atomic_json_write(STATE_FILE, old_state)
                st.clear(); st.update(old_state)
            if old_config is not None:
                CONFIG_FILE.write_text(old_config, encoding="utf-8")
            restart_service(best_effort=True)
            raise RuntimeError("Xray 重启失败，已恢复上一个配置")
