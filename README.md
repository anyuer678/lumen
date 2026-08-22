<p align="center">
  <strong>🦞 Lumen · 流明</strong>
</p>
<p align="center">
  <em>Your personal intelligence layer · 你的个人智能中枢</em>
</p>
<p align="center">
  <em>一个 24/7 常驻运行、能操控整台电脑的开源 AI Runtime — 拥有记忆、推理、工具调用与计算机操作能力，成为你数字世界的那一点光</em>
</p>

<p align="center">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8">
  <img alt="Frontend" src="https://img.shields.io/badge/React-18-61dafb">
  <img alt="Benchmark" src="https://img.shields.io/badge/Benchmark-v3%20(100%20tasks)-brightgreen">
  <img alt="Tests" src="https://img.shields.io/badge/tests-38%20passed-brightgreen">
  <img alt="Local" src="https://img.shields.io/badge/local-first-4b6fff">
</p>

---

> ⚠️ **测试版本警告**：本项目处于早期测试阶段，尽管已实现 Token 认证、权限分级与基础注入防护，**安全性仍未经过完整的独立审计**，当前仅作为测试版本使用。请勿在未加固的情况下公网部署。

跟它说"帮我整理下载目录"，它会自己规划 → 调用工具 → 执行 → 沉淀记忆 → 每天总结。**全本地运行，默认只监听本机。**

## Why

市面上多数 Agent 停在 *"LLM + Tools"*。Lumen 额外补齐了很多人忽视、却决定"Agent 能否长期可用"的部分：

- **可靠性**：上下文预算、宽容工具调用修复、Checkpoint 断点恢复
- **可观测**：每阶段轨迹追踪、失败自动分类
- **记忆**：评分 + 生命周期（防记忆膨胀）+ 用户画像提炼
- **主动**：事件驱动 + 策略规则，不只是被动回答问题

它不是又一个聊天壳，而是一个可以被桌宠、手机 App、CLI 复用的 **Runtime 底座**。

---

## ✨ 核心能力

| 能力 | 说明 |
|------|------|
| 🧠 **真实 Agent Loop** | 意图路由 → Planner → 执行 → 评估 → Replanner → 记忆 → 反馈 |
| 🔀 **多模型路由** | 智谱 / DeepSeek / Ollama / OpenAI 兼容，按复杂度自动选模型 |
| 🛠️ **9 类工具** | Shell / 文件 / 浏览器 / 系统 / Windows / 子代理 / Computer / 安全 / MCP |
| 👁️ **多模态 Vision** | 截图 → base64 → 视觉模型 → UI 元素理解 |
| 🧹 **Tool Repair** | 宽容解析 LLM 输出：JSON 修复、工具名归一化、参数修复 |
| 📏 **Context Manager** | Token 预算估算 + 滑动窗口，防止上下文泄漏 |
| 🕵️ **Agent Trace** | 每阶段 span 记录（stage/latency/success），排错神器 |
| 🧠 **Memory 2.0** | 记忆评分 + 生命周期（active→forgotten）+ 用户画像 |
| ⚡ **事件驱动** | EventBus + Policy YAML，主动响应文件/任务/系统事件 |
| 🧩 **工作流 DAG** | 任务编排，支持依赖与并行 |
| 📊 **Benchmark + 失败分析** | 100 用例 + 自动归类与修复建议 |
| 🔒 **安全防护** | 命令黑名单 + 沙箱 + 权限分级 + 确认机制 + 默认仅本机监听 |

---

## 🚀 快速开始

### 环境要求

- Go 1.26+
- Node 18+（仅构建前端需要，可跳过）

### 克隆 & 运行

```sh
git clone https://github.com/anyuer678/lumen.git
cd lumen
go build -o agent.exe ./cmd/agent
$env:ZHIPU_API_KEY = "sk-你的key"   # Windows PowerShell

# 首次运行前，生成访问 token（仅显示一次）
.\agent.exe token mytoken

.\agent.exe
```

浏览器打开 **http://localhost:18080**（首页为 Today 助手日报）。

