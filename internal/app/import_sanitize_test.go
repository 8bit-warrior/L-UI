package app

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvert3xuiFiltersInternalAPIRoute(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil || db.Ping() != nil {
		t.Skip("sqlite driver unavailable")
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE settings (key TEXT, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	templ := map[string]any{
		"api": map[string]any{"tag": "api", "services": []any{"HandlerService", "StatsService"}},
		"outbounds": []any{
			map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{"domainStrategy": "AsIs"}},
			map[string]any{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}},
		},
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules": []any{
				map[string]any{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"},
				map[string]any{"type": "field", "domain": []any{"full:example.com"}, "outboundTag": "direct", "enabled": true},
			},
		},
	}
	raw, _ := json.Marshal(templ)
	if _, err = db.Exec(`INSERT INTO settings(key,value) VALUES('xrayTemplateConfig', ?)`, string(raw)); err != nil {
		t.Fatal(err)
	}
	st := DefaultState(testPaths(t))
	next, report, err := Convert3xui(db, st, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	rules := RoutingRules(next)
	if len(rules) != 1 {
		t.Fatalf("expected only user routing rule, got %d: %#v", len(rules), rules)
	}
	if got := s(rules[0]["outboundTag"]); got != "direct" {
		t.Fatalf("unexpected remaining outboundTag: %q", got)
	}
	if i(report["routing_rules"]) != 1 {
		t.Fatalf("report routing_rules=%v", report["routing_rules"])
	}
	warnings := a(report["warnings"])
	if len(warnings) == 0 || !strings.Contains(s(warnings[0]), "内部 API") {
		t.Fatalf("expected internal API warning, got %#v", warnings)
	}
	cfg, err := BuildConfig(next)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range a(m(cfg["routing"])["rules"]) {
		rr := m(v)
		if s(rr["outboundTag"]) == "api" || contains(strSlice(rr["inboundTag"]), "api") {
			t.Fatalf("3x-ui control-plane route leaked into Xray config: %#v", rr)
		}
	}
}

func TestValidateXrayConfigReturnsDiagnostic(t *testing.T) {
	p := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.XrayBin), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.XrayBin, []byte("#!/bin/sh\necho 'fixture-xray-diagnostic' >&2\nexit 23\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(p.Home, "candidate.json")
	if err := os.WriteFile(cfg, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ok, msg := ValidateXrayConfig(p, cfg)
	if ok {
		t.Fatal("expected validation failure")
	}
	if !strings.Contains(msg, "fixture-xray-diagnostic") {
		t.Fatalf("diagnostic was lost: %q", msg)
	}
}
