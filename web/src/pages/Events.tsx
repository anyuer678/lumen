import { useEffect, useState, useCallback } from 'react'
import { Card, Badge, Loading, Empty, Button } from '../components'
import { useToast } from '../components/Toast'
import { fetchJson } from '../api/client'

interface Event { id: string; source: string; type: string; timestamp: string; payload: string; priority: number }

export default function Events() {
  const [events, setEvents] = useState<Event[]>([])
  const [loading, setLoading] = useState(true)
  const [emitType, setEmitType] = useState('')
  const [emitPayload, setEmitPayload] = useState('')
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    fetchJson('/events?limit=50')
      .then(d => setEvents(Array.isArray(d) ? d : []))
      .catch(() => setEvents([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [])

  const handleEmit = async () => {
    if (!emitType.trim()) { showToast('请输入事件类型', 'warning'); return }
    try {
      await fetchJson('/events/emit', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source: 'manual', type: emitType, payload: emitPayload || '', priority: 5 }),
      })
      showToast('事件已发射', 'success')
      setEmitType(''); setEmitPayload('')
      refresh()
    } catch (e) { showToast((e as Error).message, 'error') }
  }

  const handleClear = async () => {
    if (!confirm('清理30天前的事件？')) return
    try {
      const r = await fetchJson('/events?keep_days=30', { method: 'DELETE' })
      showToast(`已清理 ${r?.deleted ?? 0} 条旧事件`, 'success')
      refresh()
    } catch (e) { showToast((e as Error).message, 'error') }
  }

  const typeColor = (t: string) => {
    if (t.includes('failed')) return 'danger'
    if (t.includes('completed')) return 'success'
    if (t.includes('alert')) return 'warning'
    return 'primary'
  }

  if (loading) return <Loading block text="加载事件日志..." />

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>事件日志</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>Agent 事件系统 · 共 {events.length} 条</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <Button size="small" onClick={refresh}>刷新</Button>
          <Button size="small" variant="danger" onClick={handleClear}>清理旧事件</Button>
        </div>
      </div>

      {/* 发射测试事件 */}
      <Card title="手动发射事件" shadow style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
          <div className="kb-form-item" style={{ flex: 1, marginBottom: 0 }}>
            <label className="kb-form-label">事件类型</label>
            <select className="kb-select" value={emitType} onChange={e => setEmitType(e.target.value)}>
              <option value="">选择类型</option>
              <option value="file.created">file.created</option>
              <option value="file.modified">file.modified</option>
              <option value="task.failed">task.failed</option>
              <option value="task.completed">task.completed</option>
              <option value="system.alert">system.alert</option>
              <option value="webhook.received">webhook.received</option>
              <option value="schedule.triggered">schedule.triggered</option>
            </select>
          </div>
          <input className="kb-input" style={{ flex: 2 }} placeholder="payload（可选）" value={emitPayload} onChange={e => setEmitPayload(e.target.value)} />
          <Button variant="primary" onClick={handleEmit}>发射</Button>
        </div>
      </Card>

      {/* 事件列表 */}
      <Card title="事件流" shadow>
        {events.length === 0 ? (
          <Empty icon="📮" text="暂无事件记录" />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {events.map(ev => (
              <div key={ev.id} style={{
                padding: '10px 14px', borderRadius: 'var(--radius-md)',
                background: 'var(--color-bg)',
                borderLeft: `3px solid ${typeColor(ev.type) === 'danger' ? 'var(--color-danger)' : typeColor(ev.type) === 'success' ? 'var(--color-success)' : typeColor(ev.type) === 'warning' ? 'var(--color-warning)' : 'var(--color-primary)'}`,
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <Badge variant={typeColor(ev.type)}>{ev.type}</Badge>
                    <span style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{ev.source}</span>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>{new Date(ev.timestamp).toLocaleString()}</span>
                </div>
                {ev.payload && (
                  <div style={{ marginTop: 6, fontSize: 12, color: 'var(--color-text-2)', fontFamily: 'var(--font-mono)', wordBreak: 'break-all' }}>
                    {ev.payload}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
