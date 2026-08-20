import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { Card, Loading, Empty } from '../components'
import { useSSE } from '../hooks/useSSE'

interface TodayData {
  tasks: { total: number; completed: number; failed: number; running: number; items: any[] }
  memories: { total: number; recent: string[] }
  events: { total: number; types: Record<string, number> }
  feedback: { totalTasks: number; successRate: number }
  digest: string
  suggestions: string[]
}

export default function Today() {
  const [data, setData] = useState<TodayData | null>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const refresh = () => {
    Promise.all([
      api.listTasks({ limit: 50 }),
      api.getMemories(),
      fetch('/v1/events?limit=50').then(r => r.json()).catch(() => []),
      fetch('/v1/digest/today').then(r => r.json()).catch(() => ({ digest: '' })),
    ])
      .then(([tasksRes, memories, events, digestRes]) => {
        const items = tasksRes?.items ?? []
        const today = new Date().toISOString().slice(0, 10)

        const todayTasks = items.filter((t: any) => t.created_at?.startsWith(today))
        const completed = todayTasks.filter((t: any) => t.status === 'completed').length
        const failed = todayTasks.filter((t: any) => t.status === 'failed').length
        const running = todayTasks.filter((t: any) => t.status === 'running').length

        const memArr = Array.isArray(memories) ? memories : ((memories as any)?.value ?? [])
        const todayMems = memArr.filter((m: any) => m.created_at?.startsWith(today))

        const evArr = Array.isArray(events) ? events : ((events as any)?.value ?? [])
        const typeCounts: Record<string, number> = {}
        evArr.forEach((e: any) => { typeCounts[e.type] = (typeCounts[e.type] || 0) + 1 })

        const totalDone = completed + failed
        const successRate = totalDone > 0 ? Math.round(completed / totalDone * 100) : 0

        // 生成建议
        const suggestions: string[] = []
        if (completed > 0) suggestions.push(`完成了 ${completed} 个任务`)
        if (todayMems.length > 0) suggestions.push(`新增了 ${todayMems.length} 条记忆`)
        if (failed > 0) suggestions.push(`有 ${failed} 个任务失败，可以查看原因`)
        if (running > 0) suggestions.push(`还有 ${running} 个任务在执行中`)
        if (completed === 0 && failed === 0 && running === 0) {
          suggestions.push('今天还没有任务，试试让我帮你做点什么')
        }

        setData({
          tasks: { total: todayTasks.length, completed, failed, running, items: todayTasks.slice(0, 5) },
          memories: {
            total: todayMems.length,
            recent: todayMems.slice(0, 3).map((m: any) => m.content?.slice(0, 80) ?? ''),
          },
          events: { total: evArr.length, types: typeCounts },
          feedback: { totalTasks: totalDone, successRate },
          digest: digestRes?.digest || '',
          suggestions,
        })
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useSSE(() => refresh())
  useEffect(refresh, [])

  if (loading) return <Loading block text="加载今日概览..." />

  const hour = new Date().getHours()
  const greeting = hour < 12 ? '早上好' : hour < 18 ? '下午好' : '晚上好'
  const greetingEmoji = hour < 12 ? '🌅' : hour < 18 ? '☀️' : '🌙'
  const timeLabel = hour < 12 ? '今天' : hour < 18 ? '下午' : '晚上'

  return (
    <div style={{ maxWidth: 900, margin: '0 auto', padding: '24px 0' }}>
      {/* 问候 + 助手总结 */}
      <div style={{
        marginBottom: 24, padding: '28px 32px',
        background: 'linear-gradient(135deg, var(--color-primary-light), transparent)',
        border: '1px solid var(--color-border)', borderRadius: 'var(--radius-lg)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
          <span style={{ fontSize: 36 }}>{greetingEmoji}</span>
          <div>
            <h1 style={{ fontSize: 24, fontWeight: 700, margin: 0 }}>{greeting}</h1>
            <p style={{ color: 'var(--color-text-2)', fontSize: 14, marginTop: 4 }}>
              {new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}
            </p>
          </div>
        </div>
        {/* 助手总结 */}
        {data && data.suggestions.length > 0 && (
          <div style={{
            marginTop: 12, padding: '12px 16px', borderRadius: 8,
            background: 'rgba(255,255,255,0.6)', fontSize: 14, lineHeight: 1.8,
            color: 'var(--color-text-2)',
          }}>
            <strong>{timeLabel}我帮你：</strong>
            <ul style={{ margin: '4px 0 0 16px', padding: 0 }}>
              {data.suggestions.map((s, i) => (
                <li key={i}>{s}</li>
              ))}
            </ul>
          </div>
        )}
      </div>

      {/* 任务概览 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 24 }}>
        <Card shadow>
          <div style={{ padding: 18, textAlign: 'center' }}>
            <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-primary)' }}>{data?.tasks.completed ?? 0}</div>
            <div style={{ fontSize: 13, color: 'var(--color-text-3)', marginTop: 4 }}>任务完成</div>
          </div>
        </Card>
        <Card shadow>
          <div style={{ padding: 18, textAlign: 'center' }}>
            <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-success)' }}>{data?.feedback.successRate ?? 0}%</div>
            <div style={{ fontSize: 13, color: 'var(--color-text-3)', marginTop: 4 }}>成功率</div>
          </div>
        </Card>
        <Card shadow>
          <div style={{ padding: 18, textAlign: 'center' }}>
            <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-info)' }}>{data?.memories.total ?? 0}</div>
            <div style={{ fontSize: 13, color: 'var(--color-text-3)', marginTop: 4 }}>新增记忆</div>
          </div>
        </Card>
        <Card shadow>
          <div style={{ padding: 18, textAlign: 'center' }}>
            <div style={{ fontSize: 28, fontWeight: 700, color: 'var(--color-warning)' }}>{data?.tasks.running ?? 0}</div>
            <div style={{ fontSize: 13, color: 'var(--color-text-3)', marginTop: 4 }}>执行中</div>
          </div>
        </Card>
      </div>

      {/* 两列：今日摘要 + 最近记忆 */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 24 }}>
        <Card title="📋 今日摘要" shadow>
          {data?.digest ? (
            <div style={{ fontSize: 13, lineHeight: 1.8, whiteSpace: 'pre-wrap', color: 'var(--color-text-2)', maxHeight: 300, overflow: 'auto' }}>
              {data.digest}
            </div>
          ) : (
            <Empty icon="📭" text="暂无今日摘要" />
          )}
        </Card>

        <Card title="🧠 最近记忆" shadow>
          {data?.memories.recent && data.memories.recent.length > 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {data.memories.recent.map((m, i) => (
                <div key={i} style={{
                  padding: '8px 12px', borderRadius: 6,
                  background: 'var(--color-bg)', fontSize: 13, lineHeight: 1.6,
                  color: 'var(--color-text-2)',
                }}>
                  📝 {m}
                </div>
              ))}
            </div>
          ) : (
            <Empty icon="🧠" text="暂无最近记忆" />
          )}
          {data && data.tasks.running > 0 && (
            <div style={{ marginTop: 16, padding: '8px 12px', borderRadius: 6, background: 'var(--color-primary-light)', fontSize: 13 }}>
              🔄 有 {data.tasks.running} 个任务正在执行
            </div>
          )}
        </Card>
      </div>

      {/* 最近任务 */}
      {data && data.tasks.items.length > 0 && (
        <Card title="📋 最近任务" shadow>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {data.tasks.items.map((t: any) => (
              <div key={t.id} style={{
                display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px',
                borderRadius: 6, background: 'var(--color-bg)', fontSize: 13,
                cursor: 'pointer',
              }} onClick={() => navigate(`/tasks/${t.id}`)}>
                <span style={{ color: t.status === 'completed' ? 'var(--color-success)' : t.status === 'failed' ? 'var(--color-danger)' : 'var(--color-text-3)' }}>
                  {t.status === 'completed' ? '✅' : t.status === 'failed' ? '❌' : t.status === 'running' ? '🔄' : '⏳'}
                </span>
                <span style={{ flex: 1, color: 'var(--color-text-2)' }}>
                  {t.goal?.length > 40 ? t.goal.slice(0, 40) + '...' : t.goal}
                </span>
                <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>
                  {new Date(t.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}
                </span>
              </div>
            ))}
          </div>
        </Card>
      )}

      {/* 快捷操作 */}
      <Card title="⚡ 快捷操作" shadow>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12 }}>
          {[
            { icon: '💬', label: '对话', to: '/chat', desc: '和 AI 聊天' },
            { icon: '📋', label: '任务', to: '/tasks', desc: '查看所有任务' },
            { icon: '🧠', label: '记忆', to: '/memories', desc: '管理记忆' },
            { icon: '📚', label: '知识库', to: '/knowledge', desc: '查看知识' },
          ].map(qa => (
            <button
              key={qa.label}
              onClick={() => navigate(qa.to)}
              style={{
                display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6,
                padding: '16px 12px', borderRadius: 8, border: '1px solid var(--color-border)',
                background: 'var(--color-bg)', cursor: 'pointer', transition: 'all 0.2s',
              }}
            >
              <span style={{ fontSize: 24 }}>{qa.icon}</span>
              <span style={{ fontSize: 13, fontWeight: 500, color: 'var(--color-text-1)' }}>{qa.label}</span>
              <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>{qa.desc}</span>
            </button>
          ))}
        </div>
      </Card>
    </div>
  )
}
