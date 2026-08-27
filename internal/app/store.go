package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func EnsureDirs(p Paths) error {
	for _, d := range []string{p.Home, p.Backup, p.LogDir, p.BinDir} {
		if err := os.MkdirAll(d, 0755); err != nil { return err }
	}
	return nil
}

func AtomicJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { return err }
	data, err := json.MarshalIndent(v, "", "  "); if err != nil { return err }; data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*"); if err != nil { return err }; tmp := f.Name(); defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil { f.Close(); return err }; if err = f.Sync(); err != nil { f.Close(); return err }; if err = f.Chmod(0600); err != nil { f.Close(); return err }; if err = f.Close(); err != nil { return err }
	return os.Rename(tmp, path)
}

func LoadState(p Paths) (*State, error) {
	if err := EnsureDirs(p); err != nil { return nil, err }
	data, err := os.ReadFile(p.State)
	if errors.Is(err, os.ErrNotExist) { st := DefaultState(p); if err = AtomicJSON(p.State, st); err != nil { return nil, err }; if err = WriteConfig(p, st); err != nil { return nil, err }; return st, nil }
	if err != nil { return nil, err }
	var st State; if err = json.Unmarshal(data, &st); err != nil { return nil, err }
	if st.Schema != SchemaVersion { return nil, fmt.Errorf("不支持的 L-UI 数据格式: schema=%d", st.Schema) }
	if st.Meta == nil { st.Meta = map[string]any{} }; if _, ok := st.Meta["next_client_id"]; !ok { st.Meta["next_client_id"] = float64(1) }; if _, ok := st.Meta["next_subscription_id"]; !ok { st.Meta["next_subscription_id"] = float64(1) }
	if st.OutboundSubscriptions == nil { st.OutboundSubscriptions = []map[string]any{} }; if st.Routing == nil { st.Routing = map[string]any{"domainStrategy":"AsIs","rules":[]any{},"balancers":[]any{}} }
	return &st, nil
}

func ValidateState(st *State) error {
	ibTags := map[string]bool{}; ports := map[string]string{}
	for _, ib := range st.Inbounds { tag := strings.TrimSpace(s(ib["tag"])); if tag == "" { return errors.New("入站 tag 不能为空") }; if ibTags[tag] { return fmt.Errorf("重复入站 tag: %s", tag) }; ibTags[tag] = true; p := i(ib["port"]); if p < 0 || p > 65535 { return fmt.Errorf("非法入站端口: %d", p) }; listen := s(ib["listen"]); if listen == "" { listen = "0.0.0.0" }; if p > 0 { k := fmt.Sprintf("%s:%d", listen, p); if old := ports[k]; old != "" { return fmt.Errorf("监听冲突: %s (%s / %s)", k, old, tag) }; ports[k] = tag } }
	obTags := map[string]bool{}; obs := append([]map[string]any{}, st.Outbounds...); obs = append(obs, ActiveSubscriptionOutbounds(st, nil)...)
	for _, ob := range obs { tag := strings.TrimSpace(s(ob["tag"])); proto := s(ob["protocol"]); if tag == "" { return errors.New("出站 tag 不能为空") }; if obTags[tag] { return fmt.Errorf("重复出站 tag: %s", tag) }; if !contains(OutboundProtocols, proto) { return fmt.Errorf("不支持的出站协议: %s", proto) }; obTags[tag] = true }
	return nil
}

func WriteConfig(p Paths, st *State) error { cfg, err := BuildConfig(st); if err != nil { return err }; return AtomicJSON(p.Config, cfg) }

func SaveState(p Paths, st *State, apply bool) error {
	if err := ValidateState(st); err != nil { return err }; cfg, err := BuildConfig(st); if err != nil { return err }; oldState, _ := os.ReadFile(p.State); oldCfg, _ := os.ReadFile(p.Config)
	if _, err = os.Stat(p.XrayBin); err == nil { tmp, err := os.CreateTemp(p.Home, "config-candidate-*.json"); if err != nil { return err }; name := tmp.Name(); tmp.Close(); defer os.Remove(name); if err = AtomicJSON(name, cfg); err != nil { return err }; ok, msg := ValidateXrayConfig(p, name); if !ok { return fmt.Errorf("Xray 配置校验失败，未修改正式配置: %s", msg) } }
	if err = AtomicJSON(p.State, st); err != nil { return err }; if err = AtomicJSON(p.Config, cfg); err != nil { return err }
	if apply { if _, e := os.Stat(p.XrayBin); e == nil { if ok, _ := RestartService(p, true); !ok { if len(oldState)>0 { _=os.WriteFile(p.State,oldState,0600) }; if len(oldCfg)>0 { _=os.WriteFile(p.Config,oldCfg,0600) }; _,_=RestartService(p,true); return errors.New("Xray 重启失败，已恢复上一个配置") } } }
	return nil
}

func BackupState(p Paths, st *State) (string,error) { if err:=EnsureDirs(p);err!=nil{return "",err}; name:=filepath.Join(p.Backup,"l-ui-"+time.Now().Format("20060102-150405")+".json"); payload:=map[string]any{"format":"l-ui-backup-v1","created_at":NowISO(),"state":st}; return name,AtomicJSON(name,payload) }
func RestoreBackup(path string)(*State,error){ data,err:=os.ReadFile(path);if err!=nil{return nil,err};var raw struct{Format string `json:"format"`;State State `json:"state"`};if err=json.Unmarshal(data,&raw);err!=nil{return nil,err};if raw.Format!="l-ui-backup-v1"{return nil,errors.New("不是有效的 L-UI 备份")};if err=ValidateState(&raw.State);err!=nil{return nil,err};return &raw.State,nil }

