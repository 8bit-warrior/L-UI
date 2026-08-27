
def import_embedded_client(st: dict[str, Any], ec: dict[str, Any], tags: list[str], report: dict[str, Any]) -> None:
    email = str(ec.get("email") or f"client-{st['meta'].get('next_client_id', 1)}")
    if any(c.get("email") == email for c in st["clients"]): return
    cid = int(st["meta"].get("next_client_id", 1)); st["meta"]["next_client_id"] = cid + 1
    c = {"id": cid, "email": email, "sub_id": ec.get("subId") or uuid.uuid4().hex[:16], "uuid": ec.get("id") or "", "password": ec.get("password") or "", "auth": ec.get("auth") or "", "flow": ec.get("flow") or "", "security": ec.get("security") or "auto", "enable": bool(ec.get("enable", True)), "group": ec.get("group") or "", "comment": ec.get("comment") or "", "total_gb": int(ec.get("totalGB") or 0), "expiry_time": int(ec.get("expiryTime") or 0), "public_key": ec.get("publicKey") or "", "private_key": ec.get("privateKey") or "", "pre_shared_key": ec.get("preSharedKey") or "", "allowed_ips": ec.get("allowedIPs") or [], "keep_alive": int(ec.get("keepAlive") or 0), "secret": ec.get("secret") or "", "inbound_tags": tags, "source": "3x-ui-embedded"}
    st["clients"].append(c); report["clients"] += 1


def analyze_3xui(path: Path) -> dict[str, Any]:
    conn, holder, kind = open_3xui_source(path)
    try:
        tables = sqlite_table_names(conn)
        def count(t: str) -> int:
            if t not in tables: return 0
            try: return int(conn.execute(f'SELECT COUNT(*) FROM "{t}"').fetchone()[0])
            except Exception: return 0
        return {"format": kind, "tables": sorted(tables), "inbounds": count("inbounds"), "clients": count("clients"), "groups": count("client_groups"), "relations": count("client_inbounds"), "external_links": count("client_external_links"), "outbound_subscriptions": count("outbound_subscriptions"), "valid": "inbounds" in tables or "settings" in tables}
    finally:
        conn.close()
        with contextlib.suppress(Exception): holder.cleanup()


def import_3xui_file(st: dict[str, Any], path: Path, conflict: str) -> tuple[dict[str, Any], dict[str, Any]]:
    conn, holder, _ = open_3xui_source(path)
    try: return convert_3xui(conn, st, conflict)
    finally:
        conn.close()
        with contextlib.suppress(Exception): holder.cleanup()


def show_json(obj: Any) -> None:
    print(json.dumps(obj, ensure_ascii=False, indent=2))
