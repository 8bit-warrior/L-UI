
def import_3xui_menu(st: dict[str, Any]) -> None:
    path=Path(prompt("3x-ui .db / migration .dump 文件路径")).expanduser()
    if not path.exists():print("文件不存在");return
    try:
        info=analyze_3xui(path);show_json(info)
        if not info["valid"]:print("不是有效的 3x-ui 数据");return
        if not yesno("继续导入",False):return
        c=select("冲突策略",["跳过现有项目","使用 3x-ui 数据覆盖","自动重命名冲突 Tag/客户端"])
        if c==0:return
        strategy={1:"skip",2:"overwrite",3:"rename"}[c];bp=backup_state(st);print("已创建导入前回滚点:",bp);new,report=import_3xui_file(st,path,strategy);show_json(report)
        if yesno("应用导入结果",True):st.clear();st.update(new);save_state(st)
    except Exception as e:print("导入失败:",e)


def backup_menu(st: dict[str, Any]) -> None:
    items=["创建完整备份","查看备份列表","恢复备份","删除备份","导出可迁移数据包","从可迁移数据包恢复"]
    while True:
        n=select("L-UI 数据备份 / 恢复",items)
        if n==0:return
        try:
            if n in {1,5}:print("备份:",backup_state(st))
            elif n==2:
                for p in sorted(BACKUP_DIR.glob("*.json")):print(p)
            elif n in {3,6}:
                p=Path(prompt("备份文件路径")).expanduser();new=restore_backup(p);st.clear();st.update(new);save_state(st)
            elif n==4:Path(prompt("备份文件路径")).expanduser().unlink()
        except Exception as e:print("错误:",e)


def service_menu(st: dict[str, Any]) -> None:
    items=["查看服务状态","启动 Xray","停止 Xray","重启 Xray","开启开机自启","关闭开机自启","查看 Xray 版本","查看运行配置路径"]
    while True:
        n=select("服务管理",items)
        if n==0:return
        if n==1:print("状态:",service_status())
        elif 2<=n<=6:
            action={2:"start",3:"stop",4:"restart",5:"enable",6:"disable"}[n];ok,msg=service_action(action);print("成功" if ok else "失败",msg)
        elif n==7:print(run([str(XRAY_BIN),"version"]).stdout if XRAY_BIN.exists() else "未安装")
        elif n==8:print(CONFIG_FILE)


def tail_file(path: Path, lines: int=100) -> str:
    if not path.exists():return "日志不存在"
    try:return "\n".join(path.read_text(encoding="utf-8",errors="replace").splitlines()[-lines:])
    except Exception as e:return str(e)


def diagnostics_menu(st: dict[str, Any]) -> None:
    items=["检查 Xray 配置","查看 Xray 实时日志(tail)","查看最近错误日志","查看完整生成配置","查看监听端口","查看入站 / 出站状态","查看配置生成错误","重载当前配置"]
    while True:
        n=select("配置检查 / 日志",items)
        if n==0:return
        try:
            if n==1:write_config(st);ok,msg=validate_xray_config(CONFIG_FILE);print("通过" if ok else "失败",msg)
            elif n==2:print(tail_file(ACCESS_LOG,100))
            elif n==3:print(tail_file(ERROR_LOG,100))
            elif n==4:show_json(build_config(st))
            elif n==5:
                if shutil.which("ss"):subprocess.run(["ss","-lntup"])
                else:print("缺少 ss")
            elif n==6:list_inbounds(st);list_outbounds(st)
            elif n==7:ok,msg=validate_xray_config(CONFIG_FILE);print(msg)
            elif n==8:write_config(st);ok,msg=validate_xray_config(CONFIG_FILE);print(msg);restart_service()
        except Exception as e:print("错误:",e)


def kernel_menu(st: dict[str, Any]) -> None:
    while True:
        try:latest=latest_xray_version()
        except Exception as e:latest=f"获取失败: {e}"
        n=select("Xray 内核管理",[f"最新版本      {latest}",f"兼容版本      {COMPAT_XRAY}","自定义版本"])
        if n==0:return
        if n==1:
            if not latest.startswith("v"):print(latest);continue
            v=latest
        elif n==2:v=COMPAT_XRAY
        else:
            while True:
                v=normalize_version(prompt("输入 Xray 版本"))
                try:r=release_for_version(v)
                except Exception as e:print("验证失败:",e);r=None
                if r:break
                print(f"不存在此版本: {v}");break
            if not r:continue
        print(f"准备安装 {v}")
        if yesno("继续",True):
            ok,msg=install_xray(v);print("成功:" if ok else "失败:",msg)
            if ok:st.update(load_state())


def main_menu() -> None:
    st=load_state()
    while True:
        status=service_status();ver=st.get("xray",{}).get("version") or ("已安装(版本未知)" if XRAY_BIN.exists() else "未安装")
        print(f"\n╔══════════════════════════════════════════════╗\n║                    L-UI                      ║\n║          Lightweight Xray Manager            ║\n╠══════════════════════════════════════════════╣\n║ Xray 内核：{ver:<31}║\n║ Xray 状态：{status:<31}║\n╚══════════════════════════════════════════════╝")
        kernel_label="切换内核" if XRAY_BIN.exists() else "安装内核";items=["入站管理","出站管理","路由管理","客户端管理","导出订阅 / 客户端配置","导入 3x-ui 数据","L-UI 数据备份 / 恢复","服务管理",kernel_label,"配置检查 / 日志","卸载 L-UI"];n=select("主菜单",items)
        if n==0:return
        if n==1:inbounds_menu(st)
        elif n==2:outbounds_menu(st)
        elif n==3:routing_menu(st)
        elif n==4:clients_menu(st)
        elif n==5:export_menu(st)
        elif n==6:import_3xui_menu(st)
        elif n==7:backup_menu(st)
        elif n==8:service_menu(st)
        elif n==9:kernel_menu(st)
        elif n==10:diagnostics_menu(st)
        elif n==11:
            if yesno("确认卸载 L-UI（保留备份目录）",False):
                if os.geteuid()==0 and shutil.which("systemctl"):run(["systemctl","disable","--now",SERVICE_NAME]);SYSTEMD_UNIT.unlink(missing_ok=True);run(["systemctl","daemon-reload"])
                XRAY_BIN.unlink(missing_ok=True);STATE_FILE.unlink(missing_ok=True);CONFIG_FILE.unlink(missing_ok=True);print("已卸载核心文件，backups 未删除");return


def cli(argv: list[str]) -> int:
    ensure_dirs()
    if len(argv)==1:main_menu();return 0
    cmd=argv[1]
    try:
        st=load_state()
        if cmd=="config":show_json(build_config(st));return 0
        if cmd=="check":write_config(st);ok,msg=validate_xray_config(CONFIG_FILE);print(msg);return 0 if ok else 1
        if cmd=="route-test":url=argv[2] if len(argv)>2 else DEFAULT_TEST_URL;tag=argv[3] if len(argv)>3 else "";r=real_route_test(st,url,tag);show_json(r);return 0 if r.get("success") else 1
        if cmd=="analyze-3xui":show_json(analyze_3xui(Path(argv[2])));return 0
        if cmd=="self-check":return self_check(st)
        if cmd=="version":print("L-UI 0.2.0");return 0
        print("用法: l-ui [config|check|route-test URL [INBOUND_TAG]|analyze-3xui FILE|self-check|version]");return 2
    except Exception as e:print("错误:",e,file=sys.stderr);return 1
