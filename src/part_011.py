
def convert_3xui(conn: sqlite3.Connection, st: dict[str, Any], conflict: str = "rename") -> tuple[dict[str, Any], dict[str, Any]]:
    tables=sqlite_table_names(conn)
    if "inbounds" not in tables and "settings" not in tables: raise ValueError("未发现 3x-ui 核心表")
    result=copy.deepcopy(st); report={"inbounds":0,"clients":0,"groups":0,"outbounds":0,"routing_rules":0,"outbound_subscriptions":0,"external_links":0,"skipped":[],"warnings":[]}; id_to_tag={}; old_client_to_new={}; existing_ib_tags={x.get("tag") for x in result["inbounds"]}
    for row in row_dicts(conn,"inbounds") if "inbounds" in tables else []:
        proto=str(row.get("protocol") or "")
        if proto in {"mtproto","tun"}: report["skipped"].append(f"入站 {row.get('tag') or row.get('id')}: {proto} 不由 L-UI/Xray-only 第一版接管"); continue
        if proto not in INBOUND_PROTOCOLS: report["skipped"].append(f"入站 {row.get('tag') or row.get('id')}: 未知协议 {proto}"); continue
        raw_tag=str(row.get("tag") or f"in-{proto}-{row.get('id')}"); tag=raw_tag
        if tag in existing_ib_tags:
            if conflict=="skip": report["skipped"].append(f"入站冲突跳过: {tag}"); continue
            if conflict=="overwrite": result["inbounds"]=[x for x in result["inbounds"] if x.get("tag")!=tag]
            else: tag=unique_tag(result,tag,"inbound")
        settings=parse_jsonish(row.get("settings"),{}); stream=parse_jsonish(row.get("stream_settings") if "stream_settings" in row else row.get("streamSettings"),{}); sniff=parse_jsonish(row.get("sniffing"),{"enabled":True,"destOverride":["http","tls","quic","fakedns"]}); embedded=settings.pop("clients",[]) if isinstance(settings,dict) else []
        ib={"listen":row.get("listen") or "0.0.0.0","port":int(row.get("port") or 0),"protocol":proto,"settings":settings,"tag":tag,"sniffing":sniff,"_lui":{"enable":bool(row.get("enable",1)),"remark":row.get("remark") or tag,"source":"3x-ui","source_id":row.get("id")}}
        if stream: ib["streamSettings"]=stream
        result["inbounds"].append(ib); existing_ib_tags.add(tag)
        if row.get("id") is not None:id_to_tag[int(row["id"])]=tag
        report["inbounds"]+=1
        if isinstance(embedded,list) and embedded and "clients" not in tables:
            for ec in embedded:
                if isinstance(ec,dict): import_embedded_client(result,ec,[tag],report)
    if "clients" in tables:
        rels={}
        for rel in row_dicts(conn,"client_inbounds") if "client_inbounds" in tables else []:
            try:
                tag=id_to_tag.get(int(rel.get("inbound_id")))
                if tag: rels.setdefault(int(rel.get("client_id")),[]).append(tag)
            except Exception: continue
        for row in row_dicts(conn,"clients"):
            email=str(row.get("email") or "")
            if not email: report["skipped"].append("客户端缺少 email"); continue
            if any(c.get("email")==email for c in result["clients"]):
                if conflict=="skip": report["skipped"].append(f"客户端冲突跳过: {email}"); continue
                if conflict=="overwrite": result["clients"]=[c for c in result["clients"] if c.get("email")!=email]
                else:
                    base=email;i=2
                    while any(c.get("email")==f"{base}-{i}" for c in result["clients"]):i+=1
                    email=f"{base}-{i}"
            cid=int(result["meta"].get("next_client_id",1));result["meta"]["next_client_id"]=cid+1;rid=int(row.get("id") or 0)
            c={"id":cid,"email":email,"sub_id":row.get("sub_id") or row.get("subId") or uuid.uuid4().hex[:16],"uuid":row.get("uuid") or "","password":row.get("password") or "","auth":row.get("auth") or "","flow":row.get("flow") or "","security":row.get("security") or "auto","enable":bool(row.get("enable",1)),"group":row.get("group_name") or row.get("group") or "","comment":row.get("comment") or "","total_gb":int(row.get("total_gb") or 0),"expiry_time":int(row.get("expiry_time") or 0),"public_key":row.get("wg_public_key") or "","private_key":row.get("wg_private_key") or "","pre_shared_key":row.get("wg_pre_shared_key") or "","allowed_ips":parse_jsonish(row.get("wg_allowed_ips"),[]),"keep_alive":int(row.get("wg_keep_alive") or 0),"secret":row.get("secret") or "","inbound_tags":rels.get(rid,[]),"source":"3x-ui"}
            result["clients"].append(c);old_client_to_new[rid]=cid;report["clients"]+=1
    if "client_external_links" in tables:
        for row in row_dicts(conn,"client_external_links"):
            try:old_cid=int(row.get("client_id") or 0)
            except Exception:continue
            new_cid=old_client_to_new.get(old_cid)
            if not new_cid:report["skipped"].append(f"外部链接 #{row.get('id')}: 找不到对应客户端");continue
            result["external_links"].append({"client_id":new_cid,"kind":row.get("kind") or "link","value":row.get("value") or "","remark":row.get("remark") or "","enable":bool(1 if row.get("enable") is None else row.get("enable")),"expiry_time":int(row.get("expiry_time") or 0),"name_prefix":row.get("name_prefix") or "","sort_index":int(row.get("sort_index") or 0),"source":"3x-ui"});report["external_links"]+=1
    if "client_groups" in tables:
        names={g.get("name") for g in result["client_groups"]}
        for g in row_dicts(conn,"client_groups"):
            name=str(g.get("name") or "")
            if name and name not in names: result["client_groups"].append({"name":name,"reset_up":g.get("reset_up",0),"reset_down":g.get("reset_down",0)});names.add(name);report["groups"]+=1
    pristine_before_template=(len(st.get("inbounds",[]))==0 and len(st.get("clients",[]))==0 and [o.get("tag") for o in st.get("outbounds",[])]==["direct","blocked"] and not st.get("routing",{}).get("rules"))
    if "settings" in tables:
        kv={str(r.get("key")):r.get("value") for r in row_dicts(conn,"settings")};template=None
        for key in ("xrayTemplateConfig","xrayTemplateConfigV2","xrayConfig"):
            if kv.get(key):
                parsed=parse_jsonish(kv[key],None)
                if isinstance(parsed,dict):template=parsed;break
        if template:
            imported_obs=template.get("outbounds") or [];valid_imported=[copy.deepcopy(ob) for ob in imported_obs if isinstance(ob,dict) and ob.get("protocol") in OUTBOUND_PROTOCOLS and ob.get("tag")];invalid_count=len(imported_obs)-len(valid_imported)
            if invalid_count:report["skipped"].append(f"有 {invalid_count} 个出站协议/Tag 无法识别")
            if pristine_before_template or conflict=="overwrite":
                imported_tags={ob.get("tag") for ob in valid_imported};tail=[] if pristine_before_template else [o for o in result["outbounds"] if o.get("tag") not in imported_tags];result["outbounds"]=valid_imported+tail;report["outbounds"]+=len(valid_imported)
            else:
                existing={o.get("tag") for o in result["outbounds"]}
                for ob in valid_imported:
                    tag=str(ob.get("tag"))
                    if tag in existing:
                        if conflict=="skip":continue
                        tag=unique_tag(result,tag,"outbound");ob["tag"]=tag
                    result["outbounds"].append(ob);existing.add(tag);report["outbounds"]+=1
            rt=template.get("routing") or {}
            if isinstance(rt,dict):
                if rt.get("domainStrategy"):result["routing"]["domainStrategy"]=rt["domainStrategy"]
                for rule in rt.get("rules") or []:
                    if isinstance(rule,dict):rr=copy.deepcopy(rule);rr["_lui_enabled"]=True;result["routing"]["rules"].append(rr);report["routing_rules"]+=1
                if isinstance(rt.get("balancers"),list):result["routing"]["balancers"]=copy.deepcopy(rt["balancers"])
            for key in ("dns","policy","stats","observatory","burstObservatory","reverse"):
                if key in template:result[key]=copy.deepcopy(template[key])
        else:report["warnings"].append("settings 中未找到可识别的 xrayTemplateConfig；仅导入入站/客户端")
    if "outbound_subscriptions" in tables:
        current_sub_ids={int(x.get("id",0)) for x in result.get("outbound_subscriptions",[])}
        for row in row_dicts(conn,"outbound_subscriptions"):
            source_id=int(row.get("id") or 0);sid=int(result.setdefault("meta",{}).get("next_subscription_id",1))
            while sid in current_sub_ids:sid+=1
            result["meta"]["next_subscription_id"]=sid+1;raw_obs=parse_jsonish(row.get("last_fetched_outbounds"),[]);obs=[copy.deepcopy(x) for x in raw_obs if isinstance(x,dict) and x.get("protocol") in OUTBOUND_PROTOCOLS and x.get("tag")] if isinstance(raw_obs,list) else [];identities=parse_jsonish(row.get("link_identities"),{})
            if not isinstance(identities,dict):identities={}
            result.setdefault("outbound_subscriptions",[]).append({"id":sid,"source_id":source_id,"remark":row.get("remark") or "","url":row.get("url") or "","tag_prefix":row.get("tag_prefix") or f"sub{sid}-","update_interval":int(row.get("update_interval") or 600),"enabled":bool(row.get("enabled",1)),"allow_private":bool(row.get("allow_private",0)),"allow_insecure":bool(row.get("allow_insecure",0)),"prepend":bool(row.get("prepend",0)),"priority":int(row.get("priority") or len(result.get("outbound_subscriptions",[]))),"last_outbounds":obs,"identity_tags":identities,"last_updated":int(row.get("last_updated") or 0),"last_error":row.get("last_error") or "","source":"3x-ui"});current_sub_ids.add(sid);report["outbound_subscriptions"]+=1
    validate_state(result);report["generated_at"]=now_iso();result["meta"]["last_import_report"]=report;return result,report
