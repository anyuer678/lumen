import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Button, Loading, Empty, Badge } from '../components'

interface AuditEntry {
  id: number
  ts: string
  actor: string
  action: string
  target: string
  detail: string
  result: string
}

const ACTIONS = ['', 'tool.call', 'tool.success', 'tool.failed', 'task.create', 'confirm.approve']

export default function Audit() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')

  const refresh = useCallback(() => {
    api.getAudit(filter, 100)
      .then(data => setEntries(Array.isArray(data) ? data : []))
      .catch(() => setEntries([]))
      .finally(() => setLoading(false))
  }, [filter])

  useEffect(refresh, [refresh])

  const resultBadge = (result: string) => {
    if (!result) return null
    if (result === 'ok' || result === 'success') return <Badge variant="success">✓</Badge>
    if (result === 'error' || result === 'failed') return <Badge variant="danger">✗</Badge>
    return <Badge variant="info">{result}</Badge>
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>审计日志</h1>
        <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
          所有操作记录（append-only）
        </p>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        {ACTIONS.map(a => (
          <Button key={a} variant={filter === a ? 'primary' : 'default'} size="small"
            onClick={() => setFilter(a)}>
            {a === '' ? '全部' : a}
          </Button>
        ))}
      </div>

      <Card shadow style={{ padding: 0, overflow: 'auto' }}>
        {loading ? (
          <Loading block />
        ) : entries.length === 0 ? (
          <Empty icon="📜" text="暂无审计记录" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th style={{ width: 150 }}>时间</th>
                <th style={{ width: 100 }}>操作者</th>
                <th style={{ width: 120 }}>动作</th>
                <th>详情</th>
                <th style={{ width: 60 }}>结果</th>
              </tr>
            </thead>
            <tbody>
              {entries.map(e => (
                <tr key={e.id}>
                  <td style={{ fontSize: 12, color: 'var(--color-text-3)', whiteSpace: 'nowrap' }}>
                    {new Date(e.ts).toLocaleString()}
                  </td>
                  <td>{e.actor}</td>
                  <td><Badge variant="info">{e.action}</Badge></td>
                  <td style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', maxWidth: 400, whiteSpace: 'nowrap' }}>
                    {e.detail || e.target}
                  </td>
                  <td>{resultBadge(e.result)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}
