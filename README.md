# L-UI

L-UI 是一个**无 Web 面板、Xray-only** 的终端管理器。目标是把 3x-ui 中最常用的入站、出站、路由、客户端管理转换成纯终端菜单，同时保留数据迁移和真实连通性测试能力。

## 3x-ui 对照基准

L-UI 的“新增出站”协议顺序直接跟随 3x-ui 当前界面的 `OutboundProtocols`：

1. `freedom`
2. `blackhole`
3. `dns`
4. `vmess`
5. `vless`
6. `trojan`
7. `shadowsocks`
8. `wireguard`
9. `hysteria`
10. `socks`
11. `http`
12. `loopback`

不是根据 Xray 文档自行扩充。

## 安装

```bash
bash <(curl -Ls https://raw.githubusercontent.com/8bit-warrior/L-UI/main/install.sh)
l-ui
```

依赖：Python 3.9+、curl、unzip、CA certificates。安装脚本支持 Debian/Ubuntu、Fedora/RHEL、Alpine、Arch、openSUSE 的基础依赖安装；服务管理当前使用 systemd。

## 主要功能

- Xray 内核安装/切换：最新 release（包含 prerelease）、固定兼容版 `v26.6.27`、自定义版本存在性验证；架构匹配、SHA256（release 提供 digest 时）与可执行版本验证、当前配置验证、失败回滚。
- 入站：列表、新增、JSON 编辑、启停、详情、分享链接、二维码、订阅导出、克隆、删除、批量删除、JSON 导入导出。
- 出站：3x-ui 对应 12 种协议、增删改、排序、默认出站、导入导出、单个/全部真实访问测试；出站订阅支持 CRUD、预览、刷新、排序、前置/后置、Tag 前缀、私网和 TLS 校验选项。
- 路由：基础路由、规则增删改、启停、排序、导入导出、真实路由测试。
- 客户端：CRUD、批量创建/调整、启停、分组、绑定/解绑入站、外部链接、导入导出、分享链接。
- 3x-ui 迁移：读取 SQLite `.db` 和 3x-ui SQLite migration `.dump`，导入入站、客户端、关联关系、客户端分组、客户端外部链接、出站订阅，以及 Xray template 中的出站/路由/DNS 等；原生 PostgreSQL binary dump 会明确拒绝并要求使用 3x-ui Migration 导出为 `.db`。
- L-UI 自身完整备份/恢复。
- systemd 服务管理、Xray 配置检查、日志查看。

## 路由测试与 3x-ui 的差异

3x-ui 的 Route Tester 是规则模拟，返回会匹配哪个 outbound。L-UI 的 `真实路由访问测试` 会：

1. 用当前 outbounds + routing 生成一个临时 Xray 配置。
2. 在本机随机端口启动临时 SOCKS 入站。
3. 使用 `curl --socks5-hostname` 真正访问指定 `http://` / `https://` URL。
4. 从 Xray access log 读取实际命中的 outbound tag。
5. 输出 HTTP 状态、整个请求 `time_total`、连接耗时和 TTFB。`time_total` 覆盖 SOCKS 握手、代理侧域名解析/连接、TLS、HTTP 以及响应接收全过程。
6. 测试完成立即终止临时 Xray，不修改正式实例。

```bash
l-ui route-test https://www.google.com/generate_204
l-ui route-test https://example.com my-inbound-tag
```

## 数据路径

- 状态：`/etc/l-ui/state.json`
- 生成配置：`/etc/l-ui/config.json`
- 内核：`/usr/local/lib/l-ui/xray`
- 日志：`/var/log/l-ui/`
- 备份：`/etc/l-ui/backups/`
- systemd：`l-ui-xray.service`

测试/非 root 环境可以使用 `LUI_HOME`、`LUI_BIN_DIR`、`LUI_LOG_DIR`、`LUI_XRAY_BIN` 重定向路径。

## 自检

```bash
l-ui self-check
python3 -m unittest discover -s tests -v
```

自检覆盖状态结构、配置生成、Xray `-test`（安装内核后）、curl 可用性和 SQLite 支持。测试覆盖 3x-ui 出站协议顺序、禁用规则、客户端注入、access log 出口解析、3x-ui SQLite 数据迁移、SQL dump 识别、VLESS/Hysteria2 出站 URI、Base64 订阅和订阅前置/后置注入；另有 SOCKS5 + 完整 HTTP 路由集成测试。

## 已知边界

- L-UI 只管理 Xray。3x-ui 的 MTProto 依赖独立 `mtg-multi` 子进程，因此不伪装成 Xray 功能；旧 `tun` 也不会作为新建入站暴露。
- WireGuard/AmneziaWG、复杂 XHTTP/REALITY 等高级字段可用菜单中的 JSON 编辑完整表达。
- 原生 PostgreSQL `pg_dump -Fc` 文件不是 SQLite，L-UI 不直接解释；请从 3x-ui 的 Migration 功能取得 SQLite `.db`。
- “流量重置”当前以重启 Xray 运行时统计实现；L-UI v0.2.0 不维护 3x-ui 那套持久化流量数据库。
