# OneProxy 文档 Review 清单

## 📋 Review 目的

根据你的真实需求（多端口并行、固定绑定、DNS 快速刷新），所有文档已更新。请 review 以下内容，确认无误后开始代码实现。

---

## 🎯 核心需求确认

### 你的需求

1. ✅ **多端口同时暴露** - 将 JustMySocks 所有节点同时暴露为本地端口
2. ✅ **固定端口绑定** - 每个端口永久绑定一个节点，不自动切换
3. ✅ **互不干扰** - 不同应用可以同时使用不同端口
4. ✅ **密码隐藏** - 使用者只需要知道端口号，不需要密码
5. ✅ **DNS 快速刷新** - IP 变更时快速恢复（加分项）

### 架构设计

```
JustMySocks 5个节点
    ↓
OneProxy (sing-box)
    ↓
127.0.0.1:10801 → Server1 (固定)
127.0.0.1:10802 → Server2 (固定)
127.0.0.1:10803 → Server3 (固定)
127.0.0.1:10804 → Server4 (固定)
127.0.0.1:10805 → Server5 (固定)
```

**是否符合预期？** ⬜ 是 / ⬜ 否

---

## 📄 已更新的文档列表

### 1. README.md ✅
**文件路径**: `README.md`

**主要内容**:
- 项目定位：代理端口聚合器
- 与 NekoBox/Hiddify 的区别对比表
- 核心特性：多端口并行、固定绑定、密码隐藏
- 真实使用场景（开发测试、多应用分流、内网共享）
- 配置说明（强调 local_port 字段）
- 架构设计（sing-box 配置原理）
- DNS 优化策略

**Review 要点**:
- ⬜ 项目定位是否清晰？
- ⬜ 使用场景是否符合你的需求？
- ⬜ 配置说明是否易懂？
- ⬜ 与竞品的区别是否说清楚了？

### 2. QUICKSTART.md ✅
**文件路径**: `QUICKSTART.md`

**主要内容**:
- 5 分钟快速入门指南
- 配置 local_port 的详细说明
- 多代理并行使用示例
- 快速复制地址功能
- 常见问题（强调不自动切换）
- 进阶使用（应用级分流、快速故障切换）

**Review 要点**:
- ⬜ 步骤是否清晰？
- ⬜ 配置示例是否完整？
- ⬜ 多代理并行的用法是否讲清楚了？
- ⬜ 常见问题是否覆盖了你的疑虑？

### 3. PROJECT_STATUS.md ✅
**文件路径**: `PROJECT_STATUS.md`

**主要内容**:
- 项目定位和目标用户
- Phase 1 已完成和待修改清单
- Phase 2 健康检查和 DNS 优化规划
- 核心架构图解
- 下一步行动计划（预计 3-4 小时）

**Review 要点**:
- ⬜ 待修改内容是否完整？
- ⬜ Phase 2 规划是否符合预期？
- ⬜ 下一步行动是否合理？
- ⬜ 时间估算是否可接受？

### 4. configs/config.example.json ✅
**文件路径**: `configs/config.example.json`

**主要内容**:
- 5 个代理节点示例
- 每个节点都有 `local_port` 字段
- inbound 配置改为 `listen` + `proxy_type`
- dns 配置添加 `flush_interval_seconds`

**Review 要点**:
- ⬜ 配置结构是否符合需求？
- ⬜ local_port 字段是否清晰？
- ⬜ 注释是否足够？

### 5. DEVELOPMENT.md 🔄
**文件路径**: `DEVELOPMENT.md`

**状态**: 之前创建的版本需要更新，但不影响 review

**Review 要点**:
- ⬜ 是否需要更新开发文档？（可以在代码修改后再更新）

---

## 🔧 需要修改的代码文件

根据文档，以下文件需要修改：

### 1. internal/config/config.go
**修改内容**:
- ✅ `ProxyConfig` 添加 `LocalPort int` 字段
- ✅ `InboundConfig` 改为 `Listen` + `ProxyType`
- ✅ `DNSConfig` 添加 `FlushIntervalSeconds`
- ✅ 端口冲突检测逻辑

**Review 要点**:
- ⬜ 配置结构设计是否合理？

### 2. internal/config/singbox.go
**修改内容**:
- 每个代理生成一个独立 inbound（tag: `socks-in-10801`）
- 通过 route 规则实现一对一绑定
- 移除 Selector 和 URLTest 逻辑
- 只输出指定的 proxy_type（socks5/http/mixed）

**目标配置**:
```json
{
  "inbounds": [
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10801, "tag": "in-10801"},
    {"type": "socks", "listen": "127.0.0.1", "listen_port": 10802, "tag": "in-10802"}
  ],
  "outbounds": [
    {"type": "shadowsocks", "tag": "out-1", ...},
    {"type": "shadowsocks", "tag": "out-2", ...}
  ],
  "route": {
    "rules": [
      {"inbound": ["in-10801"], "outbound": "out-1"},
      {"inbound": ["in-10802"], "outbound": "out-2"}
    ]
  }
}
```

