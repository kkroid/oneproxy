# OneProxy 开发文档

本文档面向开发者，介绍 OneProxy 的代码结构、开发流程和贡献指南。

## 开发环境设置

### 前置要求

- Go 1.21 或更高版本
- Git
- sing-box 二进制文件（用于测试）

### 克隆项目

```bash
git clone https://github.com/kkroid/oneproxy.git
cd oneproxy
```

### 安装依赖

```bash
make deps
# 或
go mod download
```

### 下载 sing-box

```bash
make download-singbox
# 或手动下载到 bin/ 目录
```

## 项目结构

```
OneProxy/
├── cmd/
│   └── oneproxy/
│       └── main.go              # 程序入口，初始化和启动
├── internal/
│   ├── config/
│   │   ├── config.go           # 配置结构和加载/保存逻辑
│   │   └── singbox.go          # sing-box 配置生成器
│   ├── proxy/
│   │   ├── manager.go          # sing-box 进程管理
│   │   ├── health.go           # 健康检查（Phase 2）
│   │   └── dns.go              # DNS 缓存刷新（Phase 2）
│   └── ui/
│       ├── tray.go             # 系统托盘界面
│       └── menu.go             # 菜单构建（待实现）
├── configs/
│   └── config.example.json     # 配置文件示例
├── bin/                         # sing-box 二进制
├── logs/                        # 日志文件
├── go.mod                       # Go 模块定义
├── go.sum                       # 依赖校验和
├── Makefile                     # 构建脚本
└── README.md                    # 项目说明
```

## 核心模块说明

### 1. Config 模块 (internal/config)

**职责**: 管理用户配置和 sing-box 配置生成

#### config.go

- `Config` - 主配置结构
- `Load()` - 从 JSON 文件加载配置
- `Save()` - 保存配置到文件
- `Validate()` - 验证配置有效性
- `GetEnabledProxies()` - 获取启用的代理列表

#### singbox.go

- `SingBoxGenerator` - sing-box 配置生成器
- `Generate()` - 生成 sing-box 配置
- `SaveToFile()` - 保存到文件

**工作流程**:
```
用户配置 (config.json)
    ↓
Config.Load()
    ↓
SingBoxGenerator.Generate()
    ↓
sing-box 配置 (singbox_generated.json)
```

### 2. Proxy 模块 (internal/proxy)

**职责**: 管理 sing-box 进程生命周期

#### manager.go

- `Manager` - 进程管理器
- `Start()` - 启动 sing-box
- `Stop()` - 停止 sing-box
- `Restart()` - 重启 sing-box
- `IsRunning()` - 检查运行状态
- `GetLogs()` - 获取日志

**关键实现**:

```go
// 启动流程
func (m *Manager) Start() error {
    1. 检查二进制文件存在
    2. 检查配置文件存在
    3. 创建日志文件
    4. 启动进程 (exec.Command)
    5. 后台监控进程
}

// 停止流程
func (m *Manager) Stop() error {
    1. 发送 SIGINT（优雅关闭）
    2. 等待 5 秒
    3. 超时则发送 SIGKILL（强制终止）
}
```

### 3. UI 模块 (internal/ui)

**职责**: 系统托盘界面管理

#### tray.go

- `TrayUI` - 托盘界面
- `Run()` - 启动托盘
- `buildMenu()` - 构建菜单
- `eventLoop()` - 处理用户交互

**菜单结构**:

```
状态显示
├── 启动/停止/重启
├── 代理列表（子菜单）
├── 工具（配置、日志）
└── 退出
```

## 构建和测试

### 构建

```bash
# 当前平台
make build

# Windows
make build-windows

# Linux
make build-linux

# macOS
make build-darwin

# 所有平台
make build-all
```

### 运行

```bash
# 构建并运行
make run

# 直接运行
./oneproxy.exe
```

### 测试

```bash
# 运行所有测试
make test

# 测试特定包
go test ./internal/config
```

## 开发流程

### 添加新功能

1. **创建功能分支**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **实现功能**
   - 遵循现有代码风格
   - 添加必要的注释
   - 编写单元测试

3. **测试**
   ```bash
   make test
   make build
   # 手动测试功能
   ```

4. **提交**
   ```bash
   git add .
   git commit -m "feat: your feature description"
   ```

5. **推送和创建 PR**
   ```bash
   git push origin feature/your-feature-name
   # 在 GitHub 创建 Pull Request
   ```

### 代码规范

#### Go 代码风格

