package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var Version = "dev"
var Commit = "unknown"
var BuildDate = "unknown"

const (
	SchemaVersion  = 1
	CompatXray     = "v26.6.27"
	XrayRepo       = "XTLS/Xray-core"
	DefaultTestURL = "https://www.google.com/generate_204"
)

var OutboundProtocols = []string{"freedom", "blackhole", "dns", "vmess", "vless", "trojan", "shadowsocks", "wireguard", "hysteria", "socks", "http", "loopback"}
var InboundProtocols = []string{"vless", "vmess", "trojan", "shadowsocks", "wireguard", "hysteria", "http", "mixed", "tunnel", "amneziawg"}
var Networks = []string{"tcp", "kcp", "ws", "grpc", "httpupgrade", "xhttp", "hysteria"}
var Securities = []string{"none", "tls", "reality"}

type State struct {
	Schema                int              `json:"schema"`
	CreatedAt             string           `json:"created_at"`
	Xray                  map[string]any   `json:"xray"`
	Log                   map[string]any   `json:"log"`
	Inbounds              []map[string]any `json:"inbounds"`
	Outbounds             []map[string]any `json:"outbounds"`
	Routing               map[string]any   `json:"routing"`
	Clients               []map[string]any `json:"clients"`
	ClientGroups          []map[string]any `json:"client_groups"`
	ExternalLinks         []map[string]any `json:"external_links"`
	OutboundSubscriptions []map[string]any `json:"outbound_subscriptions"`
	Meta                  map[string]any   `json:"meta"`
	DNS                   any              `json:"dns,omitempty"`
	Policy                any              `json:"policy,omitempty"`
	Stats                 any              `json:"stats,omitempty"`
	Observatory           any              `json:"observatory,omitempty"`
	BurstObservatory      any              `json:"burstObservatory,omitempty"`
	Reverse               any              `json:"reverse,omitempty"`
}

type Paths struct {
	Home, State, Config, Backup, LogDir, AccessLog, ErrorLog, BinDir, XrayBin, PIDFile string
}

func DefaultPaths() Paths {
	home := os.Getenv("LUI_HOME")
	if home == "" {
		if os.Geteuid() == 0 {
			home = "/etc/l-ui"
		} else if h, e := os.UserHomeDir(); e == nil {
			home = filepath.Join(h, ".config", "l-ui")
		} else {
			home = ".l-ui"
		}
	}
	logDir := os.Getenv("LUI_LOG_DIR")
	if logDir == "" {
		if os.Geteuid() == 0 {
			logDir = "/var/log/l-ui"
		} else {
			logDir = filepath.Join(home, "logs")
		}
	}
	binDir := os.Getenv("LUI_BIN_DIR")
	if binDir == "" {
		if os.Geteuid() == 0 {
			binDir = "/usr/local/lib/l-ui"
		} else {
			binDir = filepath.Join(home, "bin")
		}
	}
	xray := os.Getenv("LUI_XRAY_BIN")
	if xray == "" {
		xray = filepath.Join(binDir, "xray")
	}
	return Paths{Home: home, State: filepath.Join(home, "state.json"), Config: filepath.Join(home, "config.json"), Backup: filepath.Join(home, "backups"), LogDir: logDir, AccessLog: filepath.Join(logDir, "access.log"), ErrorLog: filepath.Join(logDir, "error.log"), BinDir: binDir, XrayBin: xray, PIDFile: filepath.Join(home, "xray.pid")}
}

func NowISO() string { return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339) }

func DefaultState(p Paths) *State {
	return &State{
		Schema: SchemaVersion, CreatedAt: NowISO(),
		Xray:     map[string]any{"version": "", "installed_at": ""},
		Log:      map[string]any{"loglevel": "warning", "access": p.AccessLog, "error": p.ErrorLog},
		Inbounds: []map[string]any{},
		Outbounds: []map[string]any{
			{"tag": "direct", "protocol": "freedom", "settings": map[string]any{"domainStrategy": "AsIs"}},
			{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}},
		},
		Routing: map[string]any{"domainStrategy": "AsIs", "rules": []any{}, "balancers": []any{}},
		Clients: []map[string]any{}, ClientGroups: []map[string]any{}, ExternalLinks: []map[string]any{}, OutboundSubscriptions: []map[string]any{},
		Meta: map[string]any{"next_client_id": float64(1), "next_subscription_id": float64(1), "last_import_report": nil},
	}
}

func DeepCopy[T any](v T) T {
	b, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}

func m(v any) map[string]any {
	if x, ok := v.(map[string]any); ok {
		return x
	}
	return map[string]any{}
}
func a(v any) []any {
	if x, ok := v.([]any); ok {
		return x
	}
	return []any{}
}
func s(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}
func i64(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case float32:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	}
	return 0
}
func i(v any) int { return int(i64(v)) }
func b(v any, d bool) bool {
	if v == nil {
		return d
	}
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		r, e := strconv.ParseBool(x)
		if e == nil {
			return r
		}
		return x != "0" && x != ""
	}
	return d
}
func strSlice(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []string:
		return append(out, x...)
	case []any:
		for _, e := range x {
			if z := strings.TrimSpace(s(e)); z != "" {
				out = append(out, z)
			}
		}
	case string:
		for _, z := range strings.Split(x, ",") {
			if z = strings.TrimSpace(z); z != "" {
				out = append(out, z)
			}
		}
	}
	return out
}
func contains(xs []string, q string) bool {
	for _, x := range xs {
		if x == q {
			return true
		}
	}
	return false
}
func keysSorted(mm map[string]any) []string {
	ks := make([]string, 0, len(mm))
	for k := range mm {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
func parseJSONish(v any, def any) any {
	if v == nil || s(v) == "" {
		return DeepCopy(def)
	}
	switch v.(type) {
	case map[string]any, []any:
		return DeepCopy(v)
	}
	var out any
	if json.Unmarshal([]byte(s(v)), &out) == nil {
		return out
	}
	return DeepCopy(def)
}

func randHex(n int) string {
	p := make([]byte, (n+1)/2)
	if _, e := rand.Read(p); e != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(p)[:n]
}
func UUID() string {
	p := make([]byte, 16)
	_, _ = rand.Read(p)
	p[6] = (p[6] & 0x0f) | 0x40
	p[8] = (p[8] & 0x3f) | 0x80
	h := hex.EncodeToString(p)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func BracketHost(host string) string {
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func Architecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv7"
	case "386":
		return "386"
	case "riscv64":
		return "riscv64"
	default:
		return runtime.GOARCH
	}
}
