# Lumen 部署文档

> 与 `conf/config.yaml.example` 配套阅读。示例配置默认监听 `127.0.0.1:18080`。

## 快速开始

### 1. 构建

```powershell
# 前端（产物 embed 进二进制，必须先构建）
cd web
npm ci
npm run build
cd ..

# 后端
go build -o lumen.exe ./cmd/agent
```

### 2. 配置

```powershell
# 复制示例配置并编辑（API key 建议走环境变量，不要明文入库）
copy conf\config.yaml.example conf\config.yaml
```

要点：
- `server.host` 保持 `127.0.0.1`（仅本机访问）。改成 `0.0.0.0` 会把
  Agent API 暴露到局域网，除非你清楚自己在做什么。
- `server.port` 示例配置为 `18080`（代码内置默认 14000）。
- LLM provider 的 key 通过 `api_key_env` 引用环境变量。

### 3. 生成访问 token

```powershell
# 创建 API token（perm_level 3 / admin scopes）
.\lumen.exe token my-token
```

所有 `/v1/*` 端点（含 `/v1/events` SSE）都要求认证；SSE 无法携带
请求头，前端/脚本可用 `?token=<值>` 查询参数。

### 4. 运行

```powershell
# 前台运行（无子命令，直接执行即可）
.\lumen.exe

# 安装为 Windows Service / 状态 / 卸载
.\lumen.exe install
.\lumen.exe status
.\lumen.exe uninstall

# 版本
.\lumen.exe version

# Benchmark（在隔离的临时数据库上运行，报告写入临时目录）
.\lumen.exe benchmark --v3
```

## 配置说明

### config.yaml

```yaml
server:
  host: "127.0.0.1"
  port: 18080

service:
  name: "openagent-agent"
  display_name: "OpenAgent Agent Service"

db:
  path: "./data/agent.db"

llm:
  default_provider: "zhipu"
  providers:
    zhipu:
      type: "openai-compatible"
      base_url: "https://open.bigmodel.cn/api/paas/v4"
      api_key_env: "ZHIPU_API_KEY"
      model: "glm-4.6"

agent:
  max_concurrent_tasks: 3

permissions:
  confirm_timeout: "60s"

browser:
  engine: "playwright"
  headful: true
```

### 环境变量

| 变量 | 说明 |
|------|------|
| `ZHIPU_API_KEY` | 智谱 API 密钥 |
| `DEEPSEEK_API_KEY` | DeepSeek API 密钥 |
| `AGENT_CONFIG` | 配置文件路径 |

## API 使用

所有请求携带 `Authorization: Bearer <token>`（token 见第 3 步）。

### 创建任务

```bash
curl -X POST http://127.0.0.1:18080/v1/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $LUMEN_TOKEN" \
  -d '{"goal": "echo Hello World", "priority": 5}'
```

### 查看任务

```bash
curl -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/tasks
curl -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/tasks/{id}
```

### 控制任务

```bash
# 暂停
curl -X POST -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/tasks/{id}/pause

# 恢复
curl -X POST -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/tasks/{id}/resume

# 终止
curl -X POST -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/tasks/{id}/stop
```

### 实时事件

```bash
curl -N -H "Authorization: Bearer $LUMEN_TOKEN" http://127.0.0.1:18080/v1/events
```

## Dashboard

```bash
cd web
npm ci
npm run dev
```

## 安全说明

- 默认仅监听 `127.0.0.1`；所有 `/v1/*` 端点要求 Bearer token
  （SSE 支持 `?token=` 查询参数）。
- 危险操作（`fs:delete`、`windows:launch`、`computer:*`、`mcp:*`）
  由 PermissionEngine 策略表要求人工确认；子代理步骤同样受策略
  约束，无法发起确认时会直接拒绝。
- 编码式 PowerShell 调用（`-e/-enc/-EncodedCommand`）一律按破坏性
  处理。

## 故障排除

- **401 Unauthorized**：未携带 token 或 token 已禁用/过期，重新
  `.\lumen.exe token <名称>` 生成。
- **端口被占用**：改 `conf/config.yaml` 的 `server.port`。
- **服务安装后无法启动**：检查 `.\lumen.exe status` 与 Windows
  事件查看器；确认配置文件路径可通过 `AGENT_CONFIG` 访问。
