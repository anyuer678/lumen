# 威胁模型 — lumen

> 状态：portfolio / 本地 Agent Runtime · **非生产** · 默认仅 `127.0.0.1`

## 1. 系统概述

| 项 | 内容 |
|---|---|
| 项目 | lumen — 24/7 个人 AI Agent Runtime（Go + React） |
| 能力 | 记忆、推理、工具调用（fs/shell/browser/windows/computer/mcp） |
| 部署 | 本机单用户 HTTP API + Dashboard + SQLite |
| 版本 | 以仓内 CHANGELOG 为准 |

## 2. 资产

| 资产 | 敏感度 | 说明 |
|---|---|---|
| 本机文件系统 | 高 | fs 工具可读写/删除 workspace 内外（视沙箱） |
| API Token | 高 | 控制 Agent 全部 API |
| LLM Provider Key | 高 | `api_key_env`，不入库明文 |
| 审计与轨迹 | 中 | 操作可追溯性 |
| 用户隐私（截图/键鼠） | 高 | Computer Use |

## 3. 信任边界

```text
[不可信] 恶意/被注入 MCP · 其他本机进程 · 误暴露的公网扫描
    |
[边界] Token 认证 · Scope · PermissionEngine · 确认流 · 命令分类 · 沙箱路径检查
    |
[TCB] cmd/agent · internal/auth · internal/agent(RunTool/Execute) · 确认存储
    |
[资产] FS / Keys / 审计
```

**明确不在信任域**：被攻破的 MCP 外挂进程；同机其他用户（Windows ACL 未强化时）。

## 4. 攻击者与场景

| ID | 攻击者 | 场景 | 控制（现状/本补丁） |
|---|---|---|---|
| A1 | 窃取到旧 token 的人 | 空 scopes token 调 tools | **本补丁**：空 scopes fail-closed |
| A2 | 低权限 token | 自批 confirm | **本补丁**：confirm:approve scope + PermLevel |
| A3 | L1 token | 静默 shell | **本补丁**：shell 策略 L2 + RequiredLevel=2 |
| A4 | L0 token | fs delete | 策略 fs:delete L2 + handler action 级别 |
| A5 | 日志投毒 | `?token=` 进代理日志 | **本补丁**：query token 仅 events/sse |
| A6 | 恶意 MCP | 拉起任意进程 | MCP 策略 L2 + 确认（仍偏弱，见残余） |
| A7 | 命令注入/黑名单绕过 | 编码 PS、路径别名 | ClassifyCommand 未知=破坏性；**黑名单可绕 → 残余风险** |
| A8 | 空库 bootstrap | 无认证建 L3 | 仍允许一次；**残余**：窗口期需本机隔离 |

## 5. 控制措施（补丁后）

- Token：SHA-256 存储；启用位；过期；**scopes 显式授予**
- HTTP：`RequireScope` 于 tools/mcp/token/settings/events；confirm:approve
- RunTool：PermissionEngine + 破坏性 ClassifyCommand 确认流（fail-closed）
- 默认策略：事件策略含 shell 则必须 confirm；yaml `auto_execute: false`
- 绑定：文档要求 127.0.0.1；公网不支持

## 6. 残余风险（诚实列出）

| ID | 残余 | 缓解路线 |
|---|---|---|
| R1 | shell 仍为字符串解释（powershell/cmd/sh -c） | 白名单 argv 工具；默认关闭 shell |
| R2 | 黑名单/分类可被复杂载荷绕过 | OS 级沙箱（Job Object/容器/受限用户） |
| R3 | bootstrap 空库可建 L3 | 本机文件权限 + 首启向导；或要求本地 socket |
| R4 | Computer Use / MCP 高危面 | 默认关闭 feature flag |
| R5 | Windows 上文件 ACL 弱 | 文档 + 未来 DPAPI |
| R6 | 未做独立渗透审计 | README 声明；欢迎负责任披露 |

## 7. 安全默认值

| 配置 | 默认 |
|---|---|
| 空 scopes token | 拒绝（迁移可临时 `LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1`） |
| shell:run | L2 + 确认 |
| fs:delete/organize | L2 + 确认 |
| `?token=` | 仅 `/events` `/sse` `/stream` |
| 新 token 默认 scopes | 最小集（见 token handler） |
| 监听 | 127.0.0.1（部署文档） |

## 8. 事故响应

1. 吊销可疑 token（L3 `token:manage` 或 DB `enabled=0`）  
2. 查 `audit_logs` / 轨迹  
3. 轮换 LLM key（若 env 泄露）  
4. 见 `SECURITY.md`

## 9. 变更

| 日期 | 变更 |
|---|---|
| 2026-09-20 | 初稿；对应 Sprint1 安全补丁包 |