> ⚠️ **安全**：服务已启用 Token 认证。默认仅监听 `127.0.0.1`。如需公网访问，请先配置 token，勿直接改为 `0.0.0.0` 暴露无认证 API。

### 配置 LLM

编辑 `conf/config.yaml`：

```yaml
llm:
  default_provider: zhipu        # 默认模型
  providers:
    zhipu:
      base_url: https://open.bigmodel.cn/api/paas/v4
      api_key_env: ZHIPU_API_KEY # 也可直接填 api_key
      model: glm-4-flash
    deepseek:
      base_url: https://api.deepseek.com/v1
      api_key_env: DEEPSEEK_API_KEY
      model: deepseek-chat
    ollama:                      # 本地免费
      base_url: http://127.0.0.1:11434/v1
      model: qwen3:0.6b
    openai:
      base_url: https://open.bigmodel.cn/api/paas/v4/chat/completions
      model: glm-4.7-flash
```

---

## 🤖 Agent Loop 数据流

```
用户输入
  │
  ▼
IntentRouter ── direct_answer / tool_call / remember / kb_query / llm_needed
  │  (llm_needed)
  ▼
Planner ────── LLM 生成步骤计划（注入记忆 + 历史反馈建议）
  │
  ▼
ContextMgr ─── 裁剪历史到 token 预算，防溢出
  │
  ▼
Tool Selection ── 工具选择 + RepairToolArgs 参数修复
  │
  ▼
Permission ──── L1/L2/L3 分级，危险操作需人工确认
  │
  ▼
Execution ───── 执行 + 重试 + 每步 Checkpoint
  │
  ├─ 失败 → Replanner 重新规划剩余步骤
  │
  ▼
Memory ──────── 沉淀记忆（评分 + 生命周期）
  │
  ▼
Feedback ────── 记录成功率 / 工具 / 错误类型
  │
  ▼
EventBus ───── 广播 → Proactive 主动恢复 / 前端 SSE 实时推送
（每阶段 Trace span 落盘，GET /v1/traces/{id} 回放）
```

---

## 🏗️ 架构分层（Lumen 视角）

```
                 ✦ Lumen · 流明 ✦
                Your personal intelligence layer
                ┌────────────────────────────┐
                │    Memory 记忆             │
                │  Recall / Reflection / Profile│
                └──────────────┬─────────────┘
                ┌──────────────▼─────────────┐
                │   Reasoning 推理           │
                │  Planner / Evaluator / Router│
                └──────────────┬─────────────┘
                ┌──────────────▼─────────────┐
                │   Tools 工具               │
                │  Windows/Browser/Files/Shell│
                │  Computer(Vision)/MCP      │
                └──────────────┬─────────────┘
                ┌──────────────▼─────────────┐
                │   Action 行动              │
                │  Computer Use / 桌面操控    │
                └────────────────────────────┘

    上方所有能力，被一束光（Trace / 记忆评分 / 生命周期 / Policy）
    持续照亮、记录与守护。
```

**对应真实分层：**

```
接入层  REST API (30+ 端点) │ SSE 事件流 │ Web Dashboard(20 页)
   │
Agent Brain  Loop │ Planner │ Evaluator │ Replanner │ Router
             IntentRouter │ Policy │ Proactive │ Reflection
   │
Tools Registry  Shell │ File │ Browser │ System │ Windows
             Computer(Vision) │ SubAgent │ Safety │ MCP.*
   │
Cognition & Data  Memory Score │ Lifecycle │ User Profile │ Knowledge
             SQLite(15 表) │ Token │ Trajectory │ Checkpoint
```

---

## 📁 项目结构

