
def inbounds_menu(st: dict[str, Any]) -> None:
    items = ["查看入站列表", "新增入站", "编辑入站(JSON)", "启用 / 停用入站", "查看入站详情", "导出分享链接", "导出入站 JSON", "克隆入站", "重置入站流量(重启统计)", "删除入站全部客户端", "删除入站", "批量删除入站", "导入入站 JSON", "导出全部分享链接", "重置全部入站流量(重启统计)", "生成入站客户端二维码", "导出入站订阅内容(Base64)", "导出全部订阅内容(Base64)"]
    while True:
        n = select("入站管理", items)
        if n == 0: return
        try:
            if n == 1: list_inbounds(st)
            elif n == 2:
                p = select("新增入站协议", [x.upper() if x in {"vless","vmess"} else x for x in INBOUND_PROTOCOLS])
                if p: st["inbounds"].append(make_inbound(INBOUND_PROTOCOLS[p-1], st)); save_state(st)
            elif n in {3,4,5,6,7,8,9,10,11}:
                list_inbounds(st); tag = prompt("入站 Tag"); idx, ib = find_by_tag(st["inbounds"], tag)
                if n == 3: st["inbounds"][idx] = edit_json_value("inbound", ib); save_state(st)
                elif n == 4: ib.setdefault("_lui", {})["enable"] = not ib.get("_lui",{}).get("enable",True); save_state(st)
                elif n == 5: show_json(ib)
                elif n == 6:
                    links=[]
                    for c in st["clients"]:
                        if tag in (c.get("inbound_tags") or []):
                            with contextlib.suppress(ValueError): links.append(share_link(ib,c))
                    print("\n".join(links) or "无可导出分享链接")
                elif n == 7: show_json(ib)
                elif n == 8:
                    cp=copy.deepcopy(ib); cp["tag"]=unique_tag(st, str(ib["tag"])+"-copy", "inbound"); cp["port"]=prompt_int("新端口", int(ib.get("port",0))+1,1,65535); st["inbounds"].append(cp); save_state(st)
                elif n == 9: restart_service(best_effort=True); print("已重启 Xray；运行时统计将重新开始")
                elif n == 10: st["clients"]=[c for c in st["clients"] if tag not in (c.get("inbound_tags") or [])]; save_state(st)
                elif n == 11:
                    st["inbounds"].pop(idx)
                    for c in st["clients"]: c["inbound_tags"]=[t for t in c.get("inbound_tags",[]) if t!=tag]
                    save_state(st)
            elif n == 12:
                tags={x.strip() for x in prompt("要删除的 Tag(逗号)").split(",") if x.strip()}; st["inbounds"]=[x for x in st["inbounds"] if x.get("tag") not in tags]
                for c in st["clients"]: c["inbound_tags"]=[t for t in c.get("inbound_tags",[]) if t not in tags]
                save_state(st)
            elif n == 13:
                data=json.loads(prompt("输入入站 JSON(对象或数组)")); arr=data if isinstance(data,list) else [data]; st["inbounds"].extend(arr); save_state(st)
            elif n == 14:
                links=[]
                for c in st["clients"]: links.extend(export_client_links(st,c))
                print("\n".join(links) or "无")
            elif n == 15: restart_service(best_effort=True); print("已重启 Xray")
            elif n in {16,17}:
                list_inbounds(st); tag=prompt("入站 Tag"); _,ib=find_by_tag(st["inbounds"],tag); attached=[c for c in st["clients"] if tag in (c.get("inbound_tags") or [])]
                if n==16:
                    if not attached: print("此入站没有客户端"); continue
                    for c in attached: print(f"{c.get('id')} {c.get('email')}")
                    _,c=find_client(st,prompt("客户端 ID/Email")); link=share_link(ib,c)
                    if shutil.which("qrencode"): subprocess.run(["qrencode","-t","ANSIUTF8",link])
                    else: print("未安装 qrencode，链接为:\n"+link)
                else:
                    links=[]
                    for c in attached:
                        with contextlib.suppress(ValueError): links.append(share_link(ib,c))
                    print(base64.b64encode(("\n".join(links)+("\n" if links else "")).encode()).decode() if links else "无")
            elif n == 18:
                links=[]
                for c in st["clients"]: links.extend(export_client_links(st,c))
                print(base64.b64encode(("\n".join(links)+("\n" if links else "")).encode()).decode() if links else "无")
        except Exception as e: print("错误:",e)


def outbounds_menu(st: dict[str, Any]) -> None:
    items=["查看出站列表","新增出站","编辑出站(JSON)","删除出站","调整出站顺序","设为默认出站","真实测试单个出站","真实测试全部出站","查看出站流量(日志)","导入出站 JSON","导出出站 JSON","出站订阅管理"]
    while True:
        n=select("出站管理",items)
        if n==0:return
        try:
            if n==1:list_outbounds(st)
            elif n==2:
                p=select("新增出站协议",OUTBOUND_PROTOCOLS)
                if p: st["outbounds"].append(make_outbound(OUTBOUND_PROTOCOLS[p-1],st));save_state(st)
            elif n in {3,4,6,7}:
                list_outbounds(st);tag=prompt("出站 Tag");idx,ob=find_by_tag(st["outbounds"],tag)
                if n==3: st["outbounds"][idx]=edit_json_value("outbound",ob);save_state(st)
                elif n==4:
                    if tag in {"direct","blocked"}: raise ValueError("内置 direct/blocked 不允许删除")
                    st["outbounds"].pop(idx);save_state(st)
                elif n==6:set_default_outbound(st,tag);save_state(st)
                elif n==7:
                    tst=copy.deepcopy(st); set_default_outbound(tst, tag); tst["routing"]={"domainStrategy":"AsIs","rules":[]}; show_route_result(real_route_test(tst,prompt("测试 URL",DEFAULT_TEST_URL)))
            elif n==5:
                list_outbounds(st);tag=prompt("出站 Tag");idx,_=find_by_tag(st["outbounds"],tag);pos=prompt_int("移动到第几位",idx+1,1,len(st["outbounds"]));item=st["outbounds"].pop(idx);st["outbounds"].insert(pos-1,item);save_state(st)
            elif n==8:
                url=prompt("测试 URL",DEFAULT_TEST_URL)
                for ob in st["outbounds"]:
                    tst=copy.deepcopy(st); set_default_outbound(tst, ob["tag"]); tst["routing"]={"domainStrategy":"AsIs","rules":[]};print(f"\n[{ob['tag']}]");show_route_result(real_route_test(tst,url))
            elif n==9: print(tail_file(ACCESS_LOG,100))
            elif n==10:
                data=json.loads(prompt("输入出站 JSON(对象/数组或 {outbounds:[]})"));arr=data if isinstance(data,list) else data.get("outbounds",[data]);st["outbounds"].extend(arr);save_state(st)
            elif n==11:show_json(st["outbounds"])
            elif n==12:outbound_subscriptions_menu(st)
        except Exception as e:print("错误:",e)
