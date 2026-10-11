# Windows 发布元数据与 SHA256

执行 `pwsh -NoProfile -File ./build.ps1 -Installer`。版本、产品和贡献者名称
沿用 `trayapp/installer.iss` 的 `AppVersion`、`AppName`、`AppPublisher`；
Windows 版本须为三个 0..65535 数字，文件版本自动补第四段 `.0`。
构建为 `oneproxy-tray.exe`、`oneproxy.dll` 嵌入 VERSIONINFO，并统一暂存安装
脚本的版本资源。DLL 的运行时版本也从同一来源注入。直接执行 Go/CMake
开发构建不会自动生成这些发布资源，发布必须使用 `build.ps1`。

需要现有 Go/MinGW（含 windres）、MSVC、Qt、CMake 3.19+ 和 Inno Setup。
临时 Go 资源对象在构建后删除；生成文件不提交。版本变化只需修改上述
安装脚本的版本定义，旧 `VersionInfoVersion` 会在暂存副本中覆盖。

输出 `dist/release-manifest.json` 包含产品、贡献者、版本、完整 Git 提交、
构建开始时工作树是否有修改，以及每个发布二进制/规则集/图标的相对仓库
路径、字节数、SHA256。安装包仅在 `-Installer` 构建时纳入，避免收录旧包。
清单不包含本机 `config.json`。Qt、MSVC、sing-box 保留其原始资源及身份，
仅记录摘要，不改写为 OneProxy。`productOwned` 标记三个自有产物。
收录范围限于安装脚本声明的根目录文件模式和部署子目录，不递归扫描
整个构建目录；CMakeFiles 内的编译器探测程序及其他构建辅助文件不收录。

```powershell
pwsh -NoProfile -File ./scripts/release-hashes.ps1 -Verify
```

校验会重新读取每个文件的长度和 SHA256，并核对自有产物的版本资源。
清单是校验与追溯材料，不是签名，也不能单独证明来源可信。
`OneProxy Contributors` 沿用现有项目署名，不代表法人或受信任证书身份。
本任务不改变 Authenticode 状态，不声明已签名，不修改第三方签名。

签名会改变文件内容。与签名任务集成时，须在 EXE/DLL 签名后打包，
再对安装包签名，最后使用 `scripts/release-hashes.ps1` 重新生成清单。
传入原构建的 `-Version`、`-Product`、`-Publisher`、`-BuildCommit`、
`-SourceDirty` 和 `-Installer`；不能拿签名时的新 HEAD 冒充原构建提交。
正式发布应使用干净且已提交的源码（`sourceDirty: false`），并将此清单
与安装包一起发布。当前 CI 只上传 EXE；上传清单需由发布流程另行接入。
本任务仅生成本地材料，不上传或公开发布。
