import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import type { Task } from '../api/types'
import { Button, Card, Badge, Progress, Loading, Empty } from '../components'
import CreateTaskModal from '../components/CreateTaskModal'
import { useSSE } from '../hooks/useSSE'
import { useToast } from '../components/Toast'

const STATUS_FILTERS: { value: string; label: string }[] = [
  { value: '', label: '全部' },
  { value: 'running', label: '执行中' },
  { value: 'queued', label: '排队中' },
  { value: 'paused', label: '已暂停' },
  { value: 'waiting_confirm', label: '待确认' },
  { value: 'completed', label: '已完成' },
  { value: 'failed', label: '失败' },
  { value: 'cancelled', label: '已取消' },
]

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [filter, setFilter] = useState('')
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const navigate = useNavigate()
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.listTasks({ status: filter || undefined, page, limit: 20 })
      .then(data => {
        setTasks(data?.items ?? [])
        setTotal(data?.total ?? 0)
      })
      .catch(e => showToast(e.message, 'error'))
      .finally(() => setLoading(false))
  }, [filter, page])

  useSSE(() => refresh())

  useEffect(() => {
    setLoading(true)
    refresh()
  }, [refresh])

  const handleAction = async (id: string, action: 'pause' | 'resume' | 'stop' | 'retry') => {
    try {
      const actions = { pause: api.pauseTask, resume: api.resumeTask, stop: api.stopTask, retry: api.retryTask }
      await actions[action](id)
      showToast(`操作成功: ${action}`, 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>任务</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>共 {total} 个任务</p>
        </div>
        <Button variant="primary" onClick={() => setShowCreate(true)}>+ 创建任务</Button>
        <Button variant="danger" size="small" onClick={async () => {
          if (!window.confirm('确定清空所有已结束的任务？（保留进行中的）')) return
          try {
            const r = await api.clearTasks(true)
            showToast(`已清理 ${r.deleted} 个任务`, 'success')
            refresh()
          } catch (e) {
            showToast((e as Error).message, 'error')
          }
        }}>清空已结束</Button>
      </div>

      {/* 筛选 */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        {STATUS_FILTERS.map(f => (
          <Button
            key={f.value}
            variant={filter === f.value ? 'primary' : 'default'}
            size="small"
            onClick={() => { setPage(1); setFilter(f.value) }}
          >
            {f.label}
          </Button>
        ))}
      </div>

      <Card shadow>
        {loading ? (
          <Loading block text="加载中..." />
        ) : tasks.length === 0 ? (
          <Empty icon="📋" text="暂无任务" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th>目标</th>
                <th style={{ width: 90 }}>状态</th>
                <th style={{ width: 150 }}>进度</th>
                <th style={{ width: 80 }}>优先级</th>
                <th style={{ width: 170 }}>创建时间</th>
                <th style={{ width: 220 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map(task => (
                <tr key={task.id}>
                  <td>
                    <span
                      style={{ cursor: 'pointer', color: 'var(--color-primary)', maxWidth: 320, display: 'inline-block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                      onClick={() => navigate(`/tasks/${task.id}`)}
                    >
                      {task.goal}
                    </span>
                  </td>
                  <td><Badge variant={task.status === 'completed' ? 'success' : task.status === 'failed' ? 'danger' : task.status === 'running' ? 'primary' : 'info'}>{task.status}</Badge></td>
                  <td><Progress value={task.progress} /></td>
                  <td>P{task.priority}</td>
                  <td style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{new Date(task.created_at).toLocaleString()}</td>
                  <td>
                    <div style={{ display: 'flex', gap: 4 }}>
                      {task.status === 'running' && <Button size="small" variant="warning" onClick={() => handleAction(task.id, 'pause')}>暂停</Button>}
                      {task.status === 'paused' && <Button size="small" variant="primary" onClick={() => handleAction(task.id, 'resume')}>继续</Button>}
                      {(task.status === 'running' || task.status === 'paused' || task.status === 'queued') && (
                        <Button size="small" variant="danger" onClick={() => handleAction(task.id, 'stop')}>终止</Button>
                      )}
                      {task.status === 'failed' && <Button size="small" variant="warning" onClick={() => handleAction(task.id, 'retry')}>重试</Button>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {/* 分页 */}
      {total > 20 && (
        <div style={{ display: 'flex', justifyContent: 'center', gap: 12, marginTop: 16 }}>
          <Button size="small" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>上一页</Button>
          <span style={{ fontSize: 13, color: 'var(--color-text-3)', lineHeight: '24px' }}>{page} / {Math.ceil(total / 20)}</span>
          <Button size="small" disabled={page >= Math.ceil(total / 20)} onClick={() => setPage(p => p + 1)}>下一页</Button>
        </div>
      )}

      <CreateTaskModal open={showCreate} onClose={() => setShowCreate(false)} onCreated={refresh} />
    </div>
  )
}