**Review 要点**:
- ⬜ 一对一绑定策略是否正确？
- ⬜ 是否需要保留 Selector？（建议：不需要）

### 3. internal/ui/tray.go
**修改内容**:
- 菜单显示格式：`✓ JMS-Server1 [:10801] (45ms)`
- 右键节点可复制地址：`127.0.0.1:10801`
- 子菜单项显示端口号

**Review 要点**:
- ⬜ 菜单格式是否清晰？
- ⬜ 是否需要其他快捷操作？

---

## ✅ Review 检查项

### 整体方向

- ⬜ **核心需求是否准确理解**？
  - 多端口并行 ✓
  - 固定绑定 ✓
  - 互不干扰 ✓
  - 密码隐藏 ✓
  - DNS 快速刷新 ✓

- ⬜ **项目定位是否明确**？
  - 不是通用代理客户端
  - 是专用的代理端口聚合器
  - 目标用户：开发者、重度用户、团队协作

- ⬜ **与竞品的区别是否清晰**？
  - NekoBox/Hiddify：单一出口，自动切换
  - OneProxy：多端口并行，固定绑定

### 技术设计

- ⬜ **配置结构是否合理**？
  - local_port 字段必填且唯一
  - inbound 简化为 listen + proxy_type
  - dns 配置支持定期刷新

- ⬜ **sing-box 配置生成是否正确**？
  - 一对一端口绑定
  - 无自动切换逻辑
  - 路由规则明确

- ⬜ **UI 设计是否友好**？
  - 端口号清晰显示
  - 快速复制功能
  - 健康状态可视化

### 文档质量

- ⬜ **README.md 是否清晰**？
  - 项目介绍
  - 使用场景
  - 配置说明

- ⬜ **QUICKSTART.md 是否易懂**？
  - 步骤清晰
  - 示例完整
  - 常见问题覆盖

- ⬜ **PROJECT_STATUS.md 是否完整**？
  - 当前状态
  - 待修改内容
  - 下一步计划

---

## 🤔 需要确认的问题

### 问题 1：端口范围
**默认端口范围**：10801-10809（最多 9 个节点）

- ⬜ 是否满足需求？
- ⬜ 是否需要可配置？

### 问题 2：inbound 类型
**当前设计**：统一使用 `proxy_type` 配置（socks5/http/mixed）

- ⬜ 是否需要每个端口不同类型？
- ⬜ 建议：统一 socks5 即可

### 问题 3：DNS 刷新策略
**当前设计**：
- 定期刷新（5 分钟）
- 失败触发（连续 3 次）
- 手动触发

- ⬜ 间隔时间是否合理？
- ⬜ 失败阈值是否合理？

### 问题 4：内网共享
**当前设计**：通过 `listen: "0.0.0.0"` 开启

- ⬜ 是否需要白名单功能？
- ⬜ 建议：Phase 3 再添加

---

## 📝 Review 反馈区

### 需要修改的地方

```
（请在这里写下需要修改的地方）

示例：
1. README.md 第 XX 行，XXX 描述不清楚
2. 配置示例中缺少 XXX 说明
3. sing-box 生成逻辑应该 XXX
```

### 补充说明

```
（请在这里写下补充的需求或想法）

示例：
1. 希望增加 XXX 功能
2. XXX 场景下应该如何处理
3. XXX 是否需要考虑
```

---

## ✅ Review 通过标准

当以下条件都满足时，可以开始代码实现：

- ⬜ 核心需求理解准确
- ⬜ 项目定位清晰
- ⬜ 技术设计合理
- ⬜ 文档质量合格
- ⬜ 没有重大疑问
- ⬜ 补充说明已记录

---

## 🚀 Review 通过后的行动

1. **初始化 Git** (可选)
   ```bash
   git init
   git add .
   git commit -m "docs: update all documents for multi-port architecture"
   ```

2. **开始代码修改**（按优先级）
   - 修改 `internal/config/config.go`（30 分钟）
   - 重写 `internal/config/singbox.go`（1 小时）
   - 更新 `internal/ui/tray.go`（1 小时）
   - 测试验证（30 分钟）

3. **构建测试**
   ```bash
   make build
   ./oneproxy.exe
   ```

4. **功能验证**
   - 配置多个节点
   - 测试端口固定绑定
   - 验证托盘显示
   - 测试复制功能

---

**Review 完成日期**: _____________

**Review 结果**: ⬜ 通过，开始实现 / ⬜ 需要修改

**签字**: _____________
