# OneProxy - 项目状态文档

## 项目概览

OneProxy 是一个基于 sing-box 的**本地代理聚合器**，将多个上游代理服务器同时暴露为独立的本地 SOCKS5 端口，实现真正的多代理并行、互不干扰。

**核心理念**：不是另一个代理客户端，而是代理端口聚合器。

## 🎯 项目定位

### 与现有客户端的根本区别

| 特性 | NekoBox/Hiddify | OneProxy |
|------|-----------------|----------|
| **设计目标** | 单一出口，自动/手动切换节点 | **多端口并行，固定绑定** |
| **节点使用** | 同时只能用一个节点 | **所有节点同时可用** |
| **切换方式** | GUI 点击切换 | **直接换端口号** |
| **应用分流** | 需要配置复杂规则 | **手动指定端口，简单直接** |
| **密码管理** | 配置中可见 | **完全隐藏，只暴露端口** |
| **使用场景** | 个人日常上网 | **开发测试、多应用分流、内网共享** |

### 目标用户

1. **开发者** - 需要同时测试多个节点，快速切换
2. **重度用户** - 多应用并行，精确控制每个应用使用的节点
3. **团队协作** - 内网共享代理，隐藏上游密码
4. **节点管理员** - 管理多个 JustMySocks 节点，提供统一接口

## ✅ Phase 1: 核心功能（已完成部分）

### 已实现 ✅

1. **项目结构** ✅
   - [x] Go module 初始化
   - [x] 标准项目目录结构
   - [x] .gitignore 配置
   - [x] 依赖管理 (go.mod)

2. **基础配置管理** ✅
   - [x] 用户配置结构定义 (`internal/config/config.go`)
   - [x] 配置加载/保存/验证
   - [x] 示例配置文件 (`configs/config.example.json`)

3. **代理进程管理** ✅
   - [x] sing-box 进程启动/停止/重启 (`internal/proxy/manager.go`)
   - [x] 进程状态监控
   - [x] 日志捕获和管理
   - [x] 优雅关闭机制

4. **系统托盘 UI** ✅
   - [x] 托盘图标和菜单 (`internal/ui/tray.go`)
   - [x] 启动/停止/重启功能
   - [x] 配置文件和日志快速访问

5. **主程序** ✅
   - [x] 程序入口 (`cmd/oneproxy/main.go`)
   - [x] 配置加载逻辑
   - [x] 二进制文件检查

6. **文档和工具** ✅
   - [x] README.md - 项目说明（已更新为新架构）
   - [x] QUICKSTART.md - 快速入门指南（已更新）
   - [x] DEVELOPMENT.md - 开发文档
   - [x] LICENSE - MIT 许可证
   - [x] Makefile - 构建脚本
   - [x] download-singbox.bat/sh - 自动下载脚本

### 待修改（新需求适配）⏳

**根据新的多端口并行需求，需要调整**：

1. **配置结构修改** ⏳
   - [ ] `ProxyConfig` 添加 `LocalPort` 字段
   - [ ] `InboundConfig` 改为 `Listen` + `ProxyType`
   - [ ] 移除旧的 `SocksPort/HTTPPort/MixedPort`
   - [ ] `DNSConfig` 添加 `FlushIntervalSeconds`
   - [ ] 端口冲突检测逻辑

2. **sing-box 配置生成器重写** ⏳
   - [ ] 每个代理生成一个独立 inbound
   - [ ] 一对一端口绑定（通过 route 规则）
   - [ ] 移除 Selector 和 URLTest
   - [ ] 固定绑定，不自动切换

3. **托盘 UI 更新** ⏳
   - [ ] 显示每个节点的本地端口号
   - [ ] 复制端口地址到剪贴板
   - [ ] 节点列表格式：`✓ JMS-Server1 [:10801] (45ms)`

4. **示例配置更新** ⏳
   - [ ] 更新 `config.example.json`
   - [ ] 添加 `local_port` 示例
   - [ ] 更新注释说明

## 🚀 Phase 2: 健康检查与 DNS 优化（规划中）

**目标**：解决 IP 变更导致的连接问题

### 核心功能

1. **独立健康检查** 🔜
   - [ ] 每个节点独立检查（通过本地端口）
   - [ ] 定期检查（默认 60 秒）
   - [ ] 延迟测试和显示
   - [ ] 连续失败检测（3 次触发恢复）

