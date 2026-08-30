# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

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

