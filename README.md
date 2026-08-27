# L-UI

L-UI 是一个面向 Linux 的轻量级 Xray 终端管理器，不提供 Web 面板。目标是保留 3x-ui 常用的入站、出站、路由、客户端、订阅、迁移和内核管理能力，同时把运行环境压缩为 **L-UI 单文件 + Xray 内核**。

## 运行时依赖

L-UI v0.3.0 起使用 Go 重写并发布静态二进制。目标机器运行 L-UI 本体时不需要安装：

- Python / pip
- Go
- Node.js
- jq
- sqlite3
- curl
- qrencode
- glibc

3x-ui SQLite 数据读取、HTTP/HTTPS 请求、SOCKS5 路由测试和终端二维码均由 L-UI 自身实现。Xray 仍然是实际代理核心，因此需要单独安装 Xray；L-UI 的内核管理菜单可以完成安装与切换。

> 安装脚本本身需要一个下载工具：`curl`、`wget` 或 BusyBox `wget` 三者之一。安装完成后，L-UI 运行时不依赖这些工具。

## 支持平台

Release 默认提供以下静态 Linux 二进制：

| Linux 架构 | Release 文件 | 常见 `uname -m` |
| --- | --- | --- |
| amd64 | `l-ui-linux-amd64` | `x86_64`, `amd64` |
| arm64 | `l-ui-linux-arm64` | `aarch64`, `arm64` |
| armv7 | `l-ui-linux-armv7` | `armv7l`, `armv7` |
| 386 | `l-ui-linux-386` | `i386`, `i486`, `i586`, `i686` |

构建使用 `CGO_ENABLED=0`，因此 L-UI 本体不依赖目标系统的 glibc/musl。服务管理依次支持 systemd、OpenRC、SysV init；无法识别 init 系统时可退化为 PID 方式直接管理 Xray 进程。

## 安装

Root 安装到 `/usr/local/bin/l-ui`：

```sh
curl -fsSL https://raw.githubusercontent.com/8bit-warrior/L-UI/main/install.sh | sh
```

也可以使用：

```sh
wget -qO- https://raw.githubusercontent.com/8bit-warrior/L-UI/main/install.sh | sh
```

非 root 用户默认安装到 `$HOME/.local/bin/l-ui`。安装脚本会自动识别 CPU 架构，从最新 GitHub Release 下载对应文件，并在目标系统具有 `sha256sum` 时校验 Release 提供的 `sha256sums.txt`。

安装完成后：

```sh
l-ui
```

## 核心功能

### 入站管理

当前菜单支持：VLESS、VMess、Trojan、Shadowsocks、WireGuard、Hysteria、HTTP、Mixed、Tunnel、AmneziaWG；包括新增、JSON 编辑、启停、克隆、删除、客户端处理、分享链接、Base64 订阅和二维码。

MTProto 和 TUN 没有作为 L-UI/Xray-only 的新增入站接管：当前 3x-ui 的 MTProto 使用独立 MTG 组件；TUN 也不作为当前 Xray-only 新建入站处理。

### 出站管理

“新增出站”的协议顺序与当前 3x-ui 对齐：

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

VLESS 使用当前 Xray/3x-ui 的扁平 `settings.address/port/id/flow/encryption` 结构；VMess 保留 `settings.vnext`。默认出站与 3x-ui 一致：**出站数组第一项就是默认出站**，设置默认值时通过调整出站顺序完成。

出站订阅支持 VMess、VLESS、Trojan、Shadowsocks、Hysteria2 和 WireGuard 常见 URI，以及原始 JSON outbounds。订阅 URL 默认拒绝私网、回环、链路本地、多播和未指定地址，重定向也重新检查；单次订阅响应最大 8 MiB。确有内网订阅需求时可在订阅设置中显式开启 `allow_private`。

### 路由管理与真实访问测试

