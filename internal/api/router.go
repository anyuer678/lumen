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
	// 除 /health 与 /status 外的所有端点要求 Bearer token
		if db != nil {
			verifier := auth.NewTokenVerifier(db)
			r.Use(tokenAuthMiddleware(verifier))
		}
		r.Get("/health", healthHandler)
		r.Get("/status", statusHandler)
		r.Get("/events", SSEHandler(GetBroadcaster()))

		// 浠诲姟绔偣
		taskHandler := handlers.NewTaskHandler(tm)
		r.Mount("/tasks", taskHandler.Routes())

		// 瀹氭椂浠诲姟绔偣
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

		// 纭绔偣
		if db != nil {
			confirmHandler := handlers.NewConfirmHandler(auth.NewConfirmStore(db))
			r.Mount("/confirmations", confirmHandler.Routes())
		}

		// 璁剧疆绔偣
		settingsHandler := handlers.NewSettingsHandler()
		r.Mount("/settings", settingsHandler.Routes())

		// 浜х墿绔偣锛堟埅鍥剧瓑 workspace 鏂囦欢锛?
		if db != nil {
			artifactsHandler := handlers.NewArtifactsHandler("./data/workspace")
			r.Mount("/artifacts", artifactsHandler.Routes())
		}

		// 璁板繂绔偣
		if db != nil {
			memoryHandler := handlers.NewMemoryHandler(memory.NewStore(db))
			r.Mount("/memories", memoryHandler.Routes())
		}

		// 瀹¤绔偣
		if db != nil {
			auditHandler := handlers.NewAuditHandler(db)
			r.Mount("/audit", auditHandler.Routes())
		}

		// MCP 绔偣
		if mcpRegistry != nil {
			mcpHandler := handlers.NewMcpHandler(mcpRegistry)
			r.Mount("/mcp/servers", mcpHandler.Routes())
		}

		// Token 绔偣
		if db != nil {
			tokenHandler := handlers.NewTokenHandler(db)
			r.Mount("/auth/token", tokenHandler.Routes())
		}

		// 宸ュ叿绔偣
		if agentLoop != nil {
			toolHandler := handlers.NewToolHandler(agentLoop)
			r.Mount("/tools", toolHandler.Routes())
		}

		// 鑱婂ぉ绔偣
		if db != nil {
			chatHandler := handlers.NewChatHandler(db, logger, agentLoop, llmProvider)
			r.Mount("/chat", chatHandler.Routes())
		}

		// Token 鐢ㄩ噺杩借釜绔偣
		if db != nil {
			tokenUsageHandler := handlers.NewTokenUsageHandler(db)
			r.Mount("/token-usage", tokenUsageHandler.Routes())
		}

		// 浜嬩欢鎬荤嚎绔偣
		if db != nil {
			eventHandler := handlers.NewEventHandler(db)
			r.Mount("/events", eventHandler.Routes())
		}

		// Daily Digest 绔偣
		if db != nil {
			digestHandler := handlers.NewDigestHandler(db)
			r.Mount("/digest", digestHandler.Routes())
		}

		// Workflow 工作流端点
		if db != nil {
			workflowHandler := handlers.NewWorkflowHandler(db)
			r.Mount("/workflows", workflowHandler.Routes())
		}

		// 杞ㄨ抗鍥炴斁绔偣
		trajHandler := handlers.NewTrajectoryHandler()
		r.Mount("/trajectories", trajHandler.Routes())

		// 杩借釜璁板綍绔偣锛圓gent Trace锛?
		traceHandler := handlers.NewTraceHandler(log)
		r.Mount("/traces", traceHandler.Routes())

		// 视觉分析端点（截图 -> 视觉模型 -> UI 理解）
		if llmProvider != nil {
			visionHandler := handlers.NewVisionHandler(llmProvider, log)
			r.Mount("/vision", visionHandler.Routes())
		}

		// 鐭ヨ瘑搴撶鐐?
		if db != nil {
			kbHandler := handlers.NewKBHandler(db)
			r.Mount("/knowledge", kbHandler.Routes())
		}

		// 鐢ㄦ埛鐢诲儚绔偣锛圡emory 2.0 Reflection锛?
		if db != nil {
			profileHandler := handlers.NewProfileHandler(db, log)
			r.Mount("/profiles", profileHandler.Routes())
		}

		// 璁板繂鐢熷懡鍛ㄦ湡绔偣
		if db != nil {
			lifecycleHandler := handlers.NewLifecycleHandler(db, log)
			r.Mount("/lifecycle", lifecycleHandler.Routes())
		}

		// 璁板繂璐ㄩ噺璇勫垎绔偣
		if db != nil {
			msHandler := handlers.NewMemoryScoreHandler(db, log)
			r.Mount("/memory-score", msHandler.Routes())
		}
	})

	// 静态文件（前端）：必须在 API 路由之后
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
	completedCount := 0
	if taskManager != nil {
		completedCount = taskManager.CompletedCount()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "running",
		"version":   "0.1.0",
		"uptime_sec": int(time.Since(startTime).Seconds()),
		"heartbeat": observability.IsHeartbeatOK(),
		"tasks": map[string]interface{}{
			"queued":    taskQueueDepth(),
			"running":   taskRunningCount(),
			"completed": completedCount,
		},
		"queue_depth": taskQueueDepth(),
		"llm_provider": config.Get().LLM.DefaultProvider,
	})
}

func taskQueueDepth() int {
	if taskManager != nil {
		return taskManager.QueueDepth()
	}
	return 0
}

func taskRunningCount() int {
	if taskManager != nil {
		return taskManager.RunningCount()
	}
	return 0
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// 仅允许本地开发和本机访问（精确匹配 host:port，避免前缀欺骗）
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

// tokenAuthMiddleware 校验 /v1 下的 Bearer token，豁免 /health 与 /status。
// SSE 事件流（/events）同样要求认证：前端经 ?token= 查询参数传递。
func tokenAuthMiddleware(verifier *auth.TokenVerifier) func(http.Handler) http.Handler {
	exempt := func(path string) bool {
		// 仅豁免只读公开端点；token 管理必须认证
		return strings.HasSuffix(path, "/health") ||
			strings.HasSuffix(path, "/status") 
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
