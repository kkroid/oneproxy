# CLAUDE.md — OneProxy 项目开发规范

## 环境信息

- **OS**: Windows 11 Pro, x64
- **Shell**: PowerShell (terminal), MSYS2 bash (Claude CLI 环境)
- **Go**: 1.21+, available in both MSYS2 and Windows PATH
- **MSVC**: VS 2022 Community, vcvars64.bat at `C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\`
- **Qt6**: 6.8.3 MSVC 2022, at `C:\Qt\6.8.3\msvc2022_64\`
- **CMake**: at `D:\cmake-3.30.4-windows-x86_64\bin\cmake.exe`
- **Python**: miniconda3 at `C:\Users\kkroid\miniconda3\python.exe`
- **Inno Setup 6**: at `C:\Program Files (x86)\Inno Setup 6\ISCC.exe`
- **Git**: available

## 关键规则

### 1. 编译必须自己验证通过才能交给用户
- DLL 测试：`go build -buildmode=c-shared` → 复制到 trayapp/build → `python3 -c` 加载 DLL → Start → curl 验证端口 → Stop
- `taskkill //F //IM sing-box.exe` 清理副作用
- 不要把"编译通过"等同于"能工作"

### 2. 测试必须端到端
- DLL 级别：同上
- 便携模式：`.\trayapp\build\oneproxy-tray.exe` 从项目根启动
- 安装模式：先便携验证通过 → 打包 installer → 安装到 Program Files → 再次验证
- 两种模式的 cwd 和写权限完全不同

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
.\build.ps1                         # 编译 DLL + tray + windeployqt
cp trayapp\installer.iss trayapp\build\
& 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe' trayapp\build\installer.iss
# 输出: dist\OneProxy-0.5.0-setup.exe
```

## 调试检查清单

- [ ] 报错时先查 `~/.oneproxy/logs/singbox.log`
- [ ] `grep server ~/.oneproxy/singbox_generated.json` 确认用的是真实配置而非示例
- [ ] 先便携模式验证，再安装模式测试
- [ ] 安装后 config.json 在 `~/.oneproxy/`，不在 Program Files

## 已知陷阱

- **端口占用**: 每次启动前先 kill 残留进程
- **ctypes c_char_p**: 用 `restype = ctypes.c_void_p`，不能 `c_char_p`
- **MSVC 环境**: cmake/nmake 必须在 vcvars 激活的 cmd session 中
- **Program Files 不可写**: 所有运行时产物写到 `~/.oneproxy/`
- **config.json 密码**: 永不提交
