# OneProxy 项目实现规划

## 项目概述

OneProxy 是一个基于 sing-box 的本地代理管理工具，提供系统托盘界面，用于管理多个代理服务器（主要针对 JustMySocks 等服务）。核心特性是通过主动健康检查和 DNS 优化，快速应对墙导致的 IP 变更问题。

## 技术栈

- **开发语言**：Go 1.21+
- **核心依赖**：
  - sing-box（二进制进程管理）
  - [fyne](https://fyne.io/) 或 [systray](https://github.com/getlantern/systray)（系统托盘 UI）
  - 标准库：os/exec, encoding/json, net/http
- **目标平台**：Windows（预留跨平台能力）

## 项目架构

```
OneProxy/
├── cmd/
│   └── oneproxy/
│       └── main.go                 # 程序入口
├── internal/
│   ├── config/
│   │   ├── config.go              # 配置结构定义
│   │   ├── loader.go              # 配置加载/保存
│   │   └── singbox.go             # sing-box 配置生成器
│   ├── proxy/
│   │   ├── manager.go             # sing-box 进程管理
│   │   ├── health.go              # 健康检查逻辑
│   │   └── dns.go                 # DNS 缓存刷新
│   └── ui/
│       ├── tray.go                # 系统托盘界面
│       └── menu.go                # 菜单构建逻辑
├── configs/
│   ├── config.example.json        # 配置文件示例
│   └── singbox.template.json      # sing-box 配置模板
├── bin/
│   └── sing-box.exe               # sing-box 二进制（下载后放置）
├── logs/
│   └── oneproxy.log               # 应用日志
├── go.mod
├── go.sum
├── README.md
└── .gitignore
```

## 核心模块设计

### 1. 配置管理模块 (internal/config)

#### 1.1 用户配置文件 (config.json)

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
    "servers": ["https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"]
  },
  "proxies": [
    {
      "name": "JMS-Server1",
      "enabled": true,
      "type": "shadowsocks",
      "server": "c331s1.portablesubmari",
      "port": 5299,
      "method": "aes-256-gcm",
      "password": "your-password"
    },
    {
      "name": "JMS-Server2", 
      "enabled": true,
      "type": "vmess",
      "server": "c331s3.portablesubmari",
      "port": 5299,
      "uuid": "your-uuid",
      "alter_id": 0,
      "security": "auto"
    }
  ],
  "inbound": {
    "listen": "127.0.0.1",
    "socks_port": 1080,
    "http_port": 1081,
    "mixed_port": 1082
  }
}
```

#### 1.2 sing-box 配置生成

根据用户配置动态生成 sing-box 的 JSON 配置：

```go
// internal/config/singbox.go
type SingBoxGenerator struct {
    userConfig *Config
}

func (g *SingBoxGenerator) Generate() (*SingBoxConfig, error) {
    // 生成包含以下内容的 sing-box 配置：
    // 1. DNS 配置（短 TTL，可靠 DNS 服务器）
    // 2. Inbound 配置（SOCKS/HTTP/Mixed）
    // 3. Outbound 配置（所有代理服务器）
    // 4. Selector outbound（用于切换）
    // 5. URLTest outbound（自动测速）
}
```

### 2. 代理管理模块 (internal/proxy)

#### 2.1 进程管理 (manager.go)

```go
type Manager struct {
    cmd           *exec.Cmd
    configPath    string
    singboxPath   string
    isRunning     bool
    mutex         sync.RWMutex
}

// 核心方法
func (m *Manager) Start() error
func (m *Manager) Stop() error  
func (m *Manager) Restart() error
func (m *Manager) IsRunning() bool
func (m *Manager) GetLogs() ([]string, error)
```

**实现要点**：
- 使用 `exec.Command` 启动 sing-box
- 捕获 stdout/stderr 到日志文件
- 优雅关闭（SIGTERM -> SIGKILL）
- 进程状态监控

#### 2.2 健康检查 (health.go)

```go
type HealthChecker struct {
    manager       *Manager
    config        *config.Config
    ticker        *time.Ticker
    results       map[string]*HealthResult
    onFailure     func(proxyName string)
}

type HealthResult struct {
    ProxyName    string
    IsHealthy    bool
    LastCheck    time.Time
    Latency      time.Duration
    ErrorCount   int
}

