package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

func shareHost(ib map[string]any) string {
	meta := m(ib["_lui"])
	if x := s(meta["share_host"]); x != "" {
		return x
	}
	listen := s(ib["listen"])
	if listen != "" && listen != "0.0.0.0" && listen != "::" && listen != "::0" {
		return listen
	}
	return Prompt("分享地址(域名/IP)", "127.0.0.1")
}

func ShareLink(ib, c map[string]any) (string, error) {
	proto := s(ib["protocol"])
	host := shareHost(ib)
	port := i(ib["port"])
	meta := m(ib["_lui"])
	remark := firstNonEmpty(s(meta["remark"]), s(ib["tag"]), "L-UI")
	stream := m(ib["streamSettings"])
	network := firstNonEmpty(s(stream["network"]), "tcp")
	security := firstNonEmpty(s(stream["security"]), "none")
	fragment := url.QueryEscape(remark)
	fragment = strings.ReplaceAll(fragment, "+", "%20")
	switch proto {
	case "vless":
		q := url.Values{}
		q.Set("encryption", "none")
		q.Set("type", network)
		q.Set("security", security)
		if s(c["flow"]) != "" {
			q.Set("flow", s(c["flow"]))
		}
		if security == "reality" {
			rs := m(stream["realitySettings"])
			names := strSlice(rs["serverNames"])
			sni := s(rs["serverName"])
			if len(names) > 0 {
				sni = names[0]
			}
			ids := strSlice(rs["shortIds"])
			sid := s(rs["shortId"])
			if len(ids) > 0 {
				sid = ids[0]
			}
			q.Set("sni", sni)
			q.Set("pbk", s(rs["publicKey"]))
			q.Set("sid", sid)
			q.Set("fp", "chrome")
		} else if security == "tls" {
			q.Set("sni", s(m(stream["tlsSettings"])["serverName"]))
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", url.PathEscape(s(c["uuid"])), BracketHost(host), port, q.Encode(), fragment), nil
	case "trojan":
		q := url.Values{}
		q.Set("type", network)
		q.Set("security", security)
		if security == "tls" {
			q.Set("sni", s(m(stream["tlsSettings"])["serverName"]))
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", url.PathEscape(s(c["password"])), BracketHost(host), port, q.Encode(), fragment), nil
	case "vmess":
		vm := map[string]any{"v": "2", "ps": remark, "add": host, "port": strconv.Itoa(port), "id": s(c["uuid"]), "aid": "0", "scy": firstNonEmpty(s(c["security"]), "auto"), "net": network, "type": "none", "host": "", "path": "", "tls": func() string {
			if security == "tls" {
				return "tls"
			}
			return ""
		}(), "sni": s(m(stream["tlsSettings"])["serverName"])}
		raw, _ := json.Marshal(vm)
		return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
	case "shadowsocks":
		method := firstNonEmpty(s(m(ib["settings"])["method"]), "2022-blake3-aes-128-gcm")
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + s(c["password"])))
		return fmt.Sprintf("ss://%s@%s:%d#%s", userinfo, BracketHost(host), port, fragment), nil
	}
	return "", fmt.Errorf("暂不支持 %s 的标准分享 URI；可导出 Xray JSON", proto)
}
func ExportClientLinks(st *State, c map[string]any) []string {
	out := []string{}
	for _, tag := range strSlice(c["inbound_tags"]) {
		_, ib, e := FindByTag(st.Inbounds, tag)
		if e == nil {
			if link, e := ShareLink(ib, c); e == nil {
				out = append(out, link)
			}
		}
	}
	return out
}
func SubscriptionBase64(links []string) string {
	if len(links) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n") + "\n"))
}
func PrintQRCode(text string) error {
	qr, e := qrcode.New(text, qrcode.Medium)
	if e != nil {
		return e
	}
	bmp := qr.Bitmap()
	for y := 0; y < len(bmp); y += 2 {
		var sb strings.Builder
		for x := 0; x < len(bmp[y]); x++ {
			top := bmp[y][x]
			bot := false
			if y+1 < len(bmp) {
				bot = bmp[y+1][x]
			}
			switch {
			case top && bot:
				sb.WriteRune('█')
			case top && !bot:
				sb.WriteRune('▀')
			case !top && bot:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		fmt.Println(sb.String())
	}
	return nil
}