func BuildConfig(st *State)(map[string]any,error){ ins:=[]any{};for _,ib:=range st.Inbounds{w,ok:=MaterializeInbound(ib,st.Clients);if ok{ins=append(ins,w)}};rules:=[]any{};for _,r:=range RoutingRules(st){if ruleEnabled(r){rules=append(rules,StripInternalRule(r))}};rt:=DeepCopy(st.Routing);rt["rules"]=rules;obs:=[]any{};for _,ob:=range ActiveSubscriptionOutbounds(st,boolptr(true)){obs=append(obs,ob)};for _,ob:=range st.Outbounds{obs=append(obs,DeepCopy(ob))};for _,ob:=range ActiveSubscriptionOutbounds(st,boolptr(false)){obs=append(obs,ob)};cfg:=map[string]any{"log":DeepCopy(st.Log),"inbounds":ins,"outbounds":obs,"routing":rt};if st.DNS!=nil{cfg["dns"]=DeepCopy(st.DNS)};if st.Policy!=nil{cfg["policy"]=DeepCopy(st.Policy)};if st.Stats!=nil{cfg["stats"]=DeepCopy(st.Stats)};if st.Observatory!=nil{cfg["observatory"]=DeepCopy(st.Observatory)};if st.BurstObservatory!=nil{cfg["burstObservatory"]=DeepCopy(st.BurstObservatory)};if st.Reverse!=nil{cfg["reverse"]=DeepCopy(st.Reverse)};return cfg,nil }
func boolptr(v bool)*bool{return &v}
func RoutingRules(st *State)[]map[string]any{raw:=a(st.Routing["rules"]);out:=make([]map[string]any,0,len(raw));for _,v:=range raw{if x,ok:=v.(map[string]any);ok{out=append(out,x)}};return out}
func SetRoutingRules(st *State,r []map[string]any){arr:=make([]any,len(r));for i:=range r{arr[i]=r[i]};st.Routing["rules"]=arr}
func ruleEnabled(r map[string]any)bool{return b(r["_lui_enabled"],true)&&b(r["enabled"],true)}
func StripInternalRule(r map[string]any)map[string]any{out:=map[string]any{};for k,v:=range r{if strings.HasPrefix(k,"_lui")||k=="enabled"{continue};out[k]=DeepCopy(v)};return out}
func MaterializeInbound(ib map[string]any,clients []map[string]any)(map[string]any,bool){out:=DeepCopy(ib);meta:=m(out["_lui"]);delete(out,"_lui");if !b(meta["enable"],true){return nil,false};proto:=s(out["protocol"]);settings:=DeepCopy(m(out["settings"]));tag:=s(out["tag"]);linked:=[]map[string]any{};for _,c:=range clients{if contains(strSlice(c["inbound_tags"]),tag){linked=append(linked,c)}};wires:=[]any{};for _,c:=range linked{if w:=ClientWire(c,proto,meta["source_id"]);w!=nil{wires=append(wires,w)}};switch proto{case "vless","vmess","trojan","shadowsocks","hysteria":settings["clients"]=wires;case "wireguard","amneziawg":delete(settings,"clients");settings["peers"]=wires;case "http","mixed":acc:=[]any{};for _,c:=range linked{if b(c["enable"],true){acc=append(acc,map[string]any{"user":s(c["email"]),"pass":s(c["password"])})}};settings["accounts"]=acc};out["settings"]=settings;return out,true}
func ClientWire(c map[string]any,proto string,inboundID any)map[string]any{if !b(c["enable"],true){return nil};email:=s(c["email"]);switch proto{case "vless":d:=map[string]any{"id":firstNonEmpty(s(c["uuid"]),s(c["id_value"])),"email":email};if s(c["flow"])!=""{d["flow"]=s(c["flow"])};return d;case "vmess":return map[string]any{"id":firstNonEmpty(s(c["uuid"]),s(c["id_value"])),"email":email};case "trojan","shadowsocks":d:=map[string]any{"password":s(c["password"]),"email":email};if proto=="shadowsocks"&&s(c["method"])!=""{d["method"]=s(c["method"])};return d;case "hysteria":return map[string]any{"auth":firstNonEmpty(s(c["auth"]),s(c["password"])),"email":email};case "wireguard","amneziawg":d:=map[string]any{"email":email,"level":0};if s(c["public_key"])!=""{d["publicKey"]=s(c["public_key"])};if s(c["pre_shared_key"])!=""{d["preSharedKey"]=s(c["pre_shared_key"])};if i(c["keep_alive"])!=0{d["keepAlive"]=i(c["keep_alive"])};allowed:=c["allowed_ips"];if per:=m(c["allowed_ips_by_inbound"]);inboundID!=nil&&per[s(inboundID)]!=nil{allowed=per[s(inboundID)]};if len(strSlice(allowed))>0{d["allowedIPs"]=strSlice(allowed)};return d};return nil}
func firstNonEmpty(xs ...string)string{for _,x:=range xs{if x!=""{return x}};return ""}
