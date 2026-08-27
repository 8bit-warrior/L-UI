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
    # Forward-compatible defaults for states created by early L-UI builds.
    st.setdefault("outbound_subscriptions", [])
    st.setdefault("meta", {}).setdefault("next_subscription_id", 1)
    return st


def save_state(st: dict[str, Any], apply: bool = True) -> None:
    """Validate a prospective state before replacing the live files.

    If the Xray core is installed, the generated config must pass `xray -test`
    before state/config are committed. A failed service restart restores the
    previous state and config. The caller's in-memory state is also restored on
    a validation/restart failure, preventing a later menu action from committing
    an already-rejected mutation.
    """
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

def validate_state(st: dict[str, Any]) -> None:
    if not isinstance(st.get("inbounds"), list) or not isinstance(st.get("outbounds"), list):
        raise ValueError("inbounds/outbounds 必须是数组")
    tags: set[str] = set()
    ports: dict[tuple[str, int], str] = {}
    for ib in st["inbounds"]:
        if not isinstance(ib, dict):
            raise ValueError("入站必须是 JSON 对象")
        tag = str(ib.get("tag", "")).strip()
        if not tag:
            raise ValueError("入站 tag 不能为空")
        if tag in tags:
            raise ValueError(f"重复入站 tag: {tag}")
        tags.add(tag)
        p = int(ib.get("port", 0) or 0)
        listen = str(ib.get("listen", "0.0.0.0"))
        if p < 0 or p > 65535:
            raise ValueError(f"非法入站端口: {p}")
        key = (listen, p)
        if p and key in ports:
            raise ValueError(f"监听冲突: {listen}:{p} ({ports[key]} / {tag})")
        if p:
            ports[key] = tag
    ob_tags: set[str] = set()
    all_obs = list(st["outbounds"]) + active_subscription_outbounds(st)
    for ob in all_obs:
        if not isinstance(ob, dict):
            raise ValueError("出站必须是 JSON 对象")
        tag = str(ob.get("tag", "")).strip()
        proto = str(ob.get("protocol", ""))
        if not tag:
            raise ValueError("出站 tag 不能为空")
        if tag in ob_tags:
            raise ValueError(f"重复出站 tag: {tag}")
        if proto not in OUTBOUND_PROTOCOLS:
            raise ValueError(f"不支持的出站协议: {proto}")
        ob_tags.add(tag)


def client_wire(c: dict[str, Any], proto: str, inbound_id: Any = None) -> Optional[dict[str, Any]]:
    if not c.get("enable", True):
        return None
    email = c.get("email", "")
    if proto == "vless":
        d = {"id": c.get("uuid") or c.get("id_value") or "", "email": email}
        if c.get("flow"):
            d["flow"] = c["flow"]
        return d
    if proto == "vmess":
        return {"id": c.get("uuid") or c.get("id_value") or "", "email": email}
    if proto in {"trojan", "shadowsocks"}:
        d = {"password": c.get("password") or "", "email": email}
        if proto == "shadowsocks" and c.get("method"):
            d["method"] = c["method"]
        return d
    if proto == "hysteria":
        return {"auth": c.get("auth") or c.get("password") or "", "email": email}
    if proto in {"wireguard", "amneziawg"}:
        allowed = c.get("allowed_ips") or []
        per = c.get("allowed_ips_by_inbound") or {}
        if inbound_id is not None and str(inbound_id) in per:
            allowed = per[str(inbound_id)]
        d = {"email": email, "level": 0}
        for src, dst in (("public_key", "publicKey"), ("pre_shared_key", "preSharedKey"), ("keep_alive", "keepAlive")):
            if c.get(src):
                d[dst] = c[src]
        if allowed:
            d["allowedIPs"] = allowed
        return d
    return None


