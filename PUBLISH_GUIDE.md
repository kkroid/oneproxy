# OneProxy 发布指南

## ✅ 已完成的工作

### 1. Git 仓库初始化 ✅
```bash
git init
```

### 2. .gitignore 配置 ✅
已创建 `.gitignore`，排除：
- 可执行文件 (*.exe)
- sing-box 二进制
- 生成的配置
- 用户配置
- 日志文件
- IDE 文件

### 3. Git 提交 ✅
```bash
# 首次提交
git add .
git commit -m "feat: initial release of OneProxy v0.2.0"

# Release 文档提交
git add RELEASE_NOTES.md GITHUB_RELEASE.md
git commit -m "docs: add GitHub release documentation"
```

### 4. Git 标签 ✅
```bash
git tag -a v0.2.0 -m "Release v0.2.0 - Initial Release"
```

### 5. 发布文档 ✅
- ✅ `RELEASE_NOTES.md` - 完整的 Release 说明
- ✅ `GITHUB_RELEASE.md` - GitHub Release 页面文本

---

## 📋 接下来的步骤

### 1. 创建 GitHub 仓库

1. 访问 https://github.com/new
2. 填写仓库信息：
   - **Repository name**: `oneproxy`
   - **Description**: `Multi-port proxy aggregator - Expose multiple proxy servers as independent local ports`
   - **Public** or **Private**: 选择 Public
   - **不要**勾选 "Add a README file"（我们已经有了）
   - **不要**勾选 "Add .gitignore"（我们已经有了）
   - **License**: 选择 "MIT License"（我们已经有了）

3. 点击 "Create repository"

### 2. 推送代码到 GitHub

```bash
# 添加远程仓库（替换 YOUR_USERNAME）
git remote add origin https://github.com/YOUR_USERNAME/oneproxy.git

# 推送主分支
git push -u origin master

# 推送标签
git push origin v0.2.0
```

### 3. 创建 GitHub Release

#### 方法 A: 使用 GitHub 网页界面

1. 访问仓库页面
2. 点击右侧 "Releases" → "Create a new release"
3. 填写信息：
   - **Choose a tag**: 选择 `v0.2.0`
   - **Release title**: `v0.2.0 - Initial Release 🎉`
   - **Description**: 复制粘贴 `GITHUB_RELEASE.md` 的内容
4. **Assets** - 上传文件：
   - `oneproxy.exe` - Windows 可执行文件
   - `oneproxy-v0.2.0-windows-amd64.zip` - 压缩包（可选）

5. 勾选 **"Set as the latest release"**
6. 点击 **"Publish release"**

#### 方法 B: 使用 GitHub CLI (gh)

```bash
# 安装 gh (如果还没有)
# Windows: scoop install gh

# 登录
gh auth login

# 创建 Release
gh release create v0.2.0 \
  --title "v0.2.0 - Initial Release 🎉" \
  --notes-file GITHUB_RELEASE.md \
  oneproxy.exe

# 如果有压缩包
gh release create v0.2.0 \
  --title "v0.2.0 - Initial Release 🎉" \
  --notes-file GITHUB_RELEASE.md \
  oneproxy.exe \
  oneproxy-v0.2.0-windows-amd64.zip
```

---

## 📦 准备 Release Assets

### 创建 Windows 发布包

```bash
# 确保最新构建
make build

# 创建发布目录
mkdir -p release/oneproxy-v0.2.0-windows-amd64

# 复制必要文件
cp oneproxy.exe release/oneproxy-v0.2.0-windows-amd64/
cp README.md release/oneproxy-v0.2.0-windows-amd64/
cp QUICKSTART.md release/oneproxy-v0.2.0-windows-amd64/
cp LICENSE release/oneproxy-v0.2.0-windows-amd64/
cp configs/config.example.json release/oneproxy-v0.2.0-windows-amd64/
cp download-singbox.bat release/oneproxy-v0.2.0-windows-amd64/
mkdir release/oneproxy-v0.2.0-windows-amd64/bin
mkdir release/oneproxy-v0.2.0-windows-amd64/logs

# 压缩
cd release
zip -r oneproxy-v0.2.0-windows-amd64.zip oneproxy-v0.2.0-windows-amd64/
cd ..
```

