import { useEffect, useRef, useState } from 'react'
import { getAuthTokenPublic as getAuthToken } from '../api/client'

interface SSEEvent {
  type: string
  data: unknown
}

/**
 * SSE hook：订阅服务端事件流 /v1/events
 * EventSource 无法携带自定义头，token 通过 ?token= 查询参数传递；
 * 未配置 token（认证关闭）时直接连接。
 */
export function useSSE(onEvent?: (event: SSEEvent) => void) {
  const [connected, setConnected] = useState(false)
  const [events, setEvents] = useState<SSEEvent[]>([])
  const handlerRef = useRef(onEvent)
  handlerRef.current = onEvent

  useEffect(() => {
    const token = getAuthToken()
    const url = token ? `/v1/events?token=${encodeURIComponent(token)}` : '/v1/events'
    const es = new EventSource(url)

    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)

    es.onmessage = (msg) => {
      try {
        const parsed = JSON.parse(msg.data) as SSEEvent
        setEvents(prev => [...prev.slice(-99), parsed])
        handlerRef.current?.(parsed)
      } catch {
        // ignore malformed events
      }
    }

    return () => {
      es.close()
      setConnected(false)
    }
  }, [])

  return { connected, events }
}
