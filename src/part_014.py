
def routing_menu(st: dict[str, Any]) -> None:
    items=["基础路由设置","查看路由规则","新增路由规则","编辑路由规则(JSON)","启用 / 停用规则","删除路由规则","调整规则顺序","导入路由规则 JSON","导出路由规则 JSON","真实路由访问测试"]
    while True:
        n=select("路由管理",items)
        if n==0:return
        try:
            rules=st["routing"].setdefault("rules",[])
            if n==1:
                p=select("基础路由",["设置默认出站","BitTorrent 拦截开关","设置拦截 IP","设置拦截域名","设置直连 IP","设置直连域名","设置 IPv4 强制路由域名"]);basic_routing(st,p);save_state(st)
            elif n==2:
                for i,r in enumerate(rules,1):print(f"\n#{i} {'ON' if r.get('_lui_enabled',True) else 'OFF'}");show_json(r)
            elif n==3:rules.append(add_routing_rule(st));save_state(st)
            elif n in {4,5,6}:
                idx=prompt_int("规则编号",1,1,len(rules))-1
                if n==4:rules[idx]=edit_json_value("rule",rules[idx])
                elif n==5:rules[idx]["_lui_enabled"]=not rules[idx].get("_lui_enabled",True)
                else:rules.pop(idx)
                save_state(st)
            elif n==7:
                idx=prompt_int("规则编号",1,1,len(rules))-1;pos=prompt_int("移动到第几位",idx+1,1,len(rules));item=rules.pop(idx);rules.insert(pos-1,item);save_state(st)
            elif n==8:
                data=json.loads(prompt("输入 rules JSON"));arr=data if isinstance(data,list) else data.get("rules") or data.get("routing",{}).get("rules")
                if not isinstance(arr,list):raise ValueError("找不到规则数组")
                for r in arr:rr=copy.deepcopy(r);rr["_lui_enabled"]=True;rules.append(rr)
                save_state(st)
            elif n==9:show_json([strip_internal_rule(r) for r in rules])
            elif n==10:
                url=prompt("完整测试 URL",DEFAULT_TEST_URL);in_tag=prompt("模拟 Inbound Tag(可空)","");show_route_result(real_route_test(st,url,in_tag))
        except Exception as e:print("错误:",e)


def basic_routing(st: dict[str, Any], choice: int) -> None:
    if choice==0:return
    rules=st["routing"].setdefault("rules",[])
    if choice==1:list_outbounds(st);set_default_outbound(st,prompt("默认出站 Tag","direct"));return
    markers={2:"bittorrent",3:"block-ip",4:"block-domain",5:"direct-ip",6:"direct-domain",7:"ipv4-domain"};marker=markers[choice];rules[:]=[r for r in rules if r.get("_lui_basic")!=marker]
    if choice==2:
        if yesno("启用 BitTorrent 拦截",True):rules.insert(0,{"type":"field","protocol":["bittorrent"],"outboundTag":"blocked","_lui_basic":marker,"_lui_enabled":True})
        return
    raw=prompt("值(逗号分隔)","")
    if not raw:return
    vals=[x.strip() for x in raw.split(",") if x.strip()]
    if choice==3:rules.insert(0,{"type":"field","ip":vals,"outboundTag":"blocked","_lui_basic":marker,"_lui_enabled":True})
    elif choice==4:rules.insert(0,{"type":"field","domain":vals,"outboundTag":"blocked","_lui_basic":marker,"_lui_enabled":True})
    elif choice==5:rules.insert(0,{"type":"field","ip":vals,"outboundTag":"direct","_lui_basic":marker,"_lui_enabled":True})
    elif choice==6:rules.insert(0,{"type":"field","domain":vals,"outboundTag":"direct","_lui_basic":marker,"_lui_enabled":True})
    elif choice==7:rules.insert(0,{"type":"field","domain":vals,"outboundTag":"direct","_lui_basic":marker,"_lui_enabled":True})


def show_route_result(r: dict[str, Any]) -> None:
    if not r.get("success"):
        print("访问失败:",r.get("error") or r.get("stderr") or f"curl={r.get('curl_code')}")
        if r.get("outbound"):print("命中出口:",r["outbound"])
        return
    print("访问成功");print("命中出口:",r.get("outbound","未知"));print("HTTP 状态:",r.get("http_code"))
    if r.get("total_ms") is not None:print(f"整个请求延迟: {r['total_ms']:.2f} ms")
    if r.get("connect_ms") is not None:print(f"连接耗时: {r['connect_ms']:.2f} ms")
    if r.get("ttfb_ms") is not None:print(f"TTFB: {r['ttfb_ms']:.2f} ms")
