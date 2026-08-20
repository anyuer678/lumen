package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Knowledge 知识条目
type Knowledge struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      string    `json:"tags"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// KBStore 知识库存储
type KBStore struct {
	db *sql.DB
}

// NewKBStore 创建知识库存储
func NewKBStore(db *sql.DB) *KBStore {
	return &KBStore{db: db}
}

// Add 添加知识
func (s *KBStore) Add(k *Knowledge) error {
	_, err := s.db.Exec(
		`INSERT INTO knowledge (id, title, content, tags, source, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.Title, k.Content, k.Tags, k.Source, k.CreatedAt)
	return err
}

// List 列出知识
func (s *KBStore) List(limit int) ([]*Knowledge, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, title, content, tags, source, created_at FROM knowledge ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*Knowledge
	for rows.Next() {
		k := &Knowledge{}
		if err := rows.Scan(&k.ID, &k.Title, &k.Content, &k.Tags, &k.Source, &k.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, k)
	}
	if items == nil {
		items = []*Knowledge{}
	}
	return items, nil
}

// Search 关键词检索知识库
func (s *KBStore) Search(query string, limit int) ([]*Knowledge, error) {
	if limit <= 0 {
		limit = 5
	}
	// 提取检索关键词（去掉常见虚词 + 中文 n-gram）
	keywords := extractKeywords(query)

	rows, err := s.db.Query(`SELECT id, title, content, tags, source, created_at FROM knowledge`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		k     *Knowledge
		score int
	}
	var results []scored

	for rows.Next() {
		k := &Knowledge{}
		if err := rows.Scan(&k.ID, &k.Title, &k.Content, &k.Tags, &k.Source, &k.CreatedAt); err != nil {
			continue
		}
		text := k.Title + " " + k.Content + " " + k.Tags
		score := 0
		// 整句匹配最高分
		if strings.Contains(text, query) {
			score += 10
		}
		for _, kw := range keywords {
			score += strings.Count(text, kw) * 2
		}
		if score > 0 {
			results = append(results, scored{k: k, score: score})
		}
	}

	// 按分数排序
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	var items []*Knowledge
	for i := 0; i < len(results) && i < limit; i++ {
		items = append(items, results[i].k)
	}
	if items == nil {
		items = []*Knowledge{}
	}
	return items, nil
}

// extractKeywords 提取中文检索关键词
func extractKeywords(query string) []string {
	// 去常见虚词
	stopwords := []string{"的", "是", "什么", "时候", "我的", "你的", "怎么", "为", "吗", "呢", "啊", "做", "帮", "请", "跟", "给", "一个", "一下"}
	q := query
	for _, sw := range stopwords {
		q = strings.ReplaceAll(q, sw, "")
	}
	q = strings.TrimSpace(q)

	var keywords []string
	runes := []rune(q)

	// 整词（去掉虚词后剩余的非空部分）
	if len(runes) > 0 {
		keywords = append(keywords, string(runes))
	}

	// 中文双字 n-gram
	if len(runes) >= 2 {
		for i := 0; i < len(runes)-1; i++ {
			gram := string(runes[i : i+2])
			if len(gram) > 0 && !containsStr(keywords, gram) {
				keywords = append(keywords, gram)
			}
		}
	}

	// 单字（用于英文单词和短中文词）
	return keywords
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Delete 删除知识
func (s *KBStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM knowledge WHERE id = ?`, id)
	return err
}

// Count 数量
func (s *KBStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM knowledge`).Scan(&count)
	return count, err
}

// tokenize 简单分词
func tokenize(s string) []string {
	s = strings.ToLower(s)
	// 去标点，按空白和中英混排切分
	repl := strings.NewReplacer(",", " ", "，", " ", ".", " ", "。", " ", "?", " ", "？", "!", " ", "！", "(", " ", ")", " ", ";", " ", "；", ":", " ", "：", "\"", " ", "'", " ")
	s = repl.Replace(s)
	parts := strings.Fields(s)
	var tokens []string
	for _, p := range parts {
		tokens = append(tokens, p)
	}
	return tokens
}

var _ = fmt.Sprintf
