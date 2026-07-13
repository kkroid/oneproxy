# OneProxy - 多端口代理聚合器

OneProxy 是一个基于 [sing-box](https://github.com/SagerNet/sing-box) 的**本地代理聚合器**，将多个上游代理服务器（如 JustMySocks）同时暴露为独立的本地 SOCKS5 端口，实现真正的多代理并行、互不干扰。

## 💡 核心理念

**不是另一个代理客户端，而是代理端口聚合器**

```
JustMySocks 5个节点 (各有复杂的密码/配置)
                ↓
        OneProxy 统一管理
                ↓
    对外暴露 5 个简单的 SOCKS5 端口
                ↓
127.0.0.1:10801 → JMS-Server1 (固定绑定)
127.0.0.1:10802 → JMS-Server2 (固定绑定)
127.0.0.1:10803 → JMS-Server3 (固定绑定)
127.0.0.1:10804 → JMS-Server4 (固定绑定)
127.0.0.1:10805 → JMS-Server5 (固定绑定)
```

## 🎯 为什么需要 OneProxy？

### 与现有客户端（NekoBox、Hiddify）的根本区别

| 特性 | NekoBox/Hiddify | OneProxy |
|------|-----------------|----------|
| **设计目标** | 单一出口，自动/手动切换节点 | **多端口并行，固定绑定** |
| **节点使用** | 同时只能用一个节点 | **所有节点同时可用** |
| **切换方式** | GUI 点击切换 | **直接换端口号** |
| **应用分流** | 需要配置复杂规则 | **手动指定端口，简单直接** |
| **密码管理** | 配置中可见 | **完全隐藏，只暴露端口** |
| **使用场景** | 个人日常上网 | **开发测试、多应用分流、内网共享** |

### 真实使用场景

#### 场景 1：开发测试 - 同时测试多个节点
```bash
# 同时对比所有节点的速度
for port in 10801 10802 10803 10804 10805; do
    echo "Testing port $port"
    curl -x socks5://127.0.0.1:$port -w "Time: %{time_total}s\n" https://google.com &
done
wait
```

#### 场景 2：多应用精确分流
```
Chrome Profile 1 → 10801 (JMS-Server1，延迟最低)
Chrome Profile 2 → 10802 (JMS-Server2，最稳定)
VSCode          → 10803 (JMS-Server3，IP池大，避免限流)
Telegram        → 10804 (JMS-Server4，带宽最大)
Docker          → 10805 (JMS-Server5，独立使用)
```

#### 场景 3：快速故障切换
```
10801 挂了？
不需要：打开GUI → 选择节点 → 点击切换 → 等待连接
只需要：把端口改成 10802
```

#### 场景 4：团队内网共享
```
配置 listen: "0.0.0.0"
团队成员直接使用: 192.168.1.100:10801
无需知道上游密码，无需安装客户端
```

## ✨ 核心特性

- 🔌 **多端口并行** - 所有代理节点同时暴露为独立端口，互不干扰
- 🎯 **固定绑定** - 每个端口永久绑定到特定节点，不自动切换
- 🔒 **密码隐藏** - 上游密码/UUID 完全隐藏，使用者只需要端口号
- 📊 **健康监控** - 实时显示每个节点的延迟和健康状态
- ⚡ **DNS 快速刷新** - 应对墙导致的 IP 变更，快速恢复连接
- 🖥️ **系统托盘** - 简洁的托盘界面，一键复制端口地址
- 🌐 **内网共享** - 可选暴露到内网，供其他设备使用

## 系统要求

- Windows 10/11（当前版本，已预留跨平台能力）
- sing-box v1.13.14+

## 快速开始

### 1. 下载 sing-box

```bash
# Windows
download-singbox.bat

# Linux/macOS
./download-singbox.sh
```

### 2. 配置代理节点

复制示例配置并修改：

```bash
cp configs/config.example.json config.json
```

编辑 `config.json`：

```json
{
  "proxies": [
    {
      "name": "JMS-Server1",
      "enabled": true,
      "local_port": 10801,        // 本地暴露端口
      "type": "shadowsocks",
      "server": "c331s1.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "your-password"
    },
    {
      "name": "JMS-Server2",
      "enabled": true,
      "local_port": 10802,
      "type": "shadowsocks",
      "server": "c331s2.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "your-password"
    },
    {
      "name": "JMS-Server3",
      "enabled": true,
      "local_port": 10803,
      "type": "vmess",
      "server": "c331s3.portablesubmari",
      "port": 5299,
      "uuid": "your-uuid",
      "alter_id": 0,
      "security": "auto"
    }
  ],
  "inbound": {
    "listen": "127.0.0.1",       // 仅本地，改成 "0.0.0.0" 允许内网访问
    "proxy_type": "socks5"       // 统一输出 SOCKS5
  }
}
```

**关键配置说明**：
- `local_port` - 每个节点对外暴露的本地端口（必须唯一）
- `listen: "127.0.0.1"` - 仅本地访问
- `listen: "0.0.0.0"` - 允许内网其他设备访问

### 3. 运行

```bash
./oneproxy.exe
```

右键托盘图标 → 启动代理 → 所有端口同时启动

### 4. 使用代理

#### 浏览器（推荐使用 SwitchyOmega）

创建多个情景模式，分别指向不同端口：

```
Profile-Server1:
  协议: SOCKS5
  服务器: 127.0.0.1
  端口: 10801

Profile-Server2:
  协议: SOCKS5
  服务器: 127.0.0.1
  端口: 10802
```

#### 命令行工具

```bash
# 使用节点1
curl -x socks5://127.0.0.1:10801 https://google.com

# 使用节点2
curl -x socks5://127.0.0.1:10802 https://google.com

# Git
git config --global http.proxy socks5://127.0.0.1:10801

# npm
npm config set proxy socks5://127.0.0.1:10801
```

#### 应用程序

大多数应用支持 SOCKS5 代理设置：

```
代理地址: 127.0.0.1
端口: 10801 (或 10802, 10803...)
类型: SOCKS5
```

## 系统托盘界面

```
OneProxy - 运行中 🟢
├── ───────────────
├── 启动所有代理
├── 停止所有代理
├── 重启所有代理
├── ───────────────
├── 节点列表 ▶
│   ├── ✓ JMS-Server1 [:10801] (45ms)
│   │   └── 复制地址: 127.0.0.1:10801
│   ├── ✓ JMS-Server2 [:10802] (67ms)
│   │   └── 复制地址: 127.0.0.1:10802
│   ├── ✗ JMS-Server3 [:10803] (超时)
│   │   └── 复制地址: 127.0.0.1:10803
│   ├── ✓ JMS-Server4 [:10804] (52ms)
│   └── ✓ JMS-Server5 [:10805] (89ms)
├── ───────────────
├── 健康检查: 已启用 (每60秒)
├── 立即检查所有节点
├── DNS刷新: 已启用
├── 立即刷新DNS
├── ───────────────
├── 打开配置文件
├── 查看日志
└── 退出
```

**功能说明**：
- ✓ 绿色勾 = 节点健康
- ✗ 红色叉 = 节点不可用
- (45ms) = 延迟显示
- [:10801] = 本地端口号
- 右键节点可快速复制地址

## 配置说明

### 完整配置示例

```json
{
  "version": "1.0",
  "log_level": "info",
  "health_check": {
    "enabled": true,
    "interval_seconds": 60,
    "timeout_seconds": 5,
    "test_url": "https://www.google.com/generate_204"
  },
  "dns": {
    "flush_on_failure": true,
    "flush_interval_seconds": 300,
    "servers": [
      "https://1.1.1.1/dns-query",
      "https://8.8.8.8/dns-query"
    ]
  },
  "proxies": [
    {
      "name": "JMS-Server1",
      "enabled": true,
      "local_port": 10801,
      "type": "shadowsocks",
      "server": "c331s1.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "your-password"
    }
  ],
  "inbound": {
    "listen": "127.0.0.1",
    "proxy_type": "socks5"
  }
}
```

### 配置字段说明

#### health_check（健康检查）
- `enabled` - 是否启用健康检查
- `interval_seconds` - 检查间隔（默认 60 秒）
- `timeout_seconds` - 超时时间
- `test_url` - 测试 URL

#### dns（DNS 配置）
- `flush_on_failure` - 节点失败时是否刷新 DNS
- `flush_interval_seconds` - 定期刷新间隔（默认 300 秒）
- `servers` - 可靠的 DNS 服务器列表

#### proxies（代理节点）
- `name` - 节点名称（显示在托盘）
- `enabled` - 是否启用
- `local_port` - **本地暴露端口（必填，必须唯一）**
- `type` - 代理类型（shadowsocks, vmess, trojan 等）
- `server` - 上游服务器地址（**推荐使用域名**）
- 其他字段根据代理类型而定

#### inbound（入站配置）
- `listen` - 监听地址
  - `"127.0.0.1"` - 仅本机可用（安全）
  - `"0.0.0.0"` - 允许内网访问（团队共享）
- `proxy_type` - 输出类型（当前仅支持 socks5）

## 架构设计

### 为什么使用域名而不是 IP？

JustMySocks 等服务会因为墙的封锁而更换 IP，但会自动更新域名解析。

**域名的优势**：
- 服务商自动更新 DNS 记录
- 配置一次，长期有效
- 结合 OneProxy 的 DNS 快速刷新，恢复时间从几分钟缩短到几十秒

**IP 的劣势**：
- 需要手动更新配置
- 需要订阅链接定期同步
- 维护成本高

### sing-box 配置原理

OneProxy 自动生成的 sing-box 配置：

```json
{
  "inbounds": [
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10801},
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10802},
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10803}
  ],
  "outbounds": [
    {"type": "shadowsocks", "tag": "out-1", "server": "..."},
    {"type": "shadowsocks", "tag": "out-2", "server": "..."},
    {"type": "vmess", "tag": "out-3", "server": "..."}
  ],
  "route": {
    "rules": [
      {"inbound": ["socks-in-10801"], "outbound": "out-1"},
      {"inbound": ["socks-in-10802"], "outbound": "out-2"},
      {"inbound": ["socks-in-10803"], "outbound": "out-3"}
    ]
  }
}
```

**关键设计**：
- 每个代理一个独立 inbound 端口
- 每个 inbound 通过 route 规则固定绑定到对应的 outbound
- **没有 Selector，没有 URLTest，不会自动切换**

### DNS 快速刷新机制

当健康检查发现节点连续失败 3 次：

1. **刷新系统 DNS 缓存**（Windows: `ipconfig /flushdns`）
2. **重启 sing-box 进程**（清除 sing-box 内部 DNS 缓存）
3. **等待 DNS 重新解析**（获取最新 IP）
4. **重新建立连接**

**恢复时间对比**：
- 不刷新：依赖 DNS TTL，可能需要 2-5 分钟
- 主动刷新：几十秒内恢复

## 开发计划

### ✅ Phase 1: 核心功能（当前版本）

- [x] 项目初始化和基础结构
- [x] 配置管理（加载/保存/验证）
- [x] sing-box 进程管理（启动/停止/重启）
- [x] 系统托盘界面
- [x] 基础文档

**待修改**（根据新需求）：
- [ ] 配置结构添加 `local_port` 字段
- [ ] sing-box 配置生成器改为一对一绑定
- [ ] 移除 Selector 和 URLTest 逻辑
- [ ] 托盘菜单显示端口和复制功能

### 🚧 Phase 2: 健康检查与 DNS 优化

- [ ] 每个节点独立健康检查
- [ ] 延迟测试和显示
- [ ] 连续失败检测（3 次触发）
- [ ] 自动 DNS 刷新（系统 + sing-box）
- [ ] 手动触发 DNS 刷新
- [ ] 托盘显示健康状态

### 📋 Phase 3: 增强功能

- [ ] 一键复制端口地址到剪贴板
- [ ] 日志查看器（过滤、搜索）
- [ ] 开机自启动（Windows 注册表）
- [ ] 配置热重载（修改配置后自动重启）
- [ ] 端口可用性检测（防止端口冲突）
- [ ] 内网访问控制（IP 白名单）

## 常见问题

### Q: 与 NekoBox/Hiddify 有什么区别？

**NekoBox/Hiddify**：单一出口，自动或手动切换节点，适合日常浏览
**OneProxy**：多端口并行，固定绑定，适合开发测试和多应用分流

### Q: 为什么不自动选择最快节点？

这不是我们的设计目标。OneProxy 为每个节点提供独立端口，用户根据需要**手动选择**使用哪个端口，实现精确控制。

### Q: 能同时使用多个节点吗？

**可以！**这正是 OneProxy 的核心价值。所有端口同时可用，不同应用可以同时使用不同节点。

### Q: 如何知道某个节点挂了？

托盘菜单会实时显示每个节点的健康状态（✓ 或 ✗）和延迟。Phase 2 还会添加通知功能。

### Q: DNS 刷新会影响正在使用的连接吗？

会短暂中断（几秒），因为需要重启 sing-box。但这是为了快速恢复连接的必要代价。建议在检测到故障时才触发。

### Q: 可以分享给内网其他设备吗？

可以。将配置中的 `listen` 改为 `"0.0.0.0"`，内网设备就可以通过 `你的IP:10801` 使用代理。

### Q: 支持其他代理协议吗？

支持！sing-box 支持的协议 OneProxy 都支持（Trojan、VLESS、Hysteria2 等）。参考 [sing-box 文档](https://sing-box.sagernet.org/configuration/outbound/)。

## 技术栈

- **语言**: Go 1.21+
- **核心引擎**: sing-box v1.13.14+
- **托盘库**: getlantern/systray v1.2.2
- **配置格式**: JSON
- **构建工具**: Make

## 贡献

欢迎提交 Issue 和 Pull Request！详见 [DEVELOPMENT.md](DEVELOPMENT.md)。

## 许可证

MIT License - 详见 [LICENSE](LICENSE)

## 致谢

- [sing-box](https://github.com/SagerNet/sing-box) - 强大的代理引擎
- [systray](https://github.com/getlantern/systray) - 系统托盘库

---

**注意**: 请遵守当地法律法规，仅将本工具用于合法用途。

**定位**: OneProxy 不是通用代理客户端，而是为特定场景（多代理并行、精确控制）设计的专用工具。
