
def clients_menu(st: dict[str, Any]) -> None:
    items=["查看 / 搜索客户端","新增客户端","批量新增客户端","编辑客户端(JSON)","启用 / 停用客户端","查看客户端详情","生成客户端分享内容","重置客户端流量(运行时)","删除客户端","批量调整客户端","批量启用 / 停用","客户端分组管理","绑定入站","批量绑定入站","解绑入站","批量解绑入站","管理外部链接","重置全部客户端流量","删除流量耗尽客户端","删除孤立客户端","导入客户端 JSON","导出客户端 JSON","查看客户端订阅 / 分享链接"]
    while True:
        n=select("客户端管理",items)
        if n==0:return
        try:
            if n==1:
                q=prompt("搜索(空=全部)","").lower()
                for c in st["clients"]:
                    if not q or q in str(c.get("email","")).lower() or q in str(c.get("group","")).lower():print(f"{c.get('id')} {c.get('email')} {'ON' if c.get('enable',True) else 'OFF'} [{','.join(c.get('inbound_tags') or [])}]")
            elif n==2:st["clients"].append(create_client(st));save_state(st)
            elif n==3:
                count=prompt_int("数量",2,1,1000);prefix=prompt("名称前缀","client")
                for i in range(1,count+1):
                    cid=int(st["meta"].get("next_client_id",1));st["meta"]["next_client_id"]=cid+1;st["clients"].append({"id":cid,"email":f"{prefix}-{i}","sub_id":uuid.uuid4().hex[:16],"uuid":str(uuid.uuid4()),"password":uuid.uuid4().hex,"auth":uuid.uuid4().hex,"flow":"","security":"auto","enable":True,"group":"","comment":"","total_gb":0,"expiry_time":0,"inbound_tags":[]})
                save_state(st)
            elif n in {4,5,6,7,8,9,13,15,23}:
                list_clients(st);tok=prompt("客户端 ID/Email");idx,c=find_client(st,tok)
                if n==4:st["clients"][idx]=edit_json_value("client",c);save_state(st)
                elif n==5:c["enable"]=not c.get("enable",True);save_state(st)
                elif n==6:show_json(c)
                elif n in {7,23}:print("\n".join(export_client_links(st,c)) or "没有可生成的标准分享链接");show_json({"client":c,"inbounds":[ib for ib in st["inbounds"] if ib.get("tag") in (c.get("inbound_tags") or [])]})
                elif n==8:restart_service(best_effort=True);print("已重启 Xray；运行时计数重置")
                elif n==9:st["clients"].pop(idx);save_state(st)
                elif n==13:
                    tags={x.strip() for x in prompt("绑定入站 Tag(逗号)").split(",") if x.strip()};valid={ib["tag"] for ib in st["inbounds"]}
                    if tags-valid:raise ValueError("不存在的入站: "+",".join(tags-valid))
                    c["inbound_tags"]=sorted(set(c.get("inbound_tags",[]))|tags);save_state(st)
                elif n==15:
                    tags={x.strip() for x in prompt("解绑入站 Tag(逗号)").split(",") if x.strip()};c["inbound_tags"]=[t for t in c.get("inbound_tags",[]) if t not in tags];save_state(st)
            elif n==10:
                toks={x.strip() for x in prompt("客户端 ID/Email(逗号)").split(",")};field=prompt("字段名(group/comment/total_gb/expiry_time)","group");value=prompt("新值","")
                for c in st["clients"]:
                    if str(c.get("id")) in toks or c.get("email") in toks:c[field]=int(value) if field in {"total_gb","expiry_time"} and value else value
                save_state(st)
            elif n==11:
                toks={x.strip() for x in prompt("客户端 ID/Email(逗号)").split(",")};enable=yesno("设为启用",True)
                for c in st["clients"]:
                    if str(c.get("id")) in toks or c.get("email") in toks:c["enable"]=enable
                save_state(st)
            elif n==12:
                sub=select("分组管理",["查看分组","新增分组","删除分组"])
                if sub==1:show_json(st["client_groups"])
                elif sub==2:st["client_groups"].append({"name":prompt("分组名")});save_state(st)
                elif sub==3:
                    name=prompt("分组名");st["client_groups"]=[g for g in st["client_groups"] if g.get("name")!=name];save_state(st)
            elif n in {14,16}:
                toks={x.strip() for x in prompt("客户端 ID/Email(逗号)").split(",")};tags={x.strip() for x in prompt("入站 Tag(逗号)").split(",") if x.strip()}
                for c in st["clients"]:
                    if str(c.get("id")) in toks or c.get("email") in toks:c["inbound_tags"]=sorted(set(c.get("inbound_tags",[]))|tags) if n==14 else [t for t in c.get("inbound_tags",[]) if t not in tags]
                save_state(st)
            elif n==17:show_json(st["external_links"]);print("使用 JSON 直接维护 external_links：");st["external_links"]=edit_json_value("external_links",st["external_links"]);save_state(st)
            elif n==18:restart_service(best_effort=True);print("已重启 Xray")
            elif n==19:st["clients"]=[c for c in st["clients"] if not (c.get("total_gb",0)>0 and c.get("used_bytes",0)>=c.get("total_gb",0))];save_state(st)
            elif n==20:st["clients"]=[c for c in st["clients"] if c.get("inbound_tags")];save_state(st)
            elif n==21:
                data=json.loads(prompt("客户端 JSON(对象/数组)"));arr=data if isinstance(data,list) else [data];st["clients"].extend(arr);save_state(st)
            elif n==22:show_json(st["clients"])
        except Exception as e:print("错误:",e)


def export_menu(st: dict[str, Any]) -> None:
    items=["导出单客户端分享链接","导出单客户端订阅内容(Base64)","导出单客户端 Xray JSON","生成单客户端二维码(终端若有 qrencode)","批量导出客户端分享链接","批量导出订阅内容","导出全部入站分享链接","导出完整 Xray config.json","导出 L-UI 可迁移数据包"]
    while True:
        n=select("导出订阅 / 客户端配置",items)
        if n==0:return
        try:
            if n<=4:
                list_clients(st);_,c=find_client(st,prompt("客户端 ID/Email"));links=export_client_links(st,c)
                if n==1:print("\n".join(links))
                elif n==2:print(base64.b64encode(("\n".join(links)+"\n").encode()).decode())
                elif n==3:show_json({"client":c,"inbounds":[materialize_inbound(ib,[c]) for ib in st["inbounds"] if ib.get("tag") in c.get("inbound_tags",[])]})
                elif n==4:
                    if not links:print("无链接")
                    elif shutil.which("qrencode"):subprocess.run(["qrencode","-t","ANSIUTF8",links[0]])
                    else:print("未安装 qrencode，链接为:\n"+links[0])
            elif n in {5,6,7}:
                links=[]
                for c in st["clients"]:links.extend(export_client_links(st,c))
                print(base64.b64encode(("\n".join(links)+"\n").encode()).decode() if n==6 else "\n".join(links))
            elif n==8:show_json(build_config(st))
            elif n==9:print(backup_state(st))
        except Exception as e:print("错误:",e)
