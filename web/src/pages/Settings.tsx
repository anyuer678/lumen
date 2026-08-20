import { useEffect, useState, useCallback } from 'react'
import { api } from '../api/client'
import { Card, Button, Input, Select, Loading, Badge, Tabs } from '../components'
import { useToast } from '../components/Toast'

export default function Settings() {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const { showToast } = useToast()
  const [activeTab, setActiveTab] = useState('llm')

  // LLM 配置
  const [provider, setProvider] = useState('zhipu')
  const [baseUrl, setBaseUrl] = useState('')
  const [model, setModel] = useState('')
  const [apiKeyEnv, setApiKeyEnv] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [maxTokens, setMaxTokens] = useState(4096)
  const [providers, setProviders] = useState<Record<string, any>>({})

  // Agent 配置
  const [maxConcurrent, setMaxConcurrent] = useState(3)
  const [stepTimeout, setStepTimeout] = useState('300')
  const [stepRetries, setStepRetries] = useState(3)
  const [planMaxSteps, setPlanMaxSteps] = useState(10)

  // 浏览器
  const [browserEngine, setBrowserEngine] = useState('chromedp')
  const [browserHeadless, setBrowserHeadless] = useState(true)
  const [proxySocks5, setProxySocks5] = useState('')

  // 调度器
  const [tickInterval, setTickInterval] = useState('60')

  // 权限 & 安全
  const [confirmTimeout, setConfirmTimeout] = useState('60')

  // 可观测性
  const [logLevel, setLogLevel] = useState('info')
  const [auditEnabled, setAuditEnabled] = useState(true)

  // 运行时状态
  const [status, setStatus] = useState<any>(null)

  // 预设模板
  const presets: Record<string, { base_url: string; model: string; api_key_env: string }> = {
    deepseek: { base_url: 'https://api.deepseek.com/v1', model: 'deepseek-chat', api_key_env: 'DEEPSEEK_API_KEY' },
    openai:   { base_url: 'https://api.openai.com/v1', model: 'gpt-4o', api_key_env: 'OPENAI_API_KEY' },
    ollama:   { base_url: 'http://127.0.0.1:11434/v1', model: 'qwen3:0.6b', api_key_env: '' },
    zhipu:    { base_url: 'https://open.bigmodel.cn/api/paas/v4', model: 'glm-4-flash', api_key_env: 'ZHIPU_API_KEY' },
    siliconflow: { base_url: 'https://api.siliconflow.cn/v1', model: 'Qwen/Qwen2.5-7B-Instruct', api_key_env: 'SILICONFLOW_API_KEY' },
  }

  const loadSettings = useCallback(async () => {
    try {
      const [data, st] = await Promise.all([api.getSettings(), api.getStatus()])
      const p = data.llm?.default_provider || 'zhipu'
      const pc = data.llm?.providers?.[p] || {}
      setProvider(p)
      setBaseUrl(pc.base_url || '')
      setModel(pc.model || '')
      setApiKeyEnv(pc.api_key_env || '')
      setMaxTokens(pc.max_tokens || 4096)
      setApiKey('')
      setProviders(data.llm?.providers || {})
      setMaxConcurrent(data.agent?.max_concurrent_tasks ?? 3)
      setStepTimeout(String(data.agent?.step_timeout || '300'))
      setStepRetries(data.agent?.step_max_retries ?? 3)
      setPlanMaxSteps(data.agent?.plan_max_steps ?? 10)
      setBrowserEngine(data.browser?.engine || 'chromedp')
      setBrowserHeadless(data.browser?.headful ?? true)
      setProxySocks5(data.browser?.proxy_socks5 || '')
      setTickInterval(String(data.scheduler?.tick_interval || '60'))
      setConfirmTimeout(String(data.permissions?.confirm_timeout || '60'))
      setLogLevel(data.observability?.log_level || 'info')
      setAuditEnabled(data.observability?.audit_enabled ?? true)
      setStatus(st)
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { loadSettings() }, [loadSettings])

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.updateSettings({
        llm: {
          default_provider: provider,
          api_key: apiKey || undefined,
          api_key_env: apiKeyEnv,
          providers: {
            [provider]: {
              type: 'openai-compatible',
              base_url: baseUrl,
              api_key_env: apiKeyEnv,
              api_key: apiKey || undefined,
              model,
              max_tokens: maxTokens,
            },
          },
        },
        agent: {
          max_concurrent_tasks: maxConcurrent,
          step_timeout: stepTimeout + 's',
          step_max_retries: stepRetries,
          plan_max_steps: planMaxSteps,
        },
        browser: { engine: browserEngine, headful: browserHeadless, proxy_socks5: proxySocks5 },
        scheduler: { tick_interval: tickInterval + 's' },
        permissions: { confirm_timeout: confirmTimeout + 's' },
        observability: { log_level: logLevel, audit_enabled: auditEnabled, metrics_enabled: true },
      })
      showToast('配置已保存，部分变更需重启服务生效', 'success')
      loadSettings()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSaving(false)
    }
  }

  const handleTestLLM = async () => {
    setTesting(true)
    try {
      const r = await api.testLLM({ provider, base_url: baseUrl, api_key: apiKey, model })
      showToast(r.message, r.status === 'ok' ? 'success' : 'error')
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setTesting(false)
    }
  }

  if (loading) return <Loading block text="加载配置中..." />

  return (
    <div>
      {/* 顶部栏 */}
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        marginBottom: 20, padding: '18px 20px',
        background: 'linear-gradient(135deg, var(--color-primary-light), transparent)',
        border: '1px solid var(--color-border)', borderRadius: 'var(--radius-lg)',
      }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>系统设置</h1>
          <p style={{ color: 'var(--color-text-2)', fontSize: 13, marginTop: 6 }}>配置 Agent 各模块参数 · 修改后保存需重启服务生效</p>
        </div>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          {status && (
            <Badge variant={status.heartbeat ? 'success' : 'danger'}>
              {status.heartbeat ? '运行中' : '异常'}
            </Badge>
          )}
          <Button variant="primary" onClick={handleSave} loading={saving}>
            {saving ? '保存中...' : '保存配置'}
          </Button>
        </div>
      </div>

      <Tabs
        items={[
          { key: 'llm', label: '🧠 模型' },
          { key: 'agent', label: '🤖 Agent' },
          { key: 'browser', label: '🌐 浏览器' },
          { key: 'scheduler', label: '⏰ 调度' },
          { key: 'security', label: '🔒 安全' },
          { key: 'obs', label: '📊 日志' },
        ]}
        active={activeTab}
        onChange={setActiveTab}
      />

      <div style={{ marginTop: 16 }}>
        {/* ========== 模型配置 ========== */}
        {activeTab === 'llm' && (
          <div>
            {/* 预设快捷按钮 */}
            <Card shadow style={{ marginBottom: 16 }}>
              <div style={{ marginBottom: 12, fontSize: 14, fontWeight: 600 }}>快速选择模型提供商</div>
              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                {Object.entries(presets).map(([name, p]) => (
                  <button
                    key={name}
                    className={`kb-btn ${provider === name ? 'kb-btn--primary' : ''}`}
                    onClick={() => {
                      setProvider(name); setBaseUrl(p.base_url); setModel(p.model); setApiKeyEnv(p.api_key_env)
                    }}
                  >
                    {name === 'zhipu' ? '智谱 GLM' : name === 'deepseek' ? 'DeepSeek' : name === 'ollama' ? 'Ollama (本地)' : name === 'openai' ? 'OpenAI' : 'SiliconFlow'}
                  </button>
                ))}
              </div>
            </Card>

            <Card shadow style={{ marginBottom: 16 }}>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                <div className="kb-form-item">
                  <label className="kb-form-label">Provider 名称</label>
                  <Input value={provider} onChange={e => setProvider(e.target.value)} placeholder="zhipu / deepseek / openai" />
                </div>
                <div className="kb-form-item">
                  <label className="kb-form-label">模型</label>
                  <Input value={model} onChange={e => setModel(e.target.value)} placeholder="glm-4-flash" />
                </div>
                <div className="kb-form-item">
                  <label className="kb-form-label">Base URL</label>
                  <Input value={baseUrl} onChange={e => setBaseUrl(e.target.value)} placeholder="https://open.bigmodel.cn/api/paas/v4" />
                </div>
                <div className="kb-form-item">
                  <label className="kb-form-label">API Key 环境变量</label>
                  <Input value={apiKeyEnv} onChange={e => setApiKeyEnv(e.target.value)} placeholder="ZHIPU_API_KEY（空=本地无需Key）" />
                </div>
                <div className="kb-form-item">
                  <label className="kb-form-label">API Key（填入即激活，保存时写入环境变量）</label>
                  <Input type="password" value={apiKey} onChange={e => setApiKey(e.target.value)} placeholder="sk-... 或留空" />
                </div>
                <div className="kb-form-item">
                  <label className="kb-form-label">Max Tokens</label>
                  <Input type="number" value={maxTokens} onChange={e => setMaxTokens(Number(e.target.value))} />
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
                <Button size="small" variant="primary" onClick={handleTestLLM} loading={testing}>
                  {testing ? '测试中...' : '测试连接'}
                </Button>
                <span style={{ fontSize: 12, color: 'var(--color-text-3)', lineHeight: '28px' }}>
                  测试将向当前 Base URL 发送 "hi" 请求
                </span>
              </div>
            </Card>

            {/* 当前已配置的 providers 一览 */}
            <Card title="已配置 Providers" shadow>
              {Object.keys(providers).length === 0 ? (
                <div style={{ color: 'var(--color-text-3)', fontSize: 13 }}>暂无</div>
              ) : (
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))', gap: 10 }}>
                  {Object.entries(providers).map(([name, p]: [string, any]) => (
                    <div key={name} style={{
                      padding: '10px 14px', borderRadius: 6,
                      border: name === provider ? '2px solid var(--color-primary)' : '1px solid var(--color-border)',
                      background: name === provider ? 'var(--color-primary-light)' : 'var(--color-bg-elevated)',
                      fontSize: 13,
                    }}>
                      <div style={{ fontWeight: 600 }}>{name}</div>
                      <div style={{ color: 'var(--color-text-3)', marginTop: 4 }}>{p.model}</div>
                      <div style={{ color: 'var(--color-text-3)', fontSize: 12, marginTop: 2 }}>{p.base_url}</div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          </div>
        )}

        {/* ========== Agent 配置 ========== */}
        {activeTab === 'agent' && (
          <Card shadow>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div className="kb-form-item">
                <label className="kb-form-label">最大并发任务</label>
                <Input type="number" value={maxConcurrent} onChange={e => setMaxConcurrent(Number(e.target.value))} />
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">步骤超时（秒）</label>
                <Input type="number" value={stepTimeout} onChange={e => setStepTimeout(e.target.value)} />
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">最大重试次数</label>
                <Input type="number" value={stepRetries} onChange={e => setStepRetries(Number(e.target.value))} />
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">计划最大步骤数</label>
                <Input type="number" value={planMaxSteps} onChange={e => setPlanMaxSteps(Number(e.target.value))} />
              </div>
            </div>
          </Card>
        )}

        {/* ========== 浏览器配置 ========== */}
        {activeTab === 'browser' && (
          <Card shadow>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div className="kb-form-item">
                <label className="kb-form-label">浏览器引擎</label>
                <Select value={browserEngine} onChange={e => setBrowserEngine(e.target.value)}>
                  <option value="chromedp">ChromeDP（Chrome/Edge 原生 CDP）</option>
                  <option value="playwright">Playwright</option>
                </Select>
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">可见模式</label>
                <Select value={browserHeadless ? 'true' : 'false'} onChange={e => setBrowserHeadless(e.target.value === 'true')}>
                  <option value="false">显示浏览器窗口</option>
                  <option value="true">后台无头运行</option>
                </Select>
              </div>
              <div className="kb-form-item" style={{ gridColumn: 'span 2' }}>
                <label className="kb-form-label">SOCKS5 代理（留空=直连）</label>
                <Input value={proxySocks5} onChange={e => setProxySocks5(e.target.value)} placeholder="socks5://127.0.0.1:7890" />
              </div>
            </div>
            <div style={{ marginTop: 12, fontSize: 12, color: 'var(--color-text-3)' }}>
              ChromeDP 模式使用系统已安装的 Chrome/Edge/Chromium；Playwright 模式自动下载浏览器。
            </div>
          </Card>
        )}

        {/* ========== 调度配置 ========== */}
        {activeTab === 'scheduler' && (
          <Card shadow>
            <div className="kb-form-item" style={{ maxWidth: 300 }}>
              <label className="kb-form-label">定时检查间隔（秒）</label>
              <Input type="number" value={tickInterval} onChange={e => setTickInterval(e.target.value)} />
            </div>
            <div style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
              调度器每隔此秒数检查一次定时任务（cron/interval）并触发到期任务。
            </div>
          </Card>
        )}

        {/* ========== 安全配置 ========== */}
        {activeTab === 'security' && (
          <Card shadow>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div className="kb-form-item">
                <label className="kb-form-label">确认超时（秒）</label>
                <Input type="number" value={confirmTimeout} onChange={e => setConfirmTimeout(e.target.value)} />
                <div style={{ fontSize: 12, color: 'var(--color-text-3)', marginTop: 4 }}>
                  L2/L3 危险操作等待用户确认的超时时间，超时自动拒绝。
                </div>
              </div>
            </div>
            <div style={{ marginTop: 16, padding: 14, background: 'var(--color-warning-light)', borderRadius: 6, fontSize: 13, color: 'var(--color-warning)' }}>
              ⚠️ Shell 破坏性命令（del、format、shutdown 等）已被自动拦截。命令安全分级在运行时自动生效，无需重启。
            </div>
          </Card>
        )}

        {/* ========== 日志配置 ========== */}
        {activeTab === 'obs' && (
          <Card shadow>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
              <div className="kb-form-item">
                <label className="kb-form-label">日志级别</label>
                <Select value={logLevel} onChange={e => setLogLevel(e.target.value)}>
                  <option value="debug">Debug（详细调试）</option>
                  <option value="info">Info（推荐）</option>
                  <option value="warn">Warning</option>
                  <option value="error">Error</option>
                </Select>
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">审计日志</label>
                <Select value={auditEnabled ? 'true' : 'false'} onChange={e => setAuditEnabled(e.target.value === 'true')}>
                  <option value="true">启用（记录所有工具调用）</option>
                  <option value="false">禁用</option>
                </Select>
              </div>
            </div>
            <div style={{ marginTop: 12, fontSize: 12, color: 'var(--color-text-3)' }}>
              日志文件位于 <code>data/logs/agent.log</code>，审计日志在「审计」页面可查。
            </div>
          </Card>
        )}
      </div>
    </div>
  )
}
