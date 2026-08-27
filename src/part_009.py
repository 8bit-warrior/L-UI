
def real_route_test(st: dict[str, Any], url: str, inbound_tag: str = "", timeout: int = 15) -> dict[str, Any]:
    if not XRAY_BIN.exists(): return {"success": False, "error": "Xray 尚未安装"}
    if not shutil.which("curl"): return {"success": False, "error": "缺少 curl，无法执行真实 HTTP/HTTPS 请求"}
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme not in {"http", "https"} or not parsed.hostname: return {"success": False, "error": "URL 必须是 http:// 或 https://"}
    port = free_port()
    with tempfile.TemporaryDirectory(prefix="lui-route-test-") as td:
        tdp=Path(td); access=tdp/"access.log"; error=tdp/"error.log"; cfg=build_config(st)
        cfg["log"]={"access":str(access),"error":str(error),"loglevel":"warning"}
        cfg["inbounds"]=[{"listen":"127.0.0.1","port":port,"protocol":"socks","settings":{"udp":True},"sniffing":{"enabled":True,"destOverride":["http","tls","quic"],"routeOnly":False},"tag":inbound_tag or "lui-route-test"}]
        cfg_path=tdp/"config.json"; atomic_json_write(cfg_path,cfg); ok,msg=validate_specific_xray_config(cfg_path)
        if not ok: return {"success":False,"error":"临时 Xray 配置校验失败: "+msg}
        proc=subprocess.Popen([str(XRAY_BIN),"run","-config",str(cfg_path)],stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        try:
            if not wait_port(port):
                output=""
                with contextlib.suppress(Exception): output=proc.stdout.read() if proc.stdout else ""
                return {"success":False,"error":"临时 Xray 未能监听测试端口: "+output[-1500:]}
            fmt="%{http_code}|%{time_total}|%{time_connect}|%{time_starttransfer}|%{remote_ip}"
            cmd=["curl","-sS","-L","--noproxy","","--max-time",str(timeout),"--connect-timeout",str(min(timeout,8)),"--socks5-hostname",f"127.0.0.1:{port}","-o",os.devnull,"-w",fmt,url]
            started=time.perf_counter(); cp=run(cmd,timeout=timeout+5); wall=(time.perf_counter()-started)*1000; parts=(cp.stdout or "").strip().split("|"); http_code=int(parts[0]) if parts and parts[0].isdigit() else 0
            def ms(i: int)->Optional[float]:
                try:return float(parts[i])*1000
                except Exception:return None
            log_text=""; ob="未知"; deadline=time.monotonic()+1.0
            while time.monotonic()<deadline:
                if access.exists():
                    log_text=access.read_text(encoding="utf-8",errors="replace"); ob=parse_outbound_from_access_log(log_text)
                    if ob!="未知": break
                time.sleep(0.05)
            return {"success":cp.returncode==0 and http_code>0,"curl_code":cp.returncode,"http_code":http_code,"outbound":ob,"total_ms":ms(1),"connect_ms":ms(2),"ttfb_ms":ms(3),"remote_ip":parts[4] if len(parts)>4 else "","wall_ms":wall,"stderr":"" if cp.returncode==0 else (cp.stdout or ""),"access_log":log_text.strip().splitlines()[-1] if log_text.strip() else ""}
        finally:
            proc.terminate()
            with contextlib.suppress(Exception): proc.wait(timeout=2)
            if proc.poll() is None: proc.kill()


def validate_specific_xray_config(path: Path) -> tuple[bool, str]:
    tries=[[str(XRAY_BIN),"run","-test","-config",str(path)],[str(XRAY_BIN),"-test","-config",str(path)]]; last=""
    for cmd in tries:
        cp=run(cmd,timeout=20); last=cp.stdout or ""
        if cp.returncode==0:return True,last.strip() or "OK"
    return False,last.strip()


def create_client(st: dict[str, Any]) -> dict[str, Any]:
    email=prompt("客户端名称/Email")
    if not email:raise ValueError("Email 不能为空")
    if any(c.get("email")==email for c in st["clients"]):raise ValueError("客户端已存在")
    available=[ib["tag"] for ib in st["inbounds"]]; print("可绑定入站:",", ".join(available) if available else "无"); tags=[x.strip() for x in prompt("绑定入站 Tag(逗号分隔)","").split(",") if x.strip()]; unknown=set(tags)-set(available)
    if unknown:raise ValueError("不存在的入站: "+", ".join(sorted(unknown)))
    cid=int(st["meta"].get("next_client_id",1)); st["meta"]["next_client_id"]=cid+1
    return {"id":cid,"email":email,"sub_id":uuid.uuid4().hex[:16],"uuid":str(uuid.uuid4()),"password":uuid.uuid4().hex,"auth":uuid.uuid4().hex,"flow":prompt("Flow",""),"security":"auto","enable":True,"group":prompt("分组",""),"comment":prompt("备注",""),"total_gb":0,"expiry_time":0,"inbound_tags":tags,"created_at":int(time.time()*1000)}


def list_clients(st: dict[str, Any]) -> None:
    if not st["clients"]:print("暂无客户端");return
    print("\nID\tEmail\t状态\t分组\t入站")
    for c in st["clients"]:print(f"{c.get('id')}\t{c.get('email')}\t{'启用' if c.get('enable',True) else '停用'}\t{c.get('group','')}\t{','.join(c.get('inbound_tags') or [])}")


def find_client(st: dict[str, Any], token: str) -> tuple[int, dict[str, Any]]:
    for i,c in enumerate(st["clients"]):
        if str(c.get("id"))==token or c.get("email")==token:return i,c
    raise KeyError(token)
