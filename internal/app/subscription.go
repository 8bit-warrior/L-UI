package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func decodeB64Text(v string) (string, error) {
	v = strings.TrimSpace(v)
	v = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, v)
	for len(v)%4 != 0 {
		v += "="
	}
	d, e := base64.URLEncoding.DecodeString(v)
	if e != nil {
		d, e = base64.StdEncoding.DecodeString(v)
	}
	return string(d), e
}
func one(q url.Values, k, d string) string {
	if v := q.Get(k); v != "" {
		return v
	}
	return d
}
func streamFromLink(q url.Values, defaultSecurity string) map[string]any {
	netw := one(q, "type", "tcp")
	if !contains([]string{"tcp", "kcp", "ws", "grpc", "httpupgrade", "xhttp"}, netw) {
		netw = "tcp"
	}
	sec := one(q, "security", defaultSecurity)
	if !contains(Securities, sec) {
		sec = defaultSecurity
	}
	st := map[string]any{"network": netw, "security": sec}
	host := q.Get("host")
	path := one(q, "path", "/")
	switch netw {
	case "tcp":
		st["tcpSettings"] = map[string]any{"header": map[string]any{"type": "none"}}
	case "kcp":
		st["kcpSettings"] = map[string]any{}
	case "ws":
		st["wsSettings"] = map[string]any{"path": path, "host": host, "headers": map[string]any{}}
	case "grpc":
		svc := q.Get("serviceName")
		if svc == "" && path != "/" {
			svc = path
		}
		st["grpcSettings"] = map[string]any{"serviceName": svc, "authority": q.Get("authority"), "multiMode": q.Get("mode") == "multi"}
	case "httpupgrade":
		st["httpupgradeSettings"] = map[string]any{"path": path, "host": host, "headers": map[string]any{}}
	case "xhttp":
		st["xhttpSettings"] = map[string]any{"path": path, "host": host, "mode": one(q, "mode", "auto"), "headers": map[string]any{}}
	}
	if sec == "tls" {
		tlsm := map[string]any{"serverName": q.Get("sni"), "fingerprint": q.Get("fp")}
		if alpn := q.Get("alpn"); alpn != "" {
			tlsm["alpn"] = strings.Split(alpn, ",")
		}
		st["tlsSettings"] = tlsm
	} else if sec == "reality" {
		st["realitySettings"] = map[string]any{"serverName": q.Get("sni"), "fingerprint": one(q, "fp", "chrome"), "publicKey": q.Get("pbk"), "shortId": q.Get("sid"), "spiderX": q.Get("spx")}
	}
	return st
}

