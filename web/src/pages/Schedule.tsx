import { useState, useEffect, useCallback } from 'react'
import { Card, Button, Input, Select, FormItem, Empty, Badge, Loading, Modal } from '../components'
import { useToast } from '../components/Toast'
import { fetchJson } from '../api/client'

interface Job {
  id: string
  name: string
  goal: string
  trigger_type: string
  cron_expr?: string
  interval_secs?: number
  enabled: boolean
  next_run?: string
  last_run?: string
  created_at: string
}

export default function Schedule() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [showDetail, setShowDetail] = useState<Job | null>(null)
  const { showToast } = useToast()

  // 创建表单
  const [newJob, setNewJob] = useState({
    name: '', goal: '', trigger_type: 'interval',
    cron_expr: '', interval_secs: 60, enabled: true
  })

  const refresh = useCallback(() => {
    fetchJson('/jobs')
      .then(d => setJobs(Array.isArray(d) ? d : []))
      .catch(() => setJobs([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [])

  const handleCreate = async () => {
    if (!newJob.name.trim() || !newJob.goal.trim()) {
      showToast('请填写名称和目标', 'warning'); return
    }
    try {
      await fetchJson('/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newJob)
      })
      showToast('定时任务已创建', 'success')
      setShowCreate(false)
      setNewJob({ name: '', goal: '', trigger_type: 'interval', cron_expr: '', interval_secs: 60, enabled: true })
      refresh()
    } catch (e) { showToast((e as Error).message, 'error') }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('确定删除该定时任务？')) return
    try {
      await fetchJson(`/jobs/${id}`, { method: 'DELETE' })
      showToast('已删除', 'success')
      refresh()
    } catch (e) { showToast((e as Error).message, 'error') }
  }

  const cronPresets = [
    { label: '每小时', value: '0 * * * *' },
    { label: '每天 8:00', value: '0 8 * * *' },
    { label: '每天 22:00', value: '0 22 * * *' },
    { label: '每周一 9:00', value: '0 9 * * 1' },
    { label: '每月 1 号 0:00', value: '0 0 1 * *' },
    { label: '每天 9:00/18:00', value: '0 9,18 * * *' },
  ]

  if (loading) return <Loading block text="加载定时任务..." />

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>定时任务</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>管理定时/周期/事件触发任务 · 共 {jobs.length} 个</p>
        </div>
        <Button variant="primary" onClick={() => setShowCreate(true)}>＋ 创建定时任务</Button>
      </div>

      {/* 快速模板 */}
      <Card shadow style={{ marginBottom: 16 }}>
        <div style={{ marginBottom: 8, fontSize: 13, fontWeight: 600, color: 'var(--color-text-2)' }}>快速模板</div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {cronPresets.map(preset => (
            <button key={preset.value} className="kb-btn" onClick={() => {
              setNewJob({ ...newJob, name: preset.label, trigger_type: 'cron', cron_expr: preset.value })
              setShowCreate(true)
            }}>{preset.label}</button>
          ))}
          <button className="kb-btn" onClick={() => {
            setNewJob({ ...newJob, name: '每日研究', trigger_type: 'cron', cron_expr: '0 22 * * *', goal: '搜索今日AI/Agent相关最新进展并总结' })
            setShowCreate(true)
          }}>每日研究</button>
          <button className="kb-btn" onClick={() => {
            setNewJob({ ...newJob, name: '每日摘要', trigger_type: 'cron', cron_expr: '0 23 * * *', goal: '生成今日的每日摘要' })
            setShowCreate(true)
          }}>每日摘要</button>
        </div>
      </Card>

      {/* 任务列表 */}
      <Card title="定时任务列表" shadow>
        {jobs.length === 0 ? (
          <Empty icon="◷" text="暂无定时任务，点击上方创建" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>目标</th>
                <th>触发</th>
                <th>规则</th>
                <th>状态</th>
                <th>下次执行</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map(job => (
                <tr key={job.id} className="kb-row-clickable" onClick={() => setShowDetail(job)}>
                  <td style={{ fontWeight: 600 }}>{job.name}</td>
                  <td style={{ maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'var(--color-text-2)' }}>{job.goal}</td>
                  <td><Badge variant={job.trigger_type === 'cron' ? 'primary' : job.trigger_type === 'interval' ? 'info' : 'warning'}>{job.trigger_type}</Badge></td>
                  <td style={{ fontSize: 12, fontFamily: 'var(--font-mono)' }}>{job.cron_expr || `${job.interval_secs}s`}</td>
                  <td><Badge variant={job.enabled ? 'success' : 'danger'}>{job.enabled ? '启用' : '禁用'}</Badge></td>
                  <td style={{ fontSize: 12 }}>{job.next_run || '-'}</td>
                  <td>
                    <Button size="small" variant="danger" onClick={(e) => { e.stopPropagation(); handleDelete(job.id) }}>删除</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {/* 创建任务弹窗 */}
      <Modal open={showCreate} title="创建定时任务" onClose={() => setShowCreate(false)} footer={
        <>
          <Button onClick={() => setShowCreate(false)}>取消</Button>
          <Button variant="primary" onClick={handleCreate}>创建</Button>
        </>
      }>
        <div style={{ display: 'grid', gap: 12 }}>
          <FormItem label="任务名称" required>
            <Input value={newJob.name} onChange={e => setNewJob({ ...newJob, name: e.target.value })} placeholder="每日研究" />
          </FormItem>
          <FormItem label="任务目标" required>
            <textarea className="kb-textarea" value={newJob.goal} onChange={e => setNewJob({ ...newJob, goal: e.target.value })} placeholder="搜索今日AI相关最新进展并总结" rows={3} />
          </FormItem>
          <FormItem label="触发方式">
            <Select value={newJob.trigger_type} onChange={e => setNewJob({ ...newJob, trigger_type: e.target.value })}>
              <option value="cron">Cron 表达式</option>
              <option value="interval">固定间隔（秒）</option>
            </Select>
          </FormItem>
          {newJob.trigger_type === 'cron' ? (
            <FormItem label="Cron 表达式">
              <Input value={newJob.cron_expr} onChange={e => setNewJob({ ...newJob, cron_expr: e.target.value })} placeholder="0 22 * * *" />
              <div style={{ marginTop: 4, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                {cronPresets.slice(0, 4).map(p => (
                  <button key={p.value} className="kb-btn kb-btn--small" onClick={() => setNewJob({ ...newJob, cron_expr: p.value })}>{p.label}</button>
                ))}
              </div>
            </FormItem>
          ) : (
            <FormItem label="间隔（秒）">
              <Input type="number" value={String(newJob.interval_secs)} onChange={e => setNewJob({ ...newJob, interval_secs: Number(e.target.value) })} />
            </FormItem>
          )}
        </div>
      </Modal>

      {/* 详情弹窗 */}
      <Modal open={!!showDetail} title="任务详情" onClose={() => setShowDetail(null)}>
        {showDetail && (
          <div>
            <div style={{ marginBottom: 12 }}><strong>名称：</strong>{showDetail.name}</div>
            <div style={{ marginBottom: 12 }}><strong>目标：</strong>{showDetail.goal}</div>
            <div style={{ marginBottom: 12 }}><strong>触发：</strong><Badge variant="primary">{showDetail.trigger_type}</Badge> {showDetail.cron_expr || showDetail.interval_secs + 's'}</div>
            <div style={{ marginBottom: 12 }}><strong>状态：</strong><Badge variant={showDetail.enabled ? 'success' : 'danger'}>{showDetail.enabled ? '启用' : '禁用'}</Badge></div>
            <div style={{ marginBottom: 12 }}><strong>下次执行：</strong>{showDetail.next_run || '未调度'}</div>
            <div style={{ marginBottom: 12 }}><strong>上次执行：</strong>{showDetail.last_run || '未执行'}</div>
            <div><strong>创建时间：</strong>{new Date(showDetail.created_at).toLocaleString()}</div>
          </div>
        )}
      </Modal>
    </div>
  )
}
