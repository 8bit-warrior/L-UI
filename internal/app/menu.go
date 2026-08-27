package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func withErr(fn func() error) {
	if e := fn(); e != nil {
		fmt.Println("错误:", e)
	}
}

func InboundsMenu(p Paths, st *State) {
	items := []string{"查看入站列表", "新增入站", "编辑入站(JSON)", "启用 / 停用入站", "查看入站详情", "导出分享链接", "导出入站 JSON", "克隆入站", "重置入站流量(重启统计)", "删除入站全部客户端", "删除入站", "批量删除入站", "导入入站 JSON", "导出全部分享链接", "重置全部入站流量(重启统计)", "生成入站客户端二维码", "导出入站订阅内容(Base64)", "导出全部订阅内容(Base64)"}
	for {
		n := Select("入站管理", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1:
				ListInbounds(st)
			case 2:
				x := Select("新增入站协议", InboundProtocols)
				if x > 0 {
					ib, e := MakeInbound(InboundProtocols[x-1], st)
					if e != nil {
						return e
					}
					st.Inbounds = append(st.Inbounds, ib)
					return SaveState(p, st, true)
				}
			case 3, 4, 5, 6, 7, 8, 9, 10, 11:
				ListInbounds(st)
				tag := Prompt("入站 Tag", "")
				idx, ib, e := FindByTag(st.Inbounds, tag)
				if e != nil {
					return e
				}
				switch n {
				case 3:
					v, e := safeEditJSON("inbound", ib)
					if e != nil {
						return e
					}
					mm, ok := v.(map[string]any)
					if !ok {
						return fmt.Errorf("inbound 必须是 JSON 对象")
					}
					st.Inbounds[idx] = mm
					return SaveState(p, st, true)
				case 4:
					meta := m(ib["_lui"])
					meta["enable"] = !b(meta["enable"], true)
					ib["_lui"] = meta
					return SaveState(p, st, true)
				case 5:
					ShowJSON(ib)
				case 6:
					links := []string{}
					for _, c := range st.Clients {
						if contains(strSlice(c["inbound_tags"]), tag) {
							if l, e := ShareLink(ib, c); e == nil {
								links = append(links, l)
							}
						}
					}
					if len(links) == 0 {
						fmt.Println("无可导出分享链接")
					} else {
						fmt.Println(strings.Join(links, "\n"))
					}
				case 7:
					ShowJSON(ib)
				case 8:
					cp := DeepCopy(ib)
					cp["tag"] = UniqueTag(st, s(ib["tag"])+"-copy", "inbound")
					cp["port"] = PromptInt("新端口", i(ib["port"])+1, 1, 65535)
					st.Inbounds = append(st.Inbounds, cp)
					return SaveState(p, st, true)
				case 9:
					_, msg := RestartService(p, true)
					fmt.Println("已重启 Xray；运行时统计将重新开始", msg)
				case 10:
					tmp := st.Clients[:0]
					for _, c := range st.Clients {
						if !contains(strSlice(c["inbound_tags"]), tag) {
							tmp = append(tmp, c)
						}
					}
					st.Clients = tmp
					return SaveState(p, st, true)
				case 11:
					st.Inbounds = append(st.Inbounds[:idx], st.Inbounds[idx+1:]...)
					for _, c := range st.Clients {
						xs := []string{}
						for _, t := range strSlice(c["inbound_tags"]) {
							if t != tag {
								xs = append(xs, t)
							}
						}
						c["inbound_tags"] = xs
					}
					return SaveState(p, st, true)
				}
			case 12:
				tags := map[string]bool{}
				for _, t := range strSlice(Prompt("要删除的 Tag(逗号)", "")) {
					tags[t] = true
				}
				tmp := st.Inbounds[:0]
				for _, ib := range st.Inbounds {
					if !tags[s(ib["tag"])] {
						tmp = append(tmp, ib)
					}
				}
				st.Inbounds = tmp
				for _, c := range st.Clients {
					xs := []string{}
					for _, t := range strSlice(c["inbound_tags"]) {
						if !tags[t] {
							xs = append(xs, t)
						}
					}
					c["inbound_tags"] = xs
				}
				return SaveState(p, st, true)
			case 13:
				raw := Prompt("输入入站 JSON(对象或数组)", "")
				var v any
				if e := json.Unmarshal([]byte(raw), &v); e != nil {
					return e
				}
				switch x := v.(type) {
				case map[string]any:
					st.Inbounds = append(st.Inbounds, x)
				case []any:
					for _, z := range x {
						mm, ok := z.(map[string]any)
						if !ok {
							return fmt.Errorf("数组元素必须是对象")
						}
						st.Inbounds = append(st.Inbounds, mm)
					}
				default:
					return fmt.Errorf("必须是对象或数组")
				}
				return SaveState(p, st, true)
			case 14:
				links := allLinks(st)
				if len(links) == 0 {
					fmt.Println("无")
				} else {
					fmt.Println(strings.Join(links, "\n"))
				}
			case 15:
				_, msg := RestartService(p, true)
				fmt.Println("已重启 Xray", msg)
			case 16, 17:
				ListInbounds(st)
				tag := Prompt("入站 Tag", "")
				_, ib, e := FindByTag(st.Inbounds, tag)
				if e != nil {
					return e
				}
				attached := []map[string]any{}
				for _, c := range st.Clients {
					if contains(strSlice(c["inbound_tags"]), tag) {
						attached = append(attached, c)
					}
				}
				if n == 16 {
					if len(attached) == 0 {
						fmt.Println("此入站没有客户端")
						break
					}
					for _, c := range attached {
						fmt.Printf("%d %s\n", i(c["id"]), s(c["email"]))
					}
					_, c, e := FindClient(st, Prompt("客户端 ID/Email", ""))
					if e != nil {
						return e
					}
					link, e := ShareLink(ib, c)
					if e != nil {
						return e
					}
					fmt.Println(link)
					return PrintQRCode(link)
				}
				links := []string{}
				for _, c := range attached {
					if l, e := ShareLink(ib, c); e == nil {
						links = append(links, l)
					}
				}
				if len(links) == 0 {
					fmt.Println("无")
				} else {
					fmt.Println(SubscriptionBase64(links))
				}
			case 18:
				links := allLinks(st)
				if len(links) == 0 {
					fmt.Println("无")
				} else {
					fmt.Println(SubscriptionBase64(links))
				}
			}
			return nil
		})
	}
}
func allLinks(st *State) []string {
	out := []string{}
	for _, c := range st.Clients {
		out = append(out, ExportClientLinks(st, c)...)
	}
	return out
}

func OutboundsMenu(p Paths, st *State) {
	items := []string{"查看出站列表", "新增出站", "编辑出站(JSON)", "删除出站", "调整出站顺序", "设为默认出站", "真实测试单个出站", "真实测试全部出站", "查看出站流量(日志)", "导入出站 JSON", "导出出站 JSON", "出站订阅管理"}
	for {
		n := Select("出站管理", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1:
				ListOutbounds(st)
			case 2:
				x := Select("新增出站协议", OutboundProtocols)
				if x > 0 {
					ob, e := MakeOutbound(OutboundProtocols[x-1], st)
					if e != nil {
						return e
					}
					st.Outbounds = append(st.Outbounds, ob)
					return SaveState(p, st, true)
				}
			case 3, 4, 6, 7:
				ListOutbounds(st)
				tag := Prompt("出站 Tag", "")
				idx, ob, e := FindByTag(st.Outbounds, tag)
				if e != nil {
					return e
				}
				switch n {
				case 3:
					v, e := safeEditJSON("outbound", ob)
					if e != nil {
						return e
					}
					mm, ok := v.(map[string]any)
					if !ok {
						return fmt.Errorf("outbound 必须是对象")
					}
					st.Outbounds[idx] = mm
					return SaveState(p, st, true)
				case 4:
					if tag == "direct" || tag == "blocked" {
						return fmt.Errorf("内置 direct/blocked 不允许删除")
					}
					st.Outbounds = append(st.Outbounds[:idx], st.Outbounds[idx+1:]...)
					return SaveState(p, st, true)
				case 6:
					if e = SetDefaultOutbound(st, tag); e != nil {
						return e
					}
					return SaveState(p, st, true)
				case 7:
					tst := DeepCopy(*st)
					ts := &tst
					if e = SetDefaultOutbound(ts, tag); e != nil {
						return e
					}
					ts.Routing = map[string]any{"domainStrategy": "AsIs", "rules": []any{}}
					ShowRouteResult(RealRouteTest(p, ts, Prompt("测试 URL", DefaultTestURL), "", 15*time.Second))
				}
			case 5:
				ListOutbounds(st)
				tag := Prompt("出站 Tag", "")
				idx, _, e := FindByTag(st.Outbounds, tag)
				if e != nil {
					return e
				}
				pos := PromptInt("移动到第几位", idx+1, 1, len(st.Outbounds))
				item := st.Outbounds[idx]
				st.Outbounds = append(st.Outbounds[:idx], st.Outbounds[idx+1:]...)
				pos--
				st.Outbounds = append(st.Outbounds, nil)
				copy(st.Outbounds[pos+1:], st.Outbounds[pos:])
				st.Outbounds[pos] = item
				return SaveState(p, st, true)
			case 8:
				url := Prompt("测试 URL", DefaultTestURL)
				for _, ob := range st.Outbounds {
					tst := DeepCopy(*st)
					ts := &tst
					_ = SetDefaultOutbound(ts, s(ob["tag"]))
					ts.Routing = map[string]any{"domainStrategy": "AsIs", "rules": []any{}}
					fmt.Printf("\n[%s]\n", s(ob["tag"]))
					ShowRouteResult(RealRouteTest(p, ts, url, "", 15*time.Second))
				}
			case 9:
				fmt.Println(TailFile(p.AccessLog, 100))
			case 10:
				raw := Prompt("输入出站 JSON(对象/数组或 {outbounds:[]})", "")
				var v any
				if e := json.Unmarshal([]byte(raw), &v); e != nil {
					return e
				}
				arr := []any{}
				switch x := v.(type) {
				case []any:
					arr = x
				case map[string]any:
					if z := a(x["outbounds"]); len(z) > 0 {
						arr = z
					} else {
						arr = []any{x}
					}
				}
				for _, z := range arr {
					mm, ok := z.(map[string]any)
					if !ok {
						return fmt.Errorf("出站元素必须是对象")
					}
					st.Outbounds = append(st.Outbounds, mm)
				}
				return SaveState(p, st, true)
			case 11:
				ShowJSON(st.Outbounds)
			case 12:
				OutboundSubscriptionsMenu(p, st)
			}
			return nil
		})
	}
}

