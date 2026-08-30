package agent

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// EventType 事件类型
type EventType string

const (
	EventFileCreated    EventType = "file.created"
	EventFileModified   EventType = "file.modified"
	EventFileDeleted    EventType = "file.deleted"
	EventTaskStarted    EventType = "task.started"
	EventTaskFailed     EventType = "task.failed"
	EventTaskCompleted  EventType = "task.completed"
	EventScheduleTrigger EventType = "schedule.triggered"
	EventWebhookReceived EventType = "webhook.received"
	EventSystemAlert    EventType = "system.alert"
)

// Event 事件
type Event struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   string    `json:"payload"`
	Priority  int       `json:"priority"` // 1-10，10 最高
}

// EventHandler 事件处理器
type EventHandler func(event Event) error

// EventBus 事件总线
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
	db       *sql.DB
}

// NewEventBus 创建事件总线
func NewEventBus(db *sql.DB) *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
		db:       db,
	}
}

// Subscribe 订阅事件
func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Emit 发射事件（同步）
func (b *EventBus) Emit(event Event) error {
	if event.ID == "" {
		event.ID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Priority == 0 {
		event.Priority = 5
	}

	// 保存到数据库
	if b.db != nil {
		b.db.Exec(`INSERT INTO events (id, source, event_type, timestamp, payload, priority) VALUES (?, ?, ?, ?, ?, ?)`,
			event.ID, event.Source, event.Type, event.Timestamp, event.Payload, event.Priority)
	}

	// 通知所有订阅者
	b.mu.RLock()
	handlers := b.handlers[event.Type]
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(event); err != nil {
			fmt.Printf("EventBus handler error: %v\n", err)
		}
	}
	return nil
}

// EmitAsync 异步发射事件
func (b *EventBus) EmitAsync(event Event) {
	go func() {
		b.Emit(event)
	}()
}

// GetRecent 获取最近 N 个事件
func (b *EventBus) GetRecent(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := b.db.Query(`
		SELECT id, source, event_type, timestamp, payload, priority
		FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		rows.Scan(&e.ID, &e.Source, &e.Type, &e.Timestamp, &e.Payload, &e.Priority)
		events = append(events, e)
	}
	return events, nil
}

// ClearOld 清理旧事件（保留最近 N 天）
func (b *EventBus) ClearOld(keepDays int) (int, error) {
	if keepDays <= 0 {
		keepDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	result, err := b.db.Exec("DELETE FROM events WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
