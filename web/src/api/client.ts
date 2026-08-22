// API 客户端

import type { Task, TaskListResponse, SystemStatus, Step, Job, Confirmation } from './types'

const BASE_URL = '/v1'

// token 存储 key
const TOKEN_KEY = 'lumen_token'

// 从 localStorage 读取 token（可由 Settings 页调用 setAuthToken 更新）
function getAuthToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

// 设置/清除 token（Settings 页保存时调用）
export function setAuthToken(token: string | null): void {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token)
    else localStorage.removeItem(TOKEN_KEY)
  } catch { /* ignore */ }
}

export function getAuthTokenPublic(): string | null {
  return getAuthToken()
}

class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers || {})
  headers.set('Content-Type', 'application/json')
  const token = getAuthToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
  })

  if (!res.ok) {
    if (res.status === 401) {
      throw new ApiError('未授权，请检查 Lumen API Token', res.status)
    }
    const text = await res.text().catch(() => '')
    throw new ApiError(text || `Request failed: ${res.status}`, res.status)
  }

  // 无 body 或 204 时返回 undefined（void 操作）
  const text = await res.text().catch(() => '')
  if (res.status === 204 || !text.trim()) {
    return undefined as T
  }

  return JSON.parse(text) as T
}

export const api = {
  // 系统
  getStatus: () => request<SystemStatus>('/status'),
  getHealth: () => request<{ status: string; heartbeat: boolean; version: string }>('/health'),

  // 任务
  listTasks: (params?: { status?: string; page?: number; limit?: number }) => {
    const qs = new URLSearchParams()
    if (params?.status) qs.set('status', params.status)
    if (params?.page) qs.set('page', String(params.page))
    if (params?.limit) qs.set('limit', String(params.limit))
    const query = qs.toString()
    return request<TaskListResponse>(`/tasks${query ? `?${query}` : ''}`)
  },

  getTask: (id: string) => request<Task>(`/tasks/${id}`),

  createTask: (data: { goal: string; priority?: number; type?: string }) =>
    request<Task>('/tasks', { method: 'POST', body: JSON.stringify(data) }),

  pauseTask: (id: string) =>
    request<void>(`/tasks/${id}/pause`, { method: 'POST' }),

  resumeTask: (id: string) =>
    request<void>(`/tasks/${id}/resume`, { method: 'POST' }),

  stopTask: (id: string) =>
    request<void>(`/tasks/${id}/stop`, { method: 'POST' }),

  retryTask: (id: string) =>
    request<void>(`/tasks/${id}/retry`, { method: 'POST' }),

  clearTasks: (keepRunning: boolean) =>
    request<{ deleted: number }>(`/tasks?keep_running=${keepRunning}`, { method: 'DELETE' }),

  getTaskSteps: (id: string) =>
    request<{ items: Step[] }>(`/tasks/${id}/steps`),

  // 定时任务
  listJobs: () => request<Job[]>('/jobs'),
  createJob: (data: Partial<Job>) => request<Job>('/jobs', { method: 'POST', body: JSON.stringify(data) }),
  deleteJob: (id: string) => request<void>(`/jobs/${id}`, { method: 'DELETE' }),

  // 确认
  listConfirmations: () => request<Confirmation[]>('/confirmations'),
  approveConfirmation: (id: string) =>
    request<void>(`/confirmations/${id}/approve`, { method: 'POST' }),
  rejectConfirmation: (id: string) =>
    request<void>(`/confirmations/${id}/reject`, { method: 'POST' }),

  // 设置
  getSettings: () => request<any>('/settings'),
  updateSettings: (data: any) =>
    request<any>('/settings', { method: 'PUT', body: JSON.stringify(data) }),
  testLLM: (data: { provider: string; base_url: string; api_key: string; model: string }) =>
    request<any>('/settings/test-llm', { method: 'POST', body: JSON.stringify(data) }),

  // 记忆
  getMemories: (kind?: string) =>
    request<any[]>(`/memories${kind ? `?kind=${kind}` : ''}`),
  confirmMemory: (id: string) =>
    request<void>(`/memories/${id}/confirm`, { method: 'POST' }),
  deleteMemory: (id: string) =>
    request<void>(`/memories/${id}`, { method: 'DELETE' }),

  // 审计
  getAudit: (action?: string, limit?: number) => {
    const qs = new URLSearchParams()
    if (action) qs.set('action', action)
    if (limit) qs.set('limit', String(limit))
    const q = qs.toString()
    return request<any[]>(`/audit${q ? `?${q}` : ''}`)
  },

  // MCP 服务器
  listMcpServers: () => request<any[]>('/mcp/servers'),
  registerMcpServer: (data: any) =>
    request<any>('/mcp/servers', { method: 'POST', body: JSON.stringify(data) }),
  testMcpServer: (name: string) =>
    request<{ status: string; message: string }>(`/mcp/servers/${name}/test`, { method: 'POST' }),
  unregisterMcpServer: (name: string) =>
    request<void>(`/mcp/servers/${name}`, { method: 'DELETE' }),

  // 知识库
  listKnowledge: () => request<any[]>('/knowledge'),
  addKnowledge: (data: { title: string; content: string; tags?: string }) =>
    request<any>('/knowledge', { method: 'POST', body: JSON.stringify(data) }),
  searchKnowledge: (query: string, limit?: number) =>
    request<any[]>('/knowledge/search', { method: 'POST', body: JSON.stringify({ query, limit }) }),
  deleteKnowledge: (id: string) =>
    request<void>(`/knowledge/${id}`, { method: 'DELETE' }),

  // 聊天
  listChatSessions: () => request<any[]>('/chat'),
  createChatSession: (title: string) =>
    request<{ id: string; title: string; created_at: string; updated_at: string }>('/chat', { method: 'POST', body: JSON.stringify({ title }) }),
  getChatMessages: (sessionId: string) =>
    request<any[]>(`/chat/${sessionId}/messages`),

  // 工具
  listTools: () => request<any[]>('/tools'),
  runTool: (name: string, args: any) =>
    request<{ success: boolean; raw?: string; summary?: string; error?: string }>(`/tools/${name}/run`, { method: 'POST', body: JSON.stringify({ args }) }),

  // API Token
  listTokens: () => request<any[]>('/auth/token'),
  createToken: (data: { name: string; scopes?: string; perm_level?: number }) =>
    request<{ id: string; token: string }>('/auth/token', { method: 'POST', body: JSON.stringify(data) }),
  revokeToken: (id: string) =>
    request<void>(`/auth/token/${id}`, { method: 'DELETE' }),
}

/**
 * fetchJson 与 request 相同，但暴露给页面内需要直接 fetch 的调用点。
 * 会检查 res.ok：服务器错误不会静默当作成功数据。
 */
export async function fetchJson<T = any>(path: string, options?: RequestInit): Promise<T> {
  const headers = new Headers(options?.headers || {})
  headers.set('Content-Type', 'application/json')
  const token = getAuthToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const res = await fetch(`${BASE_URL}${path}`, { ...options, headers })
  if (!res.ok) {
    throw new ApiError(`HTTP ${res.status}`, res.status)
  }
  const text = await res.text().catch(() => '')
  if (res.status === 204 || !text.trim()) return undefined as T
  return JSON.parse(text) as T
}

export { ApiError }
