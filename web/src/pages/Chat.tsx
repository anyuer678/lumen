import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import { Button } from '../components'
import { useToast } from '../components/Toast'
import { useSSE } from '../hooks/useSSE'

// ---- 类型定义 ----

interface ChatSession {
  id: string
  title: string
  created_at: string
}

interface ChatMessage {
  id: string
  session_id: string
  role: string
  content: string
  created_at: string
}

// 任务执行事件（来自 SSE）
interface TaskEvent {
  type: string
  data: Record<string, any>
}

// ---- 主组件 ----

export default function Chat() {
  const [sessions, setSessions] = useState<ChatSession[]>([])
  const [currentSession, setCurrentSession] = useState<ChatSession | null>(null)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [sending, setSending] = useState(false)
  const [loading, setLoading] = useState(true)

  // 实时任务进度状态
  const [activeTask, setActiveTask] = useState<{
    taskId: string
    goal: string
    steps: { desc: string; tool: string; status: string; result?: string }[]
    currentStep: number
    totalSteps: number
    status: string  // running / completed / failed
  } | null>(null)

  const messagesEndRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const { showToast } = useToast()

  // SSE 连接：订阅任务事件
  const { connected } = useSSE((event) => {
    const { type, data } = event as TaskEvent
    if (!data?.task_id) return

    setActiveTask(prev => {
      if (!prev || prev.taskId !== data.task_id) {
        // 新任务事件：初始化进度面板（任务可能由聊天触发）
        if (type === 'step.started' || type === 'step.completed') {
          return {
            taskId: data.task_id,
            goal: data.description || '',
            steps: [],
            currentStep: data.step || 1,
            totalSteps: data.total || 0,
            status: 'running',
          }
        }
        return prev
      }

      // 更新同一任务的进度
      const updated = { ...prev }
      switch (type) {
        case 'step.started':
          updated.currentStep = data.step
          updated.totalSteps = data.total
          updated.steps = [...updated.steps, {
            desc: data.description || data.tool || '执行中',
            tool: data.tool || '',
            status: 'running',
          }]
          break

        case 'step.completed': {
          const stepIdx = (data.step || 1) - 1
          const steps = [...updated.steps]
          while (steps.length <= stepIdx) steps.push({ desc: '', tool: '', status: 'pending' })
          steps[stepIdx] = {
            desc: data.description || data.tool || steps[stepIdx]?.desc || '',
            tool: data.tool || steps[stepIdx]?.tool || '',
            status: 'completed',
            result: data.summary || data.result || '',
          }
          updated.steps = steps
          updated.currentStep = data.step
          updated.totalSteps = data.total
          break
        }

        case 'step.failed': {
          const failIdx = (data.step || 1) - 1
          const failSteps = [...updated.steps]
          while (failSteps.length <= failIdx) failSteps.push({ desc: '', tool: '', status: 'pending' })
          failSteps[failIdx] = {
            ...failSteps[failIdx],
            status: 'failed',
            result: data.error || '',
          }
          updated.steps = failSteps
          break
        }

        case 'task.completed':
          updated.status = 'completed'
          break

        case 'task.failed':
          updated.status = 'failed'
          break
      }
      return updated
    })
  })

  // SSE 连接状态提示
  useEffect(() => {
    if (!connected) {
      showToast('SSE 连接断开，实时进度不可用', 'warning')
    }
  }, [connected])

  // 加载会话列表
  const loadSessions = useCallback(() => {
    api.listChatSessions()
      .then(data => {
        setSessions(Array.isArray(data) ? data : [])
        if (data && data.length > 0 && !currentSession) {
          setCurrentSession(data[0])
          loadMessages(data[0].id)
        }
      })
      .catch(() => setSessions([]))
      .finally(() => setLoading(false))
  }, [])

  // 加载消息历史
  const loadMessages = useCallback(async (sessionId: string) => {
    try {
      const msgs = await api.getChatMessages(sessionId)
      setMessages(Array.isArray(msgs) ? msgs : [])
    } catch {
      setMessages([])
    }
  }, [])

  useEffect(() => { loadSessions() }, [])
  useEffect(() => { messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, activeTask])

  // 创建新会话
  const handleNewSession = async () => {
    try {
      const session = await api.createChatSession('新对话')
      setSessions(prev => [session, ...prev])
      setCurrentSession(session)
      setMessages([])
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  // 发送消息（SSE 流式 + 实时进度）
  const handleSend = async () => {
    if (!input.trim() || !currentSession || sending) return

    const userMsg: ChatMessage = {
      id: `msg-${Date.now()}`,
      session_id: currentSession.id,
      role: 'user',
      content: input.trim(),
      created_at: new Date().toISOString(),
    }

    setMessages(prev => [...prev, userMsg])
    setInput('')
    setSending(true)
    setActiveTask(null)

    // 创建 assistant 占位消息
    const assistantMsg: ChatMessage = {
      id: `msg-${Date.now() + 1}`,
      session_id: currentSession.id,
      role: 'assistant',
      content: '',
      created_at: new Date().toISOString(),
    }
    setMessages(prev => [...prev, assistantMsg])

    try {
      const response = await fetch(`/v1/chat/${currentSession.id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content: userMsg.content }),
      })

      if (!response.ok) {
        throw new Error(`请求失败: ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) throw new Error('No reader')

      const decoder = new TextDecoder()
      let fullContent = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        const chunk = decoder.decode(value)
        const lines = chunk.split('\n')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6))
              if (data.done) break
              if (data.content) {
                fullContent += data.content
                setMessages(prev => {
                  const updated = [...prev]
                  const lastMsg = updated[updated.length - 1]
                  if (lastMsg && lastMsg.role === 'assistant') {
                    lastMsg.content = fullContent
                  }
                  return [...updated]
                })
              }
            } catch { /* ignore */ }
          }
        }
      }
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSending(false)
      inputRef.current?.focus()
    }
  }

  const handleSwitchSession = (session: ChatSession) => {
    setCurrentSession(session)
    loadMessages(session.id)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  if (loading) return <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}><span>加载中...</span></div>

  return (
    <div style={{ display: 'flex', height: 'calc(100vh - 0px)' }}>
      {/* 左侧会话列表 */}
      <div style={{
        width: 260, borderRight: '1px solid var(--color-border)',
        display: 'flex', flexDirection: 'column', background: 'var(--color-bg)',
        flexShrink: 0,
      }}>
        <div style={{ padding: '16px 16px 12px', borderBottom: '1px solid var(--color-border)' }}>
          <Button variant="primary" block onClick={handleNewSession}>+ 新对话</Button>
        </div>
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px 8px' }}>
          {sessions.map(s => (
            <div
              key={s.id}
              className="kb-chat-session"
              onClick={() => handleSwitchSession(s)}
              style={{
                padding: '10px 12px', borderRadius: 6, cursor: 'pointer',
                background: currentSession?.id === s.id ? 'var(--color-primary-light)' : 'transparent',
                color: currentSession?.id === s.id ? 'var(--color-primary)' : 'var(--color-text-1)',
                fontSize: 13, marginBottom: 2, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
              }}
            >
              {s.title}
            </div>
          ))}
          {sessions.length === 0 && (
            <div style={{ padding: 20, textAlign: 'center', color: 'var(--color-text-3)', fontSize: 12 }}>
              暂无对话
            </div>
          )}
        </div>
        {/* SSE 状态指示 */}
        <div style={{
          padding: '8px 16px', borderTop: '1px solid var(--color-border)',
          fontSize: 11, color: connected ? 'var(--color-success)' : 'var(--color-danger)',
          display: 'flex', alignItems: 'center', gap: 6,
        }}>
          <span style={{
            width: 6, height: 6, borderRadius: '50%',
            background: connected ? 'var(--color-success)' : 'var(--color-danger)',
          }} />
          {connected ? 'SSE 已连接' : 'SSE 未连接'}
        </div>
      </div>

      {/* 右侧聊天区 */}
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', background: 'var(--color-bg-elevated)' }}>
        {/* 消息列表 */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '20px 24px' }}>
          {messages.length === 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--color-text-3)' }}>
              <div style={{ fontSize: 48, marginBottom: 16 }}>🤖</div>
              <div style={{ fontSize: 18, fontWeight: 600, color: 'var(--color-text-1)', marginBottom: 8 }}>智能管家</div>
              <div style={{ fontSize: 14 }}>告诉我你想做什么，我会帮你完成</div>
              <div style={{ marginTop: 24, display: 'flex', gap: 8, flexWrap: 'wrap', justifyContent: 'center' }}>
                {['帮我整理下载文件夹', '查看系统状态', '帮我写一个脚本'].map(q => (
                  <button
                    key={q}
                    onClick={() => setInput(q)}
                    style={{
                      padding: '8px 16px', border: '1px solid var(--color-border)',
                      borderRadius: 8, background: 'var(--color-bg)', cursor: 'pointer',
                      fontSize: 13, color: 'var(--color-text-2)',
                    }}
                  >
                    {q}
                  </button>
                ))}
              </div>
            </div>
          ) : (
            <div style={{ maxWidth: 800, margin: '0 auto' }}>
              {messages.map(msg => (
                <div key={msg.id} className="kb-msg" style={{
                  display: 'flex', marginBottom: 16,
                  justifyContent: msg.role === 'user' ? 'flex-end' : 'flex-start',
                }}>
                  {msg.role === 'assistant' && (
                    <div style={{
                      width: 32, height: 32, borderRadius: 8, background: 'var(--color-danger)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      color: '#fff', fontSize: 14, marginRight: 8, flexShrink: 0,
                    }}>录</div>
                  )}
                  <div style={{
                    maxWidth: '70%',
                    padding: '12px 16px',
                    borderRadius: msg.role === 'user' ? '12px 12px 0 12px' : '12px 12px 12px 0',
                    background: msg.role === 'user' ? 'var(--color-primary)' : 'var(--color-bg)',
                    color: msg.role === 'user' ? '#fff' : 'var(--color-text-1)',
                    fontSize: 14, lineHeight: 1.6, whiteSpace: 'pre-wrap',
                    boxShadow: 'var(--shadow-1)',
                  }}>
                    {sending && msg.role === 'assistant' && !msg.content ? (
                      <span className="kb-typing">
                        <span className="kb-typing-dot" /><span className="kb-typing-dot" /><span className="kb-typing-dot" />
                      </span>
                    ) : msg.content ? msg.content : ''}
                  </div>
                </div>
              ))}

              {/* 实时任务进度面板 */}
              {activeTask && (
                <TaskProgressPanel task={activeTask} />
              )}

              <div ref={messagesEndRef} />
            </div>
          )}
        </div>

        {/* 输入框 */}
        <div style={{
          padding: '16px 24px', borderTop: '1px solid var(--color-border)',
          background: 'var(--color-bg-elevated)',
        }}>
          <div style={{ maxWidth: 800, margin: '0 auto', display: 'flex', gap: 12, alignItems: 'flex-end' }}>
            <textarea
              ref={inputRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="告诉我你想做什么..."
              rows={1}
              style={{
                flex: 1, resize: 'none', padding: '12px 16px',
                border: '1px solid var(--color-border)', borderRadius: 12,
                fontSize: 14, fontFamily: 'inherit', outline: 'none',
                background: 'var(--color-bg)', color: 'var(--color-text-1)',
                minHeight: 44, maxHeight: 120,
              }}
            />
            <Button
              variant="primary"
              onClick={handleSend}
              disabled={sending || !input.trim()}
              loading={sending}
              style={{ height: 44, padding: '0 20px', borderRadius: 12 }}
            >
              {sending ? '发送中' : '发送'}
            </Button>
          </div>
          <div style={{ maxWidth: 800, margin: '6px auto 0', fontSize: 11, color: 'var(--color-text-3)', textAlign: 'center' }}>
            按 Enter 发送 · Shift+Enter 换行
          </div>
        </div>
      </div>
    </div>
  )
}

