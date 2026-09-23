# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Fixed（SSE 实时推送被路由冲突静默顶掉）
- **`GET /v1/events` 不再返回 `text/event-stream`** —— 被事件总线子路由覆盖
  - 现象：前端 `useSSE()` 的 `EventSource('/v1/events')` 收到 `application/json`，
    浏览器报 *EventSource's response has a MIME type ("application/json") that is
    not "text/event-stream". Aborting the connection.* → 侧栏**恒显「连接断开」**、
    全站无实时推送；且该 group 带 `RequireScope("events:emit")`，非 admin token
    （`DefaultTokenScopes` 不含 `events:emit`）读事件列表直接 **403**
  - 根因：`chi.Mux.Mount()` 会注册 pattern 本身及其 `"/*"`，并**静默覆盖**此前注册
    在同一 pattern 上的路由。原实现把 event-bus 子路由 `Mount("/events", ...)`，
    与上方的 `r.Get("/events", SSEHandler(...))` 撞 pattern，SSE 端点被顶掉
  - 修法：事件总线 REST 改挂 **`/eventbus`**（`GET /eventbus`、
    `POST /eventbus/emit`、`DELETE /eventbus?keep_days=N`），`/v1/events` 交还 SSE
  - 前端：`Today` / `Overview` / `Events` 三个页面改调 `/eventbus*`
  - 附带收益：`/v1/eventbus` 不再命中 `?token=` 查询参数豁免
    （该豁免按 `strings.Contains(p, "/events")` 判定，仅应覆盖事件流），
    事件总线读写统一走 Header 鉴权
  - 回归锁：`internal/api/router_events_test.go` 断言 `/v1/events` 下不得挂任何子路由

### Security（Sprint3 攻击面再收敛）
- **exec.argv 默认白名单收紧**：移除 `python/node/git/go/npm` 等高副作用二进制
  - 默认仅保留只读/诊断命令：`ls/dir/echo/cat/type/ping/ipconfig/ifconfig/hostname/date/whoami/pwd/true/false/where/...`
  - 高副作用二进制需显式 `permissions.argv_extra_allow`（basename；拒绝路径形式）
  - 参数校验加严：拒绝绝对路径、路径分隔符、环境变量样式（`$VAR` / `%VAR%`）
- **Computer Use / MCP 默认关闭**（不仅 require confirm）
  - `permissions.computer_use_enabled: false`（默认不注册 computer；Execute/RunTool 拒绝）
  - `mcp.enabled: false`（默认不 Attach/不启动 servers；`mcp.register` HTTP 与 `McpRegistry.Register` 拒绝）
  - conf/policy.yaml `features.computer_use/mcp_register: false`；事件策略代码默认同步 `auto_execute: false` 且不含 shell/computer/mcp
- **Bootstrap L3 空库门**
  - 空库未认证 HTTP 建 L3 token **默认拒绝**
  - 仅当：服务绑定 loopback **且** 请求来源 loopback，**且**（0600 bootstrap secret 文件 + 匹配头 `X-Lumen-Bootstrap-Secret` **或** 本地交互确认 `LUMEN_BOOTSTRAP_INTERACTIVE=1` + `X-Lumen-Bootstrap-Confirm: local-ok`）
  - 新增 CLI：`agent bootstrap-secret`；首选仍是本机 `agent token admin --level 3`
- 文档：`docs/THREAT_MODEL.md` 残余风险 R3/R4/R9/R10；`docs/SECURITY.md` 迁移表；Sprint3 CHANGELOG

### Security（Sprint2 攻击面收敛）
- **`permissions.shell_profile` 默认 `strict`**：默认策略永不提供无 opt-in 的自由字符串 shell
  - `strict`（默认）：`shell.run` 每次强制 L2 + 人工确认 + audit_logs
  - `argv-only`：禁用 `shell.run`，仅允许 `exec.argv`
  - `full`：显式 opt-in 才放开字符串 shell（破坏性命令仍需确认）
