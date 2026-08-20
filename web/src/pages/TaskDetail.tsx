import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api/client'
import type { Task, Step } from '../api/types'
import { Button, Badge, Card, Progress, Loading, Empty, Timeline, TimelineItem, Descriptions, DescriptionsItem } from '../components'
import { useSSE } from '../hooks/useSSE'
import { useToast } from '../components/Toast'

export default function TaskDetail() {
  const { id } = useParams<{ id: string }>()
  const [task, setTask] = useState<Task | null>(null)
  const [steps, setSteps] = useState<Step[]>([])
  const [traj, setTraj] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    if (!id) return
    Promise.all([api.getTask(id), api.getTaskSteps(id)])
      .then(([t, s]) => {
        setTask(t)
        setSteps(s?.items ?? [])
      })
      .catch(e => showToast(e.message, 'error'))
      .finally(() => setLoading(false))
    // 拉取运行轨迹
    fetch(`/v1/trajectories/${encodeURIComponent(id)}`)
      .then(r => r.json())
      .then(d => setTraj(Array.isArray(d?.events) ? d.events : []))
      .catch(() => setTraj([]))
  }, [id])

  useSSE(() => refresh())
  useEffect(() => { setLoading(true); refresh() }, [refresh])

  if (loading) return <Loading block text="加载中..." />

  if (!task) {
    return (
      <div style={{ padding: 24, maxWidth: 800, margin: '0 auto' }}>
        <Empty icon="🔍" text="任务不存在">
          <Button size="small" onClick={() => navigate('/tasks')}>返回列表</Button>
        </Empty>
      </div>
    )
  }

  const handleAction = async (action: 'pause' | 'resume' | 'stop' | 'retry') => {
    try {
      const actions = { pause: api.pauseTask, resume: api.resumeTask, stop: api.stopTask, retry: api.retryTask }
      await actions[action](id!)
      showToast(`操作成功: ${action}`, 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const statusVariant = task.status === 'completed' ? 'success'
    : task.status === 'failed' ? 'danger'
    : task.status === 'running' ? 'primary'
    : 'info'

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ marginBottom: 16 }}>
        <Button size="small" variant="ghost" onClick={() => navigate('/tasks')}>← 返回任务列表</Button>
      </div>

      {/* 任务头部 */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 24 }}>
        <div style={{ flex: 1, marginRight: 16 }}>
          <h1 style={{ fontSize: 22, fontWeight: 600, margin: 0 }}>{task.goal}</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
            ID: {task.id} · 创建于 {new Date(task.created_at).toLocaleString()}
            {task.pause_reason && ` · 暂停原因: ${task.pause_reason}`}
          </p>
        </div>
        <Badge variant={statusVariant as any}>{task.status}</Badge>
      </div>

      {/* 进度 */}
      <Card shadow style={{ marginBottom: 16 }}>
        <Progress value={task.progress} variant={task.status === 'failed' ? 'danger' : task.status === 'completed' ? 'success' : 'default'} />
        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8, fontSize: 12, color: 'var(--color-text-3)' }}>
          <span>步骤 {task.current_step}/{steps.length || '?'}</span>
          <span>优先级 P{task.priority} · 重试 {task.retry_count}/{task.max_retries}</span>
        </div>
      </Card>

      {/* 操作按钮 */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 24 }}>
        {task.status === 'running' && <Button variant="warning" onClick={() => handleAction('pause')}>⏸ 暂停</Button>}
        {task.status === 'paused' && <Button variant="primary" onClick={() => handleAction('resume')}>▶ 继续</Button>}
        {(task.status === 'running' || task.status === 'paused' || task.status === 'queued') && (
          <Button variant="danger" onClick={() => handleAction('stop')}>⏹ 终止</Button>
        )}
        {task.status === 'failed' && <Button variant="warning" onClick={() => handleAction('retry')}>↻ 重试</Button>}
      </div>

      {/* 任务信息 */}
      <Card title="任务信息" shadow style={{ marginBottom: 16 }}>
        <Descriptions>
          <DescriptionsItem label="任务 ID">{task.id}</DescriptionsItem>
          <DescriptionsItem label="类型">{task.type}</DescriptionsItem>
          <DescriptionsItem label="创建时间">{new Date(task.created_at).toLocaleString()}</DescriptionsItem>
          {task.started_at && <DescriptionsItem label="开始时间">{new Date(task.started_at).toLocaleString()}</DescriptionsItem>}
          {task.finished_at && <DescriptionsItem label="结束时间">{new Date(task.finished_at).toLocaleString()}</DescriptionsItem>}
        </Descriptions>
      </Card>

      {/* 结果/错误 */}
      {task.result && (
        <Card title="结果" shadow style={{ marginBottom: 16, borderLeft: '4px solid var(--color-success)' }}>
          <pre style={{ fontSize: 13, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{task.result}</pre>
        </Card>
      )}
      {task.error && (
        <Card title="错误" shadow style={{ marginBottom: 16, borderLeft: '4px solid var(--color-danger)' }}>
          <pre style={{ fontSize: 13, whiteSpace: 'pre-wrap', wordBreak: 'break-all', color: 'var(--color-danger)' }}>{task.error}</pre>
        </Card>
      )}

      {/* 步骤时间线 */}
      <Card title={`执行步骤 (${steps.length})`} shadow>
        {steps.length === 0 ? (
          <Empty icon="🗺" text="暂无步骤" />
        ) : (
          <Timeline>
            {steps.map(step => (
              <TimelineItem
                key={step.id}
                dot={step.status === 'completed' ? 'success' : step.status === 'failed' ? 'danger' : step.status === 'running' ? 'primary' : 'default'}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <span style={{ fontWeight: 600, fontSize: 13 }}>#{step.seq + 1}</span>
                    <span>{step.description}</span>
                    {step.tool && (
                      <span style={{ fontSize: 11, padding: '1px 8px', borderRadius: 4, background: 'var(--color-primary-light)', color: 'var(--color-primary)' }}>
                        {step.tool}
                      </span>
                    )}
                  </div>
                  <Badge variant={step.status === 'completed' ? 'success' : step.status === 'failed' ? 'danger' : step.status === 'running' ? 'primary' : 'info'}>
                    {step.status}
                  </Badge>
                </div>
                {step.summary && (
                  <div style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-2)' }}>{step.summary}</div>
                )}
              </TimelineItem>
            ))}
          </Timeline>
        )}
      </Card>

      <Card title="运行轨迹" shadow style={{ marginTop: 16 }}>
        {traj.length === 0 ? (
          <Empty icon="🎞" text="该任务暂无轨迹记录" />
        ) : (
          <Timeline>
            {traj.map(ev => {
              const t = new Date(ev.ts).toLocaleTimeString()
              return (
                <TimelineItem
                  key={ev.seq}
                  dot={ev.event_type === 'task.completed' ? 'success' : ev.event_type === 'task.failed' ? 'danger' : 'default'}
                >
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 13 }}>
                    <span style={{ color: 'var(--color-text-3)' }}>#{ev.seq}</span>
                    <span>{t}</span>
                    <Badge variant={ev.event_type === 'task.completed' ? 'success' : ev.event_type === 'task.failed' ? 'danger' : 'info'}>
                      {ev.event_type}
                    </Badge>
                  </div>
                  {ev.data?.description && (
                    <div style={{ marginTop: 4, fontSize: 13, color: 'var(--color-text-2)' }}>{ev.data.description}</div>
                  )}
                  {ev.data?.error && (
                    <div style={{ marginTop: 4, fontSize: 12, color: 'var(--color-danger, #f53f3f)' }}>{ev.data.error}</div>
                  )}
                </TimelineItem>
              )
            })}
          </Timeline>
        )}
      </Card>
    </div>
  )
}
