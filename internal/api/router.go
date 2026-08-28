package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"agent/internal/agent"
	"agent/internal/api/handlers"
	"agent/internal/auth"
	"agent/internal/config"
	"agent/internal/llm"
	"agent/internal/memory"
	"agent/internal/observability"
	"agent/internal/scheduler"
	"agent/internal/task"
)

var startTime = time.Now()
var taskManager *task.Manager
var logger *zap.Logger

func NewRouter(tm *task.Manager, sched *scheduler.Scheduler, db *sql.DB, mcpRegistry *agent.McpRegistry, agentLoop *agent.Loop, llmProvider llm.Provider, log *zap.Logger) *chi.Mux {
	taskManager = tm
	logger = log
	r := chi.NewRouter()

	// 中间件
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// API 路由（优先级最高）
	r.Route("/v1", func(r chi.Router) {
		// 除 /health 与 SSE 只读流外的所有端点要求 Bearer token
		if db != nil {
			verifier := auth.NewTokenVerifier(db)
			r.Use(tokenAuthMiddleware(verifier))
		}
		r.Get("/health", healthHandler)
		r.Get("/status", statusHandler)
		r.Get("/events", SSEHandler(GetBroadcaster()))

		// 任务端点
		taskHandler := handlers.NewTaskHandler(tm)
		r.Mount("/tasks", taskHandler.Routes())

		// 定时任务端点
		if sched != nil {
			jobHandler := handlers.NewJobHandler(sched)
			r.Mount("/jobs", jobHandler.Routes())

			// Webhook 触发器端点
			r.Post("/webhooks/{jobID}", func(w http.ResponseWriter, r *http.Request) {
				jobID := chi.URLParam(r, "jobID")
				var payload map[string]interface{}
				if r.Body != nil {
					json.NewDecoder(r.Body).Decode(&payload)
				}
				ok, msg := sched.TriggerByWebhook(jobID, payload)
				if !ok {
					http.Error(w, msg, http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"status": "triggered", "job": jobID, "message": msg})
			})
		}

		// 确认端点
		if db != nil {
			confirmHandler := handlers.NewConfirmHandler(auth.NewConfirmStore(db))
			r.Mount("/confirmations", confirmHandler.Routes())
		}

		// 设置端点
		settingsHandler := handlers.NewSettingsHandler()
		r.Mount("/settings", settingsHandler.Routes())

		// 产物端点（截图等 workspace 文件）
		if db != nil {
			artifactsHandler := handlers.NewArtifactsHandler("./data/workspace")
			r.Mount("/artifacts", artifactsHandler.Routes())
		}

		// 记忆端点
		if db != nil {
			memoryHandler := handlers.NewMemoryHandler(memory.NewStore(db))
			r.Mount("/memories", memoryHandler.Routes())
		}

		// 审计端点
		if db != nil {
			auditHandler := handlers.NewAuditHandler(db)
			r.Mount("/audit", auditHandler.Routes())
		}

		// MCP 端点
		if mcpRegistry != nil {
			mcpHandler := handlers.NewMcpHandler(mcpRegistry)
			r.Mount("/mcp/servers", mcpHandler.Routes())
		}

		// Token 端点
		if db != nil {
			tokenHandler := handlers.NewTokenHandler(db)
			r.Mount("/auth/token", tokenHandler.Routes())
		}

		// 工具端点
		if agentLoop != nil {
			toolHandler := handlers.NewToolHandler(agentLoop)
			r.Mount("/tools", toolHandler.Routes())
		}

		// 聊天端点
		if db != nil {
			chatHandler := handlers.NewChatHandler(db, logger, agentLoop, llmProvider)
			r.Mount("/chat", chatHandler.Routes())
		}

		// Token 用量追踪端点
		if db != nil {
			tokenUsageHandler := handlers.NewTokenUsageHandler(db)
			r.Mount("/token-usage", tokenUsageHandler.Routes())
		}

		// 事件总线端点
		if db != nil {
			eventHandler := handlers.NewEventHandler(db)
			r.Mount("/events", eventHandler.Routes())
		}

		// Daily Digest 端点
		if db != nil {
			digestHandler := handlers.NewDigestHandler(db)
			r.Mount("/digest", digestHandler.Routes())
		}

		// Workflow 工作流端点
		if db != nil {
			workflowHandler := handlers.NewWorkflowHandler(db)
			r.Mount("/workflows", workflowHandler.Routes())
		}

		// 轨迹回放端点
		trajHandler := handlers.NewTrajectoryHandler()
		r.Mount("/trajectories", trajHandler.Routes())

		// 追踪记录端点（Agent Trace）
		traceHandler := handlers.NewTraceHandler(log)
		r.Mount("/traces", traceHandler.Routes())

		// 视觉分析端点（截图→视觉模型→UI 理解）
		if llmProvider != nil {
			visionHandler := handlers.NewVisionHandler(llmProvider, log)
			r.Mount("/vision", visionHandler.Routes())
		}

		// 知识库端点
		if db != nil {
			kbHandler := handlers.NewKBHandler(db)
			r.Mount("/knowledge", kbHandler.Routes())
		}

		// 用户画像端点（Memory 2.0 Reflection）
		if db != nil {
			profileHandler := handlers.NewProfileHandler(db, log)
			r.Mount("/profiles", profileHandler.Routes())
		}

		// 记忆生命周期端点
		if db != nil {
			lifecycleHandler := handlers.NewLifecycleHandler(db, log)
			r.Mount("/lifecycle", lifecycleHandler.Routes())
		}

		// 记忆质量评分端点
		if db != nil {
			msHandler := handlers.NewMemoryScoreHandler(db, log)
			r.Mount("/memory-score", msHandler.Routes())
		}
	})

	// 静态文件（前端）- 必须在 API 路由之后
	staticHandler := StaticHandler()
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		staticHandler.ServeHTTP(w, r)
	})
	r.Post("/*", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"uptime_sec": int(time.Since(startTime).Seconds()),
		"version":    "0.1.0",
		"heartbeat":  observability.IsHeartbeatOK(),
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "running",
		"version":   "0.1.0",
		"uptime_sec": int(time.Since(startTime).Seconds()),
		"heartbeat": observability.IsHeartbeatOK(),
		"tasks": map[string]interface{}{
			"queued":   0,
			"running":  0,
			"completed": 0,
		},
		"queue_depth": 0,
		"llm_provider": config.Get().LLM.DefaultProvider,
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// 只允许本地开发和本机访问（精确匹配 host:port，避免 localhost.evil.com 这类前缀欺骗）
		allowed := origin == "" || isLocalOrigin(origin)

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isLocalOrigin 精确判断 origin 是否为本机（localhost/127.0.0.1/0.0.0.0）。
// 用 url.Parse 只比对 host，避免 "http://localhost.evil.com" 这类前缀欺骗。
func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1"
}

// tokenAuthMiddleware 校验 /v1 下的 Bearer token。
// 豁免只读/公开路径：health/status。SSE 事件流（/events）也放行只读展示。
func tokenAuthMiddleware(verifier *auth.TokenVerifier) func(http.Handler) http.Handler {
	exempt := func(path string) bool {
		// 仅豁免只读/公开端点；token 管理必须认证，
		// 否则任何网络客户端都能未授权铸造高权限 token。
		// 创建 token 的唯一合法途径是 CLI: agent token <名称>
		return strings.HasSuffix(path, "/health") ||
			strings.HasSuffix(path, "/status") ||
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if exempt(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			principal, err := verifier.Verify(r)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			// 把 principal 存入 context，供 handler 做权限/归属判断
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
		})
	}
}
