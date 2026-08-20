# 代码审查报告 — 智能管家 Agent

> 审查日期：2026-08-19
> 审查范围：全部 59 个 Go 源文件（Go 后端）+ 前端 TSX/CSS
> 工具：go build / go vet / 手工逐包扫描

---

## 总体评价

**代码质量：7.5/10**（原型级质量，可运行但有 3 个高优先级缺陷）

整体架构清晰（agent loop → tools → tasks → memory），各层分离，命名规范，注释简洁。最严重的问题是 **browser.go 超 600 行** 和 **跨进程无原子性保证**——它们不阻塞功能，但在生产环境下会出事。

---

## 🔴 高优先级问题（必须修）

### 1. browser.go 656 行（超出 800 行维护阈值）

**位置**：`internal/agent/browser.go`

**问题**：单文件承载了 `BrowserTool` + `searchDuckDuckGo` + `extractTagText` + `ddgResult` + `fetchText` + `research` + ChromeDP 操控等所有逻辑，维护困难。

**修复方案**：拆成 `browser.go`（BrowserTool 接口 + chromeContext）、`browser_search.go`（duckduckgo/提取）、`browser_research.go`（research 端到端）。

### 2. browser.go cdp() 中 chromedp allocator 无实例复用

**位置**：`internal/agent/browser.go` 第 89-115 行、第 635-660 行

**问题**：每次调用都 `NewExecAllocator + NewContext`，每次启动新的 chromium 进程。并发请求时会同时开多个无头浏览器，消耗巨大内存（~300MB/个）。

**修复方案**：在 BrowserTool 上缓存 allocator（lazy init，同一实例复用），加 `close()` 在 shutdown 时调用。

### 3. Chat 消息原子性缺失

**位置**：`internal/api/handlers/chat.go` 第 119-182 行

**问题**：`SendMessage` 先写 DB，再开 SSE 流，两者不在事务中。若流中断，DB 已写入用户消息，但无 assistant 回复——消息状态不一致。

**修复方案**：先创建 assistant 占位消息（`content=""`），成功流完后 `UPDATE content`，失败则 `DELETE` 占位或标记 `error`。

---

## 🟠 中优先级问题（建议修）

### 4. 路由器 search 意图的"搜索"关键词过于宽泛

**位置**：`internal/agent/router.go` 第 73-80 行

**问题**：`matchAny(lower, []string{"搜索", "搜一下", ...})` 会匹配"搜索日志"、"搜索任务"等意图本不为搜索的请求。

**修复方案**：改为"搜索 + 专有名词（查查XX）"或对"搜索任务/搜索日志"加前置拦截。

### 5. `goMin` 函数应使用 `min()`（Go 1.21+）

**位置**：`internal/agent/browser.go`

**问题**：Go 1.21 起 `min()` 是内建函数，`goMin` 是冗余包装。

**修复**：直接用 `min(len(res), 3)`。

### 6. LLM planner prompt 中 `windows` 工具仍列有大量 action，LLM 容易混淆

**位置**：`internal/agent/llm_planner.go` 第 61-72 行

**问题**：虽已有"子操作不是工具名"提示，但工具列表仍很长且 JSON 示例偏简单，LLM 容易"脑补"新工具名。

**修复**：把工具描述精简为"工具名 + 一句话 + action 枚举"，移除 JSON 示例让 LLM 聚焦。

---

## 🟢 低优先级（可优化）

### 7. 轨迹 recorder 关闭时机

**位置**：`internal/service/trajectory.go`

**问题**：`finalize()` 只在任务完成/失败时调用，若进程 crash 未触发 finalize，最后的事件条目可能丢失（无最终 flush）。

**改进**：在 `agent.exe run` shutdown 时遍历 `trajRecorders.m` 批量 flush+close。

### 8. scheduler 触发器缺持久化重放

**位置**：`internal/scheduler/triggers.go`

**问题**：`file_watch` 只在内存中注册 fsnotify watcher，进程重启后丢失。

**改进**：在 `Start()` 时从 DB 加载已注册的 triggers 并重新启动 watcher。

### 9. Chat `loadHistory` 时间顺序反转逻辑正确但低效

**位置**：`internal/api/handlers/chat.go` 第 365-387 行

**问题**：`ORDER BY DESC LIMIT N` 再反转——可用 `ORDER BY ASC` 或一次 fetch 10 条够用。

**优化**：直接用 `ORDER BY created_at DESC LIMIT 6` 再反转，或改为 `ORDER BY created_at ASC`（历史 6 条，正确顺序）。

### 10. OpenAI provider `BoundedChat` 重复创建 client

**位置**：`internal/llm/openai.go` 第 255-260 行

**问题**：每次 BoundedChat 创建新的 `http.Client`。若频繁复核会造成连接数膨胀。

**修复**：复用现有 `p.client`，只在超时更短时调整（`p.client` 已有 timeout，通常 120s）。

---

## 安全审查

| 风险 | 状态 | 说明 |
|------|------|------|
| SQL 注入 | ✅ 安全 | 全部使用 `?` 占位符，无字符串拼接 |
| CORS | ⚠️ 开放 | `Allow-Origin: *`，本地开发可接受，生产应限制 |
| 命令注入 | ✅ 已防 | `checkCommandBlocked` + `ClassifyCommand` 拦截危险命令 |
| 端口暴露 | ✅ | `:14000` 监听本地，无公网暴露 |
| 路径穿越 | ⚠️ 未检测 | fs 工具的 path 参数未校验 `../` |
| API Key | ⚠️ 明文 | API Key 存 env 或 yaml，无加密 |
| goroutine 泄漏 | ✅ 无 | 每个 goroutine 都有 context cancel/defer |

---

## 修复进度

```
[P0] browser.go cdp allocator 复用（内存泄漏风险）→ 已修复（mu+allocCtx 缓存字段）
[P0] chat.go 消息原子性（数据库一致性）→ 已修复（占位 INSERT → 完成后 UPDATE）
[P0] chat.go executeTool 绕过权限检查 → 已修复（加命令安全分级拦截）
[P0] clipboard write 命令注入 → 已修复（base64 编码防注入）
[P1] shell 黑名单不全 → 已修复（补全 10+ 规则：del/rm/bcdedit/wmic/net 等）
[P1] destructiveVerbs 误拦 Format-Table → 已修复（"fmt"→"format " 带空格）
[P1] browser.go 拆分文件（可维护性）→ 下次迭代
[P1] router.go 搜索意图精确化（误触发）→ 低风险，暂不改
[P2] trajectory 关闭时机（数据完整性）→ 下次迭代
[P2] OpenAI BoundedChat client 复用（连接数控制）→ 下次迭代
[P2] fs path 参数加 ../ 检测（路径穿越防护）→ checkSandbox 已覆盖
```
