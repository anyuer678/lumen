import { useEffect, useRef, useState, useCallback } from 'react'
import { getAuthTokenPublic as getAuthToken } from '../api/client'

interface SSEEvent {
  type: string
  data: unknown
}

/**
 * SSE hook：订阅服务端事件流 /v1/events，支持断线自动重连
 * EventSource 无法携带自定义头，token 通过 ?token= 查询参数传递；
 * 未配置 token（认证关闭）时直接连接。
 * 重连策略：指数退避 1s→2s→4s→8s→16s→30s（上限），成功后重置。
 */
export function useSSE(onEvent?: (event: SSEEvent) => void) {
  const [connected, setConnected] = useState(false)
  const [events, setEvents] = useState<SSEEvent[]>([])
  const handlerRef = useRef(onEvent)
  handlerRef.current = onEvent

  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const retryCountRef = useRef(0)
  const MAX_RETRY_DELAY = 30_000

  const cleanup = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current)
      reconnectTimerRef.current = null
    }
  }, [])

  useEffect(() => {
    let es: EventSource | null = null

    function connect() {
      const token = getAuthToken()
      const url = token ? `/v1/events?token=${encodeURIComponent(token)}` : '/v1/events'
      es = new EventSource(url)

      es.onopen = () => {
        setConnected(true)
        retryCountRef.current = 0 // 成功连接，重置退避
      }

      es.onerror = () => {
        setConnected(false)
        es?.close()
        // 指数退避重连
        const delay = Math.min(1000 * Math.pow(2, retryCountRef.current), MAX_RETRY_DELAY)
        retryCountRef.current++
        reconnectTimerRef.current = setTimeout(connect, delay)
      }

      es.onmessage = (msg) => {
        try {
          const parsed = JSON.parse(msg.data) as SSEEvent
          setEvents(prev => [...prev.slice(-99), parsed])
          handlerRef.current?.(parsed)
        } catch {
          // ignore malformed events
        }
      }
    }

    connect()

    return () => {
      cleanup()
      es?.close()
      setConnected(false)
    }
  }, [cleanup])

  return { connected, events }
}
