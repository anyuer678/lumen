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

- 默认本机绑定
- 空 API scopes 不授全权
- 危险工具（shell/computer/mcp/fs:delete）需确认
- 无默认口令；token 仅创建时显示一次明文

## 已知限制

字符串 shell + 黑名单无法替代 OS 沙箱；详见 `docs/THREAT_MODEL.md`。

## 迁移说明（破坏性安全变更）

| 变更 | 影响 | 迁移 |
|---|---|---|
| 空 scopes 不再等于全权 | 旧 token 若 `scopes=''` 会 403 | 为 token 补写显式 scopes，或临时 `LUMEN_ALLOW_LEGACY_EMPTY_SCOPES=1` 后尽快迁移 |
| `?token=` 仅 events/sse | 非事件流客户端需改用 Header | `Authorization: Bearer` 或 `X-API-Token` |
| shell 策略默认 L2 | L1 token 无法静默跑 shell | 使用 L2+ token 并处理确认流 |
