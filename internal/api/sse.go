package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

// Event 事件
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// EventBroadcaster 事件广播器
type EventBroadcaster struct {
	clients map[chan Event]struct{}
	mu      sync.RWMutex
}

// NewEventBroadcaster 创建广播器
func NewEventBroadcaster() *EventBroadcaster {
	return &EventBroadcaster{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe 订阅
func (b *EventBroadcaster) Subscribe() chan Event {
	ch := make(chan Event, 100)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅
func (b *EventBroadcaster) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Publish 发布事件
func (b *EventBroadcaster) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// 客户端太慢，跳过
		}
	}
}

// SSEHandler SSE 处理器
func SSEHandler(broadcaster *EventBroadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(ch)

		// 发送连接成功事件
		data, _ := json.Marshal(map[string]string{"type": "connected"})
		w.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(event)
				if err != nil {
					continue
				}
				w.Write([]byte("data: " + string(data) + "\n\n"))
				flusher.Flush()
			}
		}
	}
}

// 全局广播器
var globalBroadcaster = NewEventBroadcaster()

// GetBroadcaster 获取全局广播器
func GetBroadcaster() *EventBroadcaster {
	return globalBroadcaster
}

// BroadcastEvent 广播事件
func BroadcastEvent(eventType string, data interface{}) {
	globalBroadcaster.Publish(Event{
		Type: eventType,
		Data: data,
	})
}