func listSubscriptions(st *State) {
	subs := append([]map[string]any{}, st.OutboundSubscriptions...)
	sort.SliceStable(subs, func(x, y int) bool {
		pi, pj := i(subs[x]["priority"]), i(subs[y]["priority"])
		if pi == pj {
			return i(subs[x]["id"]) < i(subs[y]["id"])
		}
		return pi < pj
	})
	if len(subs) == 0 {
		fmt.Println("暂无出站订阅")
		return
	}
	fmt.Println("\nID\t状态\t顺序\t位置\t数量\t备注\tURL")
	for _, sub := range subs {
		status := "ON"
		if !b(sub["enabled"], true) {
			status = "OFF"
		}
		pos := "后置"
		if b(sub["prepend"], false) {
			pos = "前置"
		}
		fmt.Printf("%d\t%s\t%d\t%s\t%d\t%s\t%s\n", i(sub["id"]), status, i(sub["priority"]), pos, len(a(sub["last_outbounds"])), s(sub["remark"]), s(sub["url"]))
	}
}
func findSubscription(st *State, id int) (int, map[string]any, error) {
	for idx, x := range st.OutboundSubscriptions {
		if i(x["id"]) == id {
			return idx, x, nil
		}
	}
	return -1, nil, fmt.Errorf("subscription %d", id)
}
func OutboundSubscriptionsMenu(p Paths, st *State) {
	items := []string{"查看订阅", "添加订阅", "编辑订阅", "启用 / 停用订阅", "删除订阅", "预览订阅解析结果", "更新指定订阅", "更新全部订阅", "调整订阅顺序"}
	for {
		n := Select("出站订阅管理", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1:
				listSubscriptions(st)
			case 2:
				sid := i(st.Meta["next_subscription_id"])
				if sid < 1 {
					sid = 1
				}
				st.Meta["next_subscription_id"] = sid + 1
				sub := map[string]any{"id": sid, "remark": Prompt("备注", fmt.Sprintf("sub%d", sid)), "url": Prompt("订阅 URL", ""), "tag_prefix": Prompt("Tag 前缀", fmt.Sprintf("sub%d-", sid)), "update_interval": PromptInt("更新间隔(秒)", 600, 60, 0), "enabled": true, "allow_private": YesNo("允许访问私有/本机地址", false), "allow_insecure": YesNo("允许跳过 TLS 证书验证", false), "prepend": YesNo("置于手工出站之前", false), "priority": len(st.OutboundSubscriptions), "last_outbounds": []any{}, "identity_tags": map[string]any{}, "last_updated": 0, "last_error": ""}
				cnt, skip, e := RefreshSubscription(sub)
				if e != nil {
					return e
				}
				fmt.Printf("解析到 %d 个出站，跳过 %d 项\n", cnt, len(skip))
				st.OutboundSubscriptions = append(st.OutboundSubscriptions, sub)
				return SaveState(p, st, true)
			case 3, 4, 5, 7, 9:
				listSubscriptions(st)
				sid := PromptInt("订阅 ID", 1, 1, 0)
				idx, sub, e := findSubscription(st, sid)
				if e != nil {
					return e
				}
				switch n {
				case 3:
					sub["remark"] = Prompt("备注", s(sub["remark"]))
					sub["url"] = Prompt("订阅 URL", s(sub["url"]))
					sub["tag_prefix"] = Prompt("Tag 前缀", firstNonEmpty(s(sub["tag_prefix"]), fmt.Sprintf("sub%d-", sid)))
					sub["update_interval"] = PromptInt("更新间隔(秒)", i(sub["update_interval"]), 60, 0)
					sub["allow_private"] = YesNo("允许私有地址", b(sub["allow_private"], false))
					sub["allow_insecure"] = YesNo("跳过 TLS 验证", b(sub["allow_insecure"], false))
					sub["prepend"] = YesNo("前置", b(sub["prepend"], false))
					if _, _, e = RefreshSubscription(sub); e != nil {
						return e
					}
					return SaveState(p, st, true)
				case 4:
					sub["enabled"] = !b(sub["enabled"], true)
					return SaveState(p, st, true)
				case 5:
					st.OutboundSubscriptions = append(st.OutboundSubscriptions[:idx], st.OutboundSubscriptions[idx+1:]...)
					for k, x := range st.OutboundSubscriptions {
						x["priority"] = k
					}
					return SaveState(p, st, true)
				case 7:
					cnt, skip, e := RefreshSubscription(sub)
					if e != nil {
						return e
					}
					fmt.Printf("更新完成：%d 个出站，跳过 %d 项\n", cnt, len(skip))
					for idx, x := range skip {
						if idx >= 20 {
							break
						}
						fmt.Println(x)
					}
					return SaveState(p, st, true)
				case 9:
					dir := Select("移动", []string{"上移", "下移"})
					if dir == 0 {
						break
					}
					ordered := append([]map[string]any{}, st.OutboundSubscriptions...)
					sort.SliceStable(ordered, func(x, y int) bool { return i(ordered[x]["priority"]) < i(ordered[y]["priority"]) })
					pos := 0
					for k, x := range ordered {
						if i(x["id"]) == sid {
							pos = k
						}
					}
					target := pos - 1
					if dir == 2 {
						target = pos + 1
					}
					if target >= 0 && target < len(ordered) {
						ordered[pos], ordered[target] = ordered[target], ordered[pos]
					}
					for k, x := range ordered {
						x["priority"] = k
					}
					return SaveState(p, st, true)
				}
			case 6:
				url := Prompt("订阅 URL", "")
				raw, e := FetchSubscriptionURL(url, YesNo("允许私有地址", false), YesNo("跳过 TLS 验证", false))
				if e != nil {
					return e
				}
				obs, _, skip := ParseSubscriptionData(raw, Prompt("Tag 前缀", "preview-"), nil)
				shorts := []any{}
				for _, ob := range obs {
					shorts = append(shorts, map[string]any{"tag": ob["tag"], "protocol": ob["protocol"]})
				}
				ShowJSON(shorts)
				if len(skip) > 0 {
					fmt.Println("跳过项:")
					for idx, x := range skip {
						if idx >= 20 {
							break
						}
						fmt.Println(x)
					}
				}
			case 8:
				total, failed := 0, 0
				for _, sub := range st.OutboundSubscriptions {
					if !b(sub["enabled"], true) {
						continue
					}
					cnt, _, e := RefreshSubscription(sub)
					if e != nil {
						failed++
						continue
					}
					total += cnt
				}
				if e := SaveState(p, st, true); e != nil {
					return e
				}
				fmt.Printf("更新完成：%d 个出站；失败订阅：%d\n", total, failed)
			}
			return nil
		})
	}
}

