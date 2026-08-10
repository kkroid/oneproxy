# CLAUDE.md — OneProxy 项目开发规范

## 构建环境

- **OS**: Windows 10/11, x64
- **Go**: 1.26+
- **MSVC**: Visual Studio 2022 C++ build tools
- **Qt6**: 6.8.3, MSVC 2022 x64
- **CMake**: available in PATH
- **Inno Setup**: 6（仅安装包构建需要）
- **sing-box**: 1.13.14，放在 `bin\sing-box.exe`

`build.ps1` 会通过 `vswhere.exe` 和 `qmake.exe` 查找 MSVC 与 Qt。非标准安装路径使用 `-VcVars`、`-QtDir`、`-InnoSetup` 参数或对应的 `ONEPROXY_*` 环境变量。

## 关键规则

### 1. 编译必须自己验证通过才能交给用户
- 运行 `go test ./...`、`go vet ./...` 和 `build.ps1 -Installer`
- 核对安装包版本、哈希与构建前后的现有代理 PID
- 不要把“编译通过”等同于“用户已完成安装验证”

### 2. 不干扰现有代理
- 构建和打包不得停止、重启或安装 OneProxy
- 不得按进程名批量终止 `oneproxy-tray.exe` 或 `sing-box.exe`
- 运行态验证和安装由明确负责验证的人执行

### 3. 任何报错，先查日志
- `~/.oneproxy/logs/singbox.log` 是第一优先级
- 不要猜原因，sing-box 日志写得非常清楚

### 4. 一个方案失败，先 debug，不要跳
- 用最小可复现代码隔离问题
- 不要因为"看起来不行"就换方案

### 5. 不要杀系统进程
- 绝对禁止 `taskkill explorer.exe`、`taskkill svchost.exe`
- 目录被占用换个目录名

## 运行时路径（关键）

**永远不要假设当前工作目录可写。** 程序可能被装在 `C:\Program Files\OneProxy\`。

| 数据类型 | 路径 |
|---------|------|
| 用户配置 config.json | 1) cwd 2) exeDir 3) `~/.oneproxy/` |
| sing-box 二进制 | `{exeDir}/bin/sing-box.exe` |
| 生成配置 | `~/.oneproxy/singbox_generated.json` |
| 日志 | `~/.oneproxy/logs/singbox.log` |
| sing-box 子进程 cwd | `~/.oneproxy/` |

## 构建流程

```powershell
.\build.ps1             # 编译规则集、DLL、tray，并运行 windeployqt
.\build.ps1 -Installer  # 额外生成安装包
# 输出: dist\OneProxy-0.6.0-setup.exe
```

生成的 DLL、EXE、Qt 部署目录和 `.srs` 规则集不进入版本控制。GitHub Actions 会下载并校验固定版本的 sing-box，然后使用同一个 `build.ps1` 构建。

## 调试检查清单

- [ ] 报错时先查 `~/.oneproxy/logs/singbox.log`
- [ ] `grep server ~/.oneproxy/singbox_generated.json` 确认用的是真实配置而非示例
- [ ] 构建前后现有代理 PID 保持不变
- [ ] 安装后 config.json 在 `~/.oneproxy/`，不在 Program Files

## 已知陷阱

- **端口占用**: 不要终止现有代理；改用未占用端口或由用户安排运行态验证
- **ctypes c_char_p**: 用 `restype = ctypes.c_void_p`，不能 `c_char_p`
- **MSVC 环境**: cmake/nmake 必须在 vcvars 激活的 cmd session 中
- **Program Files 不可写**: 所有运行时产物写到 `~/.oneproxy/`
- **config.json 密码**: 永不提交