2. **DNS 快速刷新** 🔜
   - [ ] 系统 DNS 缓存刷新（`ipconfig /flushdns`）
   - [ ] sing-box 进程重启（清除内部缓存）
   - [ ] 定期刷新（默认 5 分钟）
   - [ ] 失败时立即刷新
   - [ ] 防抖逻辑（避免频繁刷新）

3. **UI 增强** 🔜
   - [ ] 实时显示每个节点延迟
   - [ ] 健康状态图标（✓ 绿色 / ✗ 红色）
   - [ ] 手动触发健康检查按钮
   - [ ] 手动触发 DNS 刷新按钮
   - [ ] 上次检查时间显示

4. **通知功能** 🔜
   - [ ] 节点故障通知
   - [ ] DNS 刷新通知
   - [ ] 恢复成功通知

### 技术实现

```go
// internal/proxy/health.go
type HealthChecker struct {
    manager   *Manager
    config    *config.Config
    ticker    *time.Ticker
    results   map[string]*HealthResult
}

type HealthResult struct {
    ProxyName    string
    LocalPort    int
    IsHealthy    bool
    Latency      time.Duration
    LastCheck    time.Time
    ErrorCount   int
    LastError    string
}

// internal/proxy/dns.go
type DNSFlusher struct {
    lastFlush    time.Time
    flushLock    sync.Mutex
}
```

## 📋 Phase 3: 增强功能（远期规划）

1. **用户体验优化**
   - [ ] 一键复制所有端口地址
   - [ ] 配置文件编辑器（GUI）
   - [ ] 日志查看器（过滤、搜索）
   - [ ] 主题切换（明暗模式）

2. **高级功能**
   - [ ] 开机自启动（Windows 注册表）
   - [ ] 配置热重载
   - [ ] 端口可用性检测
   - [ ] 订阅链接导入（可选）

3. **网络功能**
   - [ ] 内网访问控制（IP 白名单）
   - [ ] 流量统计（每个端口独立）
   - [ ] 带宽限制

4. **跨平台支持**
   - [ ] Linux 版本
   - [ ] macOS 版本

## 📊 当前代码统计

### 文件清单

```
OneProxy/
├── cmd/oneproxy/main.go              # 程序入口 (2.7KB)
├── internal/
│   ├── config/
│   │   ├── config.go                 # 配置管理 (4.5KB)
│   │   └── singbox.go                # sing-box 配置生成 (5.2KB) [需重写]
│   ├── proxy/
│   │   └── manager.go                # 进程管理 (4.8KB)
│   └── ui/
│       └── tray.go                   # 托盘界面 (5.1KB) [需更新]
├── configs/
│   └── config.example.json           # 配置示例 (0.7KB) [需更新]
├── README.md                         # 项目说明 (已更新 18KB)
├── QUICKSTART.md                     # 快速入门 (已更新 8KB)
├── DEVELOPMENT.md                    # 开发文档 (8.3KB)
├── PROJECT_STATUS.md                 # 本文档
├── LICENSE                           # MIT 许可证
├── Makefile                          # 构建脚本 (3.3KB)
├── download-singbox.bat              # Windows 下载脚本 (2.3KB)
├── download-singbox.sh               # Linux/macOS 下载脚本 (2.2KB)
└── oneproxy.exe                      # 编译后的可执行文件 (6.7MB)
```

### 代码规模

- **Go 源代码**: 7 个文件，约 1200 行
- **文档**: 5 个文件，约 35KB
- **配置**: 1 个示例文件
- **脚本**: 3 个（Makefile + 2个下载脚本）

## 🔧 核心架构

### 多端口并行设计

```
JustMySocks 5个节点 (各有密码/UUID)
                ↓
        OneProxy 统一管理
                ↓
    sing-box 单进程运行
                ↓
    5 个独立 inbound 端口
                ↓
127.0.0.1:10801 → JMS-Server1 (Shadowsocks)
127.0.0.1:10802 → JMS-Server2 (Shadowsocks)
127.0.0.1:10803 → JMS-Server3 (VMess)
127.0.0.1:10804 → JMS-Server4 (VMess)
127.0.0.1:10805 → JMS-Server5 (Shadowsocks)
```

### sing-box 配置原理

**当前生成的配置**（需要修改）：
- 单一 Mixed/SOCKS inbound
- 多个 outbound
- Selector 自动选择

