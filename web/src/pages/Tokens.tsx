import { useCallback, useEffect, useState } from 'react'
import { api } from '../api/client'
import { Card, Badge, Button, Input, Select, FormItem, Loading, Empty, Modal } from '../components'
import { useToast } from '../components/Toast'

interface ApiToken {
  id: string
  name: string
  scopes: string
  perm_level: number
  enabled: boolean
  created_at: string
}

export default function Tokens() {
  const [tokens, setTokens] = useState<ApiToken[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [showToken, setShowToken] = useState('')
  const [form, setForm] = useState({ name: '', scopes: 'tasks:create,tasks:control,confirm:approve', perm_level: 1 })
  const [submitting, setSubmitting] = useState(false)
  const { showToast } = useToast()

  const refresh = useCallback(() => {
    api.listTokens()
      .then(data => setTokens(Array.isArray(data) ? data : []))
      .catch(() => setTokens([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(refresh, [refresh])

  const handleCreate = async () => {
    if (!form.name) {
      showToast('请填写名称', 'warning')
      return
    }
    setSubmitting(true)
    try {
      const r = await api.createToken(form)
      setShowToken(r.token || '')
      showToast('Token 已生成', 'success')
      setShowForm(false)
      setForm({ name: '', scopes: 'tasks:create,tasks:control,confirm:approve', perm_level: 1 })
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    } finally {
      setSubmitting(false)
    }
  }

  const handleRevoke = async (id: string) => {
    if (!window.confirm('确定吊销该 Token？')) return
    try {
      await api.revokeToken(id)
      showToast('已吊销', 'success')
      refresh()
    } catch (e) {
      showToast((e as Error).message, 'error')
    }
  }

  return (
    <div style={{ padding: 24, maxWidth: 900, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>API Token</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
            用于外部程序访问本 Agent 的凭证
          </p>
        </div>
        <Button variant="primary" onClick={() => setShowForm(v => !v)}>
          {showForm ? '取消' : '+ 生成 Token'}
        </Button>
      </div>

      {showForm && (
        <Card shadow style={{ marginBottom: 16 }}>
          <div className="kb-card__title" style={{ marginBottom: 16 }}>生成 API Token</div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <FormItem label="名称">
              <Input placeholder="例如: phone-app" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
            </FormItem>
            <FormItem label="权限等级">
              <Select value={form.perm_level} onChange={e => setForm({ ...form, perm_level: Number(e.target.value) })}>
                <option value={0}>L0 只读</option>
                <option value={1}>L1 普通</option>
                <option value={2}>L2 危险</option>
              </Select>
            </FormItem>
            <FormItem label="Scope（逗号分隔）">
              <Input value={form.scopes} onChange={e => setForm({ ...form, scopes: e.target.value })} />
            </FormItem>
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
            <Button onClick={() => setShowForm(false)}>取消</Button>
            <Button variant="primary" onClick={handleCreate} loading={submitting}>生成</Button>
          </div>
        </Card>
      )}

      <Card shadow>
        {loading ? (
          <Loading block />
        ) : tokens.length === 0 ? (
          <Empty icon="🔑" text="暂无 Token" />
        ) : (
          <table className="kb-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>Scope</th>
                <th>等级</th>
                <th>状态</th>
                <th>创建时间</th>
                <th style={{ width: 80 }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {tokens.map(t => (
                <tr key={t.id}>
                  <td>{t.name}</td>
                  <td style={{ fontSize: 12, fontFamily: 'var(--font-mono)' }}>{t.scopes}</td>
                  <td><Badge variant={t.perm_level >= 2 ? 'danger' : t.perm_level === 1 ? 'primary' : 'success'}>L{t.perm_level}</Badge></td>
                  <td>{t.enabled ? <Badge variant="success">启用</Badge> : <Badge variant="info">已吊销</Badge>}</td>
                  <td style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{new Date(t.created_at).toLocaleDateString()}</td>
                  <td>
                    {t.enabled ? (
                      <Button size="small" variant="danger" onClick={() => handleRevoke(t.id)}>吊销</Button>
                    ) : (
                      <span style={{ color: 'var(--color-text-3)' }}>—</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {/* 显示生成的 Token */}
      <Modal open={!!showToken} title="Token 已生成" onClose={() => setShowToken('')}
        footer={<Button variant="primary" onClick={() => setShowToken('')}>我已保存</Button>}>
        <p style={{ marginBottom: 12, color: 'var(--color-danger)', fontWeight: 600 }}>
          ⚠️ 请立即复制保存，此 Token 仅显示一次！</p>
        <div style={{
          padding: 12, background: 'var(--color-bg)', borderRadius: 'var(--radius-md)',
          fontFamily: 'var(--font-mono)', fontSize: 12, wordBreak: 'break-all', userSelect: 'all',
        }}>
          {showToken}
        </div>
      </Modal>
    </div>
  )
}
