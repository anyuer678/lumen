# 威胁模型 — lumen

> 状态：portfolio / 本地 Agent Runtime · **非生产** · 默认仅 `127.0.0.1`  
> 对应安全 Sprint2：shell 档位 + argv 白名单 + scope/perm 对齐

## 1. 系统概述

| 项 | 内容 |
|---|---|
| 项目 | lumen — 24/7 个人 AI Agent Runtime（Go + React） |
| 能力 | 记忆、推理、工具调用（exec.argv / shell.run / fs / browser / windows / computer / mcp） |
| 部署 | 本机单用户 HTTP API + Dashboard + SQLite |
| shell 档位 | `permissions.shell_profile`：`strict`（默认）/ `argv-only` / `full` |
| 版本 | 以仓内 CHANGELOG 为准 |

## 2. 资产

| 资产 | 敏感度 | 说明 |
|---|---|---|
| 本机文件系统 | 高 | fs 写/删需 L2；沙箱默认限制 workspace |
| API Token | 高 | 控制 Agent 全部 API；scopes 显式授予 |
| LLM Provider Key | 高 | `api_key_env`，不入库明文 |
| 审计与轨迹 | 中 | shell.run / authz.denied 强制写入 audit_logs |
| 用户隐私（截图/键鼠） | 高 | Computer Use |

## 3. 信任边界

```text
[不可信] 恶意/被注入 MCP · 其他本机进程 · 误暴露的公网扫描
    |
[边界] Token 认证 · Scope(fail-closed) · PermissionEngine · 确认流 · 命令分类 · 沙箱 · shell_profile
    |
[TCB] cmd/agent · internal/auth · internal/agent(RunTool/Execute) · 确认存储
    |
[资产] FS / Keys / 审计
```

**明确不在信任域**：被攻破的 MCP 外挂进程；同机其他用户（Windows ACL 未强化时）。

## 4. 攻击者与场景

| ID | 攻击者 | 场景 | 控制（Sprint1+Sprint2） |
|---|---|---|---|
| A1 | 窃取到旧 token 的人 | 空 scopes token 调 tools | 空 scopes fail-closed（含 L3） |
| A2 | 低权限 token | 自批 confirm | confirm:approve scope + PermLevel ≥ 风险级 |
| A3 | L1 token | 静默 shell | shell 策略 L2 + strict 档每次确认 + RequiredLevel=2 |
| A4 | L0/L1 token | fs delete/write | ActionRequiredLevel + 策略均为 L2 |
| A5 | 日志投毒 | `?token=` 进代理日志 | query token 仅 events/sse/stream |
| A6 | 恶意 MCP | 拉起任意进程 | MCP 策略 L2 + 确认（**残余**，见 R4） |
| A7 | 命令注入/黑名单绕过 | 编码 PS、LOLBins | 黑名单加严 + ClassifyCommand 未知=破坏性 + strict 每次确认 + 审计 |
| A8 | 空库 bootstrap | 无认证建 L3 | 仍允许一次；bootstrap 默认 AdminTokenScopes；**残余** R3 |
| A9 | 无 scope 的内部旁路 | Chat 路径调 RunTool | Chat 传递 principal；RunTool 有 principal 时强制 tools:run |
| A10 | 任意字符串 shell | 默认 profile | 默认 `strict`；`full` 需显式配置；`argv-only` 完全禁用 |

## 5. 控制措施

- Token：SHA-256 存储；启用位；过期；**scopes 显式授予**（空=无权限）
- HTTP：`RequireScope` 于 tools/mcp/token/settings/events/kb；confirm:approve
- RunTool（所有路径）：principal 存在时 **scope + EffectiveRequiredLevel**；PermissionEngine；确认流 fail-closed
- shell.run：`shell_profile` 默认 strict → **每一次** L2+确认+审计；argv-only 直接拒绝；黑名单含 LOLBins
- exec.argv：白名单二进制 + 参数元字符拒绝 + 无 shell 解释（推荐默认执行面）
- fs：read/list/exists=L0；write/mkdir/delete/organize=L2（工具 ActionRequiredLevel 与策略表一致）
- 事件策略：代码默认与 yaml 均 **不含** 静默 shell；`auto_execute: false`
- 绑定：默认 127.0.0.1；CORS 仅本机 origin

## 6. 残余风险（诚实列出）

| ID | 残余 | 缓解路线 |
|---|---|---|
| R1 | `shell_profile=full` 时仍是字符串解释（powershell/cmd/sh -c） | 避免使用 full；优先 exec.argv；OS 沙箱 |
| R2 | 黑名单/分类可被复杂载荷绕过（即使 strict，确认流若被自动化批准仍有风险） | OS 级沙箱（Job Object/容器/受限用户）；确认审批人独立 |
| R3 | bootstrap 空库可建 L3（窗口期） | 本机文件权限 + 首启向导；或要求本地 socket |
| R4 | Computer Use / MCP 高危面 | 默认关闭 feature flag / 不注册 MCP |
| R5 | Windows 上文件 ACL 弱（chmod 无效） | 文档 + BitLocker/DPAPI |
| R6 | 未做独立渗透审计 | README 声明；欢迎负责任披露 |
| R7 | 确认流超时/自动化批准会削弱 strict 档 | 审批操作审计；超时默认拒绝；勿将 confirm API 暴露给无人值守自动化 |
| R8 | Chat 路径在无 principal 的内部调用时仅靠策略门 | 所有 HTTP chat 请求已带认证 principal；禁止将 RunTool 暴露给未认证调用方 |
| R9 | exec.argv 白名单内 git/python/node 仍可造成副作用 | 白名单可配置收窄；生产建议仅 echo/ls 类诊断命令 |

## 7. 安全默认值

| 配置 | 默认 |
|---|---|
| `permissions.shell_profile` | `strict`（非 full，字符串 shell 每次确认） |
| 空 scopes token | 拒绝（迁移可临时 `LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1`） |
| shell:run | L2 + strict 每次确认 + 审计 |
| fs write/mkdir/delete/organize | L2 + 确认策略 |
| exec.argv | 注册；L1；白名单二进制 |
| `?token=` | 仅 `/events` `/sse` `/stream` |
| 新 token 默认 scopes | `tools:run,tasks:create` |
| bootstrap token scopes | `AdminTokenScopes`（全量） |
| 事件策略 tools | 不含 shell.run；`auto_execute: false` |
| 监听 | 127.0.0.1 |

## 8. 事故响应

1. 吊销可疑 token（L3 `token:manage` 或 DB `enabled=0`）  
2. 查 `audit_logs`（`shell.run` / `authz.denied` / `shell.denied`）与轨迹  
3. 将 `shell_profile` 降为 `argv-only` 并重启  
4. 轮换 LLM key（若 env 泄露）  
5. 见 `SECURITY.md`

## 9. 变更

| 日期 | 变更 |
|---|---|
| 2026-09-20 | Sprint1 初稿：空 scopes fail-closed、query token 收紧、shell L2 |
| 2026-09-21 | Sprint2：shell_profile 档位、exec.argv 白名单、fs 写删 L2 对齐、RunTool scope+审计、黑名单 LOLBins、事件策略去 shell、bootstrap AdminTokenScopes |
