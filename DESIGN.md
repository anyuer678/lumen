# OpenClaw 类常驻电脑操控 Agent — 系统设计文档 v0.3

> 版本: v0.3（诚实状态版）
> 最后更新: 2026-08-19

---

## 0. 当前真实状态

### 已建成（骨架层）

| 模块 | 完成度 | 说明 |
|------|--------|------|
| 基础设施 | 90% | Windows Service + 心跳 + SQLite + 配置 |
| 后端 API | 85% | 13 个模块 60+ 端点 |
| Agent 骨架 | 75% | Loop + LLM + 工具注册 |
| 工具执行 | 60% | Shell 真实 / 文件系统 / 浏览器打开 |
| Dashboard | 75% | 12 页水墨主题 + 真实交互 |

### 未建成（真正能力层）

| 模块 | 完成度 | 关键缺失 |
|------|--------|----------|
| Agent 能力 | 40% | 有框架缺深度，LLM 需 API Key |
| 可靠性 | 30% | 缺断点恢复/降级 |
| 安全 | 40% | 有权限但缺细粒度 |
| 长期记忆 | 30% | 有存储但缺智能检索 |
| 浏览器自动化 | 20% | 只能"打开网页" |
| 知识/RAG | 0% | 完全没有 |
| 真正的电脑控制 | 10% | 有 Shell 但缺进程/服务控制 |
| 用户体验 | 40% | Dashboard 有了但交互深度不够 |

---

## 1. 用户如何与 Agent 交互

### 下命令

```
Dashboard → 任务页 → "+ 创建任务" → 输入目标 → Agent 执行

curl POST /v1/tasks { "goal": "整理下载文件夹" }

POST /v1/webhooks/:jobID → 触发执行
```

### 查看 Agent 在干什么

```
Dashboard 总览页 → 实时统计卡片
Dashboard 任务页 → 筛选状态（执行中/已完成/失败）
Dashboard 任务详情 → 步骤时间线（每个步骤的工具/结果/耗时）
Dashboard 日志页 → SSE 实时事件流
API: GET /v1/events → SSE 实时推送
```

---

## 2. 已实现功能清单

### 基础设施（90%）
- Windows Service 安装/卸载/状态
- 崩溃重启 + 心跳自检
- SQLite WAL（8 张表）
- 配置加载 + 持久化
- 纯 Go 编译无 CGO

### 后端 API（85%）
- /v1/health, /v1/status, /v1/events
- /v1/tasks (create/list/get/pause/resume/stop/retry/clear)
- /v1/jobs (create/list/delete)
- /v1/confirmations (list/approve/reject)
- /v1/memories (list/confirm/delete)
- /v1/audit (list with action filter)
- /v1/mcp/servers (register/test/unregister)
- /v1/settings (get/put with persistence)
- /v1/auth/token (list/create/revoke)
- /v1/tools + /v1/tools/{name}/run

### Agent 核心（75%）
- Agent Loop（LLM 模式 + 简化模式）
- LLM Planner/Evaluator/Replanner
- OpenAI 兼容 Provider
- 工具注册 + 真实运行
- 权限检查框架
- 人工确认等待机制
- 审计日志记录
- 记忆检索注入 Planner
- 任务完成沉淀记忆
- 网络重试（指数退避）

### 工具（60%）
- Shell 真实执行（PTY）
- 文件系统（读/写/列目录）
- 浏览器打开
- MCP 客户端框架

### Dashboard（75%）
- 12 个页面全部可交互
- 水墨宣纸主题（#f5f0e6）
- 55+ kb-ui 风格组件

---

## 3. 未实现功能清单

### 核心能力（P0）
- 确认机制真正生效（需用户配置触发等级）
- 任务执行实时进度 SSE 推送
- LLM 真实调用（需配置 API Key）

### 浏览器自动化（P1）
- playwright-go 集成
- 截图/点击/输入/导航/读取

### 知识/RAG（P2）
- 文件解析（MD/PDF/TXT）
- Embedding + 向量索引
- 语义检索

### 长期记忆智能（P2）
- 向量召回
- 遗忘/更新
- 重要性评分

### 任务编排（P2）
- DAG/Workflow
- 子任务拆解
- 并行执行

### 断点恢复（P1）
- Checkpoint 机制
- 重启后从 Step N 续跑

### 安全增强（P1）
- 细粒度权限（L0-4）
- 沙箱限制

### 工具生态（P1-P2）
- Git 操作 / PowerShell / Office / PDF / 进程管理

---

## 4. 路线图现状（2026-11-16 对照代码实况更新；原计划态文本见 git 历史）

> 审计方法：逐项以实现文件为证。标注含义：✅ 已落地（有实现文件）/ 🟡 部分（骨架在，深度待补）/ ❌ 未落地。

### Phase A：让 Agent 真正活起来
1. ✅ LLM 真实调用 + 多步计划（`internal/llm/openai.go` + `internal/agent/llm_planner.go`）
2. ✅ 实时进度 SSE 推送（`internal/api/sse.go`，`/v1/events`）
3. ✅ 确认机制真正生效（`internal/auth/confirm.go` + `confirmations` 表 + token L0-3 分级）
4. ✅ 浏览器自动化（`internal/agent/browser.go`，chromedp 实现 DOM 操作/导航/读取）

### Phase B：让它真正懂电脑
5. 🟡 Git/PowerShell/Office/PDF 工具（✅ Git=`agent/github.go`、PowerShell=`agent/shell.go`；❌ Office/PDF 未实现）
6. 🟡 知识/RAG（`knowledge` 表 + `memory/kb.go` 文件解析就绪；Embedding+向量索引未实现）
7. 🟡 长期记忆智能（`agent/memory_score.go` 重要性评分就绪；向量召回/遗忘机制未实现）

### Phase C：让它真正可靠
8. ✅ 断点恢复（`task/checkpoint.go`）
9. ✅ 细粒度权限（`auth/permission.go` + `agent/policy*.go` + 沙箱 `sandbox_test.go`）
10. ✅ 决策日志（`audit_logs` 表 + `agent decisions` CLI 查询，2026-11-16）

### Phase D：让它真正智能
11. ✅ 任务 DAG（`task/dag.go`）
12. 🟡 多 Agent（`agent/delegate.go` 委派骨架就绪；完整多 Agent 协作待扩展）
13. ❌ 知识图谱（未实现）

### 仍值得做的（按本审计排序）
- Office/PDF 工具（Phase B 唯一整块缺口）
- Embedding/向量召回（知识/RAG 与记忆智能的共同底座）
- 多 Agent 协作深化（delegate 骨架 → 完整闭环）

---

## 5. 访问方式

```
Dashboard:  http://localhost:18080/
API:        http://localhost:18080/v1/
SSE:        http://localhost:18080/v1/events
```

---

## 6. 配置

```yaml
# conf/config.yaml
llm:
  default_provider: deepseek
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key_env: "DEEPSEEK_API_KEY"
      model: "deepseek-chat"
```

---

## 7. 下一步

**最高优先级三件事：**
1. 配置 API Key → LLM 真实调用 → Agent 自主执行
2. 集成 playwright-go → 浏览器真实操控
3. SSE 推送每步进度到 Dashboard
