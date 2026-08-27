package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var stdin = bufio.NewReader(os.Stdin)

func Prompt(text, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", text, def)
	} else {
		fmt.Printf("%s: ", text)
	}
	v, _ := stdin.ReadString('\n')
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}
func PromptInt(text string, def, lo, hi int) int {
	for {
		raw := Prompt(text, strconv.Itoa(def))
		n, e := strconv.Atoi(raw)
		if e == nil && n >= lo && (hi <= 0 || n <= hi) {
			return n
		}
		if hi > 0 {
			fmt.Printf("请输入 %d..%d 的整数\n", lo, hi)
		} else {
			fmt.Printf("请输入 >= %d 的整数\n", lo)
		}
	}
}
func YesNo(text string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", text, hint)
	x, _ := stdin.ReadString('\n')
	x = strings.ToLower(strings.TrimSpace(x))
	if x == "" {
		return def
	}
	return x == "y" || x == "yes" || x == "1" || x == "是"
}
func Select(title string, items []string) int {
	for {
		fmt.Printf("\n============== %s ==============\n", title)
		for idx, x := range items {
			fmt.Printf("%2d. %s\n", idx+1, x)
		}
		fmt.Println(" 0. 返回")
		raw := Prompt("请输入选项", "")
		n, e := strconv.Atoi(raw)
		if e == nil && n >= 0 && n <= len(items) {
			return n
		}
		fmt.Println("无效选项")
	}
}
func ShowJSON(v any) { d, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(d)) }
func EditJSONValue(label string, current any) any {
	fmt.Printf("当前 %s:\n", label)
	ShowJSON(current)
	fmt.Println("输入单行 JSON；直接回车保持不变。")
	raw := Prompt("新 "+label, "")
	if raw == "" {
		return current
	}
	var out any
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		panic(e)
	}
	return out
}
func safeEditJSON(label string, current any) (any, error) {
	fmt.Printf("当前 %s:\n", label)
	ShowJSON(current)
	fmt.Println("输入单行 JSON；直接回车保持不变。")
	raw := Prompt("新 "+label, "")
	if raw == "" {
		return current, nil
	}
	var out any
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return nil, e
	}
	return out, nil
}

func MakeInbound(proto string, st *State) (map[string]any, error) {
	tag := UniqueTag(st, Prompt("Tag", "in-"+proto), "inbound")
	listen := Prompt("监听地址", "0.0.0.0")
	port := PromptInt("端口", 443, 1, 65535)
	settings := map[string]any{}
	var stream map[string]any
	sniff := map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic", "fakedns"}, "routeOnly": false}
	switch proto {
	case "vless":
		settings = map[string]any{"decryption": "none", "clients": []any{}}
	case "vmess", "trojan":
		settings = map[string]any{"clients": []any{}}
	case "shadowsocks":
		settings = map[string]any{"method": Prompt("Shadowsocks 加密", "2022-blake3-aes-128-gcm"), "clients": []any{}}
	case "hysteria":
		settings = map[string]any{"version": 2, "clients": []any{}}
	case "http", "mixed":
		settings = map[string]any{"accounts": []any{}}
	case "tunnel":
		settings = map[string]any{"address": Prompt("转发目标地址", "127.0.0.1"), "port": PromptInt("转发目标端口", 80, 1, 65535), "network": Prompt("网络(tcp/udp/tcp,udp)", "tcp,udp")}
	case "wireguard", "amneziawg":
		fmt.Println("WireGuard/AmneziaWG 参数较多，请输入 settings JSON。")
		v, e := safeEditJSON("settings", map[string]any{})
		if e != nil {
			return nil, e
		}
		settings = m(v)
	}
	if proto != "wireguard" && proto != "tunnel" && proto != "amneziawg" {
		netw := "hysteria"
		if proto != "hysteria" {
			netw = Prompt("传输方式 tcp/kcp/ws/grpc/httpupgrade/xhttp", "tcp")
		}
		if !contains(Networks, netw) {
			return nil, fmt.Errorf("不支持的传输: %s", netw)
		}
		sec := "tls"
		if proto != "hysteria" {
			sec = Prompt("安全 none/tls/reality", "none")
		}
		if !contains(Securities, sec) {
			return nil, fmt.Errorf("不支持的安全类型: %s", sec)
		}
		stream = map[string]any{"network": netw, "security": sec}
		if sec == "tls" {
			stream["tlsSettings"] = map[string]any{"serverName": Prompt("TLS SNI", "")}
		} else if sec == "reality" {
			v, e := safeEditJSON("REALITY settings", map[string]any{"dest": "www.cloudflare.com:443", "serverNames": []string{"www.cloudflare.com"}, "privateKey": "", "shortIds": []string{""}})
			if e != nil {
				return nil, e
			}
			stream["realitySettings"] = v
		}
		if netw != "tcp" && YesNo("配置传输详细参数", false) {
			key := map[string]string{"kcp": "kcpSettings", "ws": "wsSettings", "grpc": "grpcSettings", "httpupgrade": "httpupgradeSettings", "xhttp": "xhttpSettings", "hysteria": "hysteriaSettings"}[netw]
			def := map[string]any{}
			if netw == "hysteria" {
				def["version"] = 2
			}
			v, e := safeEditJSON(key, def)
			if e != nil {
				return nil, e
			}
			stream[key] = v
		}
	}
	ib := map[string]any{"listen": listen, "port": port, "protocol": proto, "settings": settings, "tag": tag, "sniffing": sniff, "_lui": map[string]any{"enable": true, "remark": Prompt("备注", tag)}}
	if stream != nil {
		ib["streamSettings"] = stream
	}
	return ib, nil
}

