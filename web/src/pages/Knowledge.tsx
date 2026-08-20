import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge, Button, Input, Textarea, FormItem, Loading, Empty } from '../components'
import { useToast } from '../components/Toast'

interface Knowledge {
  id: string
  title: string
  content: string
  tags: string
  source: string
  created_at: string
}

export default function Knowledge() {
  const [items, setItems] = useState<Knowledge[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [search, setSearch] = useState('')
  const [form, setForm] = useState({ title: '', content: '', tags: '' })
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.listKnowledge()
      .then(data => setItems(Array.isArray(data) ? data : []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [])

  const handleAdd = async () => {
    if (!form.title || !form.content) {
      showToast('请填写标题和内容', 'warning')
      return
    }
    setSubmitting(true)
    try {
      await api.addKnowledge({ title: form.title, content: form.content, tags: form.tags })
      showToast('知识已添加', 'success')
      setForm({ title: '', content: '', tags: '' })
      setShowForm(false)
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm('确定删除该知识')) return
    try {
      await api.deleteKnowledge(id)
      showToast('已删除', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  const handleSearch = async () => {
    if (!search.trim()) { refresh(); return }
    try {
      const result = await api.searchKnowledge(search)
      setItems(Array.isArray(result) ? result : [])
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>知识库</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>Agent 的外部大脑 · 用户的偏好和资料</p>
        </div>
        <Button variant="primary" onClick={() => setShowForm(v => !v)}>{showForm ? '取消' : '+ 添加知识'}</Button>
      </div>

      {/* 搜索 */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <div style={{ flex: 1 }}>
          <Input placeholder="搜索知识库..." value={search} onChange={e => setSearch(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleSearch() }} />
        </div>
        <Button onClick={handleSearch}>搜索</Button>
      </div>

      {/* 添加表单 */}
      {showForm && (
        <Card shadow style={{ marginBottom: 16 }}>
          <div className="kb-card__title" style={{ marginBottom: 12 }}>添加知识</div>
          <FormItem label="标题">
            <Input value={form.title} onChange={e => setForm({ ...form, title: e.target.value })} placeholder="例如：我的生日 / 项目偏好" />
          </FormItem>
          <FormItem label="内容">
            <Textarea rows={4} value={form.content} onChange={e => setForm({ ...form, content: e.target.value })} placeholder="知识内容" />
          </FormItem>
          <FormItem label="标签（逗号分隔）">
            <Input value={form.tags} onChange={e => setForm({ ...form, tags: e.target.value })} placeholder="person,preference" />
          </FormItem>
          <Button variant="primary" onClick={handleAdd} loading={submitting} block>保存知识</Button>
        </Card>
      )}

      <Card shadow>
        {loading ? <Loading block /> : items.length === 0 ? (
          <Empty icon="📚" text="知识库为空">
            <Button size="small" variant="primary" onClick={() => setShowForm(true)}>添加第一条知识</Button>
          </Empty>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {items.map(k => (
              <div key={k.id} style={{ padding: 14, border: '1px solid var(--color-border)', borderRadius: 'var(--radius-md)', background: 'var(--color-bg)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
                  <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <span style={{ fontWeight: 600 }}>{k.title}</span>
                    <Badge variant="info">{k.source}</Badge>
                  </div>
                  <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>{new Date(k.created_at).toLocaleDateString()}</span>
                </div>
                <div style={{ fontSize: 13, color: 'var(--color-text-2)', marginBottom: 6 }}>{k.content}</div>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span style={{ fontSize: 11, color: 'var(--color-text-3)' }}>{k.tags}</span>
                  <Button size="small" variant="danger" onClick={() => handleDelete(k.id)}>删除</Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
