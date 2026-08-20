import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { SystemStatus, Task } from '../api/types'
import { Card, Badge, Progress, Statistic, Loading, Empty } from '../components'
import { useSSE } from '../hooks/useSSE'
import { fetchJson } from '../api/client'

interface TokenUsage { calls: number; total_tokens: number; cost_usd: number }
interface Event { id: string; type: string; source: string; payload: string; timestamp: string }

export default function Overview() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [tokenUsage, setTokenUsage] = useState<TokenUsage | null>(null)
  const [events, setEvents] = useState<Event[]>([])
  const [digest, setDigest] = useState('')
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const refresh = () => {
    Promise.all([
      api.getStatus(),
      api.listTasks({ limit: 6 }),
      fetchJson('/token-usage').catch(() => ({ calls: 0, total_tokens: 0, cost_usd: 0 })),
      fetchJson('/events?limit=5').catch(() => []),
      fetchJson('/digest/today').catch(() => ({ digest: '' })),
    ])
      .then(([s, t, tk, ev, dg]) => {
        setStatus(s)
        setTasks(t?.items ?? [])
        setTokenUsage(tk)
        setEvents(ev)
        setDigest(dg?.digest || '')
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useSSE(() => refresh())
  useEffect(refresh, [])

  if (loading) return <Loading block text="加载中..." />

  const uptime = status
    ? `${Math.floor(status.uptime_sec / 3600)}h ${Math.floor((status.uptime_sec % 3600) / 60)}m`
    : '-'

  const running = tasks.filter(t => t.status === 'running')

  const quickActions = [
    { icon: '＋', label: '新建任务', to: '', onClick: () => window.dispatchEvent(new Event('kb:new-task')) },
    { icon: '💬', label: '对话', to: '/chat' },
    { icon: '▤', label: '任务', to: '/tasks' },
    { icon: '⚒', label: '工具', to: '/tools' },
    { icon: '🖥', label: 'Computer', to: '/tools', extra: 'computer' },
    { icon: '📊', label: 'Token统计', to: '/token-usage' },
    { icon: '📮', label: '事件日志', to: '/events' },
    { icon: '🕐', label: '每日摘要', to: '' },
  ]

  return (
    <div>
      {/* 顶部欢迎条 */}
      <div style={{
        display: 'flex', justifyContent: 'space-between', alignItems: 'center',
        marginBottom: 20, padding: '20px 24px',
        background: 'linear-gradient(135deg, var(--color-primary-light), transparent)',
        border: '1px solid var(--color-border)', borderRadius: 'var(--radius-lg)',
      }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0, letterSpacing: '0.06em' }}>智能管家</h1>
            <Badge variant={status?.heartbeat ? 'success' : 'danger'}>
              {status?.heartbeat ? '运行中' : '异常'}
            </Badge>
          </div>
          <p style={{ color: 'var(--color-text-2)', fontSize: 13, marginTop: 6 }}>
            Agent v0.1.0 · {uptime} · 8 工具 · {tasks.length} 任务
          </p>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          <button className="kb-btn" onClick={() => navigate('/logs')}>日志</button>
          <button className="kb-btn kb-btn--primary" onClick={() => window.dispatchEvent(new Event('kb:new-task'))}>＋ 新建任务</button>
        </div>
      </div>

      {/* 快捷操作 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, marginBottom: 20 }}>
        {quickActions.map(qa => (
          <button
            key={qa.label}
            className="kb-quick-action"
            onClick={() => {
              if (qa.to) navigate(qa.to)
              else if (qa.onClick) qa.onClick()
              else window.dispatchEvent(new Event('kb:new-task'))
            }}
          >
            <span className="kb-quick-action__icon">{qa.icon}</span>
            <span className="kb-quick-action__label">{qa.label}</span>
          </button>
        ))}
      </div>

      {/* 统计卡片 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 20 }}>
        <div className="kb-card kb-card--shadow kb-stat-card" onClick={() => navigate('/tasks')}>
          <div style={{ padding: 18 }}>
            <Statistic label="任务队列" value={status?.queue_depth ?? 0} />
          </div>
        </div>
        <div className="kb-card kb-card--shadow kb-stat-card" onClick={() => navigate('/tasks')}>
          <div style={{ padding: 18 }}>
            <Statistic label="执行中" value={status?.tasks?.running ?? 0} />
          </div>
        </div>
        <div className="kb-card kb-card--shadow kb-stat-card" onClick={() => navigate('/memories')}>
          <div style={{ padding: 18 }}>
            <Statistic label="已完成" value={status?.tasks?.completed ?? 0} />
          </div>
        </div>
        <div className="kb-card kb-card--shadow kb-stat-card" onClick={() => navigate('/token-usage')}>
          <div style={{ padding: 18 }}>
            <Statistic label="Token 调用" value={tokenUsage?.calls ?? 0} suffix={` (${(tokenUsage?.total_tokens ?? 0).toLocaleString()} tokens)`} />
          </div>
        </div>
      </div>

      {/* 两列：最近任务 + 最近事件 */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 320px', gap: 16, marginBottom: 20 }}>
        {/* 最近任务 */}
        <Card
          title="最近任务"
          extra={<button className="kb-btn kb-btn--link" onClick={() => navigate('/tasks')}>查看全部 →</button>}
          shadow
        >
          {tasks.length === 0 ? (
            <Empty icon="📋" text="暂无任务，点击上方「新建任务」开始" />
          ) : (
            <>
              {running.length > 0 && (
                <div style={{ marginBottom: 12, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  {running.map(t => (
                    <div key={t.id} style={{
                      display: 'inline-flex', alignItems: 'center', gap: 8,
                      padding: '6px 12px', borderRadius: 'var(--radius-round)',
                      background: 'var(--color-primary-light)', color: 'var(--color-primary)',
                      fontSize: 12, cursor: 'pointer',
                    }} onClick={() => navigate(`/tasks/${t.id}`)}>
                      <span className="kb-live-dot" />
                      {t.goal.length > 20 ? t.goal.slice(0, 20) + '…' : t.goal}
                      <Progress value={t.progress} showText={false} />
                    </div>
                  ))}
                </div>
              )}
              <table className="kb-table">
                <thead>
                  <tr>
                    <th>目标</th>
                    <th style={{ width: 80 }}>状态</th>
                    <th style={{ width: 120 }}>进度</th>
                    <th style={{ width: 80 }}>优先级</th>
                    <th style={{ width: 150 }}>创建时间</th>
                  </tr>
                </thead>
                <tbody>
                  {tasks.map(task => (
                    <tr key={task.id} className="kb-row-clickable" style={{ cursor: 'pointer' }} onClick={() => navigate(`/tasks/${task.id}`)}>
                      <td className="text-ellipsis" style={{ maxWidth: 280 }}>{task.goal}</td>
                      <td><Badge variant={task.status === 'completed' ? 'success' : task.status === 'failed' ? 'danger' : task.status === 'running' ? 'primary' : 'info'}>{task.status}</Badge></td>
                      <td><Progress value={task.progress} variant={task.status === 'failed' ? 'danger' : task.status === 'completed' ? 'success' : 'default'} /></td>
                      <td>P{task.priority}</td>
                      <td style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{new Date(task.created_at).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </Card>

        {/* 最近事件流 */}
        <Card title="最近事件" shadow>
          {events.length === 0 ? (
            <Empty icon="📮" text="暂无事件" />
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {events.map(ev => (
                <div key={ev.id} style={{
                  padding: '8px 12px', borderRadius: 'var(--radius-md)',
                  background: 'var(--color-bg)', fontSize: 12,
                  borderLeft: `3px solid ${ev.type === 'task.failed' ? 'var(--color-danger)' : ev.type === 'task.completed' ? 'var(--color-success)' : 'var(--color-primary)'}`,
                }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                    <span style={{ fontWeight: 600 }}>{ev.type}</span>
                    <span style={{ color: 'var(--color-text-3)' }}>{new Date(ev.timestamp).toLocaleTimeString()}</span>
                  </div>
                  <div style={{ color: 'var(--color-text-3)', marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {ev.source}: {ev.payload}
                  </div>
                </div>
              ))}
              <button className="kb-btn kb-btn--link" onClick={() => navigate('/events')} style={{ fontSize: 12 }}>
                查看所有事件 →
              </button>
            </div>
          )}
        </Card>
      </div>

      {/* 每日摘要 */}
      {digest && (
        <Card title="📊 今日摘要" shadow style={{ marginBottom: 20 }}>
          <div style={{
            whiteSpace: 'pre-wrap', fontSize: 13, lineHeight: 1.6,
            color: 'var(--color-text-2)', maxHeight: 300, overflowY: 'auto',
            padding: '8px 0',
          }}>
            {digest}
          </div>
        </Card>
      )}
    </div>
  )
}