- 新增 **`exec.argv` 白名单工具**：不经 shell 解释，二进制 basename 白名单 + 参数元字符拒绝
- **FilesystemTool 权限对齐**：`ActionRequiredLevel`（read/list/exists=L0；write/mkdir/delete/organize=L2）；策略表 `fs:write/mkdir` 升至 L2，`fs:*` fail-closed L2
- **RunTool 全路径**：principal 存在时强制 `tools:run` scope + `EffectiveRequiredLevel ≤ PermLevel`；shell.run 审计必写
- `?token=` 查询鉴权仅限 events/sse/stream（写/工具 API 一律 Header）
- 空 scopes fail-closed（含 L3）；bootstrap 默认 `AdminTokenScopes`；CLI token scopes 与 `auth.HasScope` 冒号命名对齐
- shell 黑名单加严：certutil/bitsadmin/mshta/rundll32/regsvr32/IEX/EncodedCommand 等
- 事件策略（代码默认 + conf/policy.yaml）不再默认放行 `shell.run`
- 文档：`docs/THREAT_MODEL.md` 残余风险 R1–R9 诚实更新

### Security（Sprint1 权限硬化）
- **空 API token scopes 不再等于全部权限**（fail-closed）；迁移：为旧 token 写入显式 scopes，或临时 `LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1`
- `?token=` 查询参数鉴权 **仅** 允许 `/events`、`/sse`、`/stream`
- `/confirmations` 需要 `confirm:approve` scope；批准仍校验 PermLevel ≥ 风险级别
- 默认新 token scopes 最小化（`tools:run,tasks:create`）；bootstrap 管理员 token 仍可显式全量
- `shell.run` / `fs:delete` 等策略升至 L2（需确认）；`ShellTool.RequiredLevel=2`
- RunTool：策略确认与破坏性命令确认合并为 **单次** fail-closed 门
- 文档：`docs/THREAT_MODEL.md`、`docs/SECURITY.md`


- 9095d83 ci: lumen CI 加 govulncheck 漏洞扫描
- 63f7b62 feat: useSSE 断线自动重连（指数退避 1s→30s）
- a68d446 feat: 多模型回退链（FallbackProvider 自动切换备用 provider）
- 1eb344f feat: 破坏性命令走确认流（loop 层拦截 + shell.go 保留最后防线）
- 2873575 feat: 新增 fs.grep 文本搜索工具（正则/沙箱/截断）
- 1a3c4dd feat: statusHandler 接入真实 completed 计数（CountTasks SQL）
- 84bbf20 docs: 更新 README 项目结构与测试徽章，DEPLOY 补充 openagent 服务名说明
- 641cc04 chore: web/dist 加入 .gitignore（postbuild 会重新生成，不跟踪）
- 076f478 build: web postbuild 自动同步 dist 到 internal/api/static
- d51cd5f chore: 清理仓库残留——benchmark 产物出库、web/dist 停止跟踪、policy.yaml 收敛
- 664ca9f fix: 权限三件套接入 + benchmark 隔离 + DEPLOY.md 对齐现实
- b59d05f fix: P0 修复——前端构建回归、host 回退、真 recover、SSE 认证链路
- 7397c4b fix: config.yaml.example security fixes (restored from clean commit)
- 305e796 fix: add splitShellOperators for command safety classification
- ddfcd39 fix: restore all Go files from clean commit, re-apply legitimate fixes
- 2c7a3a8 fix: service.go - replace corrupted file with clean ASCII version
- 67740ee fix: 修复 service.go 随机空格损坏，恢复编译
- e845521 fix: Tools.tsx 移除硬编码路径改用 HOME
- a87b965 fix(security): 示例配置 host 改 127.0.0.1、占位 key 改 env 引用、修复 ss 笔误
- 94c272a fix(security): 移除 /events 认证豁免

## Sprint12 (argv adversarial)

- checkArgvSafe now rejects quote characters (\" / ') to block breakout tokens even without a shell.
- Extra tests: unicode/metachar/UNC/env injection, path-form extra_allow rejection, dangerous binaries stay default-off, shell/computer/MCP default-off lock.
