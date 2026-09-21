package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/config"
	agentDB "agent/internal/db"
)

func newTokenHandlerForBootstrap(t *testing.T) (*TokenHandler, *config.Config) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "bootstrap-test.db")
	db, err := agentDB.Init(dbPath)
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	cfg.Server.Host = "127.0.0.1"
	cfg.MCP.Enabled = false
	cfg.Permissions.ComputerUseEnabled = false
	secretPath := filepath.Join(t.TempDir(), "bootstrap.secret")
	t.Setenv("LUMEN_BOOTSTRAP_INTERACTIVE", "")
	cfg.Bootstrap.SecretFile = secretPath
	h := NewTokenHandler(db).WithBootstrapSecretPath(secretPath)
	return h, cfg
}

func bootstrapBody() *strings.Reader {
	b, _ := json.Marshal(map[string]any{"name": "admin", "perm_level": 3})
	return strings.NewReader(string(b))
}

// 空库 + 无 bootstrap secret：HTTP bootstrap 必须拒绝（403）。
func TestTokenCreateBootstrapRefusedWithoutSecret(t *testing.T) {
	h, _ := newTokenHandlerForBootstrap(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bootstrapBody())
	req.RemoteAddr = "127.0.0.1:40000"
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bootstrap without secret = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

// 空库 + 非 loopback 绑定：即使有正确 secret 也拒绝。
func TestTokenCreateBootstrapRefusedWhenBindNotLoopback(t *testing.T) {
	h, cfg := newTokenHandlerForBootstrap(t)
	cfg.Server.Host = "0.0.0.0"
	secretPath := cfg.BootstrapSecretFile()
	if err := os.WriteFile(secretPath, []byte("local-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bootstrapBody())
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Lumen-Bootstrap-Secret", "local-secret")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bootstrap on non-loopback bind = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

// 空库 + 非 loopback 来源：拒绝。
func TestTokenCreateBootstrapRefusedWhenRemoteNotLoopback(t *testing.T) {
	h, cfg := newTokenHandlerForBootstrap(t)
	secretPath := cfg.BootstrapSecretFile()
	if err := os.WriteFile(secretPath, []byte("local-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bootstrapBody())
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Lumen-Bootstrap-Secret", "local-secret")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bootstrap from non-loopback remote = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}

// 空库 + loopback + 正确 0600 secret：允许创建 L3 bootstrap token。
func TestTokenCreateBootstrapAllowedWithSecret(t *testing.T) {
	h, cfg := newTokenHandlerForBootstrap(t)
	secretPath := cfg.BootstrapSecretFile()
	if err := os.WriteFile(secretPath, []byte("local-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(secretPath, 0o600)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bootstrapBody())
	req.RemoteAddr = "127.0.0.1:40000"
	req.Header.Set("X-Lumen-Bootstrap-Secret", "local-secret")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("valid local bootstrap = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if lvl, _ := resp["perm_level"].(float64); int(lvl) != 3 {
		t.Fatalf("bootstrap token perm_level=%v want 3", resp["perm_level"])
	}
	if ok, _ := resp["bootstrap"].(bool); !ok {
		t.Fatalf("response should mark bootstrap=true, got %v", resp)
	}
	scopes, _ := resp["scopes"].(string)
	if scopes == "" {
		t.Fatal("bootstrap token must carry admin scopes")
	}

	// 已有 token 后，无认证请求应 401（bootstrap 窗口关闭）
	req2 := httptest.NewRequest(http.MethodPost, "/v1/auth/token", bootstrapBody())
	req2.RemoteAddr = "127.0.0.1:40000"
	req2.Header.Set("X-Lumen-Bootstrap-Secret", "local-secret")
	w2 := httptest.NewRecorder()
	h.Create(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("post-bootstrap unauthenticated create = %d, want 401", w2.Code)
	}
}

// MCP HTTP 注册在默认配置下必须拒绝。
func TestMcpRegisterDisabledByDefault(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MCP.Enabled = false
	h := NewMcpHandler(nil)
	body := strings.NewReader(`{"name":"x","command":"node","args":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", body)
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("mcp register default = %d, want 403; body=%s", w.Code, w.Body.String())
	}
}
