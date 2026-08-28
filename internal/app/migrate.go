package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

func Detect3xuiInput(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	head = head[:n]
	if strings.HasPrefix(string(head), "SQLite format 3\x00") {
		return "sqlite", nil
	}
	if strings.HasPrefix(string(head), "PGDMP") {
		return "pgdump", nil
	}
	u := strings.ToUpper(string(head))
	if strings.Contains(u, "CREATE TABLE") || strings.Contains(u, "BEGIN TRANSACTION") {
		return "sql", nil
	}
	return "unknown", nil
}
func Open3xuiSource(path string) (*sql.DB, func(), string, error) {
	kind, e := Detect3xuiInput(path)
	if e != nil {
		return nil, nil, "", e
	}
	switch kind {
	case "sqlite":
		db, e := sql.Open("sqlite", path)
		if e != nil {
			return nil, nil, "", e
		}
		if e = db.Ping(); e != nil {
			db.Close()
			return nil, nil, "", e
		}
		return db, func() { db.Close() }, kind, nil
	case "sql":
		td, e := os.MkdirTemp("", "lui-3xui-*")
		if e != nil {
			return nil, nil, "", e
		}
		dbpath := filepath.Join(td, "import.db")
		db, e := sql.Open("sqlite", dbpath)
		if e != nil {
			os.RemoveAll(td)
			return nil, nil, "", e
		}
		data, e := os.ReadFile(path)
		if e == nil {
			_, e = db.Exec(string(data))
		}
		if e != nil {
			db.Close()
			os.RemoveAll(td)
			return nil, nil, "", e
		}
		return db, func() { db.Close(); os.RemoveAll(td) }, kind, nil
	case "pgdump":
		return nil, nil, kind, errors.New("这是 PostgreSQL 原生二进制 .dump；请在 3x-ui 中导出 Migration，得到跨平台 SQLite .db 后再导入")
	}
	return nil, nil, kind, errors.New("无法识别文件格式")
}
func tableNames(db *sql.DB) map[string]bool {
	out := map[string]bool{}
	rows, e := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if e != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var x string
		if rows.Scan(&x) == nil {
			out[x] = true
		}
	}
	return out
}
func rowDicts(db *sql.DB, table string) []map[string]any {
	q := `SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`
	rows, e := db.Query(q)
	if e != nil {
		return nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for idx := range vals {
			ptrs[idx] = &vals[idx]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		r := map[string]any{}
		for idx, k := range cols {
			v := vals[idx]
			if bb, ok := v.([]byte); ok {
				v = string(bb)
			}
			r[k] = v
		}
		out = append(out, r)
	}
	return out
}
func tableCount(db *sql.DB, t string) int {
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM "`+strings.ReplaceAll(t, `"`, `""`)+`"`).Scan(&n) != nil {
		return 0
	}
	return n
}

type ImportInfo struct {
	Format                string   `json:"format"`
	Tables                []string `json:"tables"`
	Inbounds              int      `json:"inbounds"`
	Clients               int      `json:"clients"`
	Groups                int      `json:"groups"`
	Relations             int      `json:"relations"`
	ExternalLinks         int      `json:"external_links"`
	OutboundSubscriptions int      `json:"outbound_subscriptions"`
	Valid                 bool     `json:"valid"`
}

func Analyze3xui(path string) (ImportInfo, error) {
	db, close, kind, e := Open3xuiSource(path)
	if e != nil {
		return ImportInfo{}, e
	}
	defer close()
	tabs := tableNames(db)
	names := []string{}
	for k := range tabs {
		names = append(names, k)
	}
	sort.Strings(names)
	return ImportInfo{Format: kind, Tables: names, Inbounds: tableCount(db, "inbounds"), Clients: tableCount(db, "clients"), Groups: tableCount(db, "client_groups"), Relations: tableCount(db, "client_inbounds"), ExternalLinks: tableCount(db, "client_external_links"), OutboundSubscriptions: tableCount(db, "outbound_subscriptions"), Valid: tabs["inbounds"] || tabs["settings"]}, nil
}

