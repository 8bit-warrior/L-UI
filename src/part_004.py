
def select(title: str, items: list[str], allow_zero: bool = True) -> int:
    while True:
        print(f"\n{'='*14} {title} {'='*14}")
        for i, x in enumerate(items, 1):
            print(f"{i:2d}. {x}")
        if allow_zero:
            print(" 0. 返回")
        raw = input("\n请输入选项: ").strip()
        try:
            n = int(raw)
            if allow_zero and n == 0:
                return 0
            if 1 <= n <= len(items):
                return n
        except ValueError:
            pass
        print("无效选项")


def edit_json_value(label: str, current: Any) -> Any:
    print(f"当前 {label}:\n{json.dumps(current, ensure_ascii=False, indent=2)}")
    print("输入单行 JSON；直接回车保持不变。")
    raw = input(f"新 {label}: ").strip()
    if not raw:
        return current
    return json.loads(raw)


def unique_tag(st: dict[str, Any], base: str, kind: str) -> str:
    arr = st["inbounds"] if kind == "inbound" else st["outbounds"]
    used = {str(x.get("tag", "")) for x in arr}
    if base not in used:
        return base
    i = 2
    while f"{base}-{i}" in used:
        i += 1
    return f"{base}-{i}"


def make_inbound(proto: str, st: dict[str, Any]) -> dict[str, Any]:
    tag = unique_tag(st, prompt("Tag", f"in-{proto}"), "inbound")
    listen = prompt("监听地址", "0.0.0.0")
    port = prompt_int("端口", 443, 1, 65535)
    settings: dict[str, Any] = {}
    stream: Optional[dict[str, Any]] = None
    sniffing = {"enabled": True, "destOverride": ["http", "tls", "quic", "fakedns"], "routeOnly": False}
    if proto in {"vless", "vmess", "trojan"}:
        settings["decryption" if proto == "vless" else "clients"] = "none" if proto == "vless" else []
        if proto != "vless": settings["clients"] = []
    elif proto == "shadowsocks": settings = {"method": prompt("Shadowsocks 加密", "2022-blake3-aes-128-gcm"), "clients": []}
    elif proto == "hysteria": settings = {"version": 2, "clients": []}
    elif proto in {"http", "mixed"}: settings = {"accounts": []}
    elif proto == "tunnel":
        settings = {"address": prompt("转发目标地址", "127.0.0.1"), "port": prompt_int("转发目标端口", 80, 1, 65535), "network": prompt("网络(tcp/udp/tcp,udp)", "tcp,udp")}
    elif proto in {"wireguard", "amneziawg"}:
        print("WireGuard/AmneziaWG 参数较多，请输入 settings JSON。")
        settings = edit_json_value("settings", {})
    if proto not in {"wireguard", "tunnel", "amneziawg"}:
        net = "hysteria" if proto == "hysteria" else prompt("传输方式 tcp/kcp/ws/grpc/httpupgrade/xhttp", "tcp")
        if net not in NETWORKS: raise ValueError(f"不支持的传输: {net}")
        sec = "tls" if proto == "hysteria" else prompt("安全 none/tls/reality", "none")
        if sec not in SECURITIES: raise ValueError(f"不支持的安全类型: {sec}")
        stream = {"network": net, "security": sec}
        if sec == "tls": stream["tlsSettings"] = {"serverName": prompt("TLS SNI", "")}
        elif sec == "reality": stream["realitySettings"] = edit_json_value("REALITY settings", {"dest": "www.cloudflare.com:443", "serverNames": ["www.cloudflare.com"], "privateKey": "", "shortIds": [""]})
        if net != "tcp" and yesno("配置传输详细参数", False):
            key = {"kcp": "kcpSettings", "ws": "wsSettings", "grpc": "grpcSettings", "httpupgrade": "httpupgradeSettings", "xhttp": "xhttpSettings", "hysteria": "hysteriaSettings"}[net]
            stream[key] = edit_json_value(key, {"version": 2} if net == "hysteria" else {})
    ib = {"listen": listen, "port": port, "protocol": proto, "settings": settings, "tag": tag, "sniffing": sniffing, "_lui": {"enable": True, "remark": prompt("备注", tag)}}
    if stream is not None: ib["streamSettings"] = stream
    return ib


def _b64decode_text(value: str) -> str:
    raw = value.strip(); pad = "=" * (-len(raw) % 4)
    return base64.urlsafe_b64decode((raw + pad).encode()).decode("utf-8")


def _stream_from_link(params: dict[str, list[str]], default_security: str = "none") -> dict[str, Any]:
    def one(k: str, default: str = "") -> str:
        vals = params.get(k) or []; return vals[0] if vals else default
    net = one("type", "tcp")
    if net not in {"tcp", "kcp", "ws", "grpc", "httpupgrade", "xhttp"}: net = "tcp"
    sec = one("security", default_security)
    if sec not in SECURITIES: sec = default_security
    stream: dict[str, Any] = {"network": net, "security": sec}; host = one("host", ""); path = one("path", "/")
    if net == "tcp": stream["tcpSettings"] = {"header": {"type": "none"}}
    elif net == "kcp": stream["kcpSettings"] = {}
    elif net == "ws": stream["wsSettings"] = {"path": path, "host": host, "headers": {}}
    elif net == "grpc": stream["grpcSettings"] = {"serviceName": one("serviceName", path if path != "/" else ""), "authority": one("authority", ""), "multiMode": one("mode") == "multi"}
    elif net == "httpupgrade": stream["httpupgradeSettings"] = {"path": path, "host": host, "headers": {}}
    elif net == "xhttp": stream["xhttpSettings"] = {"path": path, "host": host, "mode": one("mode", "auto"), "headers": {}}
    if sec == "tls":
        tls: dict[str, Any] = {"serverName": one("sni", ""), "fingerprint": one("fp", "")}; alpn = one("alpn", "")
        if alpn: tls["alpn"] = [x for x in alpn.split(",") if x]
        stream["tlsSettings"] = tls
    elif sec == "reality": stream["realitySettings"] = {"serverName": one("sni", ""), "fingerprint": one("fp", "chrome"), "publicKey": one("pbk", ""), "shortId": one("sid", ""), "spiderX": one("spx", "")}
    return stream
