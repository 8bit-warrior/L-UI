
def platform_asset_candidates() -> list[str]:
    arch = platform.machine().lower()
    amap = {"x86_64":["64"],"amd64":["64"],"aarch64":["arm64-v8a","arm64"],"arm64":["arm64-v8a","arm64"],"armv7l":["arm32-v7a","arm32-v7"],"i386":["32"],"i686":["32"]}
    suffixes = amap.get(arch, [arch])
    return [f"Xray-linux-{s}.zip" for s in suffixes]


def select_release_asset(release: dict[str, Any]) -> Optional[dict[str, Any]]:
    by_name = {a.get("name"): a for a in release.get("assets", [])}
    for name in platform_asset_candidates():
        if name in by_name: return by_name[name]
    return None


def download(url: str, dest: Path) -> None:
    req = urllib.request.Request(url, headers={"User-Agent":"L-UI/1"})
    with urllib.request.urlopen(req, timeout=60) as r, dest.open("wb") as f: shutil.copyfileobj(r,f)


def install_xray(version: str) -> tuple[bool,str]:
    version=normalize_version(version); rel=release_for_version(version)
    if not rel: return False,f"不存在此版本: {version}"
    asset=select_release_asset(rel)
    if not asset: return False,f"版本存在，但没有当前架构 {platform.machine()} 对应的 Linux 安装包"
    ensure_dirs(); previous_state=load_state(); previous_xray_meta=copy.deepcopy(previous_state.get("xray") or {}); had_old_binary=XRAY_BIN.exists(); old=BIN_DIR/"xray.old"
    with tempfile.TemporaryDirectory(prefix="lui-xray-") as td:
        z=Path(td)/"xray.zip"; download(asset["browser_download_url"],z)
        digest=str(asset.get("digest") or "")
        if digest.startswith("sha256:"):
            actual=hashlib.sha256(z.read_bytes()).hexdigest()
            if actual.lower()!=digest.split(":",1)[1].lower(): return False,"Xray release SHA256 校验失败"
        with zipfile.ZipFile(z) as arc:
            xname=next((n for n in arc.namelist() if Path(n).name=="xray"),None)
            if not xname: return False,"release 压缩包内未找到 xray"
            arc.extract(xname,td); candidate=Path(td)/xname; candidate.chmod(0o755); cp=run([str(candidate),"version"],timeout=10)
            if cp.returncode!=0: return False,"下载的 Xray 无法执行: "+(cp.stdout or "")
            expected=version.lstrip("v"); first_line=(cp.stdout or "").splitlines()[0] if (cp.stdout or "").splitlines() else ""
            if expected not in first_line: return False,f"内核版本校验失败：期望 {version}，实际输出 {first_line!r}"
            BIN_DIR.mkdir(parents=True,exist_ok=True); staged=BIN_DIR/"xray.new"; shutil.copy2(candidate,staged); staged.chmod(0o755)
            with contextlib.suppress(FileNotFoundError): old.unlink()
            if had_old_binary: shutil.copy2(XRAY_BIN,old)
            os.replace(staged,XRAY_BIN)
    def rollback(reason: str) -> tuple[bool,str]:
        if old.exists(): os.replace(old,XRAY_BIN)
        elif not had_old_binary: XRAY_BIN.unlink(missing_ok=True)
        restored=load_state(); restored["xray"]=previous_xray_meta; atomic_json_write(STATE_FILE,restored); restart_service(best_effort=True)
        return False,reason+"，已回滚旧内核和版本状态"
    install_systemd_unit(); ok,msg=validate_xray_config(CONFIG_FILE)
    if not ok: return rollback("新内核无法通过当前配置校验: "+msg)
    next_state=load_state(); next_state["xray"]={"version":version,"installed_at":now_iso()}; atomic_json_write(STATE_FILE,next_state)
    if os.geteuid()==0 and shutil.which("systemctl") and not restart_service(best_effort=True): return rollback("新内核安装后服务启动失败")
    with contextlib.suppress(FileNotFoundError): old.unlink()
    return True,f"已安装 {version}"


def install_systemd_unit() -> None:
    if os.geteuid()!=0 or shutil.which("systemctl") is None: return
    unit=f"""[Unit]\nDescription=L-UI managed Xray\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nExecStart={XRAY_BIN} run -config {CONFIG_FILE}\nRestart=on-failure\nRestartSec=3\nLimitNOFILE=1048576\n\n[Install]\nWantedBy=multi-user.target\n"""
    SYSTEMD_UNIT.write_text(unit,encoding="utf-8"); run(["systemctl","daemon-reload"])


def service_status() -> str:
    if shutil.which("systemctl") and os.geteuid()==0:
        cp=run(["systemctl","is-active",SERVICE_NAME]); return (cp.stdout or "").strip()
    return "unknown"


def restart_service(best_effort: bool=False) -> bool:
    if not XRAY_BIN.exists(): return False
    if shutil.which("systemctl") and os.geteuid()==0:
        install_systemd_unit(); cp=run(["systemctl","restart",SERVICE_NAME])
        if cp.returncode!=0 and not best_effort: raise RuntimeError(cp.stdout or "systemctl restart failed")
        return cp.returncode==0
    return False


def service_action(action: str) -> tuple[bool,str]:
    if os.geteuid()!=0 or not shutil.which("systemctl"): return False,"服务管理需要 root + systemd"
    install_systemd_unit()
    if action in {"start","stop","restart","enable","disable"}:
        args=["systemctl",action]
        if action in {"enable","disable"}: args.append("--now")
        args.append(SERVICE_NAME); cp=run(args); return cp.returncode==0,cp.stdout or ""
    return False,"未知服务操作"


def prompt(text: str, default: Optional[str]=None) -> str:
    suffix=f" [{default}]" if default is not None else ""; v=input(f"{text}{suffix}: ").strip(); return v if v else (default or "")


def prompt_int(text: str, default: int=0, lo: int=0, hi: int=65535) -> int:
    while True:
        raw=prompt(text,str(default))
        try:
            n=int(raw)
            if lo<=n<=hi: return n
        except ValueError: pass
        print(f"请输入 {lo}..{hi} 的整数")


def yesno(text: str, default: bool=True) -> bool:
    s=input(f"{text} [{'Y/n' if default else 'y/N'}]: ").strip().lower()
    if not s: return default
    return s in {"y","yes","1","是"}