// 核心方法
func (hc *HealthChecker) Start()
func (hc *HealthChecker) Stop()
func (hc *HealthChecker) CheckAll() map[string]*HealthResult
func (hc *HealthChecker) CheckProxy(proxyName string) *HealthResult
```

**健康检查流程**：
1. 每 60 秒检查一次所有启用的代理
2. 检查方式：通过代理访问测试 URL（如 Google 204）
3. 失败处理：
   - 记录错误次数
   - 连续失败 3 次触发故障恢复
   - 调用 DNS 刷新
   - 重启 sing-box 进程
4. 成功后重置错误计数

#### 2.3 DNS 缓存刷新 (dns.go)

```go
type DNSFlusher struct{}

func (f *DNSFlusher) FlushSystem() error {
    // Windows: ipconfig /flushdns
    // Linux: systemd-resolve --flush-caches
    // macOS: dscacheutil -flushcache
}

func (f *DNSFlusher) FlushSingBox(manager *Manager) error {
    // 方法1: 重启 sing-box 进程（简单有效）
    // 方法2: 如果 sing-box 提供 API，调用刷新接口
}
```

### 3. UI 模块 (internal/ui)

#### 3.1 系统托盘 (tray.go)

```go
type TrayUI struct {
    manager      *proxy.Manager
    health       *proxy.HealthChecker
    config       *config.Config
}

func (t *TrayUI) Run() error {
    // 初始化托盘图标
    // 构建菜单
    // 启动事件循环
}