// ---- 任务进度面板组件 ----

function TaskProgressPanel({ task }: {
  task: {
    taskId: string
    goal: string
    steps: { desc: string; tool: string; status: string; result?: string }[]
    currentStep: number
    totalSteps: number
    status: string
  }
}) {
  const progress = task.totalSteps > 0
    ? Math.round((task.currentStep / task.totalSteps) * 100)
    : 0

  const statusConfig: Record<string, { color: string; label: string; bg: string }> = {
    running:   { color: 'var(--color-primary)', label: '执行中', bg: 'var(--color-primary-light)' },
    completed: { color: 'var(--color-success)', label: '已完成', bg: '#f0fdf4' },
    failed:    { color: 'var(--color-danger)',  label: '失败',   bg: '#fef2f2' },
  }
  const st = statusConfig[task.status] || statusConfig.running

  return (
    <div style={{
      border: `1px solid ${st.color}`,
      borderRadius: 12, padding: 16, marginBottom: 16,
      background: st.bg,
    }}>
      {/* 标题栏 */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{
            fontSize: 11, fontWeight: 600, padding: '2px 8px', borderRadius: 4,
            background: st.color, color: '#fff',
          }}>{st.label}</span>
          {task.goal && (
            <span style={{ fontSize: 13, color: 'var(--color-text-2)' }}>
              {task.goal.length > 40 ? task.goal.slice(0, 40) + '...' : task.goal}
            </span>
          )}
        </div>
        <span style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
          {task.currentStep}/{task.totalSteps} 步
        </span>
      </div>

      {/* 进度条 */}
      <div style={{
        height: 4, borderRadius: 2, background: 'var(--color-border)',
        overflow: 'hidden', marginBottom: 12,
      }}>
        <div style={{
          height: '100%', borderRadius: 2, background: st.color,
          width: `${progress}%`, transition: 'width 0.3s ease',
        }} />
      </div>

      {/* 步骤列表 */}
      {task.steps.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          {task.steps.map((step, i) => (
            <StepItem key={i} step={step} isCurrent={i === task.steps.length - 1 && task.status === 'running'} />
          ))}
        </div>
      )}

      {/* 完成提示 */}
      {task.status === 'completed' && (
        <div style={{ marginTop: 8, fontSize: 12, color: 'var(--color-success)', fontWeight: 500 }}>
          ✓ 任务执行完成
        </div>
      )}
      {task.status === 'failed' && (
        <div style={{ marginTop: 8, fontSize: 12, color: 'var(--color-danger)', fontWeight: 500 }}>
          ✗ 任务执行失败
        </div>
      )}
    </div>
  )
}

function StepItem({ step, isCurrent }: {
  step: { desc: string; tool: string; status: string; result?: string }
  isCurrent: boolean
}) {
  const [expanded, setExpanded] = useState(false)
  const statusIcon: Record<string, string> = {
    running: '◉',
    completed: '✓',
    failed: '✗',
    pending: '○',
  }
  const statusColor: Record<string, string> = {
    running: 'var(--color-primary)',
    completed: 'var(--color-success)',
    failed: 'var(--color-danger)',
    pending: 'var(--color-text-3)',
  }

  return (
    <div style={{
      padding: '6px 8px', borderRadius: 6, fontSize: 12,
      background: isCurrent ? 'rgba(0,0,0,0.02)' : 'transparent',
      borderLeft: `3px solid ${statusColor[step.status] || 'transparent'}`,
      paddingLeft: 10, transition: 'background 0.2s',
    }}>
      <div
        style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: step.result ? 'pointer' : 'default' }}
        onClick={() => step.result && setExpanded(!expanded)}
      >
        <span style={{ color: statusColor[step.status], fontWeight: 600, width: 14, textAlign: 'center' }}>
          {statusIcon[step.status] || '○'}
        </span>
        <span style={{ fontWeight: 500, minWidth: 60, color: 'var(--color-text-3)' }}>{step.tool}</span>
        <span style={{ color: 'var(--color-text-1)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {step.desc || '...'}
        </span>
        {step.result && (
          <span style={{ color: 'var(--color-text-3)', fontSize: 10 }}>{expanded ? '▾' : '▸'}</span>
        )}
      </div>
      {expanded && step.result && (
        <pre style={{
          margin: '4px 0 0 20px', padding: 6, borderRadius: 4,
          background: 'var(--color-bg)', fontSize: 11, color: 'var(--color-text-2)',
          whiteSpace: 'pre-wrap', wordBreak: 'break-all', maxHeight: 120, overflow: 'auto',
        }}>{step.result}</pre>
      )}
    </div>
  )
}
