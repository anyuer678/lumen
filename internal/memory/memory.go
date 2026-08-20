package memory

import (
	"database/sql"
	"fmt"
	"time"
)

// MemoryType 记忆类型
type MemoryType string

const (
	MemoryShortTerm  MemoryType = "short_term"  // 短期记忆（任务内）
	MemoryWorking    MemoryType = "working"      // 工作记忆（恢复点）
	MemoryLongTerm   MemoryType = "long_term"    // 长期记忆（全局）
)

// Memory 记忆
type Memory struct {
	ID         string     `json:"id"`
	Type       MemoryType `json:"type"`
	Content    string     `json:"content"`
	Tags       string     `json:"tags,omitempty"`
	SourceTask string     `json:"source_task,omitempty"`
	Confirmed  bool       `json:"confirmed"`
	Importance int        `json:"importance"` // 1-10，10 最重要
	Date       string     `json:"date"`       // 日期分组 (YYYY-MM-DD)
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Store 记忆存储
type Store struct {
	db *sql.DB
}

// NewStore 创建存储
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Save 保存记忆
func (s *Store) Save(mem *Memory) error {
	if mem.Date == "" {
		mem.Date = time.Now().Format("2006-01-02")
	}
	if mem.Importance == 0 {
		mem.Importance = 5
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO memories 
		(id, kind, content, tags, source_task, confirmed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		mem.ID, mem.Type, mem.Content, mem.Tags, mem.SourceTask, mem.Confirmed, mem.CreatedAt, mem.UpdatedAt,
	)
	return err
}

// Get 获取记忆
func (s *Store) Get(id string) (*Memory, error) {
	mem := &Memory{}
	err := s.db.QueryRow(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE id = ?`, id).Scan(
		&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query memory: %w", err)
	}
	return mem, nil
}

// ListByType 按类型列出记忆
func (s *Store) ListByType(memType MemoryType, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE kind = ? ORDER BY created_at DESC LIMIT ?`, memType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(
			&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt,
		); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// Search 搜索记忆（简化版：关键词匹配）
func (s *Store) Search(keyword string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE content LIKE ? ORDER BY created_at DESC LIMIT ?`,
		"%"+keyword+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(
			&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt,
		); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// Confirm 确认记忆
func (s *Store) Confirm(id string) error {
	_, err := s.db.Exec("UPDATE memories SET confirmed = 1, updated_at = ? WHERE id = ?", time.Now(), id)
	return err
}

// Delete 删除记忆
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec("DELETE FROM memories WHERE id = ?", id)
	return err
}

// GetByDate 按日期获取记忆
func (s *Store) GetByDate(date string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE created_at >= ? AND created_at < ?
		ORDER BY created_at DESC LIMIT ?`,
		date, date+"T23:59:59", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// GetRecentByType 按类型获取最近 N 条
func (s *Store) GetRecentByType(memType MemoryType, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE kind = ? ORDER BY created_at DESC LIMIT ?`, memType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// GetRecent 按时间获取最近 N 条（不管类型）
func (s *Store) GetRecent(limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

// GetCount 获取记忆总数
func (s *Store) GetCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	return count, err
}

// GetCountByType 按类型获取记忆数
func (s *Store) GetCountByType(memType MemoryType) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM memories WHERE kind = ?", memType).Scan(&count)
	return count, err
}

// SearchByTags 按标签搜索记忆
func (s *Store) SearchByTags(tags string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, kind, content, tags, source_task, confirmed, created_at, updated_at
		FROM memories WHERE tags LIKE ? ORDER BY created_at DESC LIMIT ?`,
		"%"+tags+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []*Memory
	for rows.Next() {
		mem := &Memory{}
		if err := rows.Scan(&mem.ID, &mem.Type, &mem.Content, &mem.Tags, &mem.SourceTask, &mem.Confirmed, &mem.CreatedAt, &mem.UpdatedAt); err != nil {
			return nil, err
		}
		memories = append(memories, mem)
	}
	return memories, nil
}