func outboundServerSettings(proto string) map[string]any {
	address := Prompt("服务器地址", "127.0.0.1")
	port := PromptInt("服务器端口", 443, 1, 65535)
	switch proto {
	case "vless":
		return map[string]any{"address": address, "port": port, "id": Prompt("UUID", UUID()), "flow": Prompt("Flow", ""), "encryption": "none"}
	case "vmess":
		return map[string]any{"vnext": []any{map[string]any{"address": address, "port": port, "users": []any{map[string]any{"id": Prompt("UUID", UUID()), "security": Prompt("Security", "auto")}}}}}
	case "trojan":
		return map[string]any{"servers": []any{map[string]any{"address": address, "port": port, "password": Prompt("Password", UUID())}}}
	case "shadowsocks":
		return map[string]any{"servers": []any{map[string]any{"address": address, "port": port, "method": Prompt("Method", "2022-blake3-aes-128-gcm"), "password": Prompt("Password", ""), "uot": false, "UoTVersion": 1}}}
	case "socks", "http":
		server := map[string]any{"address": address, "port": port}
		user := Prompt("用户名(可空)", "")
		if user != "" {
			server["users"] = []any{map[string]any{"user": user, "pass": Prompt("密码(可空)", "")}}
		}
		return map[string]any{"servers": []any{server}}
	case "hysteria":
		return map[string]any{"address": address, "port": port, "version": 2}
	}
	return map[string]any{}
}
func MakeOutbound(proto string, st *State) (map[string]any, error) {
	tag := UniqueTag(st, Prompt("Tag", proto), "outbound")
	ob := map[string]any{"tag": tag, "protocol": proto, "settings": map[string]any{}}
	switch proto {
	case "freedom":
		ob["settings"] = map[string]any{"domainStrategy": Prompt("DomainStrategy", "AsIs")}
	case "blackhole":
		typ := Prompt("响应类型(空/none/http)", "")
		if typ != "" {
			ob["settings"] = map[string]any{"response": map[string]any{"type": typ}}
		}
	case "dns":
		v, e := safeEditJSON("DNS settings", map[string]any{})
		if e != nil {
			return nil, e
		}
		ob["settings"] = v
	case "vmess", "vless", "trojan", "shadowsocks", "socks", "http", "hysteria":
		ob["settings"] = outboundServerSettings(proto)
		if proto == "hysteria" {
			ob["streamSettings"] = map[string]any{"network": "hysteria", "security": "tls", "hysteriaSettings": map[string]any{"version": 2}}
		} else if contains([]string{"vmess", "vless", "trojan", "shadowsocks"}, proto) && YesNo("配置 streamSettings", false) {
			v, e := safeEditJSON("streamSettings", map[string]any{"network": "tcp", "security": "none"})
			if e != nil {
				return nil, e
			}
			ob["streamSettings"] = v
		} else if contains([]string{"socks", "http"}, proto) && YesNo("配置 Sockopt", false) {
			v, e := safeEditJSON("sockopt", map[string]any{})
			if e != nil {
				return nil, e
			}
			ob["streamSettings"] = map[string]any{"sockopt": v}
		}
	case "wireguard":
		v, e := safeEditJSON("WireGuard settings", map[string]any{"address": []string{"172.16.0.2/32"}, "peers": []any{}})
		if e != nil {
			return nil, e
		}
		ob["settings"] = v
	case "loopback":
		ob["settings"] = map[string]any{"inboundTag": Prompt("目标 Inbound Tag", "")}
	}
	if YesNo("使用高级 JSON 覆盖/补充整个出站", false) {
		v, e := safeEditJSON("outbound", ob)
		if e != nil {
			return nil, e
		}
		mm, ok := v.(map[string]any)
		if !ok || !contains(OutboundProtocols, s(mm["protocol"])) {
			return nil, fmt.Errorf("高级 JSON 中 protocol 不受支持")
		}
		ob = mm
	}
	return ob, nil
}