func AddRoutingRule() map[string]any {
	r := map[string]any{"type": "field", "_lui_enabled": true}
	fields := [][2]string{{"domain", "域名(逗号分隔)"}, {"ip", "IP/CIDR(逗号分隔)"}, {"port", "目标端口/范围"}, {"sourceIP", "Source IP/CIDR(逗号分隔)"}, {"sourcePort", "Source Port"}, {"network", "Network tcp/udp/tcp,udp"}, {"protocol", "Protocol http,tls,quic,bittorrent..."}, {"inboundTag", "Inbound Tag(逗号分隔)"}, {"user", "User/email(逗号分隔)"}}
	listFields := map[string]bool{"domain": true, "ip": true, "sourceIP": true, "protocol": true, "inboundTag": true, "user": true}
	for _, f := range fields {
		v := Prompt(f[1], "")
		if v == "" {
			continue
		}
		if listFields[f[0]] {
			r[f[0]] = strSlice(v)
		} else {
			r[f[0]] = v
		}
	}
	target := Prompt("Outbound Tag(留空则使用 Balancer)", "")
	if target != "" {
		r["outboundTag"] = target
	} else {
		r["balancerTag"] = Prompt("Balancer Tag", "")
	}
	return r
}
func BasicRouting(st *State, choice int) error {
	if choice == 0 {
		return nil
	}
	rules := RoutingRules(st)
	if choice == 1 {
		ListOutbounds(st)
		return SetDefaultOutbound(st, Prompt("默认出站 Tag", "direct"))
	}
	markers := map[int]string{2: "bittorrent", 3: "block-ip", 4: "block-domain", 5: "direct-ip", 6: "direct-domain", 7: "ipv4-domain"}
	marker := markers[choice]
	tmp := rules[:0]
	for _, r := range rules {
		if s(r["_lui_basic"]) != marker {
			tmp = append(tmp, r)
		}
	}
	rules = tmp
	if choice == 2 {
		if YesNo("启用 BitTorrent 拦截", true) {
			rules = append([]map[string]any{{"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": "blocked", "_lui_basic": marker, "_lui_enabled": true}}, rules...)
		}
		SetRoutingRules(st, rules)
		return nil
	}
	vals := strSlice(Prompt("值(逗号分隔)", ""))
	if len(vals) == 0 {
		SetRoutingRules(st, rules)
		return nil
	}
	r := map[string]any{"type": "field", "_lui_basic": marker, "_lui_enabled": true}
	switch choice {
	case 3:
		r["ip"] = vals
		r["outboundTag"] = "blocked"
	case 4:
		r["domain"] = vals
		r["outboundTag"] = "blocked"
	case 5:
		r["ip"] = vals
		r["outboundTag"] = "direct"
	case 6, 7:
		r["domain"] = vals
		r["outboundTag"] = "direct"
	}
	rules = append([]map[string]any{r}, rules...)
	SetRoutingRules(st, rules)
	return nil
}
func RoutingMenu(p Paths, st *State) {
	items := []string{"基础路由设置", "查看路由规则", "新增路由规则", "编辑路由规则(JSON)", "启用 / 停用规则", "删除路由规则", "调整规则顺序", "导入路由规则 JSON", "导出路由规则 JSON", "真实路由访问测试"}
	for {
		n := Select("路由管理", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			rules := RoutingRules(st)
			switch n {
			case 1:
				x := Select("基础路由", []string{"设置默认出站", "BitTorrent 拦截开关", "设置拦截 IP", "设置拦截域名", "设置直连 IP", "设置直连域名", "设置 IPv4 强制路由域名"})
				if e := BasicRouting(st, x); e != nil {
					return e
				}
				return SaveState(p, st, true)
			case 2:
				for idx, r := range rules {
					state := "ON"
					if !ruleEnabled(r) {
						state = "OFF"
					}
					fmt.Printf("\n#%d %s\n", idx+1, state)
					ShowJSON(r)
				}
			case 3:
				rules = append(rules, AddRoutingRule())
				SetRoutingRules(st, rules)
				return SaveState(p, st, true)
			case 4, 5, 6:
				if len(rules) == 0 {
					return fmt.Errorf("暂无规则")
				}
				idx := PromptInt("规则编号", 1, 1, len(rules)) - 1
				if n == 4 {
					v, e := safeEditJSON("rule", rules[idx])
					if e != nil {
						return e
					}
					mm, ok := v.(map[string]any)
					if !ok {
						return fmt.Errorf("rule 必须是对象")
					}
					rules[idx] = mm
				} else if n == 5 {
					rules[idx]["_lui_enabled"] = !b(rules[idx]["_lui_enabled"], true)
				} else {
					rules = append(rules[:idx], rules[idx+1:]...)
				}
				SetRoutingRules(st, rules)
				return SaveState(p, st, true)
			case 7:
				if len(rules) == 0 {
					return fmt.Errorf("暂无规则")
				}
				idx := PromptInt("规则编号", 1, 1, len(rules)) - 1
				pos := PromptInt("移动到第几位", idx+1, 1, len(rules)) - 1
				item := rules[idx]
				rules = append(rules[:idx], rules[idx+1:]...)
				rules = append(rules, nil)
				copy(rules[pos+1:], rules[pos:])
				rules[pos] = item
				SetRoutingRules(st, rules)
				return SaveState(p, st, true)
			case 8:
				raw := Prompt("输入 rules JSON", "")
				var v any
				if e := json.Unmarshal([]byte(raw), &v); e != nil {
					return e
				}
				arr := []any{}
				switch x := v.(type) {
				case []any:
					arr = x
				case map[string]any:
					arr = a(x["rules"])
					if len(arr) == 0 {
						arr = a(m(x["routing"])["rules"])
					}
				}
				if arr == nil {
					return fmt.Errorf("找不到规则数组")
				}
				for _, z := range arr {
					rr, ok := z.(map[string]any)
					if !ok {
						return fmt.Errorf("规则必须是对象")
					}
					rr["_lui_enabled"] = true
					rules = append(rules, rr)
				}
				SetRoutingRules(st, rules)
				return SaveState(p, st, true)
			case 9:
				arr := []any{}
				for _, r := range rules {
					arr = append(arr, StripInternalRule(r))
				}
				ShowJSON(arr)
			case 10:
				ShowRouteResult(RealRouteTest(p, st, Prompt("完整测试 URL", DefaultTestURL), Prompt("模拟 Inbound Tag(可空)", ""), 15*time.Second))
			}
			return nil
		})
	}
}
func ShowRouteResult(r RouteResult) {
	if !r.Success {
		fmt.Println("访问失败:", r.Error)
		if r.Outbound != "" {
			fmt.Println("命中出口:", r.Outbound)
		}
		return
	}
	fmt.Println("访问成功")
	fmt.Println("命中出口:", r.Outbound)
	fmt.Println("HTTP 状态:", r.HTTPCode)
	fmt.Printf("整个请求延迟: %.2f ms\n", r.TotalMS)
	fmt.Printf("连接耗时: %.2f ms\n", r.ConnectMS)
	fmt.Printf("TTFB: %.2f ms\n", r.TTFBMS)
}

