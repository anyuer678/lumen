import { useEffect, useState } from 'react'
import { Button, Card, Empty } from '../components'
import { useSSE } from '../hooks/useSSE'

interface LogLine {
  ts: string
  level: string
  logger: string
  msg: string
}

export default function Logs() {
  const [logs, setLogs] = useState<LogLine[]>([])
  const [level, setLevel] = useState('')
  const { events } = useSSE()

  useEffect(() => {
    events.forEach(ev => {
      setLogs(prev => [
        ...prev.slice(-499),
        {
          ts: new Date().toLocaleTimeString(),
          level: ev.type.startsWith('task.') ? 'TASK' : 'EVENT',
          logger: ev.type,
          msg: typeof ev.data === 'object' && ev.data ? JSON.stringify(ev.data).slice(0, 200) : String(ev.data ?? ''),
        },
      ])
    })
  }, [events])

  const filtered = level ? logs.filter(l => l.level === level) : logs

  const levelColor = (lv: string) => {
    switch (lv) {
      case 'ERROR': case 'TASK': return 'var(--color-danger)'
      case 'WARN': return 'var(--color-warning)'
      case 'EVENT': return 'var(--color-primary)'
      default: return 'var(--color-text-2)'
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>日志</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>实时任务事件流（SSE）</p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          {['', 'TASK', 'EVENT'].map(lv => (
            <Button key={lv} variant={level === lv ? 'primary' : 'default'} size="small" onClick={() => setLevel(lv)}>
              {lv || '全部'}
            </Button>
          ))}
          <Button size="small" variant="ghost" onClick={() => setLogs([])}>清空</Button>
        </div>
      </div>

      <Card shadow style={{ padding: 0, overflow: 'auto', maxHeight: 'calc(100vh - 180px)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
        {filtered.length === 0 ? (
          <Empty icon="📄" text="暂无日志" />
        ) : (
          filtered.map((log, i) => (
            <div key={i} style={{
              padding: '6px 16px', borderBottom: '1px solid var(--color-border)',
              display: 'flex', gap: 12, whiteSpace: 'nowrap',
            }}>
              <span style={{ color: 'var(--color-text-3)', flexShrink: 0 }}>{log.ts}</span>
              <span style={{ color: levelColor(log.level), flexShrink: 0, fontWeight: 600, width: 56 }}>{log.level}</span>
              <span style={{ color: 'var(--color-text-3)', flexShrink: 0 }}>{log.logger}</span>
              <span style={{ color: 'var(--color-text-1)', overflow: 'hidden', textOverflow: 'ellipsis' }}>{log.msg}</span>
            </div>
          ))
        )}
      </Card>
    </div>
  )
}
