package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func mustWriteSecret(t *testing.T, path, secret string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, mode)
}

// 非 loopback 绑定：禁止空库 bootstrap。
func TestBootstrapRefusedWhenBindNotLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	secretPath := filepath.Join(t.TempDir(), "bootstrap.secret")
	mustWriteSecret(t, secretPath, "s3cret", 0o600)
	req.Header.Set(BootstrapSecretHeader, "s3cret")

	for _, host := range []string{"0.0.0.0", "192.168.1.10", "example.com", "[::]"} {
		err := ValidateBootstrap(req, host, secretPath)
		if err == nil {
			t.Errorf("bind host %q must refuse bootstrap", host)
		}
	}
}

// loopback 绑定但无 bootstrap secret 且未交互确认：拒绝。
func TestBootstrapRefusedWhenNoSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	t.Setenv(BootstrapInteractiveEnv, "")
	missing := filepath.Join(t.TempDir(), "no-such-bootstrap.secret")
	err := ValidateBootstrap(req, "127.0.0.1", missing)
	if err == nil {
		t.Fatal("bootstrap without secret file must be refused")
	}
}

// 非 loopback 请求来源：即使有 secret 也拒绝。
func TestBootstrapRefusedWhenRemoteNotLoopback(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "bootstrap.secret")
	mustWriteSecret(t, secretPath, "s3cret", 0o600)
	for _, remote := range []string{"8.8.8.8:1234", "10.0.0.5:9999", "203.0.113.9:1"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
		req.RemoteAddr = remote
		req.Header.Set(BootstrapSecretHeader, "s3cret")
		if err := ValidateBootstrap(req, "127.0.0.1", secretPath); err == nil {
			t.Errorf("remote %q must refuse bootstrap", remote)
		}
	}
}

// secret 文件权限过宽（非 Windows）：拒绝。
func TestBootstrapRefusedWhenSecretPermsTooWide(t *testing.T) {
	if filepath.Separator == '\\' {
		t.Skip("unix mode bits not enforced on Windows")
	}
	secretPath := filepath.Join(t.TempDir(), "bootstrap.secret")
	mustWriteSecret(t, secretPath, "s3cret", 0o644)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set(BootstrapSecretHeader, "s3cret")
	if err := ValidateBootstrap(req, "127.0.0.1", secretPath); err == nil {
		t.Fatal("world-readable bootstrap secret must be refused on unix")
	}
}

// 正确路径：loopback + 0600 secret + 匹配头。
func TestBootstrapAllowedWithLocalSecret(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "bootstrap.secret")
	mustWriteSecret(t, secretPath, "s3cret-token-value", 0o600)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set(BootstrapSecretHeader, "s3cret-token-value")
	if err := ValidateBootstrap(req, "127.0.0.1", secretPath); err != nil {
		t.Fatalf("valid local bootstrap should pass: %v", err)
	}
	// 错误 secret
	req2 := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req2.RemoteAddr = "127.0.0.1:5555"
	req2.Header.Set(BootstrapSecretHeader, "wrong")
	if err := ValidateBootstrap(req2, "127.0.0.1", secretPath); err == nil {
		t.Fatal("mismatched secret must be refused")
	}
	// 缺头
	req3 := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req3.RemoteAddr = "127.0.0.1:5555"
	if err := ValidateBootstrap(req3, "127.0.0.1", secretPath); err == nil {
		t.Fatal("missing secret header must be refused when secret file exists")
	}
}

// 本地交互确认路径。
func TestBootstrapInteractiveConfirm(t *testing.T) {
	t.Setenv(BootstrapInteractiveEnv, "1")
	missing := filepath.Join(t.TempDir(), "none.secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set(BootstrapConfirmHeader, BootstrapConfirmValue)
	if err := ValidateBootstrap(req, "localhost", missing); err != nil {
		t.Fatalf("interactive local confirm should pass: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/v1/auth/token", nil)
	req2.RemoteAddr = "127.0.0.1:1"
	if err := ValidateBootstrap(req2, "127.0.0.1", missing); err == nil {
		t.Fatal("interactive mode without confirm header must refuse")
	}
}

func TestIsLoopbackHost(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "localhost", "[::1]", "::1", "127.0.0.1:14000", ""} {
		if !IsLoopbackHost(h) {
			t.Errorf("IsLoopbackHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"0.0.0.0", "192.168.0.1", "example.com"} {
		if IsLoopbackHost(h) {
			t.Errorf("IsLoopbackHost(%q) = true, want false", h)
		}
	}
}

func TestEnsureBootstrapSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "bootstrap.secret")
	s1, err := EnsureBootstrapSecret(path)
	if err != nil {
		t.Fatalf("EnsureBootstrapSecret: %v", err)
	}
	if len(s1) < 32 {
		t.Fatalf("secret too short: %q", s1)
	}
	s2, err := EnsureBootstrapSecret(path)
	if err != nil || s2 != s1 {
		t.Fatalf("second call should reuse secret: %q %v", s2, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 && filepath.Separator != '\\' {
		t.Fatalf("secret file mode too wide: %v", info.Mode().Perm())
	}
}
