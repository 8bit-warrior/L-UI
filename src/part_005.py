
def parse_outbound_link(link: str) -> Optional[dict[str, Any]]:
    """Parse the same common outbound URI families exposed by current 3x-ui."""
    link = link.strip()
    if not link: return None
    try:
        if link.startswith("vmess://"):
            obj=json.loads(_b64decode_text(link[8:]))
            if not isinstance(obj,dict): return None
            net=str(obj.get("net") or "tcp"); sec="tls" if obj.get("tls")=="tls" else "none"; qs={"type":[net],"security":[sec]}
            for src,dst in (("host","host"),("path","path"),("sni","sni"),("fp","fp"),("alpn","alpn"),("authority","authority")):
                if obj.get(src) not in (None,""): qs[dst]=[str(obj[src])]
            return {"protocol":"vmess","tag":str(obj.get("ps") or ""),"settings":{"vnext":[{"address":str(obj.get("add") or ""),"port":int(obj.get("port") or 443),"users":[{"id":str(obj.get("id") or ""),"security":str(obj.get("scy") or "auto")}]}]},"streamSettings":_stream_from_link(qs)}
        if link.startswith(("vless://","trojan://")):
            u=urllib.parse.urlsplit(link); proto=u.scheme
            if not u.hostname: return None
            q=urllib.parse.parse_qs(u.query,keep_blank_values=True); remark=urllib.parse.unquote(u.fragment or ""); user=urllib.parse.unquote(u.username or ""); port=u.port or 443
            if proto=="vless": settings={"address":u.hostname,"port":port,"id":user,"flow":(q.get("flow") or [""])[0],"encryption":(q.get("encryption") or ["none"])[0]}; default_sec="none"
            else: settings={"servers":[{"address":u.hostname,"port":port,"password":user}]}; default_sec="tls"
            return {"protocol":proto,"tag":remark,"settings":settings,"streamSettings":_stream_from_link(q,default_sec)}
        if link.startswith("ss://"):
            nofrag,_,frag=link.partition("#"); payload=nofrag.split("?",1)[0][5:]
            if "@" in payload:
                ui,hp=payload.split("@",1)
                try: auth=urllib.parse.unquote(ui) if ":" in ui else _b64decode_text(ui)
                except Exception: auth=urllib.parse.unquote(ui)
            else:
                decoded=_b64decode_text(payload)
                if "@" not in decoded: return None
                auth,hp=decoded.rsplit("@",1)
            hpu=urllib.parse.urlsplit("x://"+hp)
            if not hpu.hostname: return None
            method,_,password=auth.partition(":")
            if not password: password,method=method,"2022-blake3-aes-128-gcm"
            return {"protocol":"shadowsocks","tag":urllib.parse.unquote(frag),"settings":{"servers":[{"address":hpu.hostname,"port":hpu.port or 443,"password":password,"method":method,"uot":False,"UoTVersion":1}]}}
        if link.startswith(("hysteria2://","hy2://")):
            u=urllib.parse.urlsplit(link)
            if not u.hostname: return None
            q=urllib.parse.parse_qs(u.query,keep_blank_values=True); auth=urllib.parse.unquote(u.username or "")
            stream={"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"auth":auth,"udpIdleTimeout":60},"tlsSettings":{"serverName":(q.get("sni") or [""])[0],"fingerprint":(q.get("fp") or [""])[0],"alpn":((q.get("alpn") or ["h3"])[0]).split(",")}}
            return {"protocol":"hysteria","tag":urllib.parse.unquote(u.fragment or ""),"settings":{"address":u.hostname,"port":u.port or 443,"version":2},"streamSettings":stream}
        if link.startswith(("wireguard://","wg://")):
            u=urllib.parse.urlsplit(link)
            if not u.hostname: return None
            q=urllib.parse.parse_qs(u.query,keep_blank_values=True)
            def q1(*keys: str, default: str="") -> str:
                for k in keys:
                    if q.get(k): return q[k][0]
                return default
            allowed=[x.strip() for x in q1("allowedips","allowed_ips",default="0.0.0.0/0,::/0").split(",") if x.strip()]
            peer={"publicKey":q1("publickey","publicKey","public_key","peerPublicKey"),"endpoint":f"{u.hostname}:{u.port}" if u.port else u.hostname,"allowedIPs":allowed}
            psk=q1("presharedkey","preshared_key","pre-shared-key","psk")
            if psk: peer["preSharedKey"]=psk
            settings={"secretKey":urllib.parse.unquote(u.username or ""),"address":[x.strip() for x in q1("address","ip").split(",") if x.strip()],"peers":[peer]}
            return {"protocol":"wireguard","tag":urllib.parse.unquote(u.fragment or ""),"settings":settings}
    except Exception: return None
    return None


def decode_subscription_body(raw: bytes) -> Union[list[str],list[dict[str,Any]]]:
    if len(raw)>8*1024*1024: raise ValueError("订阅响应超过 8 MiB 限制")
    text=raw.decode("utf-8",errors="replace").strip()
    if not text: return []
    with contextlib.suppress(Exception):
        obj=json.loads(text)
        if isinstance(obj,list) and all(isinstance(x,dict) for x in obj): return obj
        if isinstance(obj,dict) and isinstance(obj.get("outbounds"),list): return obj["outbounds"]
    schemes=("vmess://","vless://","trojan://","ss://","hysteria2://","hy2://","wireguard://","wg://")
    if not any(sc in text for sc in schemes):
        with contextlib.suppress(Exception):
            decoded=_b64decode_text("".join(text.split()))
            if any(sc in decoded for sc in schemes): text=decoded
    return [ln.strip() for ln in text.splitlines() if ln.strip() and not ln.lstrip().startswith("#")]
