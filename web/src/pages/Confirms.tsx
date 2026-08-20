import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { Confirmation } from '../api/types'
import { Button, Card, Badge, Loading, Empty } from '../components'
import { useToast } from '../components/Toast'
import { useSSE } from '../hooks/useSSE'

export default function Confirms() {
  const [confs, setConfs] = useState<Confirmation[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const { showToast } = useToast()

  const refresh = () => {
    api.listConfirmations()
      .then(data => setConfs(Array.isArray(data) ? data : []))
      .catch(() => setConfs([]))
      .finally(() => setLoading(false))
  }

  useSSE(() => refresh())
  useEffect(refresh, [])

  const handleApprove = async (id: string) => {
    try {
      await api.approveConfirmation(id)
      showToast('已批准', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const handleReject = async (id: string) => {
    try {
      await api.rejectConfirmation(id)
      showToast('已拒绝', 'warning')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const levelLabel = (level: number) => (
    <Badge variant={level >= 3 ? 'danger' : level === 2 ? 'warning' : 'info'}>
      L{level} {level >= 3 ? '高危' : level === 2 ? '危险' : '普通'}
    </Badge>
  )

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>确认队列</h1>
        <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>危险 / 高危操作需要人工确认</p>
      </div>

      {loading ? (
        <Loading block text="加载中..." />
      ) : confs.length === 0 ? (
        <Card shadow>
          <Empty icon="✅" text="暂无待确认操作" />
        </Card>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {confs.map(conf => (
            <Card key={conf.id} shadow>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                  <span style={{ fontWeight: 600 }}>{conf.operation}</span>
                  {levelLabel(conf.risk_level)}
                  <Button size="small" variant="link" onClick={() => navigate(`/tasks/${conf.task_id}`)}>
                    查看任务 →
                  </Button>
                </div>
                <span style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
                  {new Date(conf.created_at).toLocaleString()}
                </span>
              </div>

              {conf.reason && (
                <div style={{ fontSize: 13, color: 'var(--color-text-2)', marginBottom: 12 }}>
                  {conf.reason}
                </div>
              )}

              {conf.args_json && (
                <pre style={{
                  fontSize: 12, background: 'var(--color-bg)', padding: 12,
                  borderRadius: 8, overflow: 'auto', maxHeight: 160, marginBottom: 12,
                }}>
                  {conf.args_json}
                </pre>
              )}

              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', alignItems: 'center' }}>
                <span style={{ fontSize: 12, color: 'var(--color-text-3)', marginRight: 'auto' }}>
                  超时 {conf.timeout_secs}s · 请求者 {conf.requester || 'unknown'}
                </span>
                <Button size="small" variant="danger" onClick={() => handleReject(conf.id)}>✕ 拒绝</Button>
                <Button size="small" variant="success" onClick={() => handleApprove(conf.id)}>✓ 批准</Button>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
