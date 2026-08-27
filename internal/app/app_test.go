package app

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	return Paths{Home: root, State: filepath.Join(root, "state.json"), Config: filepath.Join(root, "config.json"), Backup: filepath.Join(root, "backups"), LogDir: filepath.Join(root, "logs"), AccessLog: filepath.Join(root, "logs", "access.log"), ErrorLog: filepath.Join(root, "logs", "error.log"), BinDir: filepath.Join(root, "bin"), XrayBin: filepath.Join(root, "bin", "xray"), PIDFile: filepath.Join(root, "xray.pid")}
}

func TestDefaultStateAndConfig(t *testing.T) {
	p := testPaths(t)
	st := DefaultState(p)
	if err := ValidateState(st); err != nil {
		t.Fatal(err)
	}
	cfg, err := BuildConfig(st)
	if err != nil {
		t.Fatal(err)
	}
	obs := a(cfg["outbounds"])
	if len(obs) != 2 {
		t.Fatalf("outbounds=%d", len(obs))
	}
	if s(m(obs[0])["tag"]) != "direct" {
		t.Fatal("default outbound must be first")
	}
}
func TestVMessClientWireStripsSecurity(t *testing.T) {
	c := map[string]any{"uuid": "11111111-1111-4111-8111-111111111111", "email": "a", "security": "aes-128-gcm", "enable": true}
	w := ClientWire(c, "vmess", nil)
	if _, ok := w["security"]; ok {
		t.Fatal("vmess inbound client must not emit security")
	}
	if s(w["id"]) == "" {
		t.Fatal("missing id")
	}
}
func TestVLESSOutboundLinkIsFlat(t *testing.T) {
	link := "vless://11111111-1111-4111-8111-111111111111@example.com:443?security=reality&type=tcp&sni=example.com&pbk=pk&sid=01#demo"
	ob, err := ParseOutboundLink(link)
	if err != nil {
		t.Fatal(err)
	}
	settings := m(ob["settings"])
	if settings["vnext"] != nil {
		t.Fatal("vless outbound must not use vnext")
	}
	if s(settings["address"]) != "example.com" || i(settings["port"]) != 443 {
		t.Fatalf("unexpected settings: %#v", settings)
	}
}
func TestSubscriptionBase64(t *testing.T) {
	src := []string{"vless://a", "trojan://b"}
	got := SubscriptionBase64(src)
	raw, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "vless://a\ntrojan://b\n" {
		t.Fatalf("got %q", raw)
	}
}
func TestDisabledRoutingRuleStripped(t *testing.T) {
	p := testPaths(t)
	st := DefaultState(p)
	SetRoutingRules(st, []map[string]any{{"type": "field", "domain": []string{"example.com"}, "outboundTag": "blocked", "_lui_enabled": false}, {"type": "field", "ip": []string{"1.1.1.1"}, "outboundTag": "direct", "_lui_enabled": true}})
	cfg, _ := BuildConfig(st)
	rules := a(m(cfg["routing"])["rules"])
	if len(rules) != 1 {
		t.Fatalf("active rules=%d", len(rules))
	}
	if m(rules[0])["_lui_enabled"] != nil {
		t.Fatal("internal field leaked")
	}
}
func TestSubscriptionInjectionOrder(t *testing.T) {
	p := testPaths(t)
	st := DefaultState(p)
	st.OutboundSubscriptions = []map[string]any{{"id": 1, "enabled": true, "prepend": true, "priority": 0, "last_outbounds": []any{map[string]any{"tag": "pre", "protocol": "freedom", "settings": map[string]any{}}}}, {"id": 2, "enabled": true, "prepend": false, "priority": 1, "last_outbounds": []any{map[string]any{"tag": "post", "protocol": "freedom", "settings": map[string]any{}}}}}
	cfg, err := BuildConfig(st)
	if err != nil {
		t.Fatal(err)
	}
	obs := a(cfg["outbounds"])
	tags := []string{}
	for _, v := range obs {
		tags = append(tags, s(m(v)["tag"]))
	}
	if strings.Join(tags, ",") != "pre,direct,blocked,post" {
		t.Fatalf("order=%v", tags)
	}
}

func Test3xuiSQLiteConversion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil || db.Ping() != nil {
		t.Skip("sqlite driver unavailable in local stub build")
	}
	defer db.Close()
	stmts := []string{`CREATE TABLE inbounds (id INTEGER, protocol TEXT, tag TEXT, listen TEXT, port INTEGER, settings TEXT, stream_settings TEXT, sniffing TEXT, enable INTEGER, remark TEXT)`, `CREATE TABLE settings (key TEXT, value TEXT)`, `INSERT INTO inbounds VALUES (1,'vless','in-vless','0.0.0.0',443,'{"decryption":"none","clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"u"}]}','{}','{"enabled":true}',1,'demo')`, `INSERT INTO settings VALUES ('xrayTemplateConfig','{"outbounds":[{"tag":"direct","protocol":"freedom","settings":{"domainStrategy":"AsIs"}},{"tag":"blocked","protocol":"blackhole","settings":{}}],"routing":{"domainStrategy":"AsIs","rules":[]}}')`}
	for _, q := range stmts {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	p := testPaths(t)
	st := DefaultState(p)
	next, report, err := Convert3xui(db, st, "rename")
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Inbounds) != 1 {
		t.Fatalf("inbounds=%d", len(next.Inbounds))
	}
	if i(report["inbounds"]) != 1 {
		t.Fatalf("report=%v", report)
	}
}

func TestRealRouteTestIntegration(t *testing.T) {
	fake := os.Getenv("LUI_FAKE_XRAY_BIN")
	if fake == "" {
		t.Skip("set LUI_FAKE_XRAY_BIN")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	p := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.XrayBin), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fake)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p.XrayBin, data, 0755); err != nil {
		t.Fatal(err)
	}
	st := DefaultState(p)
	if err = WriteConfig(p, st); err != nil {
		t.Fatal(err)
	}
	r := RealRouteTest(p, st, srv.URL, "", 8*time.Second)
	d, _ := json.Marshal(r)
	t.Log(string(d))
	if !r.Success || r.HTTPCode != 204 {
		t.Fatalf("route failed: %+v", r)
	}
	if r.Outbound != "direct" {
		t.Fatalf("outbound=%q", r.Outbound)
	}
	if r.TotalMS <= 0 {
		t.Fatalf("total_ms=%f", r.TotalMS)
	}
}
