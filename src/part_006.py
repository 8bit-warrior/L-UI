
def _reject_private_subscription_host(url: str, allow_private: bool) -> None:
    u=urllib.parse.urlsplit(url)
    if u.scheme not in {"http","https"} or not u.hostname: raise ValueError("订阅 URL 必须是 http:// 或 https://")
    if allow_private: return
    try: infos=socket.getaddrinfo(u.hostname,u.port or (443 if u.scheme=="https" else 80),type=socket.SOCK_STREAM)
    except socket.gaierror as e: raise ValueError(f"订阅域名解析失败: {e}") from e
    for info in infos:
        ip=ipaddress.ip_address(info[4][0])
        if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved or ip.is_multicast or ip.is_unspecified: raise ValueError(f"订阅 URL 指向非公网地址 {ip}；如确需访问请显式启用 allowPrivate")


class _SafeRedirect(urllib.request.HTTPRedirectHandler):
    def __init__(self,allow_private: bool): self.allow_private=allow_private; super().__init__()
    def redirect_request(self,req: urllib.request.Request,fp: Any,code: int,msg: str,headers: Any,newurl: str)->Optional[urllib.request.Request]:
        _reject_private_subscription_host(newurl,self.allow_private); return super().redirect_request(req,fp,code,msg,headers,newurl)


def fetch_subscription_url(url: str,allow_private: bool=False,allow_insecure: bool=False,timeout: int=30)->bytes:
    _reject_private_subscription_host(url,allow_private); ctx=ssl._create_unverified_context() if allow_insecure else ssl.create_default_context(); opener=urllib.request.build_opener(_SafeRedirect(allow_private),urllib.request.HTTPSHandler(context=ctx)); req=urllib.request.Request(url,headers={"User-Agent":"L-UI-outbound-sub/1.0"})
    with opener.open(req,timeout=timeout) as resp:
        if getattr(resp,"status",200)!=200: raise ValueError(f"订阅 HTTP 状态 {getattr(resp,'status','?')}")
        data=resp.read(8*1024*1024+1)
    if len(data)>8*1024*1024: raise ValueError("订阅响应超过 8 MiB 限制")
    return data


def _subscription_identity(link: str)->str: return hashlib.sha256(link.split("#",1)[0].encode()).hexdigest()

def _slug_tag(value: str,fallback: str)->str:
    s=re.sub(r"[^A-Za-z0-9_.-]+","-",value.strip()).strip("-."); return s[:64] or fallback


def parse_subscription_data(raw: bytes,prefix: str,previous: Optional[dict[str,str]]=None)->tuple[list[dict[str,Any]],dict[str,str],list[str]]:
    previous=previous or {}; decoded=decode_subscription_body(raw); out=[]; identities={}; skipped=[]; used=set()
    if decoded and isinstance(decoded[0],dict):
        for i,ob0 in enumerate(decoded):
            ob=copy.deepcopy(ob0)
            if not isinstance(ob,dict) or ob.get("protocol") not in OUTBOUND_PROTOCOLS: skipped.append(f"JSON #{i+1}: 不支持的出站"); continue
            tag=_slug_tag(str(ob.get("tag") or ""),f"{prefix}{i+1}"); base=tag; k=2
            while tag in used: tag=f"{base}-{k}"; k+=1
            ob["tag"]=tag; used.add(tag); out.append(ob)
        return out,identities,skipped
    for i,line0 in enumerate(decoded):
        line=str(line0); ob=parse_outbound_link(line)
        if not ob: skipped.append(f"#{i+1}: 无法解析 {line[:80]}"); continue
        ident=_subscription_identity(line); candidate=previous.get(ident) or (prefix+_slug_tag(str(ob.get("tag") or ""),str(i+1))); base=candidate; n=2
        while candidate in used: candidate=f"{base}-{n}"; n+=1
        ob["tag"]=candidate; used.add(candidate); identities[ident]=candidate; out.append(ob)
    return out,identities,skipped


def active_subscription_outbounds(st: dict[str,Any],prepend: Optional[bool]=None)->list[dict[str,Any]]:
    subs=sorted(st.get("outbound_subscriptions") or [],key=lambda x:(int(x.get("priority",0)),int(x.get("id",0)))); result=[]
    for sub in subs:
        if not sub.get("enabled",True): continue
        if prepend is not None and bool(sub.get("prepend",False))!=prepend: continue
        result.extend(copy.deepcopy(sub.get("last_outbounds") or []))
    return result


def refresh_subscription(sub: dict[str,Any])->tuple[int,list[str]]:
    if not sub.get("enabled",True): sub["last_outbounds"]=[]; return 0,[]
    raw=fetch_subscription_url(str(sub.get("url") or ""),bool(sub.get("allow_private",False)),bool(sub.get("allow_insecure",False))); obs,identities,skipped=parse_subscription_data(raw,str(sub.get("tag_prefix") or f"sub{sub.get('id')}-"),sub.get("identity_tags") or {}); sub["last_outbounds"]=obs; sub["identity_tags"]=identities; sub["last_updated"]=int(time.time()); sub["last_error"]=""; return len(obs),skipped


def find_subscription(st: dict[str,Any],sid: int)->tuple[int,dict[str,Any]]:
    for i,sub in enumerate(st.get("outbound_subscriptions") or []):
        if int(sub.get("id",0))==sid: return i,sub
    raise KeyError(f"subscription {sid}")


def list_subscriptions(st: dict[str,Any])->None:
    subs=sorted(st.get("outbound_subscriptions") or [],key=lambda x:(int(x.get("priority",0)),int(x.get("id",0))))
    if not subs: print("暂无出站订阅"); return
    print("\nID\t状态\t顺序\t位置\t数量\t备注\tURL")
    for sub in subs: print(f"{sub.get('id')}\t{'ON' if sub.get('enabled',True) else 'OFF'}\t{sub.get('priority',0)}\t{'前置' if sub.get('prepend') else '后置'}\t{len(sub.get('last_outbounds') or [])}\t{sub.get('remark','')}\t{sub.get('url','')}")
