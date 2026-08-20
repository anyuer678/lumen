package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"agent/internal/config"
)

// SettingsHandler 设置处理器
type SettingsHandler struct{}

// NewSettingsHandler 创建处理器
func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{}
}

// Routes 注册路由
func (h *SettingsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetSettings)
	r.Put("/", h.UpdateSettings)
	r.Post("/test-llm", h.TestLLM)
	return r
}

// GetSettings 获取配置
func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()

	settings := map[string]interface{}{
		"server": map[string]interface{}{
			"host": cfg.Server.Host,
			"port": cfg.Server.Port,
		},
		"llm": map[string]interface{}{
			"default_provider": cfg.LLM.DefaultProvider,
			"providers":        cfg.LLM.Providers,
		},
		"agent": map[string]interface{}{
			"max_concurrent_tasks": cfg.Agent.MaxConcurrentTasks,
			"step_timeout":         cfg.Agent.StepTimeout,
			"step_max_retries":     cfg.Agent.StepMaxRetries,
		},
		"browser": map[string]interface{}{
			"engine":       cfg.Browser.Engine,
			"headful":      cfg.Browser.Headful,
			"proxy_socks5": cfg.Browser.ProxySocks5,
		},
		"scheduler": map[string]interface{}{
			"tick_interval": cfg.Scheduler.TickInterval,
		},
		"observability": map[string]interface{}{
			"metrics_enabled": cfg.Observability.MetricsEnabled,
			"audit_enabled":   cfg.Observability.AuditEnabled,
			"log_level":       cfg.Observability.LogLevel,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// UpdateSettings 更新配置
func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg == nil {
		http.Error(w, "config not loaded", http.StatusInternalServerError)
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// LLM - 深度合并，不丢弃其他 provider
	if llmRaw, ok := updates["llm"].(map[string]interface{}); ok {
		if dp, ok := llmRaw["default_provider"].(string); ok && dp != "" {
			cfg.LLM.DefaultProvider = dp
		}
		// 如果传了 api_key，直接设置环境变量
		if ak, ok := llmRaw["api_key"].(string); ok && ak != "" {
			if env, ok := llmRaw["api_key_env"].(string); ok && env != "" {
				os.Setenv(env, ak)
			}
		}
		if prow, ok := llmRaw["providers"].(map[string]interface{}); ok {
			if cfg.LLM.Providers == nil {
				cfg.LLM.Providers = map[string]config.LLMProvider{}
			}
			for name, prowVal := range prow {
				pv, _ := prowVal.(map[string]interface{})
				// 从现有配置获取默认值（保留未传入的字段）
				p := cfg.LLM.Providers[name]
				if b, ok := pv["base_url"].(string); ok { p.BaseURL = b }
				if m, ok := pv["model"].(string); ok { p.Model = m }
				if k, ok := pv["api_key_env"].(string); ok { p.APIKeyEnv = k }
				if t, ok := pv["type"].(string); ok { p.Type = t }
				if v, ok := pv["max_tokens"].(float64); ok { p.MaxTokens = int(v) }
				if v, ok := pv["timeout"].(float64); ok { p.Timeout = fmt.Sprintf("%.0fs", v) }
				if v, ok := pv["timeout"].(string); ok { p.Timeout = v }
				if ak, ok := pv["api_key"].(string); ok && ak != "" { p.APIKey = ak }
				cfg.LLM.Providers[name] = p
			}
		}
	}
	// Agent
	if a, ok := updates["agent"].(map[string]interface{}); ok {
		if v, ok := a["max_concurrent_tasks"].(float64); ok { cfg.Agent.MaxConcurrentTasks = int(v) }
		if v, ok := a["step_timeout"].(string); ok { cfg.Agent.StepTimeout = v }
		if v, ok := a["step_max_retries"].(float64); ok { cfg.Agent.StepMaxRetries = int(v) }
	}
	// Browser
	if b, ok := updates["browser"].(map[string]interface{}); ok {
		if v, ok := b["engine"].(string); ok { cfg.Browser.Engine = v }
		if v, ok := b["headful"].(bool); ok { cfg.Browser.Headful = v }
		if v, ok := b["proxy_socks5"].(string); ok { cfg.Browser.ProxySocks5 = v }
	}
	// Scheduler
	if s, ok := updates["scheduler"].(map[string]interface{}); ok {
		if v, ok := s["tick_interval"].(string); ok { cfg.Scheduler.TickInterval = v }
	}
	// Observability
	if o, ok := updates["observability"].(map[string]interface{}); ok {
		if v, ok := o["log_level"].(string); ok { cfg.Observability.LogLevel = v }
		if v, ok := o["audit_enabled"].(bool); ok { cfg.Observability.AuditEnabled = v }
		if v, ok := o["metrics_enabled"].(bool); ok { cfg.Observability.MetricsEnabled = v }
	}

	// 持久化到 config.yaml
	if err := config.Save(); err != nil {
		http.Error(w, "failed to save config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "Settings saved (restart required for some changes)",
	})
}

// TestLLM 测试 LLM 连接（真正发送请求）
func (h *SettingsHandler) TestLLM(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.BaseURL == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Base URL is required",
		})
		return
	}

	// 构造 OpenAI 兼容请求
	url := req.BaseURL + "/chat/completions"
	body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"hi"}],"max_tokens":5}`, req.Model)

	httpReq, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Failed to create request: %v", err),
		})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Connection failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("HTTP %d: %s", resp.StatusCode, func() string { s := string(respBody); if len(s) > 200 { return s[:200] }; return s }()),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("LLM connection test passed (model: %s)", req.Model),
		"provider": req.Provider,
		"model":    req.Model,
	})
}