路由规则支持 domain、IP、目标端口、`sourceIP`、sourcePort、network、protocol、inboundTag、user、outboundTag/balancerTag 等字段，并在生成 Xray 配置时剔除 L-UI 内部启停字段。

真实路由测试不是只计算“理论命中哪个出口”。L-UI 会：

1. 生成临时 Xray 配置；
2. 创建仅监听 `127.0.0.1` 的临时 SOCKS 入站；
3. 用实际 Xray `-test` 校验该临时配置；
4. 启动临时 Xray；
5. 使用 L-UI 内置 SOCKS5 客户端完成完整 HTTP/HTTPS 请求；
6. 从临时 Xray access log 读取实际命中的 outbound tag；
7. 输出 HTTP 状态、整个请求延迟、连接耗时和 TTFB；
8. 立即结束临时 Xray，不修改正式实例。

因此目标机无需 `curl`。

### 客户端与导出

支持客户端新增/批量新增、启停、编辑、分组、绑定/解绑入站、外部链接、导入导出，以及 VLESS/Trojan/VMess/Shadowsocks 标准分享链接和 Base64 订阅。二维码由内置 Go 库直接在终端渲染，不需要 `qrencode`。

高级协议如果没有稳定、通用的标准分享 URI，可使用 Xray JSON 导出。

### 3x-ui 数据迁移

可读取当前/兼容版本 3x-ui 的 SQLite `.db`，并处理常见的：

- inbounds
- clients / client_inbounds
- client_groups
- client_external_links
- `xrayTemplateConfig` 中的 outbounds、routing、DNS、policy、stats 等
- outbound_subscriptions

也支持 SQLite SQL 文本 `.dump`。如果文件是 PostgreSQL 原生二进制 `PGDMP`，L-UI 会明确拒绝，因为它不是 SQLite 数据；应先通过 3x-ui 的 Migration 功能得到跨数据库迁移用 SQLite `.db` 再导入。

导入前会生成 L-UI 回滚备份，并提供跳过、覆盖、自动重命名三种冲突策略。

## 数据位置

root 模式默认：

```text
/etc/l-ui/state.json
/etc/l-ui/config.json
/etc/l-ui/backups/
/var/log/l-ui/access.log
/var/log/l-ui/error.log
/usr/local/lib/l-ui/xray
```

旧 Python 版和 Go 版沿用同一 `schema=1` 状态格式，因此正常升级时会继续读取现有 `/etc/l-ui/state.json`，不要求重新配置。

## 命令行

```text
l-ui
l-ui version
l-ui config
l-ui check
l-ui route-test URL [INBOUND_TAG]
l-ui analyze-3xui FILE
l-ui self-check
```

## 构建与 Release

仓库源码使用 Go。目标设备**不需要 Go**；GitHub Actions 负责交叉编译静态二进制。

本地构建示例：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags='-s -w' -o l-ui ./cmd/l-ui
```

CI 会执行格式检查、`go vet`、单元测试、真实 SOCKS5 路由集成测试、静态 amd64 冒烟测试，以及 amd64/arm64/armv7/386 交叉编译。Release 工作流发布四种架构文件和 `sha256sums.txt`。

## 当前边界

- L-UI 是终端管理器，不提供 Web UI，也不依赖 3x-ui 运行时。
- 当前“重置流量”指重启 Xray 运行时统计；尚未实现 3x-ui 那种长期持久化流量数据库，因此不要把它当作完整的长期流量计费系统。
- Xray 配置最终仍由所安装的 Xray 内核校验；某个协议字段是否可用取决于对应 Xray 版本。

## 开发自检

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/l-ui
```

真实路由集成测试使用仓库内的 Go fake-Xray 实现实际 SOCKS5 CONNECT 与 TCP 转发，不只是 mock 返回值。正式 Xray release 的下载/安装仍由目标环境或 GitHub 网络执行；离线开发环境无法替代对官方 release 二进制的在线下载验证。