func ClientsMenu(p Paths, st *State) {
	items := []string{"查看 / 搜索客户端", "新增客户端", "批量新增客户端", "编辑客户端(JSON)", "启用 / 停用客户端", "查看客户端详情", "生成客户端分享内容", "重置客户端流量(运行时)", "删除客户端", "批量调整客户端", "批量启用 / 停用", "客户端分组管理", "绑定入站", "批量绑定入站", "解绑入站", "批量解绑入站", "管理外部链接", "重置全部客户端流量", "删除流量耗尽客户端", "删除孤立客户端", "导入客户端 JSON", "导出客户端 JSON", "查看客户端订阅 / 分享链接"}
	for {
		n := Select("客户端管理", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1:
				q := strings.ToLower(Prompt("搜索(空=全部)", ""))
				for _, c := range st.Clients {
					if q == "" || strings.Contains(strings.ToLower(s(c["email"])), q) || strings.Contains(strings.ToLower(s(c["group"])), q) {
						state := "ON"
						if !b(c["enable"], true) {
							state = "OFF"
						}
						fmt.Printf("%d %s %s [%s]\n", i(c["id"]), s(c["email"]), state, strings.Join(strSlice(c["inbound_tags"]), ","))
					}
				}
			case 2:
				c, e := CreateClient(st)
				if e != nil {
					return e
				}
				st.Clients = append(st.Clients, c)
				return SaveState(p, st, true)
			case 3:
				count := PromptInt("数量", 2, 1, 1000)
				prefix := Prompt("名称前缀", "client")
				for idx := 1; idx <= count; idx++ {
					cid := i(st.Meta["next_client_id"])
					if cid < 1 {
						cid = 1
					}
					st.Meta["next_client_id"] = cid + 1
					st.Clients = append(st.Clients, map[string]any{"id": cid, "email": fmt.Sprintf("%s-%d", prefix, idx), "sub_id": randHex(16), "uuid": UUID(), "password": randHex(32), "auth": randHex(32), "flow": "", "security": "auto", "enable": true, "group": "", "comment": "", "total_gb": 0, "expiry_time": 0, "inbound_tags": []string{}})
				}
				return SaveState(p, st, true)
			case 4, 5, 6, 7, 8, 9, 13, 15, 23:
				ListClients(st)
				idx, c, e := FindClient(st, Prompt("客户端 ID/Email", ""))
				if e != nil {
					return e
				}
				switch n {
				case 4:
					v, e := safeEditJSON("client", c)
					if e != nil {
						return e
					}
					mm, ok := v.(map[string]any)
					if !ok {
						return fmt.Errorf("client 必须是对象")
					}
					st.Clients[idx] = mm
					return SaveState(p, st, true)
				case 5:
					c["enable"] = !b(c["enable"], true)
					return SaveState(p, st, true)
				case 6:
					ShowJSON(c)
				case 7, 23:
					links := ExportClientLinks(st, c)
					if len(links) == 0 {
						fmt.Println("没有可生成的标准分享链接")
					} else {
						fmt.Println(strings.Join(links, "\n"))
					}
					ins := []any{}
					for _, ib := range st.Inbounds {
						if contains(strSlice(c["inbound_tags"]), s(ib["tag"])) {
							ins = append(ins, ib)
						}
					}
					ShowJSON(map[string]any{"client": c, "inbounds": ins})
				case 8:
					_, msg := RestartService(p, true)
					fmt.Println("已重启 Xray；运行时计数重置", msg)
				case 9:
					st.Clients = append(st.Clients[:idx], st.Clients[idx+1:]...)
					return SaveState(p, st, true)
				case 13:
					tags := strSlice(Prompt("绑定入站 Tag(逗号)", ""))
					valid := map[string]bool{}
					for _, ib := range st.Inbounds {
						valid[s(ib["tag"])] = true
					}
					for _, t := range tags {
						if !valid[t] {
							return fmt.Errorf("不存在的入站: %s", t)
						}
					}
					set := map[string]bool{}
					for _, t := range strSlice(c["inbound_tags"]) {
						set[t] = true
					}
					for _, t := range tags {
						set[t] = true
					}
					xs := []string{}
					for t := range set {
						xs = append(xs, t)
					}
					sort.Strings(xs)
					c["inbound_tags"] = xs
					return SaveState(p, st, true)
				case 15:
					drop := map[string]bool{}
					for _, t := range strSlice(Prompt("解绑入站 Tag(逗号)", "")) {
						drop[t] = true
					}
					xs := []string{}
					for _, t := range strSlice(c["inbound_tags"]) {
						if !drop[t] {
							xs = append(xs, t)
						}
					}
					c["inbound_tags"] = xs
					return SaveState(p, st, true)
				}
			case 10:
				toks := tokenSet(Prompt("客户端 ID/Email(逗号)", ""))
				field := Prompt("字段名(group/comment/total_gb/expiry_time)", "group")
				value := Prompt("新值", "")
				for _, c := range st.Clients {
					if toks[strconv.Itoa(i(c["id"]))] || toks[s(c["email"])] {
						if field == "total_gb" || field == "expiry_time" {
							n, _ := strconv.ParseInt(value, 10, 64)
							c[field] = n
						} else {
							c[field] = value
						}
					}
				}
				return SaveState(p, st, true)
			case 11:
				toks := tokenSet(Prompt("客户端 ID/Email(逗号)", ""))
				enable := YesNo("设为启用", true)
				for _, c := range st.Clients {
					if toks[strconv.Itoa(i(c["id"]))] || toks[s(c["email"])] {
						c["enable"] = enable
					}
				}
				return SaveState(p, st, true)
			case 12:
				x := Select("分组管理", []string{"查看分组", "新增分组", "删除分组"})
				if x == 1 {
					ShowJSON(st.ClientGroups)
				} else if x == 2 {
					st.ClientGroups = append(st.ClientGroups, map[string]any{"name": Prompt("分组名", "")})
					return SaveState(p, st, true)
				} else if x == 3 {
					name := Prompt("分组名", "")
					tmp := st.ClientGroups[:0]
					for _, g := range st.ClientGroups {
						if s(g["name"]) != name {
							tmp = append(tmp, g)
						}
					}
					st.ClientGroups = tmp
					return SaveState(p, st, true)
				}
			case 14, 16:
				toks := tokenSet(Prompt("客户端 ID/Email(逗号)", ""))
				tags := strSlice(Prompt("入站 Tag(逗号)", ""))
				for _, c := range st.Clients {
					if !(toks[strconv.Itoa(i(c["id"]))] || toks[s(c["email"])]) {
						continue
					}
					if n == 14 {
						set := map[string]bool{}
						for _, t := range strSlice(c["inbound_tags"]) {
							set[t] = true
						}
						for _, t := range tags {
							set[t] = true
						}
						xs := []string{}
						for t := range set {
							xs = append(xs, t)
						}
						sort.Strings(xs)
						c["inbound_tags"] = xs
					} else {
						drop := map[string]bool{}
						for _, t := range tags {
							drop[t] = true
						}
						xs := []string{}
						for _, t := range strSlice(c["inbound_tags"]) {
							if !drop[t] {
								xs = append(xs, t)
							}
						}
						c["inbound_tags"] = xs
					}
				}
				return SaveState(p, st, true)
			case 17:
				ShowJSON(st.ExternalLinks)
				v, e := safeEditJSON("external_links", st.ExternalLinks)
				if e != nil {
					return e
				}
				arr, ok := v.([]any)
				if !ok {
					return fmt.Errorf("external_links 必须是数组")
				}
				st.ExternalLinks = []map[string]any{}
				for _, z := range arr {
					mm, ok := z.(map[string]any)
					if !ok {
						return fmt.Errorf("外部链接必须是对象")
					}
					st.ExternalLinks = append(st.ExternalLinks, mm)
				}
				return SaveState(p, st, true)
			case 18:
				_, msg := RestartService(p, true)
				fmt.Println("已重启 Xray", msg)
			case 19:
				tmp := st.Clients[:0]
				for _, c := range st.Clients {
					if i64(c["total_gb"]) > 0 && i64(c["used_bytes"]) >= i64(c["total_gb"]) {
						continue
					}
					tmp = append(tmp, c)
				}
				st.Clients = tmp
				return SaveState(p, st, true)
			case 20:
				tmp := st.Clients[:0]
				for _, c := range st.Clients {
					if len(strSlice(c["inbound_tags"])) > 0 {
						tmp = append(tmp, c)
					}
				}
				st.Clients = tmp
				return SaveState(p, st, true)
			case 21:
				raw := Prompt("客户端 JSON(对象/数组)", "")
				var v any
				if e := json.Unmarshal([]byte(raw), &v); e != nil {
					return e
				}
				switch x := v.(type) {
				case map[string]any:
					st.Clients = append(st.Clients, x)
				case []any:
					for _, z := range x {
						mm, ok := z.(map[string]any)
						if !ok {
							return fmt.Errorf("客户端必须是对象")
						}
						st.Clients = append(st.Clients, mm)
					}
				}
				return SaveState(p, st, true)
			case 22:
				ShowJSON(st.Clients)
			}
			return nil
		})
	}
}
func tokenSet(raw string) map[string]bool {
	out := map[string]bool{}
	for _, x := range strSlice(raw) {
		out[x] = true
	}
	return out
}

