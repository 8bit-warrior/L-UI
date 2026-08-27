
def make_outbound(proto: str, st: dict[str, Any]) -> dict[str, Any]:
    tag = unique_tag(st, prompt("Tag", proto), "outbound")
    ob: dict[str, Any] = {"tag": tag, "protocol": proto, "settings": {}}
    if proto == "freedom": ob["settings"] = {"domainStrategy": prompt("DomainStrategy", "AsIs")}
    elif proto == "blackhole":
        typ = prompt("响应类型(空/none/http)", ""); ob["settings"] = {"response": {"type": typ}} if typ else {}
    elif proto == "dns": ob["settings"] = edit_json_value("DNS settings", {})
    elif proto in {"vmess", "vless", "trojan", "shadowsocks", "socks", "http", "hysteria"}:
        ob["settings"] = outbound_server_settings(proto)
        if proto == "hysteria": ob["streamSettings"] = {"network": "hysteria", "security": "tls", "hysteriaSettings": {"version": 2}}
        elif proto in {"vmess", "vless", "trojan", "shadowsocks"} and yesno("配置 streamSettings", False): ob["streamSettings"] = edit_json_value("streamSettings", {"network": "tcp", "security": "none"})
        elif proto in {"socks", "http"} and yesno("配置 Sockopt", False): ob["streamSettings"] = {"sockopt": edit_json_value("sockopt", {})}
    elif proto == "wireguard": ob["settings"] = edit_json_value("WireGuard settings", {"address": ["172.16.0.2/32"], "peers": []})
    elif proto == "loopback": ob["settings"] = {"inboundTag": prompt("目标 Inbound Tag")}
    if yesno("使用高级 JSON 覆盖/补充整个出站", False):
        ob = edit_json_value("outbound", ob)
        if ob.get("protocol") not in OUTBOUND_PROTOCOLS: raise ValueError("高级 JSON 中 protocol 不受支持")
    return ob


def find_by_tag(items: list[dict[str, Any]], tag: str) -> tuple[int, dict[str, Any]]:
    for i, x in enumerate(items):
        if x.get("tag") == tag: return i, x
    raise KeyError(tag)


def list_inbounds(st: dict[str, Any]) -> None:
    if not st["inbounds"]: print("暂无入站"); return
    print("\nTag\t协议\t监听\t端口\t状态\t备注")
    for ib in st["inbounds"]:
        m = ib.get("_lui") or {}; print(f"{ib.get('tag')}\t{ib.get('protocol')}\t{ib.get('listen','')}\t{ib.get('port','')}\t{'启用' if m.get('enable',True) else '停用'}\t{m.get('remark','')}")


def list_outbounds(st: dict[str, Any]) -> None:
    default = get_default_outbound(st); print("\n#\tTag\t协议\t默认")
    for i, ob in enumerate(st["outbounds"], 1): print(f"{i}\t{ob.get('tag')}\t{ob.get('protocol')}\t{'*' if ob.get('tag') == default else ''}")


def get_default_outbound(st: dict[str, Any]) -> str:
    if st.get("outbounds") and st["outbounds"][0].get("tag"): return str(st["outbounds"][0]["tag"])
    return "direct"


def set_default_outbound(st: dict[str, Any], tag: str) -> None:
    idx, _ = find_by_tag(st["outbounds"], tag)
    if idx > 0: item = st["outbounds"].pop(idx); st["outbounds"].insert(0, item)


def strip_internal_rule(rule: dict[str, Any]) -> dict[str, Any]:
    return {k: copy.deepcopy(v) for k, v in rule.items() if not k.startswith("_lui") and k != "enabled"}


def add_routing_rule(st: dict[str, Any]) -> dict[str, Any]:
    r: dict[str, Any] = {"type": "field", "_lui_enabled": True}
    fields = [("domain","域名(逗号分隔)"),("ip","IP/CIDR(逗号分隔)"),("port","目标端口/范围"),("sourceIP","Source IP/CIDR(逗号分隔)"),("sourcePort","Source Port"),("network","Network tcp/udp/tcp,udp"),("protocol","Protocol http,tls,quic,bittorrent..."),("inboundTag","Inbound Tag(逗号分隔)"),("user","User/email(逗号分隔)")]
    for key, label in fields:
        v = prompt(label, "")
        if not v: continue
        r[key] = [x.strip() for x in v.split(",") if x.strip()] if key in {"domain","ip","sourceIP","protocol","inboundTag","user"} else v
    target = prompt("Outbound Tag(留空则使用 Balancer)", "")
    if target: r["outboundTag"] = target
    else: r["balancerTag"] = prompt("Balancer Tag")
    return r


def active_rules(st: dict[str, Any]) -> list[dict[str, Any]]:
    return [r for r in st.get("routing", {}).get("rules", []) if r.get("_lui_enabled", True) and r.get("enabled", True) is not False]


def build_config(st: dict[str, Any]) -> dict[str, Any]:
    inbounds = [x for ib in st["inbounds"] if (x := materialize_inbound(ib, st["clients"]))]
    routing = copy.deepcopy(st.get("routing") or {"domainStrategy": "AsIs", "rules": []}); routing["rules"] = [strip_internal_rule(r) for r in active_rules(st)]
    cfg = {"log": copy.deepcopy(st.get("log") or {}), "inbounds": inbounds, "outbounds": active_subscription_outbounds(st, prepend=True) + copy.deepcopy(st["outbounds"]) + active_subscription_outbounds(st, prepend=False), "routing": routing}
    for key in ("dns", "policy", "stats", "observatory", "burstObservatory", "reverse"):
        if key in st: cfg[key] = copy.deepcopy(st[key])
    return cfg


def free_port() -> int:
    with socket.socket() as s: s.bind(("127.0.0.1", 0)); return int(s.getsockname()[1])


def wait_port(port: int, timeout: float = 5.0) -> bool:
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        with socket.socket() as s:
            s.settimeout(0.2)
            if s.connect_ex(("127.0.0.1", port)) == 0: return True
        time.sleep(0.05)
    return False


def parse_outbound_from_access_log(text: str) -> str:
    matches = re.findall(r"\[[^\]\n]*?->\s*([^\]\s]+)\]", text); return matches[-1] if matches else "未知"
