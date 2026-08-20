package agent

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// MemoryScore 记忆质量评分
type MemoryScore struct {
	ID          string  `json:"id"`
	Content     string  `json:"content"`
	Importance  float64 `json:"importance"`  // 重要性 0-1
	Frequency   float64 `json:"frequency"`   // 访问频率 0-1
	Recency     float64 `json:"recency"`     // 时效性 0-1
	Confidence  float64 `json:"confidence"`  // 置信度 0-1
	TotalScore  float64 `json:"total_score"` // 综合得分 0-1
	Lifecycle   string  `json:"lifecycle"`   // active/consolidated/archived
}

// MemoryScorer 记忆质量评分器
type MemoryScorer struct {
	db *sql.DB
}

// NewMemoryScorer 创建评分器
func NewMemoryScorer(db *sql.DB) *MemoryScorer {
	return &MemoryScorer{db: db}
}

// InitSchema 初始化评分字段
func (s *MemoryScorer) InitSchema() error {
	// 保证 quality_score 列存在
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('memories') WHERE name='quality_score'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.db.Exec(`ALTER TABLE memories ADD COLUMN quality_score REAL DEFAULT 0.5`); err != nil {
			return err
		}
	}
	// 保证 importance 列存在（Memory struct 有该字段但旧库没建列）
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('memories') WHERE name='importance'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := s.db.Exec(`ALTER TABLE memories ADD COLUMN importance INTEGER DEFAULT 5`); err != nil {
			return err
		}
	}
	return nil
}

// ScoreMemory 计算单条记忆的质量得分
// 公式: importance × 0.4 + frequency × 0.3 + recency × 0.2 + confidence × 0.1
func (s *MemoryScorer) ScoreMemory(mem *MemoryScore) float64 {
	// 频率归一化（0-10 次 → 0-1）
	freq := mem.Frequency / 10.0
	if freq > 1.0 {
		freq = 1.0
	}

	// 时效性：最近访问越近分越高（30 天内满分，90 天后衰减）
	recency := mem.Recency

	total := mem.Importance*0.4 + freq*0.3 + recency*0.2 + mem.Confidence*0.1
	return math.Min(total, 1.0)
}

// ScoreAll 计算所有记忆的得分并更新数据库（先读后写，避免读写锁冲突）
func (s *MemoryScorer) ScoreAll() (int, error) {
	now := time.Now()

	// 1. 先读取所有记忆并计算得分（读取完成后立刻关闭 rows）
	type scoreEntry struct {
		id    string
		score float64
	}
	var entries []scoreEntry

	rows, err := s.db.Query(`
		SELECT id, importance, access_count, last_accessed, created_at, lifecycle
		FROM memories WHERE lifecycle != 'forgotten'`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id string
		var importance int
		var accessCount int
		var lastAccessed sql.NullTime
		var createdAt time.Time
		var lifecycle string
		if err := rows.Scan(&id, &importance, &accessCount, &lastAccessed, &createdAt, &lifecycle); err != nil {
			continue // 跳过扫描失败的行
		}

		impScore := float64(importance) / 10.0
		freqScore := float64(accessCount) / 10.0
		if freqScore > 1.0 {
			freqScore = 1.0
		}

		var recScore float64
		if lastAccessed.Valid {
			daysSince := now.Sub(lastAccessed.Time).Hours() / 24
			recScore = math.Max(0, 1.0-daysSince/90.0)
		} else {
			daysSince := now.Sub(createdAt).Hours() / 24
			recScore = math.Max(0, 1.0-daysSince/90.0)
		}

		var confScore float64
		switch lifecycle {
		case "active":
			confScore = 0.8
		case "consolidated":
			confScore = 0.5
		case "archived":
			confScore = 0.3
		default:
			confScore = 0.6
		}

		entries = append(entries, scoreEntry{
			id:    id,
			score: impScore*0.4 + freqScore*0.3 + recScore*0.2 + confScore*0.1,
		})
	}
	// 关闭读取连接，释放读锁，再开启写事务
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// 2. 事务批量更新
	if len(entries) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE memories SET quality_score = ? WHERE id = ?`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.score, e.id); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(entries), nil
}

// GetLowQualityMemories 获取低质量记忆（得分低于阈值）
func (s *MemoryScorer) GetLowQualityMemories(threshold float64, limit int) ([]MemoryScore, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, content, quality_score, lifecycle
		FROM memories
		WHERE quality_score < ? AND lifecycle != 'forgotten'
		ORDER BY quality_score ASC
		LIMIT ?`, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mems []MemoryScore
	for rows.Next() {
		var m MemoryScore
		if err := rows.Scan(&m.ID, &m.Content, &m.TotalScore, &m.Lifecycle); err != nil {
			continue
		}
		mems = append(mems, m)
	}
	return mems, nil
}

// GetTopQualityMemories 获取高质量记忆（用于 Planner 注入）
func (s *MemoryScorer) GetTopQualityMemories(limit int) ([]MemoryScore, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(`
		SELECT id, content, quality_score, lifecycle
		FROM memories
		WHERE lifecycle != 'forgotten'
		ORDER BY quality_score DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mems []MemoryScore
	for rows.Next() {
		var m MemoryScore
		if err := rows.Scan(&m.ID, &m.Content, &m.TotalScore, &m.Lifecycle); err != nil {
			continue
		}
		mems = append(mems, m)
	}
	return mems, nil
}

// GetScoreHint 生成记忆质量提示（注入 Planner）
func (s *MemoryScorer) GetScoreHint() string {
	mems, err := s.GetTopQualityMemories(5)
	if err != nil || len(mems) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[高质量记忆] ")
	for i, m := range mems {
		if i >= 3 {
			break
		}
		sb.WriteString(fmt.Sprintf("%s(%.0f%%)", truncateScoreContent(m.Content, 30), m.TotalScore*100))
		if i < min(len(mems)-1, 2) {
			sb.WriteString("、")
		}
	}
	return sb.String()
}

func truncateScoreContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
