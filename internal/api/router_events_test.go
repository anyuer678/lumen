package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	agentDB "agent/internal/db"
)

// 回归锁：/v1/events 必须留给 SSE 事件流，其下不允许挂任何子路由。
//
// 背景（2026-09-23 实测确认）：chi 的 Mux.Mount() 会用 mALL|mSTUB 注册 pattern
// 本身以及 pattern+"/*"，并**静默覆盖**此前注册在同一 pattern 上的路由，
// 且 Mount 时所处 group 的中间件会一并生效。
//
// 原实现把 event-bus 子路由挂在 /events，与 router.go 里
// `r.Get("/events", SSEHandler(...))` 撞车，结果是：
//   - GET /v1/events 返回 application/json（或 403，因为该 group 带
//     RequireScope("events:emit")），而不是 text/event-stream；
//   - 前端 useSSE() 的 EventSource 永远连不上 → 侧栏恒显「连接断开」、无实时推送；
//   - 非 admin token（DefaultTokenScopes 不含 events:emit）读事件列表直接 403。
//
// 这与 DESIGN.md / DEPLOY.md 记录的「GET /v1/events → SSE 实时推送」矛盾。
// 因此：任何在 /v1/events 下的 Mount 都会让本测试失败。
func TestNoSubrouterMountedUnderEvents(t *testing.T) {
	db, err := agentDB.Init(filepath.Join(t.TempDir(), "router-events.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	r := NewRouter(nil, nil, db, nil, nil, nil, zap.NewNop())

	var underEvents []string
	sawSSE := false

	if err := chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if route == "/v1/events" {
			sawSSE = true
		}
		if strings.HasPrefix(route, "/v1/events/") {
			underEvents = append(underEvents, method+" "+route)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	if !sawSSE {
		t.Fatalf("GET /v1/events 未注册：SSE 事件流端点缺失")
	}
	if len(underEvents) > 0 {
		t.Fatalf("/v1/events 下不允许挂任何子路由（会静默覆盖 SSE 端点），发现 %v", underEvents)
	}
}
