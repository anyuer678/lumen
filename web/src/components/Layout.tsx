import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useSSE } from '../hooks/useSSE'
import CreateTaskModal from './CreateTaskModal'

type NavItem = { path: string; icon: string; label: string }

// 普通模式导航（6 个核心页面）
const NAV_NORMAL: NavItem[] = [
  { path: '/', icon: '◈', label: 'Today' },
  { path: '/chat', icon: '💬', label: '对话' },
  { path: '/tasks', icon: '▤', label: '任务' },
  { path: '/memories', icon: '🧠', label: '记忆' },
  { path: '/knowledge', icon: '📚', label: '知识库' },
  { path: '/settings', icon: '⚙', label: '设置' },
]

// 专家模式额外导航（14 个页面）
const NAV_EXPERT: { group: string; items: NavItem[] }[] = [
  {
    group: '运行',
    items: [
      { path: '/overview', icon: '◈', label: '仪表盘' },
      { path: '/jobs', icon: '◷', label: '定时任务' },
      { path: '/confirms', icon: '✋', label: '确认队列' },
      { path: '/profiles', icon: '👤', label: '用户画像' },
    ],
  },
  {
    group: '开发',
    items: [
      { path: '/tools', icon: '⚒', label: '工具' },
      { path: '/mcp', icon: '🔌', label: 'MCP 服务器' },
      { path: '/artifacts', icon: '🖼', label: '产物' },
      { path: '/events', icon: '📮', label: '事件' },
      { path: '/token-usage', icon: '📊', label: 'Token' },
      { path: '/schedule', icon: '⏱', label: '调度' },
    ],
  },
  {
    group: '系统',
    items: [
      { path: '/audit', icon: '📜', label: '审计' },
      { path: '/tokens', icon: '🔑', label: 'API Token' },
      { path: '/logs', icon: '≡', label: '日志' },
    ],
  },
]

function pageTitle(path: string): string {
  for (const item of NAV_NORMAL) {
    if (item.path === path) return item.label
  }
  for (const g of NAV_EXPERT) {
    const it = g.items.find(i => i.path === path)
    if (it) return it.label
  }
  return 'Today'
}

