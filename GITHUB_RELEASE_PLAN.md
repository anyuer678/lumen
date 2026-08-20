# GitHub 发布准备清单

## 当前状态（诚实评估）

### ✅ 已有
- 70+ Go 文件，12000+ 行
- 9 个工具（shell/fs/browser/system/windows/subagent/safety/computer/mcp）
- LLM Benchmark 85%
- Computer Use（截图/鼠标/键盘）
- 事件驱动 + 策略引擎
- 工作流 DAG + Model Router
- Token 追踪 + 轨迹回放 + 断点恢复
- 18 个前端页面

### ❌ 缺失（发布前必须修）

| 优先级 | 项目 | 说明 |
|--------|------|------|
| 🔴 P0 | **硬编码路径** | 5 处引用 `30816`，其他用户无法运行 |
| 🔴 P0 | **README.md** | 需要完整安装/使用说明 + 截图 |
| 🔴 P0 | **LICENSE** | 需要选择开源协议 |
| 🔴 P0 | **.gitignore** | 排除二进制/数据/临时文件 |
| 🔴 P0 | **go.mod 依赖清理** | 移除未使用的依赖 |
| 🟠 P1 | **基础测试** | 至少覆盖核心模块 |
| 🟠 P1 | **API 文档** | 每个端点的说明 |
| 🟠 P1 | **清理 TODO/FIXME** | 1 个 TODO 需要实现或删除 |
| 🟠 P1 | **清理未使用代码** | 移除死代码 |
| 🟡 P2 | **Docker 支持** | Dockerfile + docker-compose |
| 🟡 P2 | **CI/CD** | GitHub Actions 自动构建 |
| 🟡 P2 | **Demo GIF** | 录制使用演示 |
| 🟡 P2 | **版本号管理** | 语义化版本 |

## 建议的发布时间线

### 第一周：基础发布（MVP）
1. 修复硬编码路径（P0）
2. 创建 README.md（P0）
3. 创建 LICENSE（P0）
4. 创建 .gitignore（P0）
5. 清理代码（移除死代码/TODO）

### 第二周：质量提升
6. 添加基础测试（核心模块）
7. 创建 API 文档
8. 添加 Docker 支录

### 第三周：演示准备
9. 录制 Demo GIF
10. 完善 README（使用示例）
11. 准备 Release Notes

## 发布策略

### 版本号
```
v0.1.0 - 首次发布（MVP）
v0.2.0 - 测试覆盖
v0.3.0 - Docker 支持
v1.0.0 - 稳定版
```

### 仓库名建议
```
openclaw-agent          # 简洁明了
personal-ai-agent      # 强调个人助手
computer-use-agent     # 强调 Computer Use
```

### 标签建议
```
ai, agent, computer-use, go, personal-assistant, llm, mcp, 24-7
```

## 何时发布？

**建议：第二周末发布 v0.1.0**

理由：
1. 第一周修复所有 P0 问题（硬编码/README/LICENSE）
2. 第二周添加基础测试 + API 文档
3. 此时项目已经"能用"，可以接受社区反馈

**不要等到"完美"再发布**——开源项目的核心是快速迭代，社区反馈比自己闭门造车更有价值。

## 发布后的迭代计划

```
v0.1.0  首次发布（MVP）
v0.1.1  修复硬编码路径
v0.1.2  添加基础测试
v0.1.3  Docker 支持
v0.2.0  API 文档完善
v0.3.0  Demo GIF + 使用示例
v1.0.0  稳定版（所有 P0/P1 完成）
```
