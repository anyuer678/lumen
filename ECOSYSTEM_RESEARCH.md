# Agent 生态调研报告 — 提炼可行动项

> 来源：用户调研（10+ 个 Agent 框架 + 5+ 个 AI Companion 项目）
> 目标：筛选对我们"常驻电脑操控 Agent"项目真正有价值的技术/设计

---

## 一、框架类项目（架构/工程借鉴）

### 1. Microsoft Agent Framework Go（最推荐）
**核心价值**：Workflow + Checkpoint + Observability 是成熟的
- Workflow：Sequential/Concurrent + 条件路由（我们缺 DAG）
- Checkpoint：重启后从断点续跑（我们有框架但不完善）
- Observability：OpenTelemetry 全链路（我们有基础日志但缺追踪）
- **行动**：看它的 workflow/checkpoint 实现，补我们的任务编排

### 2. Phero（能力百科全书）
**核心价值**：RAG + Memory + Multi-Agent + Tracing + Token tracking
- RAG：向量检索 + 文档解析（我们缺）
- Multi-Agent：Orchestrator/Worker 模式（我们有子代理但简单）
- Token/latency tracking：成本追踪（我们完全没做）
- **行动**：看它的 RAG 和 token tracking 实现

### 3. Felix（产品形态参考）
**核心价值**：单 binary + 多模型 + 本地运行 + MCP
- 与我们路线高度重叠，可参考它的产品设计
- **行动**：看它的架构图和 README，对照我们的路线

### 4. RobotGo（Computer Use 底层）
**核心价值**：Go 原生鼠标/键盘/屏幕控制
- 我们现在只有 Shell + 文件 + 浏览器，缺 GUI 控制
- **行动**：集成 robotgo 作为 Computer Use 工具

---

## 二、Companion 类项目（长期存在/主动行为借鉴）

### 1. Temporal Memory（时间记忆）
**我们的差距**：记忆只是 key-value，没有时间结构
**借鉴**：按天/周/月组织记忆，支持"最近关注什么"查询
**行动**：给 memory 表加 `date` + `importance` 字段

### 2. Daily Digest（每日摘要）
**我们的差距**：任务完成后没有自动总结
**借鉴**：每天自动生成"今日执行了N个任务/新增知识M条"
**行动**：加一个 daily digest 任务（cron 触发）

### 3. Proactive Behavior（主动行为）
**我们的差距**：完全是被动的（用户说才做）
**借鉴**：Scheduler → Observe → Detect → Decide → Act 循环
**行动**：在 Scheduler 中加"主动检查"触发器

### 4. Relationship Memory（关系记忆）
**我们的差距**：记忆只有内容，没有"与用户的关系"
**借鉴**：记录互动频率、偏好变化、项目进展
**行动**：长期可做，先不急

---

## 三、我们项目的能力矩阵（vs 行业）

| 能力 | 我们 | 行业最佳 | 差距 |
|------|------|---------|------|
| Agent Loop | ✅ | MAF/Phero/Felix | 基本对齐 |
| Memory | ✅ 基础 | AIRI（时间记忆） | 缺时间结构 |
| Knowledge/RAG | ✅ 基础 | Phero（向量检索） | 缺向量索引 |
| Workflow | ❌ | MAF（DAG） | 完全缺 |
| Checkpoint | ⚠️ 框架 | MAF（完善） | 不完善 |
| MCP | ✅ | 全部项目 | 对齐 |
| Browser | ✅ 基础 | Scout/Phero | 缺自动化深度 |
| Computer Use | ❌ | Windows-Use/RobotGo | 完全缺 |
| Multi-Agent | ⚠️ 简单 | MAF/Phero | 太简单 |
| Security | ✅ 基础 | PocketPaw（7层） | 只有基础 |
| Observability | ✅ 基础 | MAF/Phero | 缺追踪 |
| Scheduler | ✅ | MAF/Micro | 基本对齐 |
| Proactive | ❌ | Companion 项目 | 完全缺 |

---

## 四、建议下一步（按 ROI 排序）

### 第一批（1-2 周，让系统更成熟）
1. **Checkpoint 完善**（从 MAF 借鉴）
2. **Token/成本追踪**（从 Phero 借鉴）
3. **RobotGo 集成**（从 RobotGo 借鉴，做 Computer Use）

### 第二批（2-4 周，让系统更智能）
4. **Temporal Memory**（从 AIRI 借鉴）
5. **Workflow/DAG**（从 MAF 借鉴）
6. **Proactive Behavior**（从 Companion 项目借鉴）

### 第三批（4-8 周，让系统更完整）
7. **RAG 向量检索**（从 Phero 借鉴）
8. **Multi-Agent 完善**（从 MAF/Phero 借鉴）
9. **Daily Digest**（自动生成每日摘要）

---

## 五、关键项目链接索引

> 以下项目均为开源，按用途分类，随时可查

### Agent 框架
| 项目 | 语言 | 核心亮点 | 链接 |
|------|------|---------|------|
| Microsoft Agent Framework Go | Go | Workflow/Checkpoint/Observability | https://github.com/microsoft/agent-framework-go |
| Go Micro (Agent Harness) | Go | Durable Workflow/Guardrails/Runtime | https://github.com/micro/go-micro |
| Phero | Go | RAG/Memory/Multi-Agent/Tracing/MCP | https://github.com/henomis/phero |
| Felix | Go | 单 binary/多模型/本地运行/MCP | https://github.com/sausheong/felix |
| PocketPaw | - | 50+ 工具/7 层安全/多 Agent | https://github.com/pocketpaw/pocketpaw |
| Alfred | - | Windows 本地优先/加密记忆/Voice | https://github.com/Heisen111/alfred |

### Computer Use / GUI 自动化
| 项目 | 语言 | 核心亮点 | 链接 |
|------|------|---------|------|
| RobotGo | Go | 鼠标/键盘/屏幕/窗口控制 | https://github.com/go-vgo/robotgo |
| Windows-Use | Python | Windows UI Automation GUI 控制 | https://github.com/Jeomon/Windows-Use |
| Scout | Go | 浏览器自动化单 binary/DOM 提取 | https://github.com/klarlabs-studio/scout |

### AI Companion / 长期存在
| 项目 | 核心亮点 | 链接 |
|------|---------|------|
| Project AIRI | 数字生命/RAG/Memory/Live2D/Agent | https://github.com/moeru-ai/airi |
| Super Agent Party | 角色/VRM/语音/长期记忆/MCP/电脑控制 | https://github.com/heshengtao/super-agent-party |
| vibe-ai-partner-entity | Active Memory/日记/Temporal/桌面存在 | GitHub topics: ai-companion |

---

## 六、架构演进路线（我们的终点）

```
现在：
  用户 → Agent → 工具 → 结果

终点：
  Persistent Personal Agent
  ├── Cognition (Planner/Reasoning/Evaluator/Reflection)
  ├── Memory (Long-term/Temporal/Episodic/Semantic)
  ├── Tools (Computer/Browser/Shell/MCP)
  ├── Personality (人格连续性)
  ├── Emotional State (状态变量系统)
  ├── Relationship (与用户的共同经历)
  └── Proactive Policy (主动行动决策)
```

**核心公式**：会干活 + 有记忆 + 有人格 + 能主动行动 + 能感知环境 + 能操作电脑 = Persistent Personal Agent
