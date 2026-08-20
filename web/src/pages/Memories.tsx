import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge, Button, Loading, Empty } from '../components'
import { useToast } from '../components/Toast'

interface Memory {
  id: string
  kind: string
  content: string
  tags: string
  source_task?: string
  confirmed: boolean
  created_at: string
}

export default function Memories() {
  const [mems, setMems] = useState<Memory[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState('')
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.getMemories(filter)
      .then(data => setMems(Array.isArray(data) ? data : []))
      .catch(() => setMems([]))
      .finally(() => setLoading(false))
  }, [filter])

  useEffect(refresh, [refresh])

  const handleConfirm = async (id: string) => {
    try {
      await api.confirmMemory(id)
      showToast('记忆已确认', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm('确定删除这条记忆？')) return
    try {
      await api.deleteMemory(id)
      showToast('已删除', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const kindLabel = (kind: string) => {
    switch (kind) {
      case 'long_term': return <Badge variant="primary">长期记忆</Badge>
      case 'working': return <Badge variant="warning">工作记忆</Badge>
      case 'short_term': return <Badge variant="info">短期记忆</Badge>
      default: return <Badge variant="info">{kind}</Badge>
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>记忆</h1>
        <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
          长期 / 工作 / 短期记忆 · 确认后参与 Agent 规划
        </p>
      </div>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        {['', 'long_term', 'working', 'short_term'].map(k => (
          <Button key={k} variant={filter === k ? 'primary' : 'default'} size="small"
            onClick={() => setFilter(k)}>
            {k === '' ? '全部' : k === 'long_term' ? '长期' : k === 'working' ? '工作' : '短期'}
          </Button>
        ))}
      </div>

      <Card shadow>
        {loading ? (
          <Loading block text="加载中..." />
        ) : mems.length === 0 ? (
          <Empty icon="🧠" text="暂无记忆" />
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {mems.map(mem => (
              <div key={mem.id} style={{
                padding: 12, border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius-md)', background: 'var(--color-bg)',
              }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    {kindLabel(mem.kind)}
                    {mem.confirmed
                      ? <Badge variant="success">已确认</Badge>
                      : <Badge variant="warning">待确认</Badge>}
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>
                    {new Date(mem.created_at).toLocaleDateString()}
                  </span>
                </div>
                <div style={{ fontSize: 14, marginBottom: 8 }}>{mem.content}</div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span style={{ fontSize: 12, color: 'var(--color-text-3)' }}>
                    {mem.tags}
                    {mem.source_task && ` · 来源: ${mem.source_task}`}
                  </span>
                  <div style={{ display: 'flex', gap: 8 }}>
                    {!mem.confirmed && (
                      <Button size="small" variant="success" onClick={() => handleConfirm(mem.id)}>确认</Button>
                    )}
                    <Button size="small" variant="danger" onClick={() => handleDelete(mem.id)}>删除</Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
