package agent

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// MemoryLifecycle 记忆生命周期管理器
// 记忆状态流转：Created → Active → Consolidated → Archived → Forgotten
type MemoryLifecycle struct {
	db *sql.DB
}

// NewMemoryLifecycle 创建生命周期管理器
func NewMemoryLifecycle(db *sql.DB) *MemoryLifecycle {
	return &MemoryLifecycle{db: db}
}

// InitSchema 初始化 lifecycle 相关列
func (m *MemoryLifecycle) InitSchema() error {
	cols := []struct{ name, ddl string }{
		{"lifecycle", "TEXT DEFAULT 'active'"},
		{"access_count", "INTEGER DEFAULT 0"},
		{"last_accessed", "DATETIME"},
	}
	for _, c := range cols {
		var count int
		if err := m.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('memories') WHERE name=?`, c.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := m.db.Exec(`ALTER TABLE memories ADD COLUMN ` + c.name + ` ` + c.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// LifecycleStats 生命周期统计
type LifecycleStats struct {
	Total         int `json:"total"`
	Active        int `json:"active"`
	Consolidated  int `json:"consolidated"`
	Archived      int `json:"archived"`
	Forgotten     int `json:"forgotten"`
}

// GetStats 获取各状态的记忆数量
func (m *MemoryLifecycle) GetStats() (*LifecycleStats, error) {
	stats := &LifecycleStats{}

	rows, err := m.db.Query(`SELECT lifecycle, COUNT(*) FROM memories GROUP BY lifecycle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var lifecycle string
		var count int
		rows.Scan(&lifecycle, &count)
		stats.Total += count
		switch lifecycle {
		case "active", "":
			stats.Active += count
		case "consolidated":
			stats.Consolidated += count
		case "archived":
			stats.Archived += count
		case "forgotten":
			stats.Forgotten += count
		}
	}
	return stats, nil
}

// RunLifecycle 执行一次生命周期检查
// 规则：
//   active → 30 天未访问 → consolidated
//   consolidated → 90 天未访问 → archived
//   archived → 180 天未访问 → forgotten
func (m *MemoryLifecycle) RunLifecycle() (int, error) {
	now := time.Now()
	totalChanged := 0
	var firstErr error

	// 1. active → consolidated（30 天未访问）
	cutoff30 := now.AddDate(0, 0, -30)
	result, err := m.db.Exec(`
		UPDATE memories SET lifecycle = 'consolidated'
		WHERE (lifecycle = 'active' OR lifecycle = '' OR lifecycle IS NULL)
		AND (last_accessed < ? OR (last_accessed IS NULL AND created_at < ?))
		AND id NOT LIKE 'm-bench-%'`, // 保护 benchmark 记忆
		cutoff30, cutoff30)
	if err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("consolidate: %w", err)
		}
	} else if n, e2 := result.RowsAffected(); e2 == nil {
		totalChanged += int(n)
	}

	// 2. consolidated → archived（90 天未访问）
	cutoff90 := now.AddDate(0, 0, -90)
	result, err = m.db.Exec(`
		UPDATE memories SET lifecycle = 'archived'
		WHERE lifecycle = 'consolidated'
		AND (last_accessed < ? OR (last_accessed IS NULL AND created_at < ?))`,
		cutoff90, cutoff90)
	if err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("archive: %w", err)
		}
	} else if n, e2 := result.RowsAffected(); e2 == nil {
		totalChanged += int(n)
	}

	// 3. archived → forgotten（180 天未访问）
	cutoff180 := now.AddDate(0, 0, -180)
	result, err = m.db.Exec(`
		UPDATE memories SET lifecycle = 'forgotten'
		WHERE lifecycle = 'archived'
		AND (last_accessed < ? OR (last_accessed IS NULL AND created_at < ?))`,
		cutoff180, cutoff180)
	if err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("forget: %w", err)
		}
	} else if n, e2 := result.RowsAffected(); e2 == nil {
		totalChanged += int(n)
	}

	return totalChanged, firstErr
}

// TouchMemory 记忆被访问时调用（更新 access_count 和 last_accessed）
func (m *MemoryLifecycle) TouchMemory(id string) {
	_, _ = m.db.Exec(`UPDATE memories SET access_count = access_count + 1, last_accessed = ? WHERE id = ?`,
		time.Now(), id)
}

// PromoteMemory 手动提升记忆优先级（如用户确认重要信息）
func (m *MemoryLifecycle) PromoteMemory(id string) {
	_, _ = m.db.Exec(`UPDATE memories SET lifecycle = 'active', access_count = access_count + 1, last_accessed = ? WHERE id = ?`,
		time.Now(), id)
}

// GetActiveMemories 获取活跃记忆（用于 Planner 注入）
func (m *MemoryLifecycle) GetActiveMemories(limit int) ([]struct {
	ID      string
	Content string
	Tags    string
}, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.db.Query(`
		SELECT id, content, tags FROM memories
		WHERE lifecycle = 'active' OR lifecycle = '' OR lifecycle IS NULL
		ORDER BY importance DESC, created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mems []struct {
		ID      string
		Content string
		Tags    string
	}
	for rows.Next() {
		var m struct {
			ID      string
			Content string
			Tags    string
		}
		rows.Scan(&m.ID, &m.Content, &m.Tags)
		mems = append(mems, m)
	}
	return mems, nil
}

// GetMemoryHint 获取记忆提示（注入 Planner）
func (m *MemoryLifecycle) GetMemoryHint() string {
	mems, err := m.GetActiveMemories(10)
	if err != nil || len(mems) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[活跃记忆] ")
	for i, mem := range mems {
		if i >= 5 {
			break
		}
		sb.WriteString(mem.Content)
		if i < len(mems)-1 && i < 4 {
			sb.WriteString("；")
		}
	}
	return sb.String()
}