### 可选：创建便携版

```bash
# 包含 sing-box（如果已下载）
cp bin/sing-box.exe release/oneproxy-v0.2.0-windows-amd64-portable/bin/

# 压缩便携版
cd release
zip -r oneproxy-v0.2.0-windows-amd64-portable.zip oneproxy-v0.2.0-windows-amd64-portable/
cd ..
```

---

## 📝 Release Assets 清单

上传到 GitHub Release 的文件：

1. **oneproxy.exe** (必需)
   - Windows 可执行文件
   - ~6.8 MB

2. **oneproxy-v0.2.0-windows-amd64.zip** (推荐)
   - 包含程序、文档、示例配置
   - ~7 MB

3. **oneproxy-v0.2.0-windows-amd64-portable.zip** (可选)
   - 包含 sing-box 二进制的便携版
   - ~15 MB

4. **Source code (zip)** (自动)
   - GitHub 自动生成

5. **Source code (tar.gz)** (自动)
   - GitHub 自动生成

---

## 🔍 发布前检查清单

### 代码
- [x] 所有功能实现完成
- [x] 代码编译通过
- [x] 无明显 Bug
- [x] 性能测试通过

### 文档
- [x] README.md 完整
- [x] QUICKSTART.md 清晰
- [x] DEVELOPMENT.md 详细
- [x] RELEASE_NOTES.md 完善
- [x] LICENSE 包含

### Git
- [x] .gitignore 正确
- [x] 提交历史清晰
- [x] Tag 已创建
- [x] Commit 信息规范

### Release
- [x] Release 说明完整
- [x] Assets 准备就绪
- [x] 版本号正确

---

## 🎯 发布后工作

### 1. 验证 Release

- [ ] 检查 GitHub Release 页面显示正确
- [ ] 下载 Assets 并测试
- [ ] 验证文档链接有效

### 2. 推广

- [ ] 发布到相关社区（可选）
- [ ] 更新项目主页（如果有）
- [ ] 发布博客文章（可选）

### 3. 监控反馈

- [ ] 关注 GitHub Issues
- [ ] 收集用户反馈
- [ ] 记录 Bug 和功能请求

---

## 🔄 后续版本发布流程

### 开发新功能

```bash
# 创建功能分支
git checkout -b feature/new-feature

# 开发和提交
git add .
git commit -m "feat: add new feature"

# 合并到主分支
git checkout master
git merge feature/new-feature
```

### 发布新版本

```bash
# 更新版本号（在代码中）
# 更新 RELEASE_NOTES.md

# 提交更改
git add .
git commit -m "chore: bump version to v0.3.0"

# 创建标签
git tag -a v0.3.0 -m "Release v0.3.0"

# 推送
git push origin master
git push origin v0.3.0

# 创建 GitHub Release（使用 gh 或网页）
gh release create v0.3.0 \
  --title "v0.3.0 - Feature Update" \
  --notes-file RELEASE_NOTES.md \
  oneproxy.exe
```

---

## 📞 获取帮助

### GitHub 相关
- [Creating a release](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository)
- [GitHub CLI documentation](https://cli.github.com/manual/)

### Git 相关
- [Git 标签](https://git-scm.com/book/zh/v2/Git-基础-打标签)
- [Git 远程仓库](https://git-scm.com/book/zh/v2/Git-基础-远程仓库的使用)

---

## 🎉 总结

**已完成**:
- ✅ Git 仓库初始化
- ✅ 首次提交
- ✅ Release 文档
- ✅ Git 标签

**待执行**:
1. 在 GitHub 创建仓库
2. 推送代码和标签
3. 创建 GitHub Release
4. 上传 Assets

---

**准备好发布了！** 🚀

按照上面的步骤，你就可以将 OneProxy 发布到 GitHub 了。
