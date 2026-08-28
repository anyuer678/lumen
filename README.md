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
  <img alt="Tests" src="https://img.shields.io/badge/tests-38%20passed-brightgreen">
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

## Quick Start

```bash
# 克隆
git clone https://github.com/anyuer678/lumen.git
cd lumen

# 构建
go build -o lumen ./cmd/agent

# 运行
./lumen serve
```

## Tech Stack

- **Backend**: Go 1.26 + chi + SQLite (WAL)
- **Frontend**: React 18 + TypeScript + Vite
- **Testing**: go test (38 tests)

## Project Structure

```
cmd/agent/          CLI 入口
internal/
  agent/            Agent 核心循环
  api/              HTTP API handlers
  auth/             Token 认证
  llm/              LLM 调用封装
  tools/            工具执行（沙箱、命令安全）
  memory/           记忆存储与评分
  scheduler/        定时任务
web/                React 前端
conf/               配置文件
```

## License

MIT