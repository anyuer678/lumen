// API 类型定义

export interface Task {
  id: string
  type: string
  goal: string
  status: TaskStatus
  progress: number
  current_step: number
  priority: number
  perm_level: number
  owner: string
  retry_count: number
  max_retries: number
  result?: string
  error?: string
  pause_reason?: string
  created_at: string
  started_at?: string
  finished_at?: string
  updated_at: string
  steps?: Step[]
}

export interface Step {
  id: string
  task_id: string
  seq: number
  description: string
  status: StepStatus
  tool?: string
  result?: string
  summary?: string
  retries: number
  started_at?: string
  finished_at?: string
}

export type TaskStatus =
  | 'pending'
  | 'queued'
  | 'running'
  | 'paused'
  | 'waiting_confirm'
  | 'completed'
  | 'failed'
  | 'cancelled'

export type StepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped'

export interface TaskListResponse {
  items: Task[]
  total: number
  page: number
}

export interface SystemStatus {
  status: string
  version: string
  uptime_sec: number
  heartbeat: boolean
  tasks: {
    queued: number
    running: number
    completed: number
  }
  queue_depth: number
  llm_provider: string
}

export interface Job {
  id: string
  name: string
  trigger_type: string
  cron_expr?: string
  interval_secs?: number
  watch_path?: string
  goal_template: string
  priority: number
  enabled: boolean
  catch_up: boolean
  concurrency: string
  last_run_at?: string
  next_run_at?: string
  last_status?: string
  miss_count: number
  created_at: string
}

export interface Confirmation {
  id: string
  task_id: string
  step_seq: number
  operation: string
  tool: string
  args_json?: string
  risk_level: number
  reason?: string
  status: string
  requester: string
  created_at: string
  decided_at?: string
  decided_by?: string
  timeout_secs: number
}

export interface ToolInfo {
  name: string
  description: string
  category: string
  required_level: number
  sandbox_only: boolean
}