func UniqueTag(st *State, base, kind string) string {
	used := map[string]bool{}
	arr := st.Outbounds
	if kind == "inbound" {
		arr = st.Inbounds
	}
	for _, x := range arr {
		used[s(x["tag"])] = true
	}
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		x := fmt.Sprintf("%s-%d", base, n)
		if !used[x] {
			return x
		}
	}
}
func FindByTag(arr []map[string]any, tag string) (int, map[string]any, error) {
	for idx, x := range arr {
		if s(x["tag"]) == tag {
			return idx, x, nil
		}
	}
	return -1, nil, fmt.Errorf("找不到 Tag: %s", tag)
}
func ImportEmbeddedClient(st *State, ec map[string]any, tags []string, report map[string]any) {
	email := s(ec["email"])
	if email == "" {
		email = fmt.Sprintf("client-%d", i(st.Meta["next_client_id"]))
	}
	for _, c := range st.Clients {
		if s(c["email"]) == email {
			return
		}
	}
	cid := i(st.Meta["next_client_id"])
	if cid < 1 {
		cid = 1
	}
	st.Meta["next_client_id"] = cid + 1
	c := map[string]any{"id": cid, "email": email, "sub_id": firstNonEmpty(s(ec["subId"]), randHex(16)), "uuid": s(ec["id"]), "password": s(ec["password"]), "auth": s(ec["auth"]), "flow": s(ec["flow"]), "security": firstNonEmpty(s(ec["security"]), "auto"), "enable": b(ec["enable"], true), "group": s(ec["group"]), "comment": s(ec["comment"]), "total_gb": i64(ec["totalGB"]), "expiry_time": i64(ec["expiryTime"]), "public_key": s(ec["publicKey"]), "private_key": s(ec["privateKey"]), "pre_shared_key": s(ec["preSharedKey"]), "allowed_ips": strSlice(ec["allowedIPs"]), "keep_alive": i(ec["keepAlive"]), "secret": s(ec["secret"]), "inbound_tags": tags, "source": "3x-ui-embedded"}
	st.Clients = append(st.Clients, c)
	report["clients"] = i(report["clients"]) + 1
}

