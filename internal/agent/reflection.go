package agent

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"agent/internal/memory"
)

// UserProfile 用户画像（从记忆中自动提炼）
type UserProfile struct {
	ID         string    `json:"id"`
	Category   string    `json:"category"`   // preference/habit/skill/info
	Content    string    `json:"content"`     // 画像内容
	Confidence float64   `json:"confidence"`  // 置信度 0-1
	Source     string    `json:"source"`      // 来源（reflection/chat/task）
	Count      int       `json:"count"`       // 支撑证据数量
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ReflectionEngine 记忆反思引擎：从原始记忆中提炼用户画像
type ReflectionEngine struct {
	db        *sql.DB
	store     *memory.Store
	feedback  *FeedbackStore
}

// NewReflectionEngine 创建反思引擎
func NewReflectionEngine(db *sql.DB, memStore *memory.Store, fbStore *FeedbackStore) *ReflectionEngine {
	return &ReflectionEngine{
		db:       db,
		store:    memStore,
		feedback: fbStore,
	}
}

// InitSchema 初始化用户画像表
func (e *ReflectionEngine) InitSchema() error {
	_, err := e.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_profiles (
			id TEXT PRIMARY KEY,
			category TEXT,
			content TEXT,
			confidence REAL,
			source TEXT,
			count INTEGER DEFAULT 1,
			created_at DATETIME,
			updated_at DATETIME
		)
	`)
	return err
}

// Reflect 执行一次反思：扫描最近的记忆，提炼用户画像
func (e *ReflectionEngine) Reflect() ([]*UserProfile, error) {
	// 1. 获取最近的记忆
	mems, err := e.store.GetRecent(100)
	if err != nil {
		return nil, fmt.Errorf("get recent memories: %w", err)
	}

	if len(mems) == 0 {
		return nil, nil
	}

	// 2. 提炼模式
	var profiles []*UserProfile

	// 2.1 提取偏好（从"我喜欢/偏好/习惯"类记忆中）
	preferences := extractPreferences(mems)
	for _, pref := range preferences {
		profile := &UserProfile{
			ID:         fmt.Sprintf("pref-%d", hashString(pref)),
			Category:   "preference",
			Content:    pref,
			Confidence: 0.7,
			Source:     "reflection",
			Count:      1,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		profiles = append(profiles, profile)
	}

	// 2.2 提取技能（从任务完成记忆中）
	skills := extractSkills(mems)
	for _, skill := range skills {
		profile := &UserProfile{
			ID:         fmt.Sprintf("skill-%d", hashString(skill)),
			Category:   "skill",
			Content:    skill,
			Confidence: 0.6,
			Source:     "reflection",
			Count:      1,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		profiles = append(profiles, profile)
	}

	// 2.3 提取习惯（从反馈统计中）
	if e.feedback != nil {
		habits := e.extractHabits()
		for _, habit := range habits {
			profile := &UserProfile{
				ID:         fmt.Sprintf("habit-%d", hashString(habit)),
				Category:   "habit",
				Content:    habit,
				Confidence: 0.8,
				Source:     "reflection",
				Count:      1,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			profiles = append(profiles, profile)
		}
	}

	// 3. 保存到数据库（合并已有的）
	for _, p := range profiles {
		e.saveOrUpdateProfile(p)
	}

	return profiles, nil
}

// saveOrUpdateProfile 保存或更新用户画像（如果已存在则增加 count 和 confidence）
func (e *ReflectionEngine) saveOrUpdateProfile(p *UserProfile) {
	// 检查是否已存在
	var existingCount int
	err := e.db.QueryRow(`SELECT count FROM user_profiles WHERE id = ?`, p.ID).Scan(&existingCount)
	if err == nil {
		// 已存在，更新
		newCount := existingCount + 1
		newConfidence := p.Confidence + 0.05*float64(newCount)
		if newConfidence > 0.95 {
			newConfidence = 0.95
		}
		e.db.Exec(`UPDATE user_profiles SET count = ?, confidence = ?, updated_at = ? WHERE id = ?`,
			newCount, newConfidence, time.Now(), p.ID)
		return
	}

	// 新建
	e.db.Exec(`INSERT INTO user_profiles (id, category, content, confidence, source, count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Category, p.Content, p.Confidence, p.Source, p.Count, p.CreatedAt, p.UpdatedAt)
}

// GetProfiles 获取所有用户画像
func (e *ReflectionEngine) GetProfiles() ([]*UserProfile, error) {
	rows, err := e.db.Query(`
		SELECT id, category, content, confidence, source, count, created_at, updated_at
		FROM user_profiles ORDER BY confidence DESC, count DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*UserProfile
	for rows.Next() {
		p := &UserProfile{}
		if err := rows.Scan(&p.ID, &p.Category, &p.Content, &p.Confidence, &p.Source, &p.Count, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// GetProfilesAsHint 生成画像提示（注入 Planner）
func (e *ReflectionEngine) GetProfilesAsHint() string {
	profiles, err := e.GetProfiles()
	if err != nil || len(profiles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[用户画像] ")
	for i, p := range profiles {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%s(%.0f%%)", p.Content, p.Confidence*100))
		if i < len(profiles)-1 && i < 4 {
			sb.WriteString("、")
		}
	}
	return sb.String()
}

// extractPreferences 从记忆中提取偏好
func extractPreferences(mems []*memory.Memory) []string {
	var prefs []string
	seen := make(map[string]bool)

	keywords := []string{"喜欢", "偏好", "习惯", "常用", "偏好用", "prefer", "like", "favorite"}

	for _, m := range mems {
		content := m.Content
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				pref := extractSentence(content, kw)
				if pref != "" && !seen[pref] {
					seen[pref] = true
					prefs = append(prefs, pref)
				}
				break
			}
		}
	}
	return prefs
}

// extractSkills 从记忆中提取技能
func extractSkills(mems []*memory.Memory) []string {
	var skills []string
	seen := make(map[string]bool)

	keywords := []string{"任务执行成功", "成功完成", "已完成"}

	for _, m := range mems {
		content := m.Content
		for _, kw := range keywords {
			if strings.Contains(content, kw) {
				skill := extractAfterKeyword(content, "任务执行成功")
				if skill == "" {
					skill = extractAfterKeyword(content, "已完成")
				}
				if skill != "" && !seen[skill] {
					seen[skill] = true
					skills = append(skills, skill)
				}
				break
			}
		}
	}
	return skills
}

// extractHabits 从反馈统计中提取习惯
func (e *ReflectionEngine) extractHabits() []string {
	var habits []string

	// 获取成功模式
	patterns := e.feedback.GetSuccessPatterns()
	for _, p := range patterns {
		if p.Rate > 80 && p.Count >= 3 {
			habits = append(habits, fmt.Sprintf("常使用 %s 处理 %s 类任务（成功率 %.0f%%）", p.Tool, p.Category, p.Rate))
		}
	}

	// 获取常见错误（避免的习惯）
	errors := e.feedback.GetCommonErrors(3)
	for _, err := range errors {
		if err.Count >= 3 {
			habits = append(habits, fmt.Sprintf("需注意 %s 类错误（出现 %d 次）", err.ErrorType, err.Count))
		}
	}

	return habits
}

// extractSentence 从文本中提取包含关键词的句子
func extractSentence(text, keyword string) string {
	idx := strings.Index(text, keyword)
	if idx == -1 {
		return ""
	}

	// 将字节偏移转换为 rune 偏移
	runeIdx := utf8.RuneCountInString(text[:idx])
	runes := []rune(text)

	// 找到句子的开始（向前找句号/换行）
	start := runeIdx
	for i := runeIdx - 1; i >= 0; i-- {
		c := runes[i]
		if c == '。' || c == '\n' || c == '！' || c == '？' {
			start = i + 1
			break
		}
	}

	// 找到句子的结束（向后找句号/换行）
	keyRunes := utf8.RuneCountInString(keyword)
	end := runeIdx + keyRunes
	for i := runeIdx + keyRunes; i < len(runes); i++ {
		c := runes[i]
		if c == '。' || c == '\n' || c == '！' || c == '？' {
			end = i + 1
			break
		}
	}
	if end == runeIdx+keyRunes {
		end = len(runes)
	}

	if start >= len(runes) || end > len(runes) || start >= end {
		return ""
	}

	sentence := string(runes[start:end])
	sentence = strings.TrimSpace(sentence)
	if utf8.RuneCountInString(sentence) > 100 {
		runes = []rune(sentence)
		sentence = string(runes[:100]) + "..."
	}
	return sentence
}

// extractAfterKeyword 提取关键词后面的内容
func extractAfterKeyword(text, keyword string) string {
	idx := strings.Index(text, keyword)
	if idx == -1 {
		return ""
	}
	after := text[idx+len(keyword):]
	after = strings.TrimLeft(after, "：: 的")
	// 截断到下一个句号/换行
	for i, c := range after {
		if c == '。' || c == '\n' || c == '！' || c == '？' {
			return strings.TrimSpace(after[:i])
		}
	}
	if len(after) > 100 {
		after = after[:100] + "..."
	}
	return strings.TrimSpace(after)
}

// hashString 简单哈希（用于生成 ID）
func hashString(s string) int {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}