func (t *TrayUI) UpdateMenu() {
    // 动态更新菜单内容
}
```

**托盘菜单结构**：
```
OneProxy
├── 🟢 运行中 / 🔴 已停止
├── ───────────────
├── 启动代理
├── 停止代理
├── 重启代理
├── ───────────────
├── 代理列表 ▶
│   ├── ✓ JMS-Server1 (45ms)
│   ├── ✓ JMS-Server2 (67ms)
│   └── ✗ JMS-Server3 (超时)
├── ───────────────
├── 健康检查: 已启用
├── 立即检查
├── ───────────────
├── 打开配置文件
├── 查看日志
├── ───────────────
├── 开机启动 ☑
├── 关于
└── 退出
```

**状态显示**：
- 托盘图标颜色：绿色（正常）、黄色（部分故障）、红色（停止/全部故障）
- 鼠标悬停提示：显示当前状态摘要
- 实时更新代理延迟

## 实现阶段

### Phase 1: 核心功能（MVP）

**目标**：实现基本的代理管理和进程控制

1. **项目初始化**
   - 创建 Go module
   - 设置项目目录结构
   - 配置 .gitignore

2. **配置管理**
   - 定义配置结构
   - 实现配置加载/保存
   - 实现 sing-box 配置生成器

3. **进程管理**
   - 实现 sing-box 进程启动/停止
   - 日志捕获
   - 状态监控

4. **系统托盘 UI**
   - 基础托盘图标
   - 启动/停止菜单
   - 退出功能

5. **测试**
   - 手动配置一个代理
   - 测试启动/停止功能
   - 验证代理连接

### Phase 2: 健康检查与 DNS 优化

**目标**：解决 IP 变更导致的连接问题

1. **健康检查模块**
   - 实现定期检查逻辑
   - TCP 连接测试
   - HTTP 测试 URL 访问

2. **DNS 刷新**
   - 实现系统 DNS 缓存刷新
   - sing-box 进程重启策略
   - 故障恢复流程

3. **UI 增强**
   - 显示代理健康状态
   - 显示延迟信息
   - 手动触发检查

4. **测试**
   - 模拟 IP 变更场景
   - 验证自动恢复
   - 验证 DNS 刷新效果

### Phase 3: 增强功能

**目标**：提升用户体验

1. **配置 UI**
   - 配置文件编辑器（可选）
   - 配置验证

2. **日志查看**
   - 日志查看器
   - 日志级别过滤

3. **开机启动**
   - Windows 注册表配置

4. **订阅更新（可选）**
   - 从订阅链接更新服务器列表

## sing-box 配置示例

根据用户配置生成的 sing-box 配置大致如下：

```json
{
  "log": {
    "level": "info",
    "output": "logs/singbox.log"
  },
  "dns": {
    "servers": [
      {
        "tag": "cloudflare",
        "address": "https://1.1.1.1/dns-query",
        "detour": "direct"
      },
      {
        "tag": "google", 
        "address": "https://8.8.8.8/dns-query",
        "detour": "direct"
      }
    ],
    "rules": [
      {
        "domain_suffix": [".portablesubmari"],
        "server": "cloudflare"
      }
    ],
    "strategy": "prefer_ipv4"
  },
  "inbounds": [
    {
      "type": "mixed",
      "tag": "mixed-in",
      "listen": "127.0.0.1",
      "listen_port": 1082
    }
  ],
  "outbounds": [
    {
      "type": "shadowsocks",
      "tag": "jms-server1",
      "server": "c331s1.portablesubmari",
      "server_port": 5299,
      "method": "aes-256-gcm",
      "password": "password"
    },
    {
      "type": "vmess",
      "tag": "jms-server2",
      "server": "c331s3.portablesubmari",
      "server_port": 5299,
      "uuid": "uuid",
      "alter_id": 0,
      "security": "auto"
    },
    {
      "type": "selector",
      "tag": "proxy",
      "outbounds": ["jms-server1", "jms-server2"],
      "default": "jms-server1"
    },
    {
      "type": "direct",
      "tag": "direct"
    }
  ],
  "route": {
    "rules": [
      {
        "protocol": "dns",
        "outbound": "direct"
      }
    ],
    "final": "proxy"
  }
}
```

## 关键技术决策

### 1. 单进程 vs 多进程
**决策**：单个 sing-box 进程管理所有代理
**理由**：sing-box 原生支持多 outbound，资源占用少，配置简单

### 2. 域名 vs IP
**决策**：优先使用域名，配合主动健康检查和 DNS 刷新
**理由**：
- JustMySocks 通过 DNS 自动更新 IP
- 健康检查可在几十秒内发现问题
- DNS 刷新可快速获取新 IP
- 无需维护 IP 订阅

### 3. 托盘库选择
**决策**：使用 [systray](https://github.com/getlantern/systray)
**理由**：
- 轻量级，专注于托盘功能
- 跨平台支持好
- API 简单直观
- 社区活跃

### 4. 健康检查策略
**决策**：60 秒间隔 + 连续失败 3 次触发恢复
**理由**：
- 60 秒平衡及时性和资源消耗
- 3 次失败避免误判
- 及时发现 IP 变更问题

### 5. DNS 刷新方式
**决策**：系统 DNS 刷新 + sing-box 进程重启
**理由**：
- 简单可靠
- 清除所有层级缓存
- 重启开销可接受（几秒）

## 依赖管理

### 外部二进制
- **sing-box**：从 GitHub Release 下载对应平台版本
  - 位置：`bin/sing-box.exe`
  - 版本：v1.13.14+

### Go 依赖
```go
require (
    github.com/getlantern/systray v1.2.2
    gopkg.in/yaml.v3 v3.0.1  // 可选：支持 YAML 配置
)
```

## 配置文件位置

- **用户配置**：`./config.json`（程序同目录）
- **sing-box 配置**：`./singbox_generated.json`（自动生成，不提交）
- **日志文件**：`./logs/`
- **sing-box 二进制**：`./bin/sing-box.exe`

## 测试计划

### 单元测试
- 配置加载/生成逻辑
- sing-box 配置生成器
- DNS 刷新命令执行

### 集成测试
- 完整启动/停止流程
- 健康检查触发恢复
- 配置变更热重载

### 手动测试场景
1. 正常启动代理，验证连接
2. 模拟 IP 变更（修改 hosts 文件），验证自动恢复
3. 禁用某个代理，验证健康检查跳过
4. 所有代理失败，验证状态显示

## 未来扩展

- [ ] Web 管理界面（可选）
- [ ] 流量统计
- [ ] 规则分流（GeoIP/GeoSite）
- [ ] 订阅链接导入
- [ ] 多配置文件支持（工作/家庭切换）
- [ ] API 接口（供其他程序调用）

## 风险与挑战

1. **sing-box 版本兼容性**
   - 缓解：使用稳定版本，固定配置格式

2. **Windows 权限问题**
   - 缓解：避免需要管理员权限的操作

3. **健康检查影响性能**
   - 缓解：可配置间隔，异步执行

4. **DNS 刷新可能失败**
   - 缓解：提供手动重启选项

## 下一步

如果这个规划通过 review，将按照 Phase 1 开始实施：
1. 初始化 Go 项目
2. 实现配置管理模块
3. 实现 sing-box 进程管理
4. 实现基础托盘 UI
5. 完成 MVP 测试
