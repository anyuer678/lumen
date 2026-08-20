import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge, Button, Input, Select, FormItem, Loading, Empty } from '../components'
import { useToast } from '../components/Toast'

interface McpServer {
  name: string
  command: string
  args: string[]
  transport: string
  url?: string
}

export default function Mcp() {
  const [servers, setServers] = useState<McpServer[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<{ name: string; command: string; args: string; transport: string; url: string }>({
    name: '', command: '', args: '', transport: 'stdio', url: '',
  })
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.listMcpServers()
      .then(data => setServers(Array.isArray(data) ? data : []))
      .catch(() => setServers([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [refresh])

  const handleCreate = async () => {
    if (!form.name || !form.command) {
      showToast('请填写名称和命令', 'warning')
      return
    }
    setSubmitting(true)
    try {
      const args = form.args ? form.args.split(/[\s,]+/).filter(Boolean) : []
      await api.registerMcpServer({ name: form.name, command: form.command, args, transport: form.transport })
      showToast('MCP 服务器已注册', 'success')
      setShowForm(false)
      setForm({ name: '', command: '', args: '', transport: 'stdio', url: '' })
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (name: string) => {
    if (!window.confirm(`确定移除 MCP 服务器 ${name}？`)) return
    try {
      await api.unregisterMcpServer(name)
      showToast('已移除', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const handleTest = async (name: string) => {
    try {
      const r = await api.testMcpServer(name)
      showToast(r.message, r.status === 'ok' ? 'success' : 'error')
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>MCP 服务器</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>管理 MCP 工具服务器</p>
        </div>
        <Button variant="primary" onClick={() => setShowForm(v => !v)}>
          {showForm ? '取消' : '+ 注册服务器'}
        </Button>
      </div>

      {showForm && (
        <Card shadow style={{ marginBottom: 16 }}>
          <div className="kb-card__title" style={{ marginBottom: 16 }}>注册 MCP 服务器</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <FormItem label="名称">
              <Input placeholder="例如: github" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </FormItem>
            <FormItem label="传输方式">
              <Select value={form.transport} onChange={e => setForm({ ...form, transport: e.target.value })}>
                <option value="stdio">stdio</option>
                <option value="sse">SSE</option>
                <option value="streamablehttp">StreamableHTTP</option>
              </Select>
            </FormItem>
            <FormItem label="命令">
              <Input placeholder="例如: npx" value={form.command} onChange={e => setForm({ ...form, command: e.target.value })} />
            </FormItem>
            <FormItem label="参数（空格分隔）">
              <Input placeholder="例如: -y @modelcontextprotocol/server-github" value={form.args} onChange={e => setForm({ ...form, args: e.target.value })} />
            </FormItem>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => setShowForm(false)}>取消</Button>
            <Button variant="primary" onClick={handleCreate} loading={submitting}>注册</Button>
          </div>
        </Card>
      )}

      <Card shadow>
        {loading ? (
          <Loading block />
        ) : servers.length === 0 ? (
          <Empty icon="🔌" text="暂无 MCP 服务器">
            <Button size="small" variant="primary" onClick={() => setShowForm(true)}>注册第一个</Button>
          </Empty>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {servers.map(s => (
              <div key={s.name} style={{
                padding: 16, border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius-md)', background: 'var(--color-bg)',
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <span style={{ fontWeight: 600 }}>{s.name}</span>
                    <Badge variant="success">{s.transport}</Badge>
                  </div>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <Button size="small" onClick={() => handleTest(s.name)}>测试</Button>
                    <Button size="small" variant="danger" onClick={() => handleDelete(s.name)}>移除</Button>
                  </div>
                </div>
                <div style={{ marginTop: 8, fontSize: 12, color: 'var(--color-text-3)', fontFamily: 'var(--font-mono)' }}>
                  {s.command} {s.args?.join(' ') || ''}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