```
agent/
├── cmd/agent/             # 入口（run / benchmark / install / status）
├── internal/
│   ├── agent/             # 核心（28 文件）
│   │     loop / planner / evaluator / replanner / router
│   │     tools / trace / policy / proactive / reflection
│   │     lifecycle / memory_score / failure_analysis / benchmark_v3
│   ├── api/               # HTTP 路由 + SSE + 静态文件
│   ├── api/handlers/      # 20+ 端点处理器
│   ├── auth/              # 权限 / 确认 / 审计
│   ├── config/            # YAML 配置
│   ├── contextmgr/        # Token 估算 + 消息滑动窗口
│   ├── db/                # SQLite WAL 初始化
│   ├── llm/               # Provider + Router + 多模态 + 成本
│   ├── memory/            # 记忆存储 + 知识库 + 工作记忆
│   ├── observability/     # 日志 + 心跳 + 审计
│   ├── scheduler/         # 定时 + 文件监听 + Webhook
│   ├── service/           # 服务装配 + Windows Service
│   ├── task/              # 状态机 + Checkpoint + DAG
│   ├── toolrepair/        # JSON 宽容修复 + 参数修复
│   ├── trajectory/        # 轨迹 JSONL 记录
│   └── vision/            # 视觉分析（截图 → UI 理解）
├── web/                   # 前端 React + TypeScript（20 页）
├── conf/config.yaml       # 主配置
├── conf/policy.yaml       # 策略规则
└── DESIGN.md              # 设计文档（路线/能力矩阵）
```

---

## 🛠️ 工具系统

| 工具 | 级别 | 说明 | 主要 action |
|------|------|------|------------|
| `shell.run` | L1 | 命令执行 | 自动识别 cmd / PowerShell |
| `fs` | L0 | 文件系统 | read / write / list / exists / mkdir / delete / organize |
| `browser` | L0 | 浏览器 | open / read / search / research / screenshot / click / type |
| `system` | L1 | 系统信息 | processes / services / network / disk / git_status |
| `windows` | L1 | Windows 控制 | powershell / env / clipboard / notify / app_list / keyboard |
| `computer` | L2 | Computer Use | screenshot(+视觉分析) / mouse / keyboard / window |
| `subagent` | L1 | 子代理 | 并行委派独立子任务（不嵌套）|
| `safety` | L0 | 安全分类 | 命令只读/读写/破坏性判定 |
| `mcp.*` | L2 | MCP 插件 | stdio + SSE 双传输，工具独立注册 |

---

## 📖 API 摘要

### 系统
| 端点 | 功能 |
|------|------|
| `GET /v1/health` | 健康检查 |
| `GET /v1/status` | 系统状态（uptime/tasks/provider）|
| `GET /v1/events` · `GET /v1/sse` | 事件 / 实时事件流 |

### 任务 / 对话
| 端点 | 功能 |
|------|------|
| `GET/POST /v1/tasks` `GET /v1/tasks/{id}` | 任务 CRUD |
| `POST /v1/tasks/{id}/pause·resume·stop·retry` | 任务控制 |
| `GET /v1/tasks/{id}/steps` | 步骤详情 |
| `GET/POST /v1/chat` `GET/POST /v1/chat/{id}/messages` | 会话 + SSE 流式对话 |

### 工具 / 知识
| 端点 | 功能 |
|------|------|
| `GET /v1/tools` `POST /v1/tools/{name}/run` | 工具列表 / 调用 |
| `GET/POST /v1/knowledge` | 知识库 |
| `POST /v1/vision/analyze` · `/locate` | 截图视觉分析 / 元素定位 |

### 记忆 / 画像
| 端点 | 功能 |
|------|------|
| `GET/POST /v1/memories` | 记忆 |
| `GET/POST /v1/profiles` · `/reflect` | 用户画像 / 触发反思 |
| `GET /v1/lifecycle/stats` `POST /v1/lifecycle/run` | 记忆生命周期 |
| `POST /v1/memory-score/recalc` `GET /top` `GET /low` | 记忆评分 |

### 观测 / 管理
| 端点 | 功能 |
|------|------|
| `GET /v1/traces/{taskID}` | Agent Trace 回放 |
| `GET /v1/token-usage` | Token 用量 / 成本 |
| `GET /v1/audit` | 审计日志 |
| `GET/POST /v1/jobs` · `POST /v1/webhooks/{jobID}` | 定时 / Webhook |
| `GET/POST /v1/mcp/servers` | MCP 服务器 |
| `GET /v1/digest/today` | 每日摘要 |
| `GET/POST/DELETE /v1/settings` · `/v1/auth/token` | 设置 / API Token |

---

## 🖥️ 前端页面（20 页）

