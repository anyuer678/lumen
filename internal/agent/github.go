package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// GitHubTool GitHub 集成工具：只读操作，查看仓库和活动
type GitHubTool struct {
	token  string
	client *http.Client
}

// NewGitHubTool 创建 GitHub 工具
func NewGitHubTool() *GitHubTool {
	return &GitHubTool{
		token:  os.Getenv("GITHUB_TOKEN"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *GitHubTool) Name() string        { return "github" }
func (t *GitHubTool) Description() string { return "GitHub 集成（只读）：仓库列表、活动查询" }
func (t *GitHubTool) RequiredLevel() int  { return 0 }

func (t *GitHubTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	action, _ := args["action"].(string)
	switch action {
	case "repos":
		return t.listRepos(args)
	case "activity":
		return t.getActivity(args)
	default:
		return nil, fmt.Errorf("github: 未知操作 %q，支持 repos / activity", action)
	}
}

// listRepos 列出用户仓库
func (t *GitHubTool) listRepos(args map[string]any) (*ToolResult, error) {
	filter, _ := args["filter"].(string)

	url := "https://api.github.com/user/repos?per_page=100&sort=updated"
	body, rateInfo, err := t.doGet(url)
	if err != nil {
		return nil, fmt.Errorf("github repos: %w", err)
	}

	var repos []struct {
		Name        string `json:"name"`
		Language    string `json:"language"`
		PushedAt    string `json:"pushed_at"`
		OpenIssues  int    `json:"open_issues_count"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("github repos: 解析失败: %w", err)
	}

	// 过滤
	if filter != "" {
		filter = strings.ToLower(filter)
		var filtered []struct {
			Name        string `json:"name"`
			Language    string `json:"language"`
			PushedAt    string `json:"pushed_at"`
			OpenIssues  int    `json:"open_issues_count"`
			Description string `json:"description"`
		}
		for _, r := range repos {
			if strings.Contains(strings.ToLower(r.Name), filter) ||
				strings.Contains(strings.ToLower(r.Description), filter) {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	result := map[string]any{
		"count": len(repos),
		"repos": repos,
	}
	if rateInfo != "" {
		result["rate_limit"] = rateInfo
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &ToolResult{
		Raw:     string(data),
		Kind:    "json",
		Summary: fmt.Sprintf("找到 %d 个仓库", len(repos)),
	}, nil
}

// getActivity 查询仓库最近活动
func (t *GitHubTool) getActivity(args map[string]any) (*ToolResult, error) {
	repo, _ := args["repo"].(string)
	days := 7
	if v, ok := args["days"].(float64); ok && v > 0 {
		days = int(v)
	}

	if repo == "" {
		return nil, fmt.Errorf("github activity: repo 参数必填（格式: owner/name）")
	}

	since := time.Now().AddDate(0, 0, -days).Format(time.RFC3339)

	// 查询 commits
	commitsURL := fmt.Sprintf("https://api.github.com/repos/%s/commits?since=%s&per_page=30", repo, since)
	commitsBody, _, err := t.doGet(commitsURL)
	if err != nil {
		return nil, fmt.Errorf("github activity commits: %w", err)
	}
	var commits []struct {
		Sha    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name  string `json:"name"`
				Date  string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	json.Unmarshal(commitsBody, &commits)

	// 查询 issues
	issuesURL := fmt.Sprintf("https://api.github.com/repos/%s/issues?since=%s&state=all&per_page=30", repo, since)
	issuesBody, _, err := t.doGet(issuesURL)
	if err != nil {
		return nil, fmt.Errorf("github activity issues: %w", err)
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
	}
	json.Unmarshal(issuesBody, &issues)

	commitsSummary := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		commitsSummary = append(commitsSummary, map[string]any{
			"sha":     c.Sha[:8],
			"message": c.Commit.Message,
			"author":  c.Commit.Author.Name,
			"date":    c.Commit.Author.Date,
		})
	}

	result := map[string]any{
		"repo":       repo,
		"days":       days,
		"commits":    len(commits),
		"issues":     len(issues),
		"commit_list": commitsSummary,
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return &ToolResult{
		Raw:     string(data),
		Kind:    "json",
		Summary: fmt.Sprintf("%s: %d commits, %d issues (最近 %d 天)", repo, len(commits), len(issues), days),
	}, nil
}

// doGet 执行 GET 请求，返回 body + rate limit 信息
func (t *GitHubTool) doGet(url string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// rate limit 信息
	rateInfo := ""
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	if remaining != "" {
		rateInfo = fmt.Sprintf("remaining=%s", remaining)
		var remainingInt int
		if _, scanErr := fmt.Sscanf(remaining, "%d", &remainingInt); scanErr == nil && remainingInt < 10 {
			rateInfo += " ⚠️ 即将耗尽"
		}
	}

	if resp.StatusCode == 403 {
		return nil, rateInfo, fmt.Errorf("GitHub API 限流（403）：%s", rateInfo)
	}
	if resp.StatusCode >= 500 {
		return nil, rateInfo, fmt.Errorf("GitHub 服务器错误（%d）", resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, rateInfo, fmt.Errorf("GitHub API 返回 %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}

	return body, rateInfo, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