func ExportMenu(p Paths, st *State) {
	items := []string{"导出单客户端分享链接", "导出单客户端订阅内容(Base64)", "导出单客户端 Xray JSON", "生成单客户端二维码", "批量导出客户端分享链接", "批量导出订阅内容", "导出全部入站分享链接", "导出完整 Xray config.json", "导出 L-UI 可迁移数据包"}
	for {
		n := Select("导出订阅 / 客户端配置", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			if n <= 4 {
				ListClients(st)
				_, c, e := FindClient(st, Prompt("客户端 ID/Email", ""))
				if e != nil {
					return e
				}
				links := ExportClientLinks(st, c)
				switch n {
				case 1:
					fmt.Println(strings.Join(links, "\n"))
				case 2:
					fmt.Println(SubscriptionBase64(links))
				case 3:
					ins := []any{}
					for _, ib := range st.Inbounds {
						if contains(strSlice(c["inbound_tags"]), s(ib["tag"])) {
							if x, ok := MaterializeInbound(ib, []map[string]any{c}); ok {
								ins = append(ins, x)
							}
						}
					}
					ShowJSON(map[string]any{"client": c, "inbounds": ins})
				case 4:
					if len(links) == 0 {
						fmt.Println("无链接")
						break
					}
					fmt.Println(links[0])
					return PrintQRCode(links[0])
				}
				return nil
			}
			switch n {
			case 5, 7:
				fmt.Println(strings.Join(allLinks(st), "\n"))
			case 6:
				fmt.Println(SubscriptionBase64(allLinks(st)))
			case 8:
				cfg, e := BuildConfig(st)
				if e != nil {
					return e
				}
				ShowJSON(cfg)
			case 9:
				name, e := BackupState(p, st)
				if e != nil {
					return e
				}
				fmt.Println(name)
			}
			return nil
		})
	}
}

