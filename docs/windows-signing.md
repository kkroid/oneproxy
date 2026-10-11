# Windows 发布签名与 SignPath Foundation 申请材料

## 当前状态

没有取得签名凭据或 Foundation 批准，不能宣称产物已获公信签名。本任务仅接入本地 Authenticode/SignTool 流程、执行无凭据验证并只读核查政策；未申请账号、上传文件、提交个人资料、批准签名、购买或发布。自签名证书不自动获得 Windows 公信信任，也不等于消除 SmartScreen 提示。

## 构建顺序和目标

| 阶段 | 本项目目标 | 行为 |
| --- | --- | --- |
| 封装前 | oneproxy-tray.exe、oneproxy.dll | sign-windows.ps1 -PrePackage；显式 Sign 时签名并验证 |
| Inno 编译 | 自动生成的卸载程序、最终安装包 | SignedUninstaller=yes；先签卸载程序再封装，最后签安装包 |
| 封装后 | EXE、DLL、安装包 | 只读 StrictVerify 与最终 SHA256 清单，不再次签名 |
| 第三方 | sing-box.exe、Qt、MSVC 运行库 | 不由 OneProxy 代签；保留已有字节/签名 |

普通构建保持未签名模式。以下两条均不会调用真实签名服务：

~~~powershell
pwsh -NoProfile -File .\build.ps1 -Installer
pwsh -NoProfile -File .\build.ps1 -Installer -SigningDryRun
~~~

SigningDryRun 仍执行正常编译和打包，要求本机已有构建工具与 sing-box；它在打包前后检查状态。它不是公信签名成功的证明。仅检查已有文件（没有工具链也能运行）：

~~~powershell
pwsh -NoProfile -File .\scripts\sign-windows.ps1 -VerifyOnly
pwsh -NoProfile -File .\scripts\sign-windows.ps1 -PrePackage -VerifyOnly
~~~

结果逐项显示 Path、Status（包括 NotSigned/Missing）、Timestamp 和 SHA256，长路径不会把状态列挤掉。缺失文件可用于 dry-run，但严格验证会失败。

有明确授权和现成证书后，发布环境可执行（本任务未执行真实签名）：

~~~powershell
pwsh -NoProfile -File .\build.ps1 -Installer -Sign `
  -SigningTool 'C:\Windows SDK\signtool.exe' `
  -SigningThumbprint '<certificate thumbprint>' `
  -SigningTimestampUrl 'http://timestamp.digicert.com'
~~~

证书选择也可使用 SigningPfxPath；必须只选一种来源。优先用证书存储/HSM；不把密码写进仓库、日志或命令历史。此构建接口不提供 PFX 密码参数。

build.ps1 和探针共同使用 scripts/sign-windows-command.ps1 构造 ISCC 参数：
- /DSignToolCommand：显式启用 trayapp/installer.iss 的签名条件块。
- /Soneproxy-sign=<command>：定义同名命令；使用 Inno 的 $q 表示引号、$f 表示带引号的目标路径，保留带空格的工具/PFX 路径。
- scripts/sign-windows-inno.ps1 接收生成的卸载程序/安装包，调用 SignTool sign /fd SHA256 /tr <URL> /td SHA256，并立即验证。
- 普通与 SigningDryRun 模式不传这些参数。Sign 与 SigningDryRun 不能同时使用，二者都要求 Installer。

封装后只读发布检查：

~~~powershell
pwsh -NoProfile -File .\scripts\sign-windows.ps1 -StrictVerify `
  -SignTool 'C:\Windows SDK\signtool.exe'
~~~

StrictVerify 和 Sign 的成功条件均包括：每个目标存在、Authenticode Status=Valid、TimeStamperCertificate 存在，以及 signtool verify /pa /all /tw 成功。缺失、未签名、无时间戳、信任验证失败均返回非零。证书对象存在本身不代表已验证公信链。卸载程序在 Inno 签名回调中验证，不要求安装真实代理；如已有合法提取的卸载程序，可用 -Uninstaller 检查。

## sing-box 来源和哈希

默认官方版本为 1.13.14，来源 https://github.com/SagerNet/sing-box/releases/tag/v1.13.14 。脚本记录来源 URL 与本地文件 SHA256，缺失时明确记为 Missing；该哈希记录只证明本地字节，不表示已同上游归档校验。维护者应从该官方版本获取归档并核对上游校验信息，保留归档名、下载 URL、归档哈希和解压后 EXE 哈希。

签名脚本在任何 signtool 调用前预检所有目标：拒绝文件名 sing-box.exe、已知第三方路径及 SHA256 相同的改名副本。-SingBox 必须指向实际打包的第三方样本才能识别改名副本；当参考样本不存在时仍拒绝 sing-box.exe 文件名。不能把未知改名二进制身份识别当作已实现的能力。所有第三方保持原字节，不因封装而以本项目证书重签。

## 无凭据回归验证

以下测试只生成独立证据目录内的样本，不运行安装包、不启动代理：

