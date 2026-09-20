> **状态**：`portfolio` / `local-tool` · **非生产就绪** · 默认仅本机 `127.0.0.1`  
> **安全**：见 [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) 与 [docs/SECURITY.md](docs/SECURITY.md)  
> **硬化**：空 scopes 不再等于全权；shell/fs:delete 默认需确认；`?token=` 仅限事件流  
<p align="center">
  <strong>Lumen - 流明</strong>
</p>
<p align="center">
  <em>Your personal intelligence layer - 你的个人智能中枢</em>
</p>
<p align="center">
  <em>一个 24/7 常驻运行、能操控整台电脑的开源 AI Runtime</em>
</p>

<p align="center">
  <a href="https://github.com/anyuer678/lumen/stargazers"><img alt="Stars" src="https://img.shields.io/github/stars/anyuer678/lumen"></a>
  <a href="https://github.com/anyuer678/lumen/network/members"><img alt="Forks" src="https://img.shields.io/github/forks/anyuer678/lumen"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8">
  <img alt="Frontend" src="https://img.shields.io/badge/React-18-61dafb">
  <img alt="Tests" src="https://img.shields.io/badge/tests-64%20passed-brightgreen">
</p>

---

> **Security Warning**: 本项目处于早期测试阶段，**安全性仍未经过完整的独立审计**，当前仅作为测试版本使用。请勿在未加固的情况下公网部署。

## Why

市面上多数 Agent 停在 "LLM + Tools"。Lumen 额外补齐了很多人忽视、却决定"Agent 能否长期可用"的部分：

- **可靠性**：上下文预算、宽容工具调用修复、Checkpoint 断点恢复
- **可观测性**：审计日志、任务轨迹、成本追踪
- **安全**：Token 认证、命令安全分类、沙箱路径检查
- **记忆**：长期记忆存储、用户画像、记忆评分

## Architecture

```
Memory → Reasoning → Tools → Action
  │         │          │        │
  ▼         ▼          ▼        ▼
SQLite    LLM API    Sandbox   Computer
```

## Quick Start（首次运行 3 步走通）

### 1. 编译启动

```bash
# 克隆
git clone https://github.com/anyuer678/lumen.git
cd lumen

# 编译后端
go build -o lumen.exe ./cmd/agent

# 编译前端（可选，Dashboard 需要）
cd web && npm install && npm run build && cd ..

# 启动
./lumen.exe
```

服务默认监听 `127.0.0.1:14000`，自动创建 SQLite 数据库。

### 2. 创建管理员 Token（首次引导）

首次启动时数据库为空，可通过 **bootstrap** 无认证创建第一个管理员 token：

```bash
# 创建 L3 管理员 token（首次运行时不需要认证）
curl -X POST http://127.0.0.1:14000/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"name":"admin","perm_level":3}'
```

响应会返回 token 明文（`agt_xxx...`），**请立即保存，仅显示一次**。

> ⚠️ Bootstrap 限制：仅在数据库为空时有效；强制 L3 级别；第二个 token 必须带认证创建。

### 3. 开始使用

```bash
TOKEN="agt_你的token"

# 执行 shell 命令
curl -X POST http://127.0.0.1:14000/v1/tools/shell.run/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"args":{"command":"echo hello from lumen"}}'

# 读取文件
curl -X POST http://127.0.0.1:14000/v1/tools/fs/run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"args":{"action":"list","path":"."}}'

# 查看可用工具
curl http://127.0.0.1:14000/v1/tools/ -H "Authorization: Bearer $TOKEN"
```

打开浏览器访问 `http://127.0.0.1:14000` 可使用 Dashboard 界面。

## Token 级别与 Scopes

| 级别 | 用途 | 能做什么 |
|---|---|---|
| L0 | 只读 | 浏览器操控、安全分类查询 |
| L1 | 普通操作 | Shell 命令、文件读写、系统信息 |
| L2 | 危险操作 | 计算机控制、MCP 工具（需人工确认） |
| L3 | 管理员 | Token 管理、MCP 注册、全部操作 |

新创建的 token 默认带有以下 scopes：`tasks:create,tasks:control,confirm:approve,tools:run,mcp:register,token:manage,events:emit,kb:write,settings:write`

访问受 scope 保护的端点时，token 必须包含对应 scope，否则返回 403。

## Tech Stack

- **Backend**: Go 1.26 + chi + SQLite (WAL)
- **Frontend**: React 18 + TypeScript + Vite
- **Testing**: go test (64 tests)

## Project Structure

```
cmd/agent/          CLI 入口
internal/
  agent/            Agent 核心循环（loop、planner、feedback、checkpoint）
  api/              HTTP API + 静态资源托管
    handlers/       REST handlers
    static/         前端构建产物
  auth/             Token 认证与权限策略
  config/           配置加载与校验
  contextmgr/       上下文预算管理（token 估算、压缩提示）
  db/               SQLite 持久化（WAL 模式）
  llm/              LLM 调用封装（多模型路由、成本追踪）
  memory/           长期记忆存储、用户画像、记忆评分
  observability/    审计日志与可观测性
  scheduler/        定时任务调度
  service/          Windows 服务生命周期管理
  task/             DAG 工作流引擎
  toolrepair/       LLM 工具调用输出修复
  trajectory/       任务轨迹记录
  vision/           截图分析（多模态）
web/                React 前端
conf/               配置文件（config.yaml.example）
```

## License

MIT