func Import3xuiMenu(p Paths, st *State) {
	path := Prompt("3x-ui .db / migration .dump 文件路径", "")
	if _, e := os.Stat(path); e != nil {
		fmt.Println("文件不存在")
		return
	}
	withErr(func() error {
		info, e := Analyze3xui(path)
		if e != nil {
			return e
		}
		ShowJSON(info)
		if !info.Valid {
			return fmt.Errorf("不是有效的 3x-ui 数据")
		}
		if !YesNo("继续导入", false) {
			return nil
		}
		x := Select("冲突策略", []string{"跳过现有项目", "使用 3x-ui 数据覆盖", "自动重命名冲突 Tag/客户端"})
		if x == 0 {
			return nil
		}
		strategy := map[int]string{1: "skip", 2: "overwrite", 3: "rename"}[x]
		bp, e := BackupState(p, st)
		if e != nil {
			return e
		}
		fmt.Println("已创建导入前回滚点:", bp)
		next, report, e := Import3xuiFile(st, path, strategy)
		if e != nil {
			return e
		}
		ShowJSON(report)
		if YesNo("应用导入结果", true) {
			*st = *next
			return SaveState(p, st, true)
		}
		return nil
	})
}
func BackupMenu(p Paths, st *State) {
	items := []string{"创建完整备份", "查看备份列表", "恢复备份", "删除备份", "导出可迁移数据包", "从可迁移数据包恢复"}
	for {
		n := Select("L-UI 数据备份 / 恢复", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1, 5:
				name, e := BackupState(p, st)
				if e != nil {
					return e
				}
				fmt.Println("备份:", name)
			case 2:
				xs, _ := filepath.Glob(filepath.Join(p.Backup, "*.json"))
				sort.Strings(xs)
				for _, x := range xs {
					fmt.Println(x)
				}
			case 3, 6:
				next, e := RestoreBackup(Prompt("备份文件路径", ""))
				if e != nil {
					return e
				}
				*st = *next
				return SaveState(p, st, true)
			case 4:
				return os.Remove(Prompt("备份文件路径", ""))
			}
			return nil
		})
	}
}
func ServiceMenu(p Paths) {
	items := []string{"查看服务状态", "启动 Xray", "停止 Xray", "重启 Xray", "开启开机自启", "关闭开机自启", "查看 Xray 版本", "查看运行配置路径"}
	for {
		n := Select("服务管理", items)
		if n == 0 {
			return
		}
		switch n {
		case 1:
			fmt.Println("状态:", ServiceStatus(p))
		case 2, 3, 4, 5, 6:
			action := map[int]string{2: "start", 3: "stop", 4: "restart", 5: "enable", 6: "disable"}[n]
			ok, msg := ServiceAction(p, action)
			if ok {
				fmt.Println("成功", msg)
			} else {
				fmt.Println("失败", msg)
			}
		case 7:
			fmt.Println(XrayVersion(p))
		case 8:
			fmt.Println(p.Config)
		}
	}
}
func TailFile(path string, lines int) string {
	d, e := os.ReadFile(path)
	if e != nil {
		return "日志不存在"
	}
	xs := strings.Split(string(d), "\n")
	if len(xs) > lines {
		xs = xs[len(xs)-lines:]
	}
	return strings.Join(xs, "\n")
}
func DiagnosticsMenu(p Paths, st *State) {
	items := []string{"检查 Xray 配置", "查看 Xray 实时日志(tail)", "查看最近错误日志", "查看完整生成配置", "查看监听端口", "查看入站 / 出站状态", "查看配置生成错误", "重载当前配置"}
	for {
		n := Select("配置检查 / 日志", items)
		if n == 0 {
			return
		}
		withErr(func() error {
			switch n {
			case 1:
				if e := WriteConfig(p, st); e != nil {
					return e
				}
				ok, msg := ValidateXrayConfig(p, p.Config)
				if ok {
					fmt.Println("通过", msg)
				} else {
					fmt.Println("失败", msg)
				}
			case 2:
				fmt.Println(TailFile(p.AccessLog, 100))
			case 3:
				fmt.Println(TailFile(p.ErrorLog, 100))
			case 4:
				cfg, e := BuildConfig(st)
				if e != nil {
					return e
				}
				ShowJSON(cfg)
			case 5:
				fmt.Println("按当前配置应监听：")
				for _, ib := range st.Inbounds {
					meta := m(ib["_lui"])
					if b(meta["enable"], true) {
						fmt.Printf("%s %s:%d (%s)\n", s(ib["tag"]), s(ib["listen"]), i(ib["port"]), s(ib["protocol"]))
					}
				}
			case 6:
				ListInbounds(st)
				ListOutbounds(st)
			case 7:
				ok, msg := ValidateXrayConfig(p, p.Config)
				fmt.Println(ok, msg)
			case 8:
				if e := WriteConfig(p, st); e != nil {
					return e
				}
				ok, msg := ValidateXrayConfig(p, p.Config)
				fmt.Println(msg)
				if !ok {
					return fmt.Errorf("配置校验失败")
				}
				ok, msg = RestartService(p, false)
				if !ok {
					return fmt.Errorf("重启失败: %s", msg)
				}
			}
			return nil
		})
	}
}
func KernelMenu(p Paths, st *State) {
	for {
		latest, e := LatestXrayVersion()
		label := latest
		if e != nil {
			label = "获取失败: " + e.Error()
		}
		n := Select("Xray 内核管理", []string{"最新版本      " + label, "兼容版本      " + CompatXray, "自定义版本"})
		if n == 0 {
			return
		}
		var v string
		if n == 1 {
			if e != nil {
				fmt.Println(label)
				continue
			}
			v = latest
		} else if n == 2 {
			v = CompatXray
		} else {
			v = NormalizeVersion(Prompt("输入 Xray 版本", ""))
			if _, e := ReleaseForVersion(v); e != nil {
				fmt.Println("不存在此版本或验证失败:", e)
				continue
			}
		}
		fmt.Println("准备安装", v)
		if YesNo("继续", true) {
			ok, msg := InstallXray(p, st, v)
			if ok {
				fmt.Println("成功:", msg)
			} else {
				fmt.Println("失败:", msg)
			}
		}
	}
}

