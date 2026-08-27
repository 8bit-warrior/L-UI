
def outbound_subscriptions_menu(st: dict[str, Any]) -> None:
    items = ["查看订阅", "添加订阅", "编辑订阅", "启用 / 停用订阅", "删除订阅", "预览订阅解析结果", "更新指定订阅", "更新全部订阅", "调整订阅顺序"]
    while True:
        n = select("出站订阅管理", items)
        if n == 0: return
        try:
            subs = st.setdefault("outbound_subscriptions", [])
            if n == 1:
                list_subscriptions(st)
            elif n == 2:
                sid = int(st.setdefault("meta", {}).get("next_subscription_id", 1)); st["meta"]["next_subscription_id"] = sid + 1
                sub = {"id": sid, "remark": prompt("备注", f"sub{sid}"), "url": prompt("订阅 URL"), "tag_prefix": prompt("Tag 前缀", f"sub{sid}-"), "update_interval": prompt_int("更新间隔(秒)", 600, 60), "enabled": True, "allow_private": yesno("允许访问私有/本机地址", False), "allow_insecure": yesno("允许跳过 TLS 证书验证", False), "prepend": yesno("置于手工出站之前", False), "priority": len(subs), "last_outbounds": [], "identity_tags": {}, "last_updated": 0, "last_error": ""}
                count, skipped = refresh_subscription(sub)
                print(f"解析到 {count} 个出站" + (f"，跳过 {len(skipped)} 项" if skipped else ""))
                subs.append(sub); save_state(st)
            elif n in {3,4,5,7,9}:
                list_subscriptions(st); sid = prompt_int("订阅 ID", 1, 1); idx, sub = find_subscription(st, sid)
                if n == 3:
                    sub["remark"] = prompt("备注", str(sub.get("remark") or "")); sub["url"] = prompt("订阅 URL", str(sub.get("url") or "")); sub["tag_prefix"] = prompt("Tag 前缀", str(sub.get("tag_prefix") or f"sub{sid}-")); sub["update_interval"] = prompt_int("更新间隔(秒)", int(sub.get("update_interval") or 600), 60); sub["allow_private"] = yesno("允许私有地址", bool(sub.get("allow_private"))); sub["allow_insecure"] = yesno("跳过 TLS 验证", bool(sub.get("allow_insecure"))); sub["prepend"] = yesno("前置", bool(sub.get("prepend"))); refresh_subscription(sub); save_state(st)
                elif n == 4:
                    sub["enabled"] = not sub.get("enabled", True); save_state(st)
                elif n == 5:
                    subs.pop(idx)
                    for i, x in enumerate(sorted(subs, key=lambda y: (int(y.get("priority",0)), int(y.get("id",0))))): x["priority"] = i
                    save_state(st)
                elif n == 7:
                    count, skipped = refresh_subscription(sub); save_state(st); print(f"更新完成：{count} 个出站，跳过 {len(skipped)} 项")
                    if skipped: print("\n".join(skipped[:20]))
                elif n == 9:
                    direction = select("移动", ["上移", "下移"]); ordered = sorted(subs, key=lambda y: (int(y.get("priority",0)), int(y.get("id",0)))); pos = ordered.index(sub); target = pos - 1 if direction == 1 else pos + 1
                    if direction and 0 <= target < len(ordered): ordered[pos], ordered[target] = ordered[target], ordered[pos]
                    for i, x in enumerate(ordered): x["priority"] = i
                    save_state(st)
            elif n == 6:
                url = prompt("订阅 URL"); allow_private = yesno("允许私有地址", False); allow_insecure = yesno("跳过 TLS 验证", False); prefix = prompt("Tag 前缀", "preview-")
                obs, _, skipped = parse_subscription_data(fetch_subscription_url(url, allow_private, allow_insecure), prefix)
                show_json([{"tag": x.get("tag"), "protocol": x.get("protocol")} for x in obs])
                if skipped: print("跳过项：\n" + "\n".join(skipped[:20]))
            elif n == 8:
                total = 0; failed = 0
                for sub in subs:
                    if not sub.get("enabled", True): continue
                    try: count, _ = refresh_subscription(sub); total += count
                    except Exception as e: sub["last_error"] = str(e); failed += 1
                save_state(st); print(f"更新完成：{total} 个出站；失败订阅：{failed}")
        except Exception as e:
            print("错误:", e)


def outbound_server_settings(proto: str) -> dict[str, Any]:
    address = prompt("服务器地址", "127.0.0.1"); port = prompt_int("服务器端口", 443, 1, 65535)
    if proto == "vless": return {"address": address, "port": port, "id": prompt("UUID", str(uuid.uuid4())), "flow": prompt("Flow", ""), "encryption": "none"}
    if proto == "vmess": return {"vnext": [{"address": address, "port": port, "users": [{"id": prompt("UUID", str(uuid.uuid4())), "security": prompt("Security", "auto")}]}]}
    if proto == "trojan": return {"servers": [{"address": address, "port": port, "password": prompt("Password", str(uuid.uuid4()))}]}
    if proto == "shadowsocks": return {"servers": [{"address": address, "port": port, "method": prompt("Method", "2022-blake3-aes-128-gcm"), "password": prompt("Password"), "uot": False, "UoTVersion": 1}]}
    if proto in {"socks", "http"}:
        user = prompt("用户名(可空)", ""); passwd = prompt("密码(可空)", ""); server = {"address": address, "port": port}
        if user: server["users"] = [{"user": user, "pass": passwd}]
        return {"servers": [server]}
    if proto == "hysteria": return {"address": address, "port": port, "version": 2}
    return {}
