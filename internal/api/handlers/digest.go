package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"agent/internal/agent"
	"agent/internal/memory"
)

// DigestHandler Daily Digest 处理器
type DigestHandler struct {
	db    *sql.DB
	store *memory.Store
}

// NewDigestHandler 创建处理器
func NewDigestHandler(db *sql.DB) *DigestHandler {
	return &DigestHandler{db: db, store: memory.NewStore(db)}
}

// Routes 注册路由
func (h *DigestHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/generate", h.Generate)
	r.Get("/today", h.GetToday)
	return r
}

// Generate 生成今日摘要
func (h *DigestHandler) Generate(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Daily Digest\n\n", today))

	// 获取最近记忆
	mems, _ := h.store.GetRecent(20)
	sb.WriteString(fmt.Sprintf("### 最近记忆 (%d 条)\n", len(mems)))
	for _, m := range mems {
		content := m.Content
		if len(content) > 60 {
			content = content[:60] + "..."
		}
		sb.WriteString(fmt.Sprintf("- %s: %s\n", m.Type, content))
	}

	// 获取事件
	eventBus := agent.NewEventBus(h.db)
	events, _ := eventBus.GetRecent(10)
	sb.WriteString(fmt.Sprintf("\n### 最近事件 (%d 条)\n", len(events)))
	for _, e := range events {
		payload := e.Payload
		if len(payload) > 60 {
			payload = payload[:60] + "..."
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", e.Type, e.Source, payload))
	}

	// 统计
	memCount, _ := h.store.GetCount()
	sb.WriteString(fmt.Sprintf("\n### 统计\n- 总记忆数: %d\n", memCount))

	digest := sb.String()

	// 保存到知识库
	kb := memory.NewKBStore(h.db)
	if kb != nil {
		kb.Add(&memory.Knowledge{
			ID:        fmt.Sprintf("digest-%s", today),
			Title:     fmt.Sprintf("Daily Digest: %s", today),
			Content:   digest,
			Tags:      "digest,daily",
			Source:    "proactive",
			CreatedAt: time.Now(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"digest": digest, "date": today})
}

// GetToday 获取今日摘要
func (h *DigestHandler) GetToday(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	var content string
	err := h.db.QueryRow(`SELECT content FROM knowledge WHERE title LIKE ? AND source='proactive' ORDER BY created_at DESC LIMIT 1`,
		"%"+today+"%").Scan(&content)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"digest": "", "date": today, "message": "今日摘要尚未生成"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"digest": content, "date": today})
}
