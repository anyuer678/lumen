import { useEffect, useCallback, useState } from 'react'
import { Card, Loading, Empty, Button } from '../components'

interface Artifact {
  name: string
  size: number
  modified_at: string
  url: string
}

export default function Artifacts() {
  const [items, setItems] = useState<Artifact[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(() => {
    fetch('/v1/artifacts')
      .then(r => r.json())
      .then(d => setItems(Array.isArray(d?.items) ? d.items : []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const isImage = (n: string) => /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(n)
  const fmtSize = (b: number) => b > 1048576 ? (b / 1048576).toFixed(1) + ' MB' : (b / 1024).toFixed(1) + ' KB'

  if (loading) return <Loading />

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 600, margin: 0 }}>产物</h1>
          <p style={{ color: 'var(--color-text-3)', fontSize: 13, marginTop: 4 }}>
            Agent 产生的文件（截图/输出等）· 共 {items.length} 个
          </p>
        </div>
        <Button size="small" onClick={refresh}>刷新</Button>
      </div>

      {items.length === 0 ? (
        <Empty text="暂无产物（截图或工具输出保存后会显示在这里）" />
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 16 }}>
          {items.map(it => (
            <Card key={it.name} title={it.name} shadow style={{ overflow: 'hidden' }}>
              <div style={{ width: '100%', height: 160, background: 'var(--color-fill-2)', display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
                {isImage(it.name) ? (
                  <img src={it.url} alt={it.name} style={{ maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }} />
                ) : (
                  <span style={{ color: 'var(--color-text-3)' }}>📄</span>
                )}
              </div>
              <div style={{ padding: 8  }}>
                <div style={{ fontSize: 12, color: 'var(--color-text-3)' }}>{fmtSize(it.size)} · {it.modified_at?.slice(0, 10)}</div>
                <a href={it.url} target="_blank" rel="noreferrer" style={{ fontSize: 13, color: 'var(--color-primary)' }}>打开 / 下载</a>
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}