~~~powershell
pwsh -NoProfile -File .\scripts\sign-windows-conflict-test.ps1 -EvidenceDir '<outside-worktree evidence directory>'
pwsh -NoProfile -File .\scripts\sign-windows-inno-test.ps1 -EvidenceDir '<outside-worktree evidence directory>'
~~~

冲突测试生成合法无签名 PE、第三方样本、改名副本及假 signtool；覆盖 Installer/Uninstaller 的六种冲突、零签名调用、签名失败传播、NotSigned/时间戳显示、严格验证缺失/无签名失败和哈希不变。Inno 探针采用真实 installer.iss 的 Setup 条件块，普通/dry-run 编译成功；证书指纹/PFX 模式用假工具验证自动生成卸载程序、证书与 RFC3161 参数并刻意返回失败，证明编译终止。它不模拟受信任签名成功，实际签名成功路径仍需真实凭据验证。

## Foundation 专门政策核查（2026-10-10）

本节依据只读取得的 https://signpath.org/terms （Code of conduct），不是商业站通用首页。首页 https://signpath.org/ 说明对开源项目免费、由 Foundation 名义提供证书并使用 HSM；https://signpath.org/about 说明当前由 SignPath GmbH 运营。https://signpath.org/apply 返回申请页及嵌入式表单容器；未填写/提交表单。尝试 https://signpath.org/faq 返回 HTTP 404，未作为政策依据。

| 核查项 | 官方条款依据与适用要求 |
| --- | --- |
| 资格 | Terms → Conditions for free OSS SignPath.io subscriptions：无恶意或潜在不受欢迎程序；所有组件使用 OSI 认可许可证、无商业双重许可；不得含私有组件，允许符合定义的 System Libraries；项目活跃维护、已按拟签形式发布、有下载页功能说明。 |
| 项目认可 | Terms → Common misunderstandings：可执行程序须有可验证声誉；Foundation 自主接受/拒绝，无获得证书的权利或独立仲裁保证。OneProxy 未获批准。 |
| 地区 | 已读取的 Terms/About/Apply 静态正文没有国家/地区允许清单，不能据此断言个人所在地符合资格。申请人所在地、制裁/出口限制及服务适用性仍需官方确认；本次未提交个人信息。 |
| 发布者 | Terms → Conditions for free code signing certificates：证书签发给 SignPath Foundation，签名发布者是 Foundation；不冒充证书签发给 OneProxy Contributors。 |
| 本项目二进制 | Terms → Sign your own projects / binaries only：团队应维护源码和构建脚本并拥有仓库控制权。上游未签名库可包含在签名包内，但不可用本项目订阅替上游签名；sing-box/Qt/CRT 因而不代签。 |
| 安全与用户行为 | Terms → No hacking tools、Respect user privacy and security、Announce system changes、Provide uninstallation：不得包含指定攻击/规避功能；数据传输需披露并提供相应选项；系统设置修改需提醒，安装要有卸载路径。代理软件的实际功能是否符合由官方评估，不能预先承诺。 |
| MFA | Terms → Follow security best practices：所有团队成员对 SignPath 和源码仓库访问均必须使用多因素认证。 |
| 角色与逐次批准 | Terms → Assign code signing roles、Don’t fight the system：明确 Authors/Reviewers/Approvers；非提交者变更需审核；每次签名请求由受信任成员批准，每个发布需要人工签名批准。不能用脚本自动批准替代。 |
| 元数据 | Terms → Artifact configuration：所有签名二进制必须设置并用 file metadata restrictions 强制产品名为项目名，同一次构建所有产品版本相同。需包含 EXE、DLL、安装包和卸载程序，实际资源一致性留发布门禁核对。 |
| 网站声明 | Terms → Specify a code signing policy：主页/下载页使用 Code signing policy 标题或链接，列明角色、成员、隐私政策和官方要求的服务/证书归属语句。未获批前不得把“Free code signing provided by SignPath.io, certificate by SignPath Foundation”当本项目既成事实发布。 |
| Draft 与终止 | Terms 页面明确标为 Draft，指当前 Code of conduct 草案，**不是控制台待签名任务或额度字段**。违规可暂停/终止服务、立即或追溯撤销证书；可追加签名覆盖/SBOM 等要求；需协助调查；Foundation 不承担因签名软件导致损失的责任。 |

申请前准备材料（草稿，不提交）：仓库 https://github.com/kkroid/oneproxy；项目 OneProxy；根 LICENSE 为 MIT，仍需完整审计 sing-box/Qt/CRT 等依赖许可与分发条件；已发布版本/下载页及维护、声誉证据；可追溯构建工作流；仅本项目签名目标及第三方来源哈希；统一产品元数据；Authors/Reviewers/Approvers 实名角色映射和 MFA 执行证据；隐私、系统修改、卸载说明。维护者身份/地区、角色成员、项目资格和最终条款接受均待用户授权及官方审核，本文件不代表已申请或获批。
