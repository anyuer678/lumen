import type { TaskStatus } from '../api/types'
import type { ReactNode } from 'react'

const STATUS_LABELS: Record<TaskStatus, string> = {
  pending: '待处理',
  queued: '排队中',
  running: '执行中',
  paused: '已暂停',
  waiting_confirm: '待确认',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

export function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span className={`badge badge-${status}`}>
      {STATUS_LABELS[status] || status}
    </span>
  )
}

export function StatusDot({ ok }: { ok: boolean }) {
  return (
    <span
      style={{
        display: 'inline-block',
        width: 8,
        height: 8,
        borderRadius: '50%',
        background: ok ? 'var(--color-success)' : 'var(--color-danger)',
      }}
    />
  )
}

export function Card({ title, children, extra }: { title?: string; children: ReactNode; extra?: ReactNode }) {
  return (
    <div className="card">
      {(title || extra) && (
        <div className="flex-between mb-3">
          {title && <div className="card-title">{title}</div>}
          {extra}
        </div>
      )}
      {children}
    </div>
  )
}

export function ProgressBar({ value, status }: { value: number; status?: TaskStatus }) {
  const className =
    status === 'failed' ? 'progress-fill danger'
    : status === 'completed' ? 'progress-fill success'
    : status === 'paused' ? 'progress-fill warning'
    : 'progress-fill'

  return (
    <div className="progress">
      <div className={className} style={{ width: `${Math.min(100, Math.max(0, value))}%` }} />
    </div>
  )
}