func ParseOutboundLink(link string) (map[string]any, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, errors.New("empty link")
	}
	if strings.HasPrefix(link, "vmess://") {
		txt, e := decodeB64Text(strings.TrimPrefix(link, "vmess://"))
		if e != nil {
			return nil, e
		}
		var o map[string]any
		if e = json.Unmarshal([]byte(txt), &o); e != nil {
			return nil, e
		}
		q := url.Values{}
		q.Set("type", firstNonEmpty(s(o["net"]), "tcp"))
		if s(o["tls"]) == "tls" {
			q.Set("security", "tls")
		} else {
			q.Set("security", "none")
		}
		for _, k := range []string{"host", "path", "sni", "fp", "alpn", "authority"} {
			if s(o[k]) != "" {
				q.Set(k, s(o[k]))
			}
		}
		port, _ := strconv.Atoi(s(o["port"]))
		if port == 0 {
			port = 443
		}
		return map[string]any{"protocol": "vmess", "tag": s(o["ps"]), "settings": map[string]any{"vnext": []any{map[string]any{"address": s(o["add"]), "port": port, "users": []any{map[string]any{"id": s(o["id"]), "security": firstNonEmpty(s(o["scy"]), "auto")}}}}}, "streamSettings": streamFromLink(q, "none")}, nil
	}
	u, e := url.Parse(link)
	if e != nil {
		return nil, e
	}
	host := u.Hostname()
	port := 443
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	}
	frag, _ := url.PathUnescape(u.Fragment)
	switch u.Scheme {
	case "vless":
		if host == "" {
			return nil, errors.New("missing host")
		}
		id := ""
		if u.User != nil {
			id = u.User.Username()
		}
		q := u.Query()
		return map[string]any{"protocol": "vless", "tag": frag, "settings": map[string]any{"address": host, "port": port, "id": id, "flow": q.Get("flow"), "encryption": one(q, "encryption", "none")}, "streamSettings": streamFromLink(q, "none")}, nil
	case "trojan":
		if host == "" {
			return nil, errors.New("missing host")
		}
		pw := ""
		if u.User != nil {
			pw = u.User.Username()
		}
		return map[string]any{"protocol": "trojan", "tag": frag, "settings": map[string]any{"servers": []any{map[string]any{"address": host, "port": port, "password": pw}}}, "streamSettings": streamFromLink(u.Query(), "tls")}, nil
	case "hysteria2", "hy2":
		if host == "" {
			return nil, errors.New("missing host")
		}
		auth := ""
		if u.User != nil {
			auth = u.User.Username()
		}
		q := u.Query()
		return map[string]any{"protocol": "hysteria", "tag": frag, "settings": map[string]any{"address": host, "port": port, "version": 2}, "streamSettings": map[string]any{"network": "hysteria", "security": "tls", "hysteriaSettings": map[string]any{"version": 2, "auth": auth, "udpIdleTimeout": 60}, "tlsSettings": map[string]any{"serverName": q.Get("sni"), "fingerprint": q.Get("fp"), "alpn": strings.Split(one(q, "alpn", "h3"), ",")}}}, nil
	case "wireguard", "wg":
		if host == "" {
			return nil, errors.New("missing host")
		}
		q := u.Query()
		q1 := func(keys ...string) string {
			for _, k := range keys {
				if v := q.Get(k); v != "" {
					return v
				}
			}
			return ""
		}
		allowed := strSlice(firstNonEmpty(q1("allowedips", "allowed_ips"), "0.0.0.0/0,::/0"))
		peer := map[string]any{"publicKey": q1("publickey", "publicKey", "public_key", "peerPublicKey"), "endpoint": net.JoinHostPort(host, strconv.Itoa(port)), "allowedIPs": allowed}
		if psk := q1("presharedkey", "preshared_key", "pre-shared-key", "psk"); psk != "" {
			peer["preSharedKey"] = psk
		}
		secret := ""
		if u.User != nil {
			secret = u.User.Username()
		}
		return map[string]any{"protocol": "wireguard", "tag": frag, "settings": map[string]any{"secretKey": secret, "address": strSlice(q1("address", "ip")), "peers": []any{peer}}}, nil
	case "ss":
		return parseSS(link)
	}
	return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
}

func parseSS(link string) (map[string]any, error) {
	raw := strings.TrimPrefix(link, "ss://")
	frag := ""
	if p := strings.Index(raw, "#"); p >= 0 {
		frag, _ = url.PathUnescape(raw[p+1:])
		raw = raw[:p]
	}
	if p := strings.Index(raw, "?"); p >= 0 {
		raw = raw[:p]
	}
	auth, hp := "", ""
	if p := strings.LastIndex(raw, "@"); p >= 0 {
		auth = raw[:p]
		hp = raw[p+1:]
		if !strings.Contains(auth, ":") {
			if x, e := decodeB64Text(auth); e == nil {
				auth = x
			}
		}
	} else {
		x, e := decodeB64Text(raw)
		if e != nil {
			return nil, e
		}
		p := strings.LastIndex(x, "@")
		if p < 0 {
			return nil, errors.New("invalid ss link")
		}
		auth = x[:p]
		hp = x[p+1:]
	}
	method, password, ok := strings.Cut(auth, ":")
	if !ok {
		password = method
		method = "2022-blake3-aes-128-gcm"
	}
	u, e := url.Parse("ss://x@" + hp)
	if e != nil || u.Hostname() == "" {
		return nil, errors.New("invalid ss host")
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}
	return map[string]any{"protocol": "shadowsocks", "tag": frag, "settings": map[string]any{"servers": []any{map[string]any{"address": u.Hostname(), "port": port, "password": password, "method": method, "uot": false, "UoTVersion": 1}}}}, nil
}

