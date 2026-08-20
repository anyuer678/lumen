import { useEffect, useState } from 'react'
import { Card, Badge, Loading, Empty, Statistic } from '../components'
import { fetchJson } from '../api/client'

interface TokenUsage { id: number; provider: string; model: string; source: string; prompt_tokens: number; completion_tokens: number; total_tokens: number; cost_usd: number; duration_ms: number; task_id: string; created_at: string }
interface Summary { prompt_tokens: number; completion_tokens: number; total_tokens: number; cost_usd: number; calls: number }

export default function TokenUsage() {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [records, setRecords] = useState<TokenUsage[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      fetchJson('/token-usage?since=week'),
      fetchJson('/token-usage/recent?limit=50'),
    ])
      .then(([s, r]) => { setSummary(s); setRecords(Array.isArray(r) ? r : []) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <Loading block text="加载 Token 用量..." />

  return (
    <div>
      <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, marginBottom: 20 }}>Token 用量</h1>

      {/* 汇总卡片 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 20 }}>
        <Card shadow><div style={{ padding: 18 }}><Statistic label="总调用" value={summary?.calls ?? 0} /></div></Card>
        <Card shadow><div style={{ padding: 18 }}><Statistic label="Prompt Tokens" value={(summary?.prompt_tokens ?? 0).toLocaleString()} /></div></Card>
        <Card shadow><div style={{ padding: 18 }}><Statistic label="Completion Tokens" value={(summary?.completion_tokens ?? 0).toLocaleString()} /></div></Card>
        <Card shadow><div style={{ padding: 18 }}><Statistic label="总成本" value={`$${(summary?.cost_usd ?? 0).toFixed(4)}`} /></div></Card>
      </div>

      {/* 记录表格 */}
      <Card title="最近调用记录" shadow>
        {records.length === 0 ? (
          <Empty icon="📊" text="暂无调用记录" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>Provider</th>
                <th>Model</th>
                <th>来源</th>
                <th style={{ textAlign: 'right' }}>Prompt</th>
                <th style={{ textAlign: 'right' }}>Completion</th>
                <th style={{ textAlign: 'right' }}>Total</th>
                <th style={{ textAlign: 'right' }}>成本</th>
              </tr>
            </thead>
            <tbody>
              {records.map(r => (
                <tr key={r.id}>
                  <td style={{ fontSize: 12 }}>{new Date(r.created_at).toLocaleString()}</td>
                  <td><Badge variant="primary">{r.provider}</Badge></td>
                  <td style={{ fontSize: 12 }}>{r.model}</td>
                  <td><Badge variant={r.source === 'chat' ? 'info' : r.source === 'task' ? 'success' : 'warning'}>{r.source}</Badge></td>
                  <td style={{ textAlign: 'right', fontSize: 12 }}>{r.prompt_tokens}</td>
                  <td style={{ textAlign: 'right', fontSize: 12 }}>{r.completion_tokens}</td>
                  <td style={{ textAlign: 'right', fontSize: 12, fontWeight: 600 }}>{r.total_tokens}</td>
                  <td style={{ textAlign: 'right', fontSize: 12, color: 'var(--color-success)' }}>${r.cost_usd.toFixed(4)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}