- 遵循 [Effective Go](https://golang.org/doc/effective_go)
- 使用 `gofmt` 格式化代码
- 导出的函数和类型必须有注释
- 错误处理要明确，不要忽略错误

#### 提交信息规范

使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```
feat: 添加新功能
fix: 修复 bug
docs: 文档更新
style: 代码格式调整
refactor: 代码重构
test: 添加测试
chore: 构建/工具链更新
```

示例：
```
feat: add health check for proxies
fix: resolve DNS cache issue on Windows
docs: update README with installation steps
```

## 调试技巧

### 1. 查看日志

```bash
# sing-box 日志
cat logs/singbox.log

# 实时查看
tail -f logs/singbox.log
```

### 2. 调试 sing-box 配置

```bash
# 检查生成的配置
cat singbox_generated.json

# 手动测试 sing-box
bin/sing-box.exe check -c singbox_generated.json
bin/sing-box.exe run -c singbox_generated.json
```

### 3. Go 调试

使用 [Delve](https://github.com/go-delve/delve)：

```bash
# 安装 Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# 调试运行
dlv debug ./cmd/oneproxy
```

### 4. 常见问题排查

**问题**: 托盘图标不显示
```
检查: systray 库是否正确初始化
日志: 查看是否有 panic
```

**问题**: sing-box 启动失败
```
检查: 配置文件格式
日志: logs/singbox.log
验证: sing-box check -c singbox_generated.json
```

**问题**: 代理无法连接
```
检查: 防火墙设置
验证: 直接用 sing-box 测试
测试: curl -x socks5://127.0.0.1:1080 https://google.com
```

## Phase 2 开发计划

### 健康检查模块 (internal/proxy/health.go)

**需要实现**:

```go
type HealthChecker struct {
    manager   *Manager
    config    *config.Config
    ticker    *time.Ticker
    results   map[string]*HealthResult
}

func (hc *HealthChecker) Start()
func (hc *HealthChecker) Stop()
func (hc *HealthChecker) CheckAll() map[string]*HealthResult
func (hc *HealthChecker) CheckProxy(name string) *HealthResult
```

**实现要点**:
- 使用 goroutine 定期检查
- 通过代理访问测试 URL
- 记录延迟和失败次数
- 失败触发 DNS 刷新和重启

### DNS 刷新模块 (internal/proxy/dns.go)

**需要实现**:

```go
type DNSFlusher struct{}

func (f *DNSFlusher) FlushSystem() error
func (f *DNSFlusher) FlushSingBox(manager *Manager) error
```

**实现要点**:
- Windows: `ipconfig /flushdns`
- Linux: `systemd-resolve --flush-caches`
- macOS: `dscacheutil -flushcache`
- 重启 sing-box 进程清除内部缓存

### UI 增强

**需要添加**:
- 实时显示代理延迟
- 健康状态指示（✓ ✗）
- 手动触发健康检查
- 显示上次检查时间

## 贡献指南

### 报告 Bug

使用 GitHub Issues，提供：
- 操作系统和版本
- OneProxy 版本
- 复现步骤
- 错误日志
- 预期行为 vs 实际行为

### 功能请求

使用 GitHub Issues，描述：
- 使用场景
- 期望功能
- 可能的实现方案

### 提交代码

1. Fork 项目
2. 创建功能分支
3. 实现功能并测试
4. 提交 Pull Request
5. 等待 Code Review

### Code Review 标准

- 代码风格一致
- 有充分的注释
- 有单元测试
- 不引入新的依赖（除非必要）
- 通过所有 CI 检查

## 发布流程

### 版本号规则

遵循 [Semantic Versioning](https://semver.org/)：

- `1.0.0` - 主版本.次版本.修订号
- `1.0.0-beta.1` - 预发布版本

### 发布步骤

1. **更新版本号**
   ```bash
   # 更新 main.go 中的版本常量
   ```

2. **创建 Git Tag**
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```

3. **构建所有平台**
   ```bash
   make build-all
   ```

4. **创建 GitHub Release**
   - 上传二进制文件
   - 编写 Release Notes
   - 标记 Pre-release（如果是测试版）

## 资源链接

- [sing-box 文档](https://sing-box.sagernet.org/)
- [systray 库](https://github.com/getlantern/systray)
- [Go 官方文档](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go)

## 联系方式

- GitHub Issues: https://github.com/kkroid/oneproxy/issues
- Email: [待添加]

---

感谢你对 OneProxy 的贡献！🎉