**普通模式（默认）：**
```
Today(助手日报) / Chat(SSE实时进度) / Tasks / TaskDetail / Memories / Knowledge / Settings
```

**专家模式**（侧边栏底部一键切换）：
```
Overview / Jobs / Confirms / Profiles / Tools / Mcp / Artifacts
/ Events / TokenUsage / Schedule / Audit / Tokens / Logs
```

---

## 🗄️ 数据库表（15 张）

```
tasks · task_steps · task_plans · task_checkpoints
chat_sessions · chat_messages
memories · knowledge · user_profiles
audit_logs · token_usage · jobs · events
workflows · feedback
```

记忆表额外支持：`lifecycle`(active/consolidated/archived/forgotten)、`access_count`、`last_accessed`、`quality_score`。

---

## 📊 Benchmark

```
Agent Benchmark v3 — 100 用例
  ├─ A. 日常助手:   文件/系统/浏览器/记忆 40 类
  ├─ B. 基础能力:   工具选择/上下文/规划    30 类
  ├─ C. 极端情况:   输入边界/工具边界       20 类
  └─ D. 安全:       危险命令拦截            10 类

Simple Mode:  71%（无 LLM，规则路由 — 诚实上限，多步骤需 LLM）
LLM Mode:     ~84%（智谱 glm-4-flash + 完整 Loop）
Safety:       100%（10 种危险命令全拦截）
```

```sh
./agent.exe benchmark            # v3 Simple（默认）
./agent.exe benchmark --llm      # v3 LLM
./agent.exe benchmark --v2       # 旧版
```

输出 `BENCHMARK_V3_REPORT.md` + `.json`，含**失败分类**（planner/tool_exec/param/timeout/network...）与**修复建议**。

---

## 🎯 能力矩阵（18 项）

```
✅ 100%  LLM / ToolSchema / ContextManager / Checkpoint / 模型路由 / 成本系统
✅ 90%+  Tool标准化 / Benchmark / Windows控制 / 浏览器 / 多模态 / 沙箱 / Feedback
✅ 80%   Workflow/DAG
✅ 70%+  并发任务 / MCP 生态
✅ 60%+  多Agent / 主动Agent
```

**全部 18 项 ≥60%**

---

## 🧭 使用场景

Lumen 被设计为可支撑三类真实场景（Dogfood 目标）：

| 场景 | 流程 |
|------|------|
| 📚 **学习助手** | PDF → 整理知识点 → 生成笔记 → 进知识库 → 晚上总结 |
| 🛠️ **项目助手** | git diff → 总结修改 → 生成 changelog → 更新项目记忆 |
| 📁 **文件管家** | Downloads → 分类 → 重命名 → 归档 → 知识库 |

---

## ⚙️ 策略配置（conf/policy.yaml）

```yaml
policies:
  - name: file_organize
    event_type: file.created      # 文件创建事件
    enabled: true
    quiet_hours: { start: 23, end: 8 }   # 静默时段不执行
    max_per_hour: 5
    keywords: [".pdf", ".docx", ".zip"]
    action:
      notify: true
      auto_execute: true
      tools: [fs]
```

---

## 🔒 安全设计

- **Token 认证**：所有 `/v1` 端点（除 /health、/status、/events）强制 Bearer token，未认证 API 调用直接 401
- **命令黑名单**：format / del /s / rm -rf / diskpart / shutdown / reg delete 等 18+ 类危险命令默认拒绝
- **路径沙箱**：阻止操作 `C:\Windows` / `C:\Program Files` 等系统目录 + symlink 逃逸防护
- **权限分级**：L0 只读 / L1 常规 / L2 危险（需人工确认）；工具调用按 token 的 perm_level 校验
- **确认机制**：危险操作等待用户审批（默认 60s 超时）
- **注入防护**：PowerShell / CORS / SSRF / env / JSON 注入均转义或白名单拦截
- **审计日志**：所有工具调用 / 成功 / 失败落库
- **网络边界**：默认仅监听 `127.0.0.1` + CORS 精确限 localhost，防局域网未授权操控

---

## 🔑 API 认证