func Convert3xui(db *sql.DB, base *State, conflict string) (*State, map[string]any, error) {
	copyState := DeepCopy(*base)
	st := &copyState
	tabs := tableNames(db)
	if !tabs["inbounds"] && !tabs["settings"] {
		return nil, nil, errors.New("未发现 3x-ui 核心表")
	}
	report := map[string]any{"inbounds": 0, "clients": 0, "groups": 0, "outbounds": 0, "routing_rules": 0, "outbound_subscriptions": 0, "external_links": 0, "skipped": []any{}, "warnings": []any{}}
	skip := func(x string) { report["skipped"] = append(a(report["skipped"]), x) }
	warn := func(x string) { report["warnings"] = append(a(report["warnings"]), x) }
	idToTag := map[int]string{}
	oldClientToNew := map[int]int{}
	existing := map[string]bool{}
	for _, ib := range st.Inbounds {
		existing[s(ib["tag"])] = true
	}
	if tabs["inbounds"] {
		for _, row := range rowDicts(db, "inbounds") {
			proto := s(row["protocol"])
			if proto == "mtproto" || proto == "tun" {
				skip(fmt.Sprintf("入站 %v: %s 不由 L-UI/Xray-only 接管", row["id"], proto))
				continue
			}
			if !contains(InboundProtocols, proto) {
				skip(fmt.Sprintf("入站 %v: 未知协议 %s", row["id"], proto))
				continue
			}
			rawTag := s(row["tag"])
			if rawTag == "" {
				rawTag = fmt.Sprintf("in-%s-%d", proto, i(row["id"]))
			}
			tag := rawTag
			if existing[tag] {
				switch conflict {
				case "skip":
					skip("入站冲突跳过: " + tag)
					continue
				case "overwrite":
					tmp := st.Inbounds[:0]
					for _, x := range st.Inbounds {
						if s(x["tag"]) != tag {
							tmp = append(tmp, x)
						}
					}
					st.Inbounds = tmp
				default:
					tag = UniqueTag(st, tag, "inbound")
				}
			}
			settings, _ := parseJSONish(row["settings"], map[string]any{}).(map[string]any)
			if settings == nil {
				settings = map[string]any{}
			}
			streamKey := "stream_settings"
			if row[streamKey] == nil {
				streamKey = "streamSettings"
			}
			stream, _ := parseJSONish(row[streamKey], map[string]any{}).(map[string]any)
			sniff, _ := parseJSONish(row["sniffing"], map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic", "fakedns"}}).(map[string]any)
			embedded := a(settings["clients"])
			delete(settings, "clients")
			ib := map[string]any{"listen": firstNonEmpty(s(row["listen"]), "0.0.0.0"), "port": i(row["port"]), "protocol": proto, "settings": settings, "tag": tag, "sniffing": sniff, "_lui": map[string]any{"enable": b(row["enable"], true), "remark": firstNonEmpty(s(row["remark"]), tag), "source": "3x-ui", "source_id": row["id"]}}
			if len(stream) > 0 {
				ib["streamSettings"] = stream
			}
			st.Inbounds = append(st.Inbounds, ib)
			existing[tag] = true
			if row["id"] != nil {
				idToTag[i(row["id"])] = tag
			}
			report["inbounds"] = i(report["inbounds"]) + 1
			if len(embedded) > 0 && !tabs["clients"] {
				for _, v := range embedded {
					if ec, ok := v.(map[string]any); ok {
						ImportEmbeddedClient(st, ec, []string{tag}, report)
					}
				}
			}
		}
	}
	if tabs["clients"] {
		rels := map[int][]string{}
		if tabs["client_inbounds"] {
			for _, r := range rowDicts(db, "client_inbounds") {
				if tag := idToTag[i(r["inbound_id"])]; tag != "" {
					cid := i(r["client_id"])
					rels[cid] = append(rels[cid], tag)
				}
			}
		}
		for _, row := range rowDicts(db, "clients") {
			email := s(row["email"])
			if email == "" {
				skip("客户端缺少 email")
				continue
			}
			dupe := false
			for _, c := range st.Clients {
				if s(c["email"]) == email {
					dupe = true
					break
				}
			}
			if dupe {
				if conflict == "skip" {
					skip("客户端冲突跳过: " + email)
					continue
				} else if conflict == "overwrite" {
					tmp := st.Clients[:0]
					for _, c := range st.Clients {
						if s(c["email"]) != email {
							tmp = append(tmp, c)
						}
					}
					st.Clients = tmp
				} else {
					baseEmail := email
					for n := 2; ; n++ {
						cand := fmt.Sprintf("%s-%d", baseEmail, n)
						found := false
						for _, c := range st.Clients {
							if s(c["email"]) == cand {
								found = true
								break
							}
						}
						if !found {
							email = cand
							break
						}
					}
				}
			}
			cid := i(st.Meta["next_client_id"])
			if cid < 1 {
				cid = 1
			}
			st.Meta["next_client_id"] = cid + 1
			rid := i(row["id"])
			allowed := parseJSONish(row["wg_allowed_ips"], []any{})
			c := map[string]any{"id": cid, "email": email, "sub_id": firstNonEmpty(s(row["sub_id"]), s(row["subId"]), randHex(16)), "uuid": s(row["uuid"]), "password": s(row["password"]), "auth": s(row["auth"]), "flow": s(row["flow"]), "security": firstNonEmpty(s(row["security"]), "auto"), "enable": b(row["enable"], true), "group": firstNonEmpty(s(row["group_name"]), s(row["group"])), "comment": s(row["comment"]), "total_gb": i64(row["total_gb"]), "expiry_time": i64(row["expiry_time"]), "public_key": s(row["wg_public_key"]), "private_key": s(row["wg_private_key"]), "pre_shared_key": s(row["wg_pre_shared_key"]), "allowed_ips": allowed, "keep_alive": i(row["wg_keep_alive"]), "secret": s(row["secret"]), "inbound_tags": rels[rid], "source": "3x-ui"}
			st.Clients = append(st.Clients, c)
			oldClientToNew[rid] = cid
			report["clients"] = i(report["clients"]) + 1
		}
	}
	if tabs["client_external_links"] {
		for _, row := range rowDicts(db, "client_external_links") {
			newID := oldClientToNew[i(row["client_id"])]
			if newID == 0 {
				skip(fmt.Sprintf("外部链接 #%d: 找不到对应客户端", i(row["id"])))
				continue
			}
			st.ExternalLinks = append(st.ExternalLinks, map[string]any{"client_id": newID, "kind": firstNonEmpty(s(row["kind"]), "link"), "value": s(row["value"]), "remark": s(row["remark"]), "enable": b(row["enable"], true), "expiry_time": i64(row["expiry_time"]), "name_prefix": s(row["name_prefix"]), "sort_index": i(row["sort_index"]), "source": "3x-ui"})
			report["external_links"] = i(report["external_links"]) + 1
		}
	}
	if tabs["client_groups"] {
		names := map[string]bool{}
		for _, g := range st.ClientGroups {
			names[s(g["name"])] = true
		}
		for _, g := range rowDicts(db, "client_groups") {
			name := s(g["name"])
			if name != "" && !names[name] {
				st.ClientGroups = append(st.ClientGroups, map[string]any{"name": name, "reset_up": i64(g["reset_up"]), "reset_down": i64(g["reset_down"])})
				names[name] = true
				report["groups"] = i(report["groups"]) + 1
			}
		}
	}
	pristine := len(base.Inbounds) == 0 && len(base.Clients) == 0 && len(base.Outbounds) == 2 && s(base.Outbounds[0]["tag"]) == "direct" && s(base.Outbounds[1]["tag"]) == "blocked" && len(RoutingRules(base)) == 0
	if tabs["settings"] {
		kv := map[string]any{}
		for _, r := range rowDicts(db, "settings") {
			kv[s(r["key"])] = r["value"]
		}
		var templ map[string]any
		for _, k := range []string{"xrayTemplateConfig", "xrayTemplateConfigV2", "xrayConfig"} {
			if kv[k] != nil {
				if mm, ok := parseJSONish(kv[k], nil).(map[string]any); ok {
					templ = mm
					break
				}
			}
		}
		if templ != nil {
			valid := []map[string]any{}
			for _, v := range a(templ["outbounds"]) {
				if ob, ok := v.(map[string]any); ok && contains(OutboundProtocols, s(ob["protocol"])) && s(ob["tag"]) != "" {
					valid = append(valid, DeepCopy(ob))
				} else {
					skip("有出站协议/Tag 无法识别")
				}
			}
			if pristine || conflict == "overwrite" {
				tags := map[string]bool{}
				for _, ob := range valid {
					tags[s(ob["tag"])] = true
				}
				tail := []map[string]any{}
				if !pristine {
					for _, ob := range st.Outbounds {
						if !tags[s(ob["tag"])] {
							tail = append(tail, ob)
						}
					}
				}
				st.Outbounds = append(valid, tail...)
				report["outbounds"] = i(report["outbounds"]) + len(valid)
			} else {
				for _, ob := range valid {
					tag := s(ob["tag"])
					found := false
					for _, x := range st.Outbounds {
						if s(x["tag"]) == tag {
							found = true
							break
						}
					}
					if found {
						if conflict == "skip" {
							continue
						}
						tag = UniqueTag(st, tag, "outbound")
						ob["tag"] = tag
					}
					st.Outbounds = append(st.Outbounds, ob)
					report["outbounds"] = i(report["outbounds"]) + 1
				}
			}
			if rt, ok := templ["routing"].(map[string]any); ok {
				if s(rt["domainStrategy"]) != "" {
					st.Routing["domainStrategy"] = rt["domainStrategy"]
				}
				rules := RoutingRules(st)
				apiTag := ""
				if apiCfg, ok := templ["api"].(map[string]any); ok {
					apiTag = s(apiCfg["tag"])
				}
				for _, v := range a(rt["rules"]) {
					if rr, ok := v.(map[string]any); ok {
						// 3x-ui injects an internal control-plane route such as api -> api.
						// L-UI does not run 3x-ui's API application/inbound, so carrying this
						// rule over creates a dangling outboundTag and makes xray -test fail.
						if apiTag != "" && s(rr["outboundTag"]) == apiTag && contains(strSlice(rr["inboundTag"]), apiTag) {
							warn("已忽略 3x-ui 内部 API 路由规则（L-UI 不使用 3x-ui API 控制面）")
							continue
						}
						cp := DeepCopy(rr)
						cp["_lui_enabled"] = b(rr["enabled"], true)
						cp["_lui_source"] = "3x-ui"
						delete(cp, "enabled")
						rules = append(rules, cp)
						report["routing_rules"] = i(report["routing_rules"]) + 1
					}
				}
				SetRoutingRules(st, rules)
				if xs := a(rt["balancers"]); len(xs) > 0 {
					st.Routing["balancers"] = DeepCopy(xs)
				}
			}
			if templ["dns"] != nil {
				st.DNS = DeepCopy(templ["dns"])
			}
			if templ["policy"] != nil {
				st.Policy = DeepCopy(templ["policy"])
			}
			if templ["stats"] != nil {
				st.Stats = DeepCopy(templ["stats"])
			}
			if templ["observatory"] != nil {
				st.Observatory = DeepCopy(templ["observatory"])
			}
			if templ["burstObservatory"] != nil {
				st.BurstObservatory = DeepCopy(templ["burstObservatory"])
			}
			if templ["reverse"] != nil {
				st.Reverse = DeepCopy(templ["reverse"])
			}
		} else {
			warn("settings 中未找到可识别的 xrayTemplateConfig；仅导入入站/客户端")
		}
	}
	if tabs["outbound_subscriptions"] {
		used := map[int]bool{}
		for _, x := range st.OutboundSubscriptions {
			used[i(x["id"])] = true
		}
		for _, row := range rowDicts(db, "outbound_subscriptions") {
			sid := i(st.Meta["next_subscription_id"])
			if sid < 1 {
				sid = 1
			}
			for used[sid] {
				sid++
			}
			st.Meta["next_subscription_id"] = sid + 1
			rawObs := parseJSONish(row["last_fetched_outbounds"], []any{})
			obs := []any{}
			for _, v := range a(rawObs) {
				if ob, ok := v.(map[string]any); ok && contains(OutboundProtocols, s(ob["protocol"])) && s(ob["tag"]) != "" {
					obs = append(obs, DeepCopy(ob))
				}
			}
			ids := parseJSONish(row["link_identities"], map[string]any{})
			if _, ok := ids.(map[string]any); !ok {
				ids = map[string]any{}
			}
			st.OutboundSubscriptions = append(st.OutboundSubscriptions, map[string]any{"id": sid, "source_id": i(row["id"]), "remark": s(row["remark"]), "url": s(row["url"]), "tag_prefix": firstNonEmpty(s(row["tag_prefix"]), fmt.Sprintf("sub%d-", sid)), "update_interval": func() int {
				x := i(row["update_interval"])
				if x == 0 {
					return 600
				}
				return x
			}(), "enabled": b(row["enabled"], true), "allow_private": b(row["allow_private"], false), "allow_insecure": b(row["allow_insecure"], false), "prepend": b(row["prepend"], false), "priority": i(row["priority"]), "last_outbounds": obs, "identity_tags": ids, "last_updated": i64(row["last_updated"]), "last_error": s(row["last_error"]), "source": "3x-ui"})
			used[sid] = true
			report["outbound_subscriptions"] = i(report["outbound_subscriptions"]) + 1
		}
	}
	if e := ValidateState(st); e != nil {
		return nil, nil, e
	}
	report["generated_at"] = NowISO()
	st.Meta["last_import_report"] = report
	return st, report, nil
}
func Import3xuiFile(base *State, path, conflict string) (*State, map[string]any, error) {
	db, close, _, e := Open3xuiSource(path)
	if e != nil {
		return nil, nil, e
	}
	defer close()
	return Convert3xui(db, base, conflict)
}

var _ = json.Valid