def materialize_inbound(ib: dict[str, Any], clients: list[dict[str, Any]]) -> dict[str, Any]:
    out = copy.deepcopy(ib)
    meta = out.pop("_lui", {})
    enabled = meta.get("enable", True)
    if not enabled:
        return {}
    proto = out.get("protocol")
    settings = copy.deepcopy(out.get("settings") or {})
    linked = [c for c in clients if out.get("tag") in (c.get("inbound_tags") or [])]
    wires = [w for c in linked if (w := client_wire(c, str(proto), meta.get("source_id"))) is not None]
    if proto in {"vless", "vmess", "trojan", "shadowsocks", "hysteria"}:
        settings["clients"] = wires
    elif proto in {"wireguard", "amneziawg"}:
        settings.pop("clients", None)
        settings["peers"] = wires
    elif proto in {"http", "mixed"}:
        settings["accounts"] = [
            {"user": c.get("email", ""), "pass": c.get("password", "")}
            for c in linked if c.get("enable", True)
        ]
    out["settings"] = settings
    return out


def write_config(st: dict[str, Any]) -> None:
    atomic_json_write(CONFIG_FILE, build_config(st))


def run(cmd: list[str], *, timeout: Optional[float] = None, capture: bool = True, check: bool = False) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        text=True,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.STDOUT if capture else None,
        timeout=timeout,
        check=check,
    )


def validate_xray_config(path: Path) -> tuple[bool, str]:
    if not XRAY_BIN.exists():
        try:
            json.loads(path.read_text(encoding="utf-8"))
            return True, "仅完成 JSON 结构校验（Xray 尚未安装）"
        except Exception as e:
            return False, str(e)
    tries = [
        [str(XRAY_BIN), "run", "-test", "-config", str(path)],
        [str(XRAY_BIN), "-test", "-config", str(path)],
    ]
    last = ""
    for cmd in tries:
        try:
            cp = run(cmd, timeout=20)
            last = cp.stdout or ""
            if cp.returncode == 0:
                return True, last.strip() or "OK"
        except Exception as e:
            last = str(e)
    return False, last.strip()


def github_json(path: str, timeout: int = 15) -> Any:
    req = urllib.request.Request(
        f"{GITHUB_API}{path}",
        headers={"Accept": "application/vnd.github+json", "User-Agent": "L-UI/1"},
    )
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.load(r)


def normalize_version(v: str) -> str:
    v = v.strip()
    return v if v.startswith("v") else "v" + v


def list_releases() -> list[dict[str, Any]]:
    data = github_json("/releases?per_page=30")
    return [r for r in data if not r.get("draft")]


def latest_xray_version() -> str:
    rels = list_releases()
    if not rels:
        raise RuntimeError("GitHub 未返回 Xray release")
    # Include prereleases by design. Pick by release publication/creation time
    # rather than relying on API list order.
    newest = max(rels, key=lambda r: str(r.get("published_at") or r.get("created_at") or ""))
    return str(newest["tag_name"])


def release_for_version(version: str) -> Optional[dict[str, Any]]:
    version = normalize_version(version)
    try:
        return github_json(f"/releases/tags/{urllib.parse.quote(version)}")
    except urllib.error.HTTPError as e:
        if e.code == 404:
            return None
        raise


def platform_asset_candidates() -> list[str]:
    arch = platform.machine().lower()
    amap = {
        "x86_64": ["64"],
        "amd64": ["64"],
        "aarch64": ["arm64-v8a", "arm64"],
        "arm64": ["arm64-v8a", "arm64"],
        "armv7l": ["arm32-v7a", "arm32-v7"],
        "i386": ["32"],
        "i686": ["32"],
    }
    suffixes = amap.get(arch, [arch])
    return [f"Xray-linux-{s}.zip" for s in suffixes]


def select_release_asset(release: dict[str, Any]) -> Optional[dict[str, Any]]:
    by_name = {a.get("name"): a for a in release.get("assets", [])}
    for name in platform_asset_candidates():
        if name in by_name:
            return by_name[name]
    return None


def download(url: str, dest: Path) -> None:
    req = urllib.request.Request(url, headers={"User-Agent": "L-UI/1"})
    with urllib.request.urlopen(req, timeout=60) as r, dest.open("wb") as f:
        shutil.copyfileobj(r, f)