**目标配置**（新架构）：
- **每个代理一个独立 inbound 端口**
- **每个 inbound 固定绑定到对应 outbound**
- **没有 Selector，没有自动切换**

```json
{
  "inbounds": [
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10801, "tag": "in-1"},
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10802, "tag": "in-2"},
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10803, "tag": "in-3"}
  ],
  "outbounds": [
    {"type": "shadowsocks", "tag": "out-1", "server": "..."},
    {"type": "shadowsocks", "tag": "out-2", "server": "..."},
    {"type": "vmess", "tag": "out-3", "server": "..."}
  ],
  "route": {
    "rules": [
      {"inbound": ["in-1"], "outbound": "out-1"},
      {"inbound": ["in-2"], "outbound": "out-2"},
      {"inbound": ["in-3"], "outbound": "out-3"}
    ]
  }
}
```

### DNS 优化策略

**为什么使用域名**：
- JustMySocks 自动更新 DNS 记录
- 配置一次，长期有效
- 结合快速 DNS 刷新，恢复时间短

**DNS 刷新流程**：
1. 健康检查检测到连续失败 3 次
2. 触发 DNS 刷新
3. 刷新系统 DNS 缓存
4. 重启 sing-box 进程
5. 重新解析域名，获取最新 IP
6. 重新建立连接

**恢复时间对比**：
- 不刷新：2-5 分钟（依赖 DNS TTL）
- 主动刷新：30-60 秒

## 🎯 下一步行动

### 立即执行（Phase 1 完善）

**优先级 P0**：

1. **修改配置结构** (30 分钟)
   - 添加 `LocalPort` 字段
   - 更新 `InboundConfig`
   - 端口冲突检测

2. **重写 sing-box 生成器** (1 小时)
   - 一对一端口绑定
   - 移除 Selector 逻辑
   - 生成独立 inbound

3. **更新托盘 UI** (1 小时)
   - 显示端口号
   - 复制地址功能
   - 更新菜单格式

4. **更新示例配置** (15 分钟)
   - 添加 `local_port`
   - 完整示例

5. **测试验证** (30 分钟)
   - 构建项目
   - 测试多端口
   - 验证端口绑定

**预计时间**：3-4 小时

### Phase 2 开发（健康检查）

**优先级 P1**：

- 实现健康检查模块
- DNS 刷新功能
- UI 显示延迟和状态

**预计时间**：1-2 天

## 📝 重要说明

### 项目价值

**有独特价值**：
- 多端口并行，现有客户端不支持
- 固定绑定，精确控制
- 开发测试、多应用分流的刚需

**不是替代品**：
- 不替代 NekoBox/Hiddify（它们是通用客户端）
- 是专用工具，解决特定场景问题

### 技术债务

- [ ] 项目未初始化 git（建议初始化）
- [ ] 缺少单元测试（Phase 2 添加）
- [ ] 图标使用占位符（需要设计真实图标）
- [ ] 错误处理可以更完善

### 已知限制

1. 当前仅支持 Windows（代码已预留跨平台能力）
2. 配置修改需要重启进程（Phase 3 支持热重载）
3. 托盘图标是占位符（需要设计）
4. 日志查看需要手动打开目录（Phase 3 添加日志查看器）

## 🐛 已知问题

暂无已知问题。Phase 1 基础功能完整且经过验证。

## ✅ 验证清单

### Phase 1 已验证 ✅

- [x] 项目结构清晰
- [x] 代码编译通过
- [x] 配置管理完整
- [x] 进程管理稳定
- [x] UI 界面可用
- [x] 文档完善
- [x] 构建工具齐全

### Phase 1 待验证 ⏳

- [ ] 多端口配置生成正确
- [ ] 端口固定绑定有效
- [ ] 托盘显示端口号
- [ ] 复制地址功能
- [ ] 端口冲突检测

## 📞 支持与反馈

- **文档**: README.md, QUICKSTART.md, DEVELOPMENT.md
- **Issues**: https://github.com/kkroid/oneproxy/issues
- **代码**: https://github.com/kkroid/oneproxy

---

**当前状态**: ✅ Phase 1 基础完成，⏳ 等待架构调整后进入 Phase 2

**最后更新**: 2026-07-13  
**版本**: v0.1.0-alpha (Phase 1)
