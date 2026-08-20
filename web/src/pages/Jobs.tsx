import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { Job } from '../api/types'
import { Button, Card, Badge, Input, Select, Loading, Empty, FormItem } from '../components'
import { useToast } from '../components/Toast'

interface JobForm {
  name: string
  trigger_type: string
  cron_expr: string
  interval_secs: number
  goal_template: string
  priority: number
}

const EMPTY_FORM: JobForm = {
  name: '',
  trigger_type: 'cron',
  cron_expr: '0 8 * * *',
  interval_secs: 3600,
  goal_template: '',
  priority: 5,
}

export default function Jobs() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState<JobForm>(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useToast()

  const refresh = () => {
    api.listJobs()
      .then(data => setJobs(Array.isArray(data) ? data : []))
      .catch(() => setJobs([]))
      .finally(() => setLoading(false))
  }

  useEffect(refresh, [])

  const handleCreate = async () => {
    if (!form.name || !form.goal_template) {
      showToast('请填写任务名称和目标模板', 'warning')
      return
    }
    setSubmitting(true)
    try {
      await api.createJob({
        name: form.name,
        trigger_type: form.trigger_type,
        cron_expr: form.trigger_type === 'cron' ? form.cron_expr : undefined,
        interval_secs: form.trigger_type === 'interval' ? form.interval_secs : undefined,
        goal_template: form.goal_template,
        priority: form.priority,
      })
      showToast('定时任务已创建', 'success')
      setForm(EMPTY_FORM)
      setShowForm(false)
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm('确定删除该定时任务？')) return
    try {
      await api.deleteJob(id)
      showToast('已删除', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const triggerLabel = (job: Job) => {
    switch (job.trigger_type) {
      case 'cron': return job.cron_expr || 'cron'
      case 'interval': return `每 ${job.interval_secs}s`
      case 'file_watch': return `监控 ${job.watch_path || '-'}`
      case 'webhook': return 'Webhook'
      case 'at': return '一次性'
      default: return job.trigger_type
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>定时任务</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>Cron / 间隔 / 文件监听 / Webhook 触发</p>
        </div>
        <Button variant="primary" onClick={() => setShowForm(v => !v)}>
          {showForm ? '取消' : '+ 新建定时任务'}
        </Button>
      </div>

      {/* 新建表单 */}
      {showForm && (
        <Card shadow style={{ marginBottom: 16 }}>
          <div className="kb-card__title" style={{ marginBottom: 16 }}>新建定时任务</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <FormItem label="名称">
              <Input placeholder="例如：每日 GitHub Trending" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </FormItem>
            <FormItem label="触发器">
              <Select value={form.trigger_type} onChange={e => setForm({ ...form, trigger_type: e.target.value })}>
                <option value="cron">Cron 表达式</option>
                <option value="interval">固定间隔</option>
                <option value="at">一次性</option>
              </Select>
            </FormItem>
            {form.trigger_type === 'cron' && (
              <FormItem label="Cron 表达式">
                <Input placeholder="0 8 * * *" value={form.cron_expr} onChange={e => setForm({ ...form, cron_expr: e.target.value })} />
              </FormItem>
            )}
            {form.trigger_type === 'interval' && (
              <FormItem label="间隔（秒）">
                <Input type="number" value={form.interval_secs} onChange={e => setForm({ ...form, interval_secs: Number(e.target.value) })} />
              </FormItem>
            )}
            <FormItem label="目标模板">
              <Input placeholder="支持 {{date}} {{datetime}}" value={form.goal_template} onChange={e => setForm({ ...form, goal_template: e.target.value })} />
            </FormItem>
            <FormItem label="优先级">
              <Select value={form.priority} onChange={e => setForm({ ...form, priority: Number(e.target.value) })}>
                {[0, 2, 5, 8, 9].map(p => <option key={p} value={p}>P{p}</option>)}
              </Select>
            </FormItem>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => setShowForm(false)}>取消</Button>
            <Button variant="primary" onClick={handleCreate} loading={submitting}>
              {submitting ? '创建中...' : '创建'}
            </Button>
          </div>
        </Card>
      )}

      <Card shadow>
        {loading ? (
          <Loading block text="加载中..." />
        ) : jobs.length === 0 ? (
          <Empty icon="⏰" text="暂无定时任务" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>触发</th>
                <th>状态</th>
                <th>最近运行</th>
                <th style={{ width: 90 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map(job => (
                <tr key={job.id}>
                  <td>{job.name}</td>
                  <td>
                    <span style={{ fontSize: 12, fontFamily: 'var(--font-mono)', background: 'var(--color-bg)', padding: '2px 8px', borderRadius: 4 }}>
                      {triggerLabel(job)}
                    </span>
                  </td>
                  <td><Badge variant={job.enabled ? 'success' : 'info'}>{job.enabled ? '启用' : '停用'}</Badge></td>
                  <td style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
                    {job.last_run_at ? new Date(job.last_run_at).toLocaleString() : '从未运行'}
                  </td>
                  <td>
                    <Button size="small" variant="danger" onClick={() => handleDelete(job.id)}>删除</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
    </div>
  )
}
