import { useEffect, useRef, useState } from 'react'

interface SSEEvent {
  type: string
  data: unknown
}

/**
 * SSE hook：订阅服务端事件流
 * 生产环境直接访问 /v1/events
 */
export function useSSE(onEvent?: (event: SSEEvent) => void) {
  const [connected, setConnected] = useState(false)
  const [events, setEvents] = useState<SSEEvent[]>([])
  const handlerRef = useRef(onEvent)
  handlerRef.current = onEvent

  useEffect(() => {
    const es = new EventSource('/v1/sse')

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
