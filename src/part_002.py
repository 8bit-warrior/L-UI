
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