func DecodeSubscriptionBody(raw []byte) ([]string, []map[string]any, error) {
	if len(raw) > 8<<20 {
		return nil, nil, errors.New("订阅响应超过 8 MiB 限制")
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, nil, nil
	}
	var obj any
	if json.Unmarshal([]byte(text), &obj) == nil {
		switch x := obj.(type) {
		case []any:
			arr := []map[string]any{}
			for _, v := range x {
				if mm, ok := v.(map[string]any); ok {
					arr = append(arr, mm)
				}
			}
			if len(arr) == len(x) {
				return nil, arr, nil
			}
		case map[string]any:
			if xs, ok := x["outbounds"].([]any); ok {
				arr := []map[string]any{}
				for _, v := range xs {
					if mm, ok := v.(map[string]any); ok {
						arr = append(arr, mm)
					}
				}
				return nil, arr, nil
			}
		}
	}
	schemes := []string{"vmess://", "vless://", "trojan://", "ss://", "hysteria2://", "hy2://", "wireguard://", "wg://"}
	has := func(t string) bool {
		for _, sc := range schemes {
			if strings.Contains(t, sc) {
				return true
			}
		}
		return false
	}
	if !has(text) {
		if d, e := decodeB64Text(text); e == nil && has(d) {
			text = d
		}
	}
	lines := []string{}
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, "#") {
			lines = append(lines, ln)
		}
	}
	return lines, nil, nil
}

func slugTag(v, fallback string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.TrimSpace(v) {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-'
		if ok {
			sb.WriteRune(r)
			dash = false
		} else if !dash {
			sb.WriteByte('-')
			dash = true
		}
	}
	x := strings.Trim(sb.String(), "-.")
	if x == "" {
		x = fallback
	}
	if len(x) > 64 {
		x = x[:64]
	}
	return x
}
func subscriptionIdentity(link string) string {
	base := strings.SplitN(link, "#", 2)[0]
	h := sha256.Sum256([]byte(base))
	return hex.EncodeToString(h[:])
}
func ParseSubscriptionData(raw []byte, prefix string, previous map[string]string) ([]map[string]any, map[string]string, []string) {
	lines, objs, err := DecodeSubscriptionBody(raw)
	if err != nil {
		return nil, nil, []string{err.Error()}
	}
	used := map[string]bool{}
	out := []map[string]any{}
	ids := map[string]string{}
	skip := []string{}
	unique := func(base string) string {
		x := base
		for n := 2; used[x]; n++ {
			x = fmt.Sprintf("%s-%d", base, n)
		}
		used[x] = true
		return x
	}
	if objs != nil {
		for idx, ob0 := range objs {
			ob := DeepCopy(ob0)
			proto := s(ob["protocol"])
			if !contains(OutboundProtocols, proto) {
				skip = append(skip, fmt.Sprintf("JSON #%d: 不支持的出站", idx+1))
				continue
			}
			tag := unique(slugTag(s(ob["tag"]), fmt.Sprintf("%s%d", prefix, idx+1)))
			ob["tag"] = tag
			out = append(out, ob)
		}
		return out, ids, skip
	}
	for idx, line := range lines {
		ob, e := ParseOutboundLink(line)
		if e != nil {
			skip = append(skip, fmt.Sprintf("#%d: 无法解析 %s", idx+1, short(line, 80)))
			continue
		}
		id := subscriptionIdentity(line)
		candidate := previous[id]
		if candidate == "" {
			candidate = prefix + slugTag(s(ob["tag"]), strconv.Itoa(idx+1))
		}
		candidate = unique(candidate)
		ob["tag"] = candidate
		ids[id] = candidate
		out = append(out, ob)
	}
	return out, ids, skip
}
func short(x string, n int) string {
	if len(x) > n {
		return x[:n]
	}
	return x
}

