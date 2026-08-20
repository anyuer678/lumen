# OpenAgent Agent 部署文档

## 快速开始

### 1. 下载

```powershell
# 下载最新版本
Invoke-WebRequest -Uri "https://github.com/your-repo/agent/releases/latest/download/agent.exe" -OutFile "agent.exe"
```

### 2. 配置

```powershell
# 首次运行会自动生成配置文件
.\agent.exe run

# 或手动创建配置文件
mkdir conf
# 编辑 conf/config.yaml
```

### 3. 运行

```powershell
# 前台运行（开发模式）
.\agent.exe run

# 安装为 Windows Service
.\agent.exe install

# 查看服务状态
.\agent.exe status

# 卸载服务
.\agent.exe uninstall
```

## 配置说明

### config.yaml

```yaml
server:
  host: "0.0.0.0"
  port: 14000

service:
  name: "openagent-agent"
  display_name: "OpenAgent Agent Service"

db:
  path: "./data/agent.db"

llm:
  default_provider: "deepseek"
  providers:
    deepseek:
      type: "openai-compatible"
      base_url: "https://api.deepseek.com/v1"
      api_key_env: "DEEPSEEK_API_KEY"
      model: "deepseek-chat"

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
| `DEEPSEEK_API_KEY` | DeepSeek API 密钥 |
| `OPENAI_API_KEY` | OpenAI API 密钥 |
| `AGENT_CONFIG` | 配置文件路径 |

## API 使用

### 创建任务

```bash
curl -X POST http://localhost:14000/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"goal": "echo Hello World", "priority": 5}'
```

### 查看任务

```bash
curl http://localhost:14000/v1/tasks
curl http://localhost:14000/v1/tasks/{id}
```

### 控制任务

```bash
# 暂停
curl -X POST http://localhost:14000/v1/tasks/{id}/pause

# 恢复
curl -X POST http://localhost:14000/v1/tasks/{id}/resume

# 终止
curl -X POST http://localhost:14000/v1/tasks/{id}/stop
```

### 实时事件

```bash
curl http://localhost:14000/v1/events
```

## Dashboard

```bash
cd web
npm install
npm run dev
```

访问 http://localhost:3000

## 故障排除

### 服务无法启动

```powershell
# 检查端口占用
netstat -ano | findstr :14000

# 查看日志
Get-Content data/logs/agent.log -Tail 50
```

### 数据库问题

```powershell
# 删除数据库重新开始
Remove-Item data/agent.db
```

### 重置配置

```powershell
Remove-Item conf/config.yaml
.\agent.exe run  # 自动生成默认配置
```

## 安全建议

1. **生产环境**：启用 HTTPS（使用反向代理）
2. **API 密钥**：使用环境变量，不要硬编码
3. **权限**：为不同用户设置不同权限等级
4. **审计**：定期检查审计日志
