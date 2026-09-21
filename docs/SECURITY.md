# 安全策略 — lumen

## 定位

个人本机 Agent Runtime 的**作品集/本地工具**实现，**非**多租户生产平台。

## 支持

| 版本 | 支持 |
|---|---|
| main / 最新 tag | 是 |
| 更早 | 尽力而为 |

## 报告漏洞

- GitHub Security Advisories（私密）优先
- 请提供：版本、复现步骤、影响、是否已利用
- 不要公网扫描他人部署（本项目本不应暴露公网）

## 安全默认承诺

- 默认本机绑定 `127.0.0.1`
- 空 API scopes 不授全权（fail-closed，含历史 L3 token）
- `permissions.shell_profile` 默认 `strict`：字符串 shell 每次 L2+确认+审计；`full` 需显式 opt-in
- 推荐执行面：`exec.argv` 白名单工具（无 shell 解释；默认仅只读诊断命令）
- **Computer Use / MCP 默认关闭**（`permissions.computer_use_enabled` / `mcp.enabled` 均为 false）
- 空库 HTTP bootstrap 默认拒绝；仅 loopback + 0600 secret（或本地交互确认）
- 危险工具（shell/computer/mcp/fs write·delete）需 L2 + 确认（启用后）
- `?token=` 仅 events/sse/stream；写 API 必须 Header 鉴权
- 无默认口令；token 仅创建时显示一次明文

## 已知限制

字符串 shell（尤其 `shell_profile=full`）+ 黑名单无法替代 OS 沙箱；Windows 上 `chmod` 对 bootstrap secret 无效，需 NTFS ACL。详见 `docs/THREAT_MODEL.md` 残余风险。

## 迁移说明（破坏性安全变更）

| 变更 | 影响 | 迁移 |
|---|---|---|
| 空 scopes 不再等于全权 | 旧 token 若 `scopes=''` 会 403 | 为 token 补写显式 scopes，或临时 `LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1` 后尽快迁移 |
| `?token=` 仅 events/sse | 非事件流客户端需改用 Header | `Authorization: Bearer` 或 `X-API-Token` |
| shell 策略默认 L2 + strict 每次确认 | L1 token 无法静默跑 shell；每次 shell 需审批 | 使用 L2+ token 并处理确认流；或改用 `exec.argv` |
| `shell_profile` 默认 `strict` | 依赖无人值守字符串 shell 的自动化会失败 | 显式配置 `permissions.shell_profile: full`（自担风险），或迁移到 `exec.argv` / 任务确认流 |
| fs write/mkdir 升至 L2 | L1 token 写文件需确认 | 使用 L2 token，或仅用 read/list |
| CLI `agent token` scopes 命名对齐 | 旧 CLI 写入的 `tools,chat` 等 scopes 无效 | 重新签发 token（新 CLI 写 `tools:run,...` / AdminTokenScopes） |
| **Sprint3：exec.argv 默认去掉 python/node/git** | 依赖解释器的 argv 调用会失败 | 配置 `permissions.argv_extra_allow: ["python","node","git"]`（自担风险），或改用确认流下的 `shell.run` |
| **Sprint3：argv 参数禁路径/env** | 传绝对路径或 `$VAR`/`%VAR%` 的调用被拒 | 改传 basename/字面量参数 |
| **Sprint3：computer / mcp 默认关闭** | dashboard/benchmark 里 computer、MCP servers 不可用 | 配置 `permissions.computer_use_enabled: true` / `mcp.enabled: true` 显式启用 |
| **Sprint3：空库 HTTP bootstrap 默认拒绝** | 首次运行 curl 建 token 会 403 | 本机 CLI：`agent token admin --level 3`；或 `agent bootstrap-secret` 后携带 `X-Lumen-Bootstrap-Secret`（loopback only） |
