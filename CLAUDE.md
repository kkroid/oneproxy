# CLAUDE.md — OneProxy 项目开发规范

## 环境信息

- **OS**: Windows 11 Pro, x64
- **Shell**: PowerShell (terminal), MSYS2 bash (Claude CLI 环境)
- **Go**: 1.21+, available in both MSYS2 and Windows PATH
- **MSVC**: VS 2022 Community, vcvars64.bat at `C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\`
- **Qt6**: 6.8.3 MSVC 2022, at `C:\Qt\6.8.3\msvc2022_64\`
- **CMake**: at `D:\cmake-3.30.4-windows-x86_64\bin\cmake.exe`
- **Python**: miniconda3 at `C:\Users\kkroid\miniconda3\python.exe`
- **Git**: available

## 关键规则

### 1. 编译必须自己验证通过才能交给用户
- Go 编译在 MSYS2 bash 环境执行：`go build -o oneproxy.exe ./cmd/oneproxy`
- **不要假设 PowerShell 和 MSYS2 bash 相同**。脚本如果给用户用，必须能在 PowerShell 执行
- 任何构建脚本、命令，必须在干净的 build 目录从头跑一遍，确认无报错

### 2. 测试必须端到端
- 编译完后立即 `./oneproxy.exe`，等 3 秒，`curl -x socks5://127.0.0.1:10801 https://ip.sb`
- `taskkill //F //IM sing-box.exe` 清理副作用
- 不要把"编译通过"等同于"能工作"

### 3. 搜索现有方案 > 自造方案
- 用户机器上有 Qt6 应用正常运行（豆包/微信/vmware-tray 都在用 Qt QSystemTrayIcon）
- OneLLMRouter 的托盘在你的机器上已验证可用
- 优先选已验证的路径，不要"重新发明"图标生成

### 4. 一个方案失败，先 debug，不要跳
- 先查日志（`logs/singbox.log`、stderr、Windows Event Viewer）
- 用最小可复现代码隔离问题
- 不要因为"看起来不行"就换方案

### 5. 先做减法，后做加法
- 别写 2000 行文档、8 个 MD 文件
- 功能完整、代码干净 > 文档冗长
- 保持项目目录清晰，不要累积失败的实验代码

### 6. 不要杀系统进程
- 绝对禁止 `taskkill explorer.exe`、`taskkill svchost.exe` 等系统进程
- 目录被占用用 `handle.exe` 查具体句柄，或换个目录名

## 项目结构

```
OneProxy/
├── cmd/
│   ├── oneproxy/          # Go CLI 守护进程
│   └── oneproxy-dll/      # Go C 共享库 DLL
├── internal/
│   ├── config/            # 配置管理 + sing-box 配置生成
│   └── proxy/             # manager.go, health.go, dns.go
├── trayapp/               # C++ Qt6 托盘 (MSVC + CMake)
│   ├── CMakeLists.txt
│   └── main.cpp
├── configs/
│   └── config.example.json
├── config.json            # 用户配置 (gitignored)
├── build.ps1              # 构建脚本 (PowerShell)
├── go.mod / go.sum
└── README.md
```

## 构建流程

```powershell
# PowerShell 执行
.\build.ps1
```

构建步骤：
1. `go build -buildmode=c-shared -o oneproxy.dll ./cmd/oneproxy-dll`
2. MSVC vcvars64 + cmake + nmake 构建 `trayapp/build/oneproxy-tray.exe`
3. `windeployqt` 部署 Qt6 运行时到 build 目录
4. 复制 `oneproxy.dll`、`bin/sing-box.exe`、`config.json` 到 build 目录

## 自测流程

```bash
# CLI 模式
go build -o oneproxy.exe ./cmd/oneproxy
taskkill //F //IM sing-box.exe 2>/dev/null; taskkill //F //IM oneproxy.exe 2>/dev/null
rm -f logs/singbox.log
./oneproxy.exe &
sleep 4
for port in 10801 10802 10803 10804 10805 10806; do
  curl -x socks5://127.0.0.1:$port https://ip.sb -s --connect-timeout 8 --max-time 12
done
taskkill //F //IM oneproxy.exe
```

## 已知陷阱

- **端口占用**: 每次启动前先 kill 残留的 sing-box/oneproxy 进程
- **ctypes c_char_p 陷阱**: Python DLL 绑定用 `restype = ctypes.c_void_p`，不能用 `c_char_p`
- **MSVC 环境**: cmake/nmake 必须在 vcvars 激活的 cmd session 中运行
- **config.json 密码**: 用户凭证，永不提交
