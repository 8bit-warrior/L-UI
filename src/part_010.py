
def host_for_share(ib: dict[str, Any]) -> str:
    m = ib.get("_lui") or {}
    if m.get("share_host"): return m["share_host"]
    listen = str(ib.get("listen") or "")
    if listen not in {"", "0.0.0.0", "::", "::0"}: return listen
    return prompt("分享地址(域名/IP)", "127.0.0.1")


def share_link(ib: dict[str, Any], c: dict[str, Any]) -> str:
    proto=str(ib.get("protocol")); host=host_for_share(ib); port=int(ib.get("port") or 0); tag=urllib.parse.quote(str((ib.get("_lui") or {}).get("remark") or ib.get("tag") or APP_NAME)); stream=ib.get("streamSettings") or {}; network=stream.get("network","tcp"); security=stream.get("security","none")
    if proto=="vless":
        q={"encryption":"none","type":network,"security":security}
        if c.get("flow"):q["flow"]=c["flow"]
        if security=="reality":
            rs=stream.get("realitySettings") or {}; q.update({"sni":(rs.get("serverNames") or [""])[0] if isinstance(rs.get("serverNames"),list) else rs.get("serverName",""),"pbk":rs.get("publicKey",""),"sid":(rs.get("shortIds") or [""])[0] if isinstance(rs.get("shortIds"),list) else rs.get("shortId",""),"fp":"chrome"})
        elif security=="tls":q["sni"]=(stream.get("tlsSettings") or {}).get("serverName","")
        return f"vless://{c.get('uuid')}@{bracket_host(host)}:{port}?{urllib.parse.urlencode(q)}#{tag}"
    if proto=="trojan":
        q={"type":network,"security":security}
        if security=="tls":q["sni"]=(stream.get("tlsSettings") or {}).get("serverName","")
        return f"trojan://{urllib.parse.quote(str(c.get('password','')))}@{bracket_host(host)}:{port}?{urllib.parse.urlencode(q)}#{tag}"
    if proto=="vmess":
        vm={"v":"2","ps":urllib.parse.unquote(tag),"add":host,"port":str(port),"id":c.get("uuid",""),"aid":"0","scy":c.get("security","auto"),"net":network,"type":"none","host":"","path":"","tls":"tls" if security=="tls" else "","sni":(stream.get("tlsSettings") or {}).get("serverName","")}
        return "vmess://"+base64.b64encode(json.dumps(vm,ensure_ascii=False,separators=(",",":")).encode()).decode()
    if proto=="shadowsocks":
        method=(ib.get("settings") or {}).get("method","2022-blake3-aes-128-gcm"); userinfo=base64.urlsafe_b64encode(f"{method}:{c.get('password','')}".encode()).decode().rstrip("="); return f"ss://{userinfo}@{bracket_host(host)}:{port}#{tag}"
    raise ValueError(f"暂不支持 {proto} 的标准分享 URI；可导出 Xray JSON")


def bracket_host(host: str)->str:
    try:
        ip=ipaddress.ip_address(host); return f"[{host}]" if ip.version==6 else host
    except ValueError:return host


def export_client_links(st: dict[str,Any],c: dict[str,Any])->list[str]:
    out=[]
    for tag in c.get("inbound_tags") or []:
        with contextlib.suppress(KeyError,ValueError): _,ib=find_by_tag(st["inbounds"],tag); out.append(share_link(ib,c))
    return out


def backup_state(st: dict[str,Any])->Path:
    ensure_dirs(); p=BACKUP_DIR/f"l-ui-{dt.datetime.now().strftime('%Y%m%d-%H%M%S')}.json"; atomic_json_write(p,{"format":"l-ui-backup-v1","created_at":now_iso(),"state":st}); return p


def restore_backup(path: Path)->dict[str,Any]:
    data=json.loads(path.read_text(encoding="utf-8"))
    if data.get("format")!="l-ui-backup-v1" or not isinstance(data.get("state"),dict):raise ValueError("不是有效的 L-UI 备份")
    validate_state(data["state"]); return data["state"]


def sqlite_table_names(conn: sqlite3.Connection)->set[str]:return {r[0] for r in conn.execute("SELECT name FROM sqlite_master WHERE type='table'")}

def row_dicts(conn: sqlite3.Connection,table: str)->list[dict[str,Any]]:
    conn.row_factory=sqlite3.Row
    try:return [dict(r) for r in conn.execute(f'SELECT * FROM "{table}"')]
    except sqlite3.DatabaseError:return []

def parse_jsonish(v: Any,default: Any)->Any:
    if v is None or v=="":return copy.deepcopy(default)
    if isinstance(v,(dict,list)):return copy.deepcopy(v)
    try:return json.loads(str(v))
    except Exception:return copy.deepcopy(default)


def detect_3xui_input(path: Path)->str:
    head=path.read_bytes()[:32]
    if head.startswith(b"SQLite format 3\x00"):return "sqlite"
    if head.startswith(b"PGDMP"):return "pgdump"
    text=path.read_text(encoding="utf-8",errors="ignore")[:4096]
    if "CREATE TABLE" in text.upper() or "BEGIN TRANSACTION" in text.upper():return "sql"
    return "unknown"


def stage_3xui_sql_dump(path: Path)->tuple[sqlite3.Connection,tempfile.TemporaryDirectory[str]]:
    td=tempfile.TemporaryDirectory(prefix="lui-3xui-"); db=Path(td.name)/"import.db"; conn=sqlite3.connect(db)
    try:conn.executescript(path.read_text(encoding="utf-8"));conn.commit()
    except Exception:conn.close();td.cleanup();raise
    return conn,td


def open_3xui_source(path: Path)->tuple[sqlite3.Connection,Any,str]:
    kind=detect_3xui_input(path)
    if kind=="sqlite":return sqlite3.connect(path),contextlib.nullcontext(),kind
    if kind=="sql":conn,td=stage_3xui_sql_dump(path);return conn,td,kind
    if kind=="pgdump":raise ValueError("这是 PostgreSQL 原生二进制 .dump；请在 3x-ui 中导出 Migration，得到跨平台 SQLite .db 后再导入")
    raise ValueError("无法识别文件格式")