服务默认启用 Token 认证。除 `/health`、`/status`、`/events`（SSE 只读流）外，所有 `/v1` 端点都需要有效 token。

### 生成 token（唯一合法途径）

```powershell
.\agent.exe token mytoken
# 输出: agt_xxx...（仅显示一次，请立即保存）
```

### 调用 API

```powershell
# 方式一：Authorization Bearer
curl -H "Authorization: Bearer agt_xxx" http://localhost:18080/v1/tasks

# 方式二：X-API-Token
curl -H "X-API-Token: agt_xxx" http://localhost:18080/v1/tasks
```

> ⚠️ 前端首次打开 Settings → 安全 Tab 粘贴 token 保存后，前端请求会自动附带认证头。

---

## 💻 开发

```sh
go build ./...        # 编译
go vet ./...          # 静态检查
go test ./internal/... -count=1   # 全部测试
.\agent.exe benchmark --llm       # 跑 Benchmark
```

**前端：**
```sh
cd web
npm install
npx tsc --noEmit      # 类型检查
npx vite build        # 构建到 internal/api/static
```

---

## 🗺️ Roadmap

```
过去（OpenClaw 引擎阶段）
v0.8  能力建设（Benchmark v2 / Context / ToolRepair / Vision / 前端闭环）
v0.9  工程化（Benchmark v3 100 用例 / Trace / 失败分析 / 稳定化）
v0.9.5 Real Usage Validation（Memory Score / Today 助手日报 / 两层模式）
════════════════════════════════════════════════════════
现在开始 —— Lumen · 你的个人智能中枢
Lumen v0.1 Alpha  个人 Agent Runtime          ← 当前
Lumen v0.5        Personal Assistant（记忆/画像/主动陪伴）
Lumen v1.0        Personal AI OS（插件生态 / 多设备）
════════════════════════════════════════════════════════
下一步：Dogfood 7 天真实使用（学习/项目/文件三场景），验证"真的能替你做事"
```

---

## 💡 灵感来源

Lumen 能从"聊天 Demo"走到"Agent Runtime 原型"，离不开两个开源项目**非常先进的设计思想**的启发。在此特别致以敬意：

- **[Reasonix](https://github.com/esengine/DeepSeek-Reasonix)**：其**子代理委派**让复杂任务能拆解并行、**轨迹回放**让 Agent 的每一次决策都能被离线复盘、**有界独立调用（boundedllm）**把复核隔离出主会话、**命令安全分解**把危险操作挡在门外——这些工程范式让 Agent 从"能跑"走向"能信"。
- **[OpenClaw](https://github.com/openclaw/openclaw)**：其**上下文分层管理（context-engine）**解决了 Agent 最致命的上下文膨胀、**宽容工具调用修复（tool-call-repair)** 让模型哪怕不按协议输出也能被兜底、**视觉理解（media-understanding）**打通了"看见屏幕"的能力——这些设计抓住了真实 Agent 的核心痛点。

**必须说明**：Lumen 借鉴的是上述项目的**设计思想与工程范式**，不是它们的代码。我们的实现是**独立重构**——用 Go 重新落地（例如上下文本复用其"分层预算"的思路、但结构独立设计；宽容解析参考其"不因模型输出不规范而整体失败"的理念、但解析器完全自研；并且 OpenClaw 源码是 TypeScript，我们也不可能照抄）。代码注释中已逐一标注"灵感来自 xxx"，尊重原始出处。若你在代码中发现任何与上述项目逐行一致的片段，请反馈，我们会立即修正。

## License

[MIT](LICENSE) — Copyright © 2026 Lumen (流明)

---

## 免责声明

本项目为个人学习与研究用途的 Agent Runtime 原型，**已实现 Token 认证、权限分级与基础注入防护，但未按生产标准做独立安全审计、负载测试与故障容错**，不建议直接部署到生产或关键环境。默认仅监听本机回路地址；如改变监听范围或对外提供服务，由此产生的服务中断、数据泄露、业务损失或第三方纠纷，由使用者自行承担。所有 Agent 自动执行的操作（尤其是 Shell 命令、文件删除）请在执行前人工确认。
