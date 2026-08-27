import importlib.util
import json
import base64
import os
import sqlite3
import tempfile
import unittest
from pathlib import Path

os.environ["LUI_HOME"] = tempfile.mkdtemp(prefix="lui-test-home-")
os.environ["LUI_LOG_DIR"] = os.path.join(os.environ["LUI_HOME"], "logs")
os.environ["LUI_BIN_DIR"] = os.path.join(os.environ["LUI_HOME"], "bin")

spec = importlib.util.spec_from_file_location("lui", Path(__file__).parents[1] / "lui.py")
lui = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(lui)

class TestLui(unittest.TestCase):
    def test_outbound_protocol_order_matches_3xui_ui(self):
        self.assertEqual(lui.OUTBOUND_PROTOCOLS, ["freedom","blackhole","dns","vmess","vless","trojan","shadowsocks","wireguard","hysteria","socks","http","loopback"])

    def test_default_state_valid(self):
        s = lui.default_state(); lui.validate_state(s); cfg = lui.build_config(s); self.assertEqual(cfg["outbounds"][0]["tag"], "direct")

    def test_disabled_rule_not_materialized(self):
        s = lui.default_state(); s["routing"]["rules"] = [{"type":"field","domain":["example.com"],"outboundTag":"blocked","_lui_enabled":False}]; self.assertEqual(lui.build_config(s)["routing"]["rules"], [])

    def test_client_injection_vless(self):
        s = lui.default_state(); s["inbounds"] = [{"listen":"127.0.0.1","port":12345,"protocol":"vless","settings":{"decryption":"none"},"tag":"v","_lui":{"enable":True}}]; s["clients"] = [{"id":1,"email":"a","uuid":"11111111-1111-1111-1111-111111111111","flow":"","enable":True,"inbound_tags":["v"]}]; self.assertEqual(lui.build_config(s)["inbounds"][0]["settings"]["clients"][0]["email"], "a")

    def test_client_injection_vmess_strips_security(self):
        s=lui.default_state();s["inbounds"]=[{"listen":"127.0.0.1","port":12346,"protocol":"vmess","settings":{"clients":[]},"tag":"m","_lui":{"enable":True}}];s["clients"]=[{"id":1,"email":"a","uuid":"11111111-1111-1111-1111-111111111111","security":"auto","enable":True,"inbound_tags":["m"]}];client=lui.build_config(s)["inbounds"][0]["settings"]["clients"][0];self.assertNotIn("security",client)

    def test_access_log_parse(self):
        self.assertEqual(lui.parse_outbound_from_access_log("2026/08/27 12:00:00 from tcp:127.0.0.1 accepted tcp:example.com:443 [test -> warp]"), "warp")

    def test_default_outbound_is_first_and_reordered(self):
        s=lui.default_state();s["outbounds"].append({"tag":"warp","protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":1080}]}});lui.set_default_outbound(s,"warp");self.assertEqual(lui.get_default_outbound(s),"warp");self.assertEqual(s["outbounds"][0]["tag"],"warp")

    def test_3xui_enabled_flag_is_ui_only(self):
        s=lui.default_state();s["routing"]["rules"]=[{"type":"field","domain":["a.example"],"outboundTag":"blocked","enabled":False},{"type":"field","sourceIP":["10.0.0.0/8"],"outboundTag":"direct","enabled":True}];rules=lui.build_config(s)["routing"]["rules"];self.assertEqual(len(rules),1);self.assertIn("sourceIP",rules[0]);self.assertNotIn("enabled",rules[0])

    def test_sqlite_3xui_import(self):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)/"x-ui.db";db=sqlite3.connect(p);db.execute("create table inbounds (id integer, listen text, port integer, protocol text, settings text, stream_settings text, tag text, sniffing text, enable integer, remark text)");db.execute("create table settings (id integer, key text, value text)");db.execute("create table clients (id integer, email text, sub_id text, uuid text, password text, auth text, flow text, security text, enable integer, group_name text, comment text, total_gb integer, expiry_time integer, wg_public_key text, wg_private_key text, wg_pre_shared_key text, wg_allowed_ips text, wg_keep_alive integer, secret text)");db.execute("create table client_inbounds (client_id integer, inbound_id integer)");db.execute("insert into inbounds values (1,'0.0.0.0',443,'vless',?,?,'vless-in',?,1,'demo')",(json.dumps({"decryption":"none"}),json.dumps({"network":"tcp","security":"none"}),json.dumps({"enabled":True})));db.execute("insert into clients values (1,'alice','sub','11111111-1111-1111-1111-111111111111','','','','auto',1,'','',0,0,'','','','[]',0,'')");db.execute("insert into client_inbounds values (1,1)");template={"outbounds":[{"tag":"warp","protocol":"socks","settings":{"servers":[{"address":"127.0.0.1","port":1080}]}}],"routing":{"rules":[{"type":"field","domain":["example.com"],"outboundTag":"warp"}]}};db.execute("insert into settings values (1,'xrayTemplateConfig',?)",(json.dumps(template),));db.commit();db.close();new,rep=lui.import_3xui_file(lui.default_state(),p,"rename");self.assertEqual(rep["inbounds"],1);self.assertEqual(rep["clients"],1);self.assertEqual(new["outbounds"][0]["tag"],"warp");self.assertIn("vless-in",new["clients"][0]["inbound_tags"])

    def test_sql_dump_detection(self):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)/"x-ui.dump";p.write_text("BEGIN TRANSACTION;\nCREATE TABLE inbounds(id integer);\nCOMMIT;\n",encoding="utf-8");self.assertEqual(lui.detect_3xui_input(p),"sql")

    def test_parse_vless_outbound_link_uses_current_flat_shape(self):
        ob=lui.parse_outbound_link("vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&security=reality&type=tcp&sni=www.example.com&pbk=abc&sid=01&fp=chrome#node");self.assertEqual(ob["settings"]["address"],"example.com");self.assertNotIn("vnext",ob["settings"]);self.assertEqual(ob["streamSettings"]["security"],"reality")

    def test_parse_hysteria2_outbound_link(self):
        ob=lui.parse_outbound_link("hysteria2://secret@example.com:8443?sni=cdn.example.com#hy");self.assertEqual(ob["protocol"],"hysteria");self.assertEqual(ob["settings"],{"address":"example.com","port":8443,"version":2});self.assertEqual(ob["streamSettings"]["hysteriaSettings"]["auth"],"secret")

    def test_subscription_outbounds_injected_pre_and_post(self):
        s=lui.default_state();s["outbound_subscriptions"]=[{"id":1,"priority":0,"enabled":True,"prepend":True,"last_outbounds":[{"tag":"pre","protocol":"freedom","settings":{}}]},{"id":2,"priority":1,"enabled":True,"prepend":False,"last_outbounds":[{"tag":"post","protocol":"freedom","settings":{}}]}];self.assertEqual([x["tag"] for x in lui.build_config(s)["outbounds"]],["pre","direct","blocked","post"])

    def test_subscription_body_base64(self):
        raw=base64.urlsafe_b64encode(b"vless://u@example.com:443?security=none#n\n");lines=lui.decode_subscription_body(raw);self.assertEqual(len(lines),1);self.assertTrue(lines[0].startswith("vless://"))

if __name__ == "__main__": unittest.main()
