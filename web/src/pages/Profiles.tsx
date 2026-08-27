import { useEffect, useState } from 'react'
import { Card, Loading, Empty, Button } from '../components'
import { useToast } from '../components/Toast'
import { fetchJson } from '../api/client'

interface UserProfile {
  id: string
  category: string
  content: string
  confidence: number
  source: string
  count: number
  created_at: string
  updated_at: string
}

const categoryColors: Record<string, string> = {
  preference: 'var(--color-primary)',
  skill: 'var(--color-success)',
  habit: 'var(--color-warning)',
  info: 'var(--color-info)',
}

const categoryLabels: Record<string, string> = {
  preference: '偏好',
  skill: '技能',
  habit: '习惯',
  info: '信息',
}

export default function Profiles() {
  const [profiles, setProfiles] = useState<UserProfile[]>([])
  const [loading, setLoading] = useState(true)
  const [reflecting, setReflecting] = useState(false)
  const { showToast } = useToast()

  const loadProfiles = () => {
    setLoading(true)
    fetchJson<{ value: UserProfile[] }>('/profiles')
      .then(d => setProfiles(d?.value ?? []))
      .catch(() => setProfiles([]))
      .finally(() => setLoading(false))
  }

  useEffect(loadProfiles, [])

  const handleReflect = async () => {
    setReflecting(true)
    try {
      const data = await fetchJson<{ count: number }>('/profiles/reflect', { method: 'POST' })
      showToast(`反思完成：生成 ${data.count ?? 0} 条画像`, 'success')
      loadProfiles()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setReflecting(false)
    }
  }

  if (loading) return <Loading block text="加载用户画像..." />

  return (
    <div style={{ maxWidth: 900, margin: '0 auto', padding: '24px 0' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>🧠 用户画像</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
            从记忆中自动提炼的用户偏好、技能和习惯
          </p>
        </div>
        <Button variant="primary" onClick={handleReflect} loading={reflecting}>
          {reflecting ? '反思中...' : '🔄 执行反思'}
        </Button>
      </div>

      {profiles.length === 0 ? (
        <Card shadow>
          <Empty icon="🧠" text="暂无用户画像">
            <p style={{ fontSize: 13, color: 'var(--color-text-3)', marginTop: 8 }}>
              点击「执行反思」从记忆中自动提炼用户偏好、技能和习惯
            </p>
          </Empty>
        </Card>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 16 }}>
          {profiles.map(p => (
            <Card key={p.id} shadow>
              <div style={{ padding: 16 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <span style={{
                    fontSize: 11, fontWeight: 600, padding: '2px 8px', borderRadius: 4,
                    background: categoryColors[p.category] || 'var(--color-info)', color: '#fff',
                  }}>
                    {categoryLabels[p.category] || p.category}
                  </span>
                  <span style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
                    置信度 {(p.confidence * 100).toFixed(0)}%
                  </span>
                </div>
                <p style={{ fontSize: 14, margin: '8px 0', lineHeight: 1.6 }}>{p.content}</p>
                <div style={{ fontSize: 11, color: 'var(--color-text-3)', display: 'flex', gap: 12 }}>
                  <span>来源: {p.source}</span>
                  <span>次数: {p.count}</span>
                  <span>更新: {new Date(p.updated_at).toLocaleDateString()}</span>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
