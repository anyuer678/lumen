package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Bootstrap 空库首次运行引导的安全门（Sprint3）。
//
// 目标：禁止未认证的网络 bootstrap 签发 L3 admin token。
// 仅允许：
//  1. 绑定地址为 loopback，且请求来源也是 loopback；
//  2. 且具备下列之一：
//     a) 一次性 bootstrap secret 文件（默认 0600）且请求头携带匹配 secret；
//     b) 本地交互确认（LUMEN_BOOTSTRAP_INTERACTIVE=1 + 确认头）。
//
// 首选路径仍是本机 CLI：`agent token admin --level 3`。

const (
	// BootstrapSecretFileDefault 默认 secret 文件路径。
	BootstrapSecretFileDefault = "./data/bootstrap.secret"
	// BootstrapSecretHeader HTTP 请求头：bootstrap secret。
	BootstrapSecretHeader = "X-Lumen-Bootstrap-Secret"
	// BootstrapConfirmHeader HTTP 请求头：本地交互确认。
	BootstrapConfirmHeader = "X-Lumen-Bootstrap-Confirm"
	// BootstrapConfirmValue 交互确认期望值。
	BootstrapConfirmValue = "local-ok"
	// BootstrapInteractiveEnv 设置为 1/true 时允许本地交互确认路径。
	BootstrapInteractiveEnv = "LUMEN_BOOTSTRAP_INTERACTIVE"
)

// IsLoopbackHost 判断 host 是否为 loopback 绑定（127.0.0.1 / ::1 / localhost）。
func IsLoopbackHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		// 空 host 按默认 loopback 对待（config 默认 127.0.0.1）
		return true
	}
	h = strings.TrimPrefix(h, "http://")
	h = strings.TrimPrefix(h, "https://")
	if strings.HasPrefix(h, "[") {
		// [::1] 或 [::1]:14000
		if end := strings.IndexByte(h, ']'); end > 0 {
			h = h[1:end]
		}
	} else if ip := net.ParseIP(h); ip != nil {
		// 整段已是 IP（含 ::1 / 127.0.0.1），不要按 host:port 切
		return ip.IsLoopback()
	} else if i := strings.LastIndex(h, ":"); i > 0 {
		// host:port — 仅当端口为纯数字时剥离
		if port := h[i+1:]; port != "" && isAllDigits(port) {
			h = h[:i]
		}
	}
	h = strings.TrimSpace(h)
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// IsLoopbackAddr 判断 HTTP RemoteAddr 是否来自 loopback。
func IsLoopbackAddr(remote string) bool {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// bootstrapInteractiveAllowed 本地交互确认是否开启。
func bootstrapInteractiveAllowed() bool {
	v := strings.TrimSpace(os.Getenv(BootstrapInteractiveEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

// secretFileUsable 检查 secret 文件是否存在且权限可接受。
// 非 Windows：要求无 group/other 权限（0600 或更严）。
// Windows：ACL 由 OS 控制；只要求普通文件存在（文档要求收紧 ACL）。
func secretFileUsable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bootstrap secret 文件不存在: %s（用 `agent bootstrap-secret` 生成，或使用本机 CLI `agent token`）", path)
		}
		return fmt.Errorf("bootstrap secret 文件不可读: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("bootstrap secret 路径是目录: %s", path)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return fmt.Errorf("bootstrap secret 文件 %s 权限过宽 %04o（要求 0600）", path, perm)
		}
	}
	return nil
}

// ReadBootstrapSecret 读取 secret 文件内容（去空白）。
func ReadBootstrapSecret(path string) (string, error) {
	if err := secretFileUsable(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read bootstrap secret: %w", err)
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", fmt.Errorf("bootstrap secret 文件为空: %s", path)
	}
	return s, nil
}

// EnsureBootstrapSecret 确保 secret 文件存在：不存在则生成并以 0600 写入。
func EnsureBootstrapSecret(path string) (string, error) {
	if path == "" {
		path = BootstrapSecretFileDefault
	}
	if s, err := ReadBootstrapSecret(path); err == nil {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write bootstrap secret: %w", err)
	}
	// Unix 上再次收紧（WriteFile 在已存在文件上不会改 mode）
	_ = os.Chmod(path, 0o600)
	return secret, nil
}

// ValidateBootstrap 校验一次「空库 + 无认证」的 HTTP bootstrap 请求。
// bindHost 来自 config.Server.Host；secretPath 来自 config/bootstrap。
// 任一条件不满足即返回 error（调用方应 403）。
func ValidateBootstrap(r *http.Request, bindHost, secretPath string) error {
	if r == nil {
		return fmt.Errorf("nil request")
	}
	if !IsLoopbackHost(bindHost) {
		return fmt.Errorf("bootstrap 拒绝：服务未绑定 loopback（host=%q）。非本机绑定下禁止未认证签发 L3 token；请绑定 127.0.0.1 并使用本机 CLI `agent token` 或 bootstrap secret", bindHost)
	}
	if !IsLoopbackAddr(r.RemoteAddr) {
		return fmt.Errorf("bootstrap 拒绝：请求来源 %q 不是 loopback。禁止网络侧 bootstrap L3 admin token", r.RemoteAddr)
	}

	if secretPath == "" {
		secretPath = BootstrapSecretFileDefault
	}

	// 路径 A：一次性 bootstrap secret 文件（0600）+ 请求头匹配
	if _, err := os.Stat(secretPath); err == nil {
		if err := secretFileUsable(secretPath); err != nil {
			return err
		}
		want, err := ReadBootstrapSecret(secretPath)
		if err != nil {
			return err
		}
		got := strings.TrimSpace(r.Header.Get(BootstrapSecretHeader))
		if got == "" {
			return fmt.Errorf("bootstrap 拒绝：存在 secret 文件但请求未携带 %s 头", BootstrapSecretHeader)
		}
		if got != want {
			return fmt.Errorf("bootstrap 拒绝：bootstrap secret 不匹配")
		}
		return nil
	}

	// 路径 B：本地交互确认
	if bootstrapInteractiveAllowed() {
		if strings.TrimSpace(r.Header.Get(BootstrapConfirmHeader)) == BootstrapConfirmValue {
			return nil
		}
		return fmt.Errorf("bootstrap 拒绝：交互模式需要请求头 %s: %s（仅限本机操作者）", BootstrapConfirmHeader, BootstrapConfirmValue)
	}

	// 默认：无 secret 且无交互确认 → 拒绝未认证网络 bootstrap
	return fmt.Errorf("bootstrap 拒绝：无 bootstrap secret 且未启用本地交互确认。首选本机 CLI `agent token admin --level 3`；或运行 `agent bootstrap-secret` 生成 %s（0600）后携带 %s 头", secretPath, BootstrapSecretHeader)
}