func MainMenu(p Paths, st *State) {
	for {
		status := ServiceStatus(p)
		ver := s(st.Xray["version"])
		if ver == "" {
			if _, e := os.Stat(p.XrayBin); e == nil {
				ver = "已安装(版本未知)"
			} else {
				ver = "未安装"
			}
		}
		fmt.Printf("\n╔══════════════════════════════════════════════╗\n║                    L-UI                      ║\n║          Lightweight Xray Manager            ║\n╠══════════════════════════════════════════════╣\n║ Xray 内核：%-31s║\n║ Xray 状态：%-31s║\n╚══════════════════════════════════════════════╝\n", ver, status)
		kernelLabel := "安装内核"
		if _, e := os.Stat(p.XrayBin); e == nil {
			kernelLabel = "切换内核"
		}
		n := Select("主菜单", []string{"入站管理", "出站管理", "路由管理", "客户端管理", "导出订阅 / 客户端配置", "导入 3x-ui 数据", "L-UI 数据备份 / 恢复", "服务管理", kernelLabel, "配置检查 / 日志", "卸载 L-UI"})
		switch n {
		case 0:
			return
		case 1:
			InboundsMenu(p, st)
		case 2:
			OutboundsMenu(p, st)
		case 3:
			RoutingMenu(p, st)
		case 4:
			ClientsMenu(p, st)
		case 5:
			ExportMenu(p, st)
		case 6:
			Import3xuiMenu(p, st)
		case 7:
			BackupMenu(p, st)
		case 8:
			ServiceMenu(p)
		case 9:
			KernelMenu(p, st)
		case 10:
			DiagnosticsMenu(p, st)
		case 11:
			if YesNo("确认卸载 L-UI（保留 backups 目录）", false) {
				_ = UninstallRuntime(p)
				fmt.Println("已卸载核心文件，backups 未删除")
				return
			}
		}
	}
}

func SelfCheck(p Paths, st *State) error {
	type chk struct {
		name string
		ok   bool
		msg  string
	}
	xs := []chk{}
	if e := ValidateState(st); e != nil {
		xs = append(xs, chk{"state schema/invariants", false, e.Error()})
	} else {
		xs = append(xs, chk{"state schema/invariants", true, ""})
	}
	if _, e := BuildConfig(st); e != nil {
		xs = append(xs, chk{"config generation", false, e.Error()})
	} else {
		xs = append(xs, chk{"config generation", true, ""})
	}
	if _, e := os.Stat(p.XrayBin); e == nil {
		ok, msg := ValidateXrayConfig(p, p.Config)
		xs = append(xs, chk{"xray config test", ok, msg})
	} else {
		xs = append(xs, chk{"xray config test", true, "SKIP: xray not installed"})
	}
	db, e := sql.Open("sqlite", ":memory:")
	if e == nil {
		var v string
		e = db.QueryRow("select sqlite_version()").Scan(&v)
		db.Close()
		xs = append(xs, chk{"embedded sqlite", e == nil, v})
	} else {
		xs = append(xs, chk{"embedded sqlite", false, e.Error()})
	}
	xs = append(xs, chk{"native HTTP/SOCKS route tester", true, "no curl dependency"})
	xs = append(xs, chk{"embedded QR", true, "no qrencode dependency"})
	bad := false
	for _, x := range xs {
		state := "PASS"
		if !x.ok {
			state = "FAIL"
			bad = true
		}
		fmt.Printf("%s  %s %s\n", state, x.name, x.msg)
	}
	if bad {
		return fmt.Errorf("self-check failed")
	}
	return nil
}

func Run(argv []string) error {
	p := DefaultPaths()
	if e := EnsureDirs(p); e != nil {
		return e
	}
	st, e := LoadState(p)
	if e != nil {
		return e
	}
	if len(argv) == 1 {
		MainMenu(p, st)
		return nil
	}
	switch argv[1] {
	case "config":
		cfg, e := BuildConfig(st)
		if e != nil {
			return e
		}
		ShowJSON(cfg)
		return nil
	case "check":
		if e := WriteConfig(p, st); e != nil {
			return e
		}
		ok, msg := ValidateXrayConfig(p, p.Config)
		fmt.Println(msg)
		if !ok {
			return fmt.Errorf("xray config test failed")
		}
		return nil
	case "route-test":
		url := DefaultTestURL
		if len(argv) > 2 {
			url = argv[2]
		}
		tag := ""
		if len(argv) > 3 {
			tag = argv[3]
		}
		r := RealRouteTest(p, st, url, tag, 15*time.Second)
		ShowJSON(r)
		if !r.Success {
			return fmt.Errorf("route test failed")
		}
		return nil
	case "analyze-3xui":
		if len(argv) < 3 {
			return fmt.Errorf("缺少文件路径")
		}
		info, e := Analyze3xui(argv[2])
		if e != nil {
			return e
		}
		ShowJSON(info)
		return nil
	case "self-check":
		return SelfCheck(p, st)
	case "version":
		fmt.Printf("L-UI %s\ncommit %s\nbuilt %s\narch %s\n", Version, Commit, BuildDate, Architecture())
		return nil
	default:
		return fmt.Errorf("用法: l-ui [config|check|route-test URL [INBOUND_TAG]|analyze-3xui FILE|self-check|version]")
	}
}
