package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 用 httptest mock GitHub API，验证 GitHubTool 的真实请求/解析/错误路径。
func newTestGitHubTool(baseURL, token string) *GitHubTool {
	return &GitHubTool{token: token, client: &http.Client{}, baseURL: baseURL}
}

func TestGitHubTool_ListRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/user/repos" {
			t.Errorf("path = %q", got)
		}
		w.Header().Set("X-RateLimit-Remaining", "50")
		json.NewEncoder(w).Encode([]map[string]any{
			{"name": "lumen", "language": "Go", "pushed_at": "2026-01-01", "open_issues_count": 3, "description": "agent runtime"},
			{"name": "kb-ui", "language": "Vue", "pushed_at": "2026-01-02", "open_issues_count": 0, "description": "component lib"},
		})
	}))
	defer srv.Close()

	tool := newTestGitHubTool(srv.URL, "test-token")
	res, err := tool.Execute(context.Background(), map[string]any{"action": "repos", "filter": "lumen"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Raw, "lumen") {
		t.Errorf("结果缺少过滤后的仓库 lumen: %s", res.Raw)
	}
	if strings.Contains(res.Raw, "kb-ui") {
		t.Errorf("filter=lumen 不应包含 kb-ui")
	}
}

func TestGitHubTool_RateLimitWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "3")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	tool := newTestGitHubTool(srv.URL, "t")
	res, err := tool.Execute(context.Background(), map[string]any{"action": "repos"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Raw, "即将耗尽") {
		t.Errorf("限流剩余 <10 应带即将耗尽警告: %s", res.Raw)
	}
}

func TestGitHubTool_NoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("无 token 时不应携带 Authorization 头")
		}
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	tool := newTestGitHubTool(srv.URL, "")
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "repos"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestGitHubTool_API500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	tool := newTestGitHubTool(srv.URL, "t")
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "repos"}); err == nil {
		t.Error("API 500 应返回错误")
	}
}

func TestGitHubTool_ActivityRequiresRepo(t *testing.T) {
	tool := newTestGitHubTool("http://unused.invalid", "t")
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "activity"}); err == nil {
		t.Error("activity 缺 repo 参数应报错")
	}
}