func ListInbounds(st *State) {
	if len(st.Inbounds) == 0 {
		fmt.Println("暂无入站")
		return
	}
	fmt.Println("\nTag\t协议\t监听\t端口\t状态\t备注")
	for _, ib := range st.Inbounds {
		meta := m(ib["_lui"])
		state := "启用"
		if !b(meta["enable"], true) {
			state = "停用"
		}
		fmt.Printf("%s\t%s\t%s\t%d\t%s\t%s\n", s(ib["tag"]), s(ib["protocol"]), s(ib["listen"]), i(ib["port"]), state, s(meta["remark"]))
	}
}
func GetDefaultOutbound(st *State) string {
	if len(st.Outbounds) > 0 && s(st.Outbounds[0]["tag"]) != "" {
		return s(st.Outbounds[0]["tag"])
	}
	return "direct"
}
func SetDefaultOutbound(st *State, tag string) error {
	idx, _, e := FindByTag(st.Outbounds, tag)
	if e != nil {
		return e
	}
	if idx > 0 {
		x := st.Outbounds[idx]
		st.Outbounds = append(st.Outbounds[:idx], st.Outbounds[idx+1:]...)
		st.Outbounds = append([]map[string]any{x}, st.Outbounds...)
	}
	return nil
}
func ListOutbounds(st *State) {
	def := GetDefaultOutbound(st)
	fmt.Println("\n#\tTag\t协议\t默认")
	for idx, ob := range st.Outbounds {
		mark := ""
		if s(ob["tag"]) == def {
			mark = "*"
		}
		fmt.Printf("%d\t%s\t%s\t%s\n", idx+1, s(ob["tag"]), s(ob["protocol"]), mark)
	}
}

func CreateClient(st *State) (map[string]any, error) {
	email := Prompt("客户端名称/Email", "")
	if email == "" {
		return nil, fmt.Errorf("Email 不能为空")
	}
	for _, c := range st.Clients {
		if s(c["email"]) == email {
			return nil, fmt.Errorf("客户端已存在")
		}
	}
	avail := []string{}
	for _, ib := range st.Inbounds {
		avail = append(avail, s(ib["tag"]))
	}
	fmt.Println("可绑定入站:", strings.Join(avail, ", "))
	tags := strSlice(Prompt("绑定入站 Tag(逗号分隔)", ""))
	for _, t := range tags {
		if !contains(avail, t) {
			return nil, fmt.Errorf("不存在的入站: %s", t)
		}
	}
	cid := i(st.Meta["next_client_id"])
	if cid < 1 {
		cid = 1
	}
	st.Meta["next_client_id"] = cid + 1
	return map[string]any{"id": cid, "email": email, "sub_id": randHex(16), "uuid": UUID(), "password": randHex(32), "auth": randHex(32), "flow": Prompt("Flow", ""), "security": "auto", "enable": true, "group": Prompt("分组", ""), "comment": Prompt("备注", ""), "total_gb": 0, "expiry_time": 0, "inbound_tags": tags, "created_at": timeNowMillis()}, nil
}
func timeNowMillis() int64 { return timeNow().UnixMilli() }

var timeNow = func() time.Time { return time.Now() }

func ListClients(st *State) {
	if len(st.Clients) == 0 {
		fmt.Println("暂无客户端")
		return
	}
	fmt.Println("\nID\tEmail\t状态\t分组\t入站")
	for _, c := range st.Clients {
		state := "启用"
		if !b(c["enable"], true) {
			state = "停用"
		}
		fmt.Printf("%d\t%s\t%s\t%s\t%s\n", i(c["id"]), s(c["email"]), state, s(c["group"]), strings.Join(strSlice(c["inbound_tags"]), ","))
	}
}
func FindClient(st *State, token string) (int, map[string]any, error) {
	for idx, c := range st.Clients {
		if strconv.Itoa(i(c["id"])) == token || s(c["email"]) == token {
			return idx, c, nil
		}
	}
	return -1, nil, fmt.Errorf("找不到客户端: %s", token)
}