export default function Layout() {
  const { connected } = useSSE()
  const location = useLocation()
  const [modalOpen, setModalOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [expertMode, setExpertMode] = useState(() => {
    return localStorage.getItem('expertMode') === 'true'
  })
  const title = pageTitle(location.pathname)

  const toggleExpert = () => {
    const next = !expertMode
    setExpertMode(next)
    localStorage.setItem('expertMode', String(next))
  }

  // 全局"新建任务"事件
  useEffect(() => {
    const open = () => setModalOpen(true)
    window.addEventListener('kb:new-task', open)
    return () => window.removeEventListener('kb:new-task', open)
  }, [])

  return (
    <div style={{ display: 'flex', minHeight: '100vh' }}>
      {/* 侧边栏 */}
      <aside style={{
        width: 224,
        background: 'var(--color-bg)',
        borderRight: '1px solid var(--color-border)',
        display: 'flex',
        flexDirection: 'column',
        position: 'fixed',
        top: 0,
        bottom: 0,
        left: 0,
        zIndex: 999,
        flexShrink: 0,
        transform: collapsed ? 'translateX(-101%)' : 'translateX(0)',
        transition: 'transform 0.25s ease',
        boxShadow: 'var(--shadow-3)',
      }}>
        {/* Logo */}
        <div style={{
          padding: '18px 20px',
          borderBottom: '1px solid var(--color-border)',
          display: 'flex',
          alignItems: 'center',
          gap: 12,
        }}>
          <div style={{
            width: 38, height: 38, borderRadius: 8,
            background: 'var(--color-danger)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: '#fff', fontWeight: 700, fontSize: 17,
            fontFamily: 'var(--font-family)',
            boxShadow: 'var(--shadow-1)',
          }}>录</div>
          <div>
            <div style={{ fontWeight: 700, fontSize: 17, color: 'var(--color-text-1)', letterSpacing: '0.1em' }}>智能管家</div>
            <div style={{ fontSize: 11, color: 'var(--color-text-3)', letterSpacing: '0.05em' }}>Agent v0.1.0</div>
          </div>
        </div>

        {/* 导航 */}
        <nav style={{ padding: '10px 10px 16px', flex: 1, overflowY: 'auto' }}>
          {/* 普通模式导航 */}
          <div style={{ marginBottom: 14 }}>
            {NAV_NORMAL.map(item => (
              <NavLink
                key={item.path}
                to={item.path}
                end={item.path === '/'}
                style={({ isActive }) => ({
                  display: 'flex', alignItems: 'center', gap: 10,
                  padding: '9px 14px', borderRadius: 7,
                  color: isActive ? 'var(--color-primary)' : 'var(--color-text-2)',
                  background: isActive ? 'var(--color-primary-light)' : 'transparent',
                  textDecoration: 'none', fontSize: 14,
                  fontWeight: isActive ? 500 : 400,
                  transition: 'all 0.15s ease', marginBottom: 2,
                })}
              >
                <span style={{ width: 20, textAlign: 'center', fontSize: 14 }}>{item.icon}</span>
                {item.label}
              </NavLink>
            ))}
          </div>

          {/* 专家模式导航（仅在专家模式下显示） */}
          {expertMode && NAV_EXPERT.map(group => (
            <div key={group.group} style={{ marginBottom: 14 }}>
              <div style={{
                fontSize: 11, color: 'var(--color-text-3)', letterSpacing: '0.12em',
                fontWeight: 600, padding: '6px 12px 4px',
              }}>
                —— {group.group}
              </div>
              {group.items.map(item => (
                <NavLink
                  key={item.path}
                  to={item.path}
                  style={({ isActive }) => ({
                    display: 'flex', alignItems: 'center', gap: 10,
                    padding: '9px 14px', borderRadius: 7,
                    color: isActive ? 'var(--color-primary)' : 'var(--color-text-2)',
                    background: isActive ? 'var(--color-primary-light)' : 'transparent',
                    textDecoration: 'none', fontSize: 14,
                    fontWeight: isActive ? 500 : 400,
                    transition: 'all 0.15s ease', marginBottom: 2,
                  })}
                >
                  <span style={{ width: 20, textAlign: 'center', fontSize: 14 }}>{item.icon}</span>
                  {item.label}
                </NavLink>
              ))}
            </div>
          ))}
        </nav>

        {/* 底部：模式切换 + 状态 */}
        <div style={{
          padding: '12px 20px', borderTop: '1px solid var(--color-border)',
          fontSize: 12, color: 'var(--color-text-3)',
        }}>
          <button
            onClick={toggleExpert}
            style={{
              width: '100%', padding: '6px 0', borderRadius: 6,
              border: '1px solid var(--color-border)', background: 'var(--color-bg-elevated)',
              cursor: 'pointer', fontSize: 11, color: expertMode ? 'var(--color-primary)' : 'var(--color-text-3)',
              marginBottom: 8,
            }}
          >
            {expertMode ? '🔧 专家模式 ON' : '🔧 专家模式 OFF'}
          </button>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span className="kb-live-dot" style={{ background: connected ? undefined : 'var(--color-danger)', animation: connected ? undefined : 'none' }} />
            {connected ? '实时连接中' : '连接断开'}
          </div>
        </div>
      </aside>

      {/* 主内容 */}
      <div style={{ flex: 1, minWidth: 0, marginLeft: collapsed ? 0 : 224, transition: 'margin 0.25s ease' }}>
        {/* 顶部栏 */}
        <header style={{
          position: 'sticky', top: 0, zIndex: 900,
          height: 56, display: 'flex', alignItems: 'center',
          padding: '0 20px', gap: 12,
          background: 'rgba(253,251,245,0.85)', backdropFilter: 'blur(8px)',
          borderBottom: '1px solid var(--color-border)',
        }}>
          <button
            className="kb-btn kb-btn--ghost"
            onClick={() => setCollapsed(c => !c)}
            style={{ padding: '0 8px', height: 34 }}
            aria-label="切换侧边栏"
          >
            ☰
          </button>
          <h1 style={{ fontSize: 16, fontWeight: 600, margin: 0, letterSpacing: '0.04em', flex: 1 }}>
            {title}
          </h1>
          <button
            className="kb-btn kb-btn--primary"
            onClick={() => setModalOpen(true)}
            style={{ height: 34 }}
          >
            ＋ 新建任务
          </button>
        </header>

        <main className="kb-page" style={{ padding: 20, maxWidth: 1200, margin: '0 auto' }}>
          <Outlet />
        </main>
      </div>

      <CreateTaskModal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        onCreated={() => window.dispatchEvent(new Event('kb:tasks-changed'))}
      />
    </div>
  )
}
