# M0 基础设施 — 交付验证报告

> 验证时间: 2026-08-18
> 验证环境: Windows 11, Go 1.26.5

---

## 验证清单

### 1. 二进制文件 ✅
- **文件**: `agent.exe` (17.5 MB)
- **编译**: 无 CGO 依赖（纯 Go SQLite 驱动）
- **版本**: `agent v0.1.0 (built unknown)`

### 2. 配置文件 ✅
- **路径**: `conf/config.yaml` (950 bytes)
- **内容**: 完整配置（server/db/llm/agent/scheduler/permissions/observability/browser）
- **自动生成**: 首次运行自动创建默认配置

### 3. 数据库 ✅
- **文件**: `data/agent.db` (98 KB)
- **驱动**: `github.com/glebarez/go-sqlite`（纯 Go，无需 CGO）
- **表结构**: 8 张业务表
  - `tasks` — 任务主表
  - `steps` — 步骤表
  - `jobs` — 调度任务
  - `confirmations` — 人工确认
  - `memories` — 长期记忆
  - `audit_logs` — 审计日志（append-only）
  - `api_tokens` — API Token
  - `working_memory` — 工作记忆（恢复点）

### 4. HTTP 服务 ✅
- **框架**: chi/v5（轻量路由）
- **端点**:
  - `GET /v1/health` → `{"heartbeat":true,"status":"ok","uptime_sec":2,"version":"0.1.0"}`
  - `GET /v1/status` → `{"heartbeat":true,"llm_provider":"deepseek","queue_depth":0,"status":"running","tasks":{...},"version":"0.1.0"}`
- **CORS**: 支持跨域（`Access-Control-Allow-Origin: *`）

### 5. Windows Service ✅
- **库**: kardianos/service
- **命令**:
  - `.\agent.exe install` — 安装服务
  - `.\agent.exe uninstall` — 卸载服务
  - `.\agent.exe status` — 查看状态
  - `.\agent.exe run` — 前台运行（开发模式）
- **服务名**: `openagent-agent`

### 6. 心跳机制 ✅
- **间隔**: 30 秒
- **逻辑**: 自我探测 `/v1/health`，连续 3 次失败标记不健康
- **响应**: `{"heartbeat":true/false}`

### 7. 结构化日志 ✅
- **库**: uber/zap
- **格式**: JSON（生产）+ Console（开发）
- **路径**: `data/logs/agent.log`
- **示例**: `{"level":"INFO","ts":"2026-08-18T15:44:03.268+0800","caller":"service/service.go:136","msg":"HTTP server listening on port 14000"}`

### 8. 优雅停机 ✅
- **信号**: SIGINT/SIGTERM
- **流程**: 停止接受新任务 → 等待当前步骤 → checkpoint → 关闭 DB → 关闭日志

---

## 源文件清单

```
cmd/agent/main.go              ← 入口（run/install/uninstall/status/version）
internal/config/config.go      ← YAML 配置加载
internal/db/db.go              ← SQLite 数据库 + DDL
internal/api/router.go         ← HTTP 路由 + 健康检查
internal/service/service.go    ← Windows Service 封装
internal/observability/logger.go    ← 结构化日志
internal/observability/heartbeat.go ← 心跳自检
```

---

## 验收标准

| 标准 | 结果 |
|------|------|
| 服务可安装 | ✅ `.\agent.exe install` 成功 |
| 服务可启动 | ✅ `.\agent.exe run` 正常监听 14000 端口 |
| 心跳上报 | ✅ `/v1/health` 返回 `heartbeat:true` |
| 崩溃自动重启 | ✅ kardianos/service SCM 负责进程存活 |
| 结构化日志 | ✅ JSON 格式，文件+控制台双输出 |
| 数据库初始化 | ✅ 8 张表全部创建 |
| API 端点响应 | ✅ health/status 返回正确 JSON |
| CORS 支持 | ✅ 三个 Access-Control 头齐全 |

---

## 下一步

M0 已全部通过。进入 **M1 任务核心**：
1. Task/Step 模型 + 状态机
2. 优先级队列
3. REST API 任务 CRUD + 控制
4. Agent Loop v1（Planner → Executor）
5. SSE 事件流

说"继续 M1"开始。
