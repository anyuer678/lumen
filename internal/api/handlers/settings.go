package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
			"providers":        redactProviders(cfg.LLM.Providers),
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
				if isValidEnvName(env) {
					os.Setenv(env, ak)
				}
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

	// SSRF 防护：拒绝内网/私有/链路本地地址，防止探测内部服务
	if err := validatePublicURL(req.BaseURL); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "该 URL 不被允许（仅允许公网地址）：" + err.Error(),
		})
		return
	}

	// 构造 OpenAI 兼容请求（用 json.Marshal 避免字段拼接注入）
	payload, _ := json.Marshal(map[string]interface{}{
		"model":     req.Model,
		"messages":  []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 5,
	})
	url := req.BaseURL + "/chat/completions"
	body := string(payload)

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

// validatePublicURL 防止 SSRF：仅允许公网 HTTP(S) 地址，
// 拒绝 localhost、私有 IP、链路本地 (169.254.x.x)、0.0.0.0 等。
func validatePublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("仅支持 http/https")
	}
	host := u.Hostname()

	// 域名无法判断是否为内网时（如 .local/纯主机名），保守拒绝
	if net.ParseIP(host) == nil {
		ip, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("无法解析主机")
		}
		for _, i := range ip {
			if isBlockedIP(i) {
				return fmt.Errorf("解析到内网地址 %s", i.String())
			}
		}
		return nil
	}
	if isBlockedIP(net.ParseIP(host)) {
		return fmt.Errorf("内网地址 %s", host)
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// 链路本地 169.254.0.0/16（一些实现未覆盖）
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	return false
}

// redactProviders 复制 providers 并脱敏 api_key，避免明文返回给前端
func redactProviders(providers map[string]config.LLMProvider) map[string]config.LLMProvider {
	result := make(map[string]config.LLMProvider, len(providers))
	for name, p := range providers {
		p.APIKey = "" // 脱敏，只保留 api_key_env 提示
		result[name] = p
	}
	return result
}

// isValidEnvName 限制可设置的环境变量名，防止覆盖 PATH/PRELOAD 等危险变量
func isValidEnvName(name string) bool {
	blocked := map[string]bool{
		"PATH": true, "LD_PRELOAD": true, "DYLD_INSERT_LIBRARIES": true,
		"PYTHONPATH": true, "HOME": true, "USER": true, "SHELL": true,
		"IFS": true, "LD_LIBRARY_PATH": true,
	}
	upper := strings.ToUpper(name)
	if blocked[upper] {
		return false
	}
	// 允许的字符：字母数字下划线，且不能以数字开头
	if name == "" {
		return false
	}
	for i, c := range name {
		if c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}
