
def self_check(st: dict[str, Any]) -> int:
    checks=[]
    try:validate_state(st);checks.append(("state schema/invariants",True,""))
    except Exception as e:checks.append(("state schema/invariants",False,str(e)))
    try:cfg=build_config(st);json.dumps(cfg);checks.append(("config generation",True,""))
    except Exception as e:checks.append(("config generation",False,str(e)))
    if XRAY_BIN.exists():ok,msg=validate_xray_config(CONFIG_FILE);checks.append(("xray config test",ok,msg))
    else:checks.append(("xray config test",True,"SKIP: xray not installed"))
    checks.append(("curl available",shutil.which("curl") is not None,"required for real route test"));checks.append(("sqlite3 stdlib",True,sqlite3.sqlite_version))
    for name,ok,msg in checks:print(f"{'PASS' if ok else 'FAIL'}  {name} {msg}")
    return 0 if all(ok for _,ok,_ in checks) else 1


if __name__ == "__main__":
    raise SystemExit(cli(sys.argv))
