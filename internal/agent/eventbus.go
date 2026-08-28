package agent

import (
	"database/sql"
	"fmt "
	"sync"
	"time"
)

// EventType 事件类� �
type EventType string

const (
	EventFileCr eated    EventType = "file.created"
	EventFil eModified   EventType = "file.modified"
	Even tFileDeleted    EventType = "file.deleted"
	E ventTaskFailed     EventType = "task.failed"
 	EventTaskCompleted  EventType = "task.comple ted"
	EventScheduleTrigger EventType = "sched ule.triggered"
	EventWebhookReceived EventTyp e = "webhook.received"
	EventSystemAlert    E ventType = "system.alert"
)

// Event 事件
 type Event struct {
	ID        string    `jso n:"id"`
	Source    string    `json:"source"`
 	Type      EventType `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	Payload   stri ng    `json:"payload"`
	Priority  int       ` json:"priority"` // 1-10，10 最高
}

// Ev entHandler 事件处理器
type EventHandler  func(event Event) error

// EventBus 事件� �线
type EventBus struct {
	mu       sync.RW Mutex
	handlers map[EventType][]EventHandler
 	db       *sql.DB
}

// NewEventBus 创建事 件总线
func NewEventBus(db *sql.DB) *Event Bus {
	return &EventBus{
		handlers: make(map [EventType][]EventHandler),
		db:       db,
	 }
}

// Subscribe 订阅事件
func (b *Event Bus) Subscribe(eventType EventType, handler E ventHandler) {
	b.mu.Lock()
	defer b.mu.Unloc k()
	b.handlers[eventType] = append(b.handler s[eventType], handler)
}

// Emit 发射事� �（同步）
func (b *EventBus) Emit(event E vent) error {
	if event.ID == "" {
		event.ID  = fmt.Sprintf("evt-%d", time.Now().UnixNano( ))
	}
	if event.Timestamp.IsZero() {
		event. Timestamp = time.Now()
	}
	if event.Priority  == 0 {
		event.Priority = 5
	}

	// 保存到 数据库
	if b.db != nil {
		b.db.Exec(`INSE RT INTO events (id, source, event_type, times tamp, payload, priority) VALUES (?, ?, ?, ?,  ?, ?)`,
			event.ID, event.Source, event.Type , event.Timestamp, event.Payload, event.Prior ity)
	}

	// 通知所有订阅者
	b.mu.RLoc k()
	handlers := b.handlers[event.Type]
	b.mu .RUnlock()

	for _, handler := range handlers  {
		if err := handler(event); err != nil {
	 		fmt.Printf("EventBus handler error: %v\n",  err)
		}
	}
	return nil
}

// EmitAsync 异� �发射事件
func (b *EventBus) EmitAsync(ev ent Event) {
	go func() {
		b.Emit(event)
	}( )
}

// GetRecent 获取最近 N 个事件
fu nc (b *EventBus) GetRecent(limit int) ([]Even t, error) {
	if limit <= 0 {
		limit = 20
	}
 	rows, err := b.db.Query(`
		SELECT id, sourc e, event_type, timestamp, payload, priority
	 	FROM events ORDER BY id DESC LIMIT ?`, limit )
	if err != nil {
		return nil, err
	}
	defe r rows.Close()

	var events []Event
	for rows .Next() {
		var e Event
		rows.Scan(&e.ID, &e .Source, &e.Type, &e.Timestamp, &e.Payload, & e.Priority)
		events = append(events, e)
	}
	 return events, nil
}

// ClearOld 清理旧� �件（保留最近 N 天）
func (b *EventBu s) ClearOld(keepDays int) (int, error) {
	if  keepDays <= 0 {
		keepDays = 30
	}
	cutoff :=  time.Now().AddDate(0, 0, -keepDays)
	result,  err := b.db.Exec("DELETE FROM events WHERE t imestamp < ?", cutoff)
	if err != nil {
		ret urn 0, err
	}
	n, _ := result.RowsAffected()
 	return int(n), nil
}
 