import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge, Button, Loading, Empty, Textarea } from '../components'
import { useToast } from '../components/Toast'

interface ToolMeta {
  name: string
  description: string
  required_level: number
}

export default function Tools() {
  const [tools, setTools] = useState<ToolMeta[]>([])
  const [loading, setLoading] = useState(true)
  const [running, setRunning] = useState<string | null>(null)
  const [args, setArgs] = useState<Record<string, string>>({})
  const [output, setOutput] = useState<{ name: string; raw: string; summary: string; error?: string } | null>(null)
  const [category, setCategory] = useState('all')
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.listTools()
      .then(data => setTools(Array.isArray(data) ? data : []))
      .catch(() => setTools([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [])
  useEffect(() => { refresh() }, [])

  const categories = ['all', ...new Set(tools.map(t => t.name.split('.')[0] || 'other'))]
  const filtered = category === 'all' ? tools : tools.filter(t => (t.name.split('.')[0] || 'other') === category)

  const handleRun = async (tool: ToolMeta) => {
    setRunning(tool.name)
    setOutput(null)
    let parsed: any = {}
    const rawArgs = args[tool.name]
    if (rawArgs && rawArgs.trim()) {
      try {
        parsed = JSON.parse(rawArgs)
      } catch {
        showToast('参数必须是合法 JSON，例如 {"command":"echo hi"}', 'error')
        setRunning(null)
        return
      }
    }
    try {
      const r = await api.runTool(tool.name, parsed)
      if (r.success) {
        setOutput({ name: tool.name, raw: r.raw || '', summary: r.summary || '' })
      } else {
        setOutput({ name: tool.name, raw: r.raw || '', summary: '', error: r.error })
      }
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setRunning(null)
    }
  }

  const preset = (tool: ToolMeta) => {
    switch (tool.name) {
      case 'shell.run': return JSON.stringify({ command: 'echo hello', timeout: 10 }, null, 2)
      case 'fs.list': return JSON.stringify({ action: 'list', path: '.' }, null, 2)
      case 'fs.read': return JSON.stringify({ action: 'read', path: 'README.md' }, null, 2)
      case 'fs.organize': return JSON.stringify({ action: 'organize', path: '.' }, null, 2)
      case 'fs': return JSON.stringify({ action: 'organize', path: '.' }, null, 2)
      case 'system': return JSON.stringify({ action: 'disk' }, null, 2)
      case 'windows': return JSON.stringify({ action: 'window_list' }, null, 2)
      case 'browser': return JSON.stringify({ action: 'research', query: 'golang news' }, null, 2)
      case 'computer': return JSON.stringify({ action: 'screenshot' }, null, 2)
      case 'subagent': return JSON.stringify({ objective: '搜索 golang 是什么' }, null, 2)
      case 'safety': return JSON.stringify({ action: 'classify', command: 'echo test' }, null, 2)
      default: return ''
    }
  }

  const levelBadge = (level: number) => (
    <Badge variant={level === 0 ? 'success' : level === 1 ? 'primary' : level === 2 ? 'warning' : 'danger'}>L{level}</Badge>
  )

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>工具</h1>
        <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>Agent 实际可用的工具 · 可填参数并真实调用</p>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        {categories.map(c => (
          <Button key={c} variant={category === c ? 'primary' : 'default'} size="small" onClick={() => setCategory(c)}>
            {c === 'all' ? '全部' : c}
          </Button>
        ))}
      </div>

      {loading ? (
        <Loading block />
      ) : tools.length === 0 ? (
        <Empty icon="⚒" text="Agent 无可用工具" />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(420px, 1fr))', gap: 16 }}>
          {filtered.map(tool => (
            <Card key={tool.name} shadow>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                <code style={{ fontSize: 13, fontWeight: 600 }}>{tool.name}</code>
                {levelBadge(tool.required_level)}
              </div>
              <div style={{ fontSize: 13, color: 'var(--color-text-2)', marginBottom: 12, minHeight: 36 }}>
                {tool.description}
              </div>
              <div className="kb-form-item">
                <label className="kb-form-label">参数 (JSON)</label>
                <Textarea
                  rows={3}
                  placeholder='{"command":"echo hi"}'
                  value={args[tool.name] ?? ''}
                  onChange={e => setArgs({ ...args, [tool.name]: e.target.value })}
                />
                {preset(tool) && (
                  <Button size="small" variant="ghost" onClick={() => setArgs({ ...args, [tool.name]: preset(tool) })}>
                    填入示例
                  </Button>
                )}
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button size="small" variant="primary" onClick={() => handleRun(tool)} loading={running === tool.name}>
                  {running === tool.name ? '执行中...' : '▶ 运行'}
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}

      {/* 输出结果 */}
      {output && (
        <Card title={`输出 · ${output.name}`} shadow style={{ marginTop: 16, borderLeft: output.error ? '4px solid var(--color-danger)' : '4px solid var(--color-success)' }}>
          {output.error && <div style={{ color: 'var(--color-danger)', marginBottom: 8, fontWeight: 600 }}>{output.error}</div>}
          {output.summary && <div style={{ color: 'var(--color-text-2)', marginBottom: 8, fontSize: 13 }}>{output.summary}</div>}
          <pre style={{ background: 'var(--color-bg)', padding: 16, borderRadius: 8, overflow: 'auto', maxHeight: 300, fontSize: 12 }}>{output.raw || '(无输出)'}</pre>
        </Card>
      )}
    </div>
  )
}