func checkPublicHost(ctx context.Context, host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("订阅域名解析失败: %w", err)
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("订阅 URL 指向非公网地址 %s；如确需访问请显式启用 allowPrivate", ip)
		}
	}
	return nil
}
func FetchSubscriptionURL(rawURL string, allowPrivate, allowInsecure bool) ([]byte, error) {
	u, e := url.Parse(rawURL)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("订阅 URL 必须是 http:// 或 https://")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e = checkPublicHost(ctx, u.Hostname(), allowPrivate); e != nil {
		return nil, e
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{InsecureSkipVerify: allowInsecure}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		h, _, e := net.SplitHostPort(address)
		if e == nil {
			if e = checkPublicHost(ctx, h, allowPrivate); e != nil {
				return nil, e
			}
		}
		return dialer.DialContext(ctx, network, address)
	}}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("redirect scheme not allowed")
		}
		return checkPublicHost(req.Context(), req.URL.Hostname(), allowPrivate)
	}}
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "L-UI-outbound-sub/"+Version)
	resp, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("订阅 HTTP 状态 %d", resp.StatusCode)
	}
	r := io.LimitReader(resp.Body, (8<<20)+1)
	data, e := io.ReadAll(r)
	if e != nil {
		return nil, e
	}
	if len(data) > 8<<20 {
		return nil, errors.New("订阅响应超过 8 MiB 限制")
	}
	return data, nil
}

func ActiveSubscriptionOutbounds(st *State, prepend *bool) []map[string]any {
	subs := append([]map[string]any{}, st.OutboundSubscriptions...)
	sort.SliceStable(subs, func(x, y int) bool {
		pi, pj := i(subs[x]["priority"]), i(subs[y]["priority"])
		if pi == pj {
			return i(subs[x]["id"]) < i(subs[y]["id"])
		}
		return pi < pj
	})
	out := []map[string]any{}
	for _, sub := range subs {
		if !b(sub["enabled"], true) {
			continue
		}
		if prepend != nil && b(sub["prepend"], false) != *prepend {
			continue
		}
		for _, v := range a(sub["last_outbounds"]) {
			if ob, ok := v.(map[string]any); ok {
				out = append(out, DeepCopy(ob))
			}
		}
	}
	return out
}
func RefreshSubscription(sub map[string]any) (int, []string, error) {
	if !b(sub["enabled"], true) {
		sub["last_outbounds"] = []any{}
		return 0, nil, nil
	}
	raw, e := FetchSubscriptionURL(s(sub["url"]), b(sub["allow_private"], false), b(sub["allow_insecure"], false))
	if e != nil {
		sub["last_error"] = e.Error()
		return 0, nil, e
	}
	prev := map[string]string{}
	for k, v := range m(sub["identity_tags"]) {
		prev[k] = s(v)
	}
	prefix := s(sub["tag_prefix"])
	if prefix == "" {
		prefix = fmt.Sprintf("sub%d-", i(sub["id"]))
	}
	obs, ids, skip := ParseSubscriptionData(raw, prefix, prev)
	arr := make([]any, len(obs))
	for idx := range obs {
		arr[idx] = obs[idx]
	}
	idm := map[string]any{}
	for k, v := range ids {
		idm[k] = v
	}
	sub["last_outbounds"] = arr
	sub["identity_tags"] = idm
	sub["last_updated"] = time.Now().Unix()
	sub["last_error"] = ""
	return len(obs), skip, nil
}
