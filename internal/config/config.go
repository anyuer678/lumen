package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Host         string `yaml:"host"`
		Port         int    `yaml:"port"`
		APITokenTTL  string `yaml:"api_token_ttl"`
	} `yaml:"server"`

	Service struct {
		Name              string `yaml:"name"`
		DisplayName       string `yaml:"display_name"`
		AutoStart         bool   `yaml:"auto_start"`
		HeartbeatInterval string `yaml:"heartbeat_interval"`
		RestartDelay      string `yaml:"restart_delay"`
		MaxRestart        int    `yaml:"max_restart"`
	} `yaml:"service"`

	DB struct {
		Path string `yaml:"path"`
		WAL  bool   `yaml:"wal"`
	} `yaml:"db"`

	Workspace struct {
		Root         string   `yaml:"root"`
		Sandbox      bool     `yaml:"sandbox"`
		AllowedPaths []string `yaml:"allowed_paths"`
	} `yaml:"workspace"`

	LLM struct {
		DefaultProvider string                 `yaml:"default_provider"`
		Providers       map[string]LLMProvider `yaml:"providers"`
		Embedding       EmbeddingConfig        `yaml:"embedding"`
	} `yaml:"llm"`

	Agent struct {
		MaxConcurrentTasks int    `yaml:"max_concurrent_tasks"`
		StepTimeout        string `yaml:"step_timeout"`
		StepMaxRetries     int    `yaml:"step_max_retries"`
		PlanMaxSteps       int    `yaml:"plan_max_steps"`
	} `yaml:"agent"`

	Scheduler struct {
		TickInterval string `yaml:"tick_interval"`
	} `yaml:"scheduler"`

	Permissions struct {
		PolicyFile     string `yaml:"policy_file"`
		ConfirmTimeout string `yaml:"confirm_timeout"`
		// ShellProfile 控制字符串 shell 的启用档位（默认 strict）。
		//   strict    — shell.run 可用，但每次调用强制 L2+确认+审计，黑名单加严
		//   argv-only — 默认推荐；禁用字符串 shell.run，仅允许 argv 白名单工具
		//   full      — 显式 opt-in：保留字符串 shell（仍要求 L2+确认；破坏性命令仍拦截）
		ShellProfile string `yaml:"shell_profile"`
	} `yaml:"permissions"`

	Observability struct {
		MetricsEnabled bool   `yaml:"metrics_enabled"`
		AuditEnabled   bool   `yaml:"audit_enabled"`
		LogLevel       string `yaml:"log_level"`
		LogFile        string `yaml:"log_file"`
	} `yaml:"observability"`

	Browser struct {
		Engine      string `yaml:"engine"`
		Headful     bool   `yaml:"headful"`
		UserDataDir string `yaml:"user_data_dir"`
		ProxySocks5 string `yaml:"proxy_socks5"`
	} `yaml:"browser"`

	MCP struct {
		Servers []McpServerConfig `yaml:"servers"`
	} `yaml:"mcp"`
}

// McpServerConfig MCP 服务器配置（对应 pgi 等外部工具）
type McpServerConfig struct {
	Name      string   `yaml:"name"`
	Command   string   `yaml:"command"`
	Args      []string `yaml:"args"`
	Transport string   `yaml:"transport"`
}

type LLMProvider struct {
	Type      string `yaml:"type" json:"type"`
	BaseURL   string `yaml:"base_url" json:"base_url"`
	APIKeyEnv string `yaml:"api_key_env" json:"api_key_env"`
	APIKey    string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	Model     string `yaml:"model" json:"model"`
	MaxTokens int    `yaml:"max_tokens" json:"max_tokens"`
	Timeout   string `yaml:"timeout" json:"timeout"`
}

type EmbeddingConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Dim      int    `yaml:"dim"`
}

var globalConfig *Config

func defaultConfig() *Config {
	cfg := &Config{}
	// 默认只监听本机，避免局域网内未授权访问（如需公网访问需显式配置并加认证）
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 14000
	cfg.Server.APITokenTTL = "365d"
	cfg.Service.Name = "openagent-agent"
	cfg.Service.DisplayName = "OpenAgent Agent Service"
	cfg.Service.AutoStart = true
	cfg.Service.HeartbeatInterval = "30s"
	cfg.Service.RestartDelay = "10s"
	cfg.Service.MaxRestart = 5
	cfg.DB.Path = "./data/agent.db"
	cfg.DB.WAL = true
	cfg.Workspace.Root = "./data/workspace"
	cfg.Workspace.Sandbox = true
	cfg.LLM.DefaultProvider = "deepseek"
	cfg.LLM.Embedding.Provider = "local"
	cfg.LLM.Embedding.Model = "bge-small-zh-v1.5"
	cfg.LLM.Embedding.Dim = 512
	cfg.Agent.MaxConcurrentTasks = 3
	cfg.Agent.StepTimeout = "300s"
	cfg.Agent.StepMaxRetries = 3
	cfg.Agent.PlanMaxSteps = 30
	cfg.Scheduler.TickInterval = "60s"
	cfg.Permissions.PolicyFile = "./conf/permission.yaml"
	cfg.Permissions.ConfirmTimeout = "60s"
	// 默认禁止「无确认的自由字符串 shell」：strict 档仍可用 shell.run，但每次强制确认。
	// 需要完全禁用字符串 shell 时设为 argv-only；需要放开时显式设为 full。
	cfg.Permissions.ShellProfile = "strict"
	cfg.Observability.MetricsEnabled = true
	cfg.Observability.AuditEnabled = true
	cfg.Observability.LogLevel = "info"
	cfg.Observability.LogFile = "./data/logs/agent.log"
	cfg.Browser.Engine = "playwright"
	cfg.Browser.Headful = true
	cfg.Browser.UserDataDir = "./data/browser-profile"
	return cfg
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = "./conf/config.yaml"
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	cfg := defaultConfig()

	// 如果配置文件存在，读取并覆盖默认值
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else {
		// 写入默认配置
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}

	globalConfig = cfg
	return cfg, nil
}

func Get() *Config {
	return globalConfig
}

// GetShellProfile 返回全局 shell 档位（未加载配置时为 strict）。
func GetShellProfile() string {
	return Get().ShellProfile()
}

// StringShellAllowed 全局便捷函数：是否允许自由字符串 shell。
func StringShellAllowed() bool {
	return Get().StringShellAllowed()
}

// ConfigPath 配置文件路径
var ConfigPath = "./conf/config.yaml"

// Shell profile 档位常量。
const (
	ShellProfileStrict   = "strict"    // 默认：字符串 shell 每次强制 L2+确认+审计
	ShellProfileArgvOnly = "argv-only" // 禁用 shell.run，仅 argv 白名单
	ShellProfileFull     = "full"      // 显式 opt-in 的自由字符串 shell
)

// ShellProfile 返回当前 shell 档位（缺省 strict；非法值回退 strict）。
func (c *Config) ShellProfile() string {
	if c == nil {
		return ShellProfileStrict
	}
	switch strings.ToLower(strings.TrimSpace(c.Permissions.ShellProfile)) {
	case ShellProfileArgvOnly, "argv_only", "argvonly":
		return ShellProfileArgvOnly
	case ShellProfileFull, "unsafe", "legacy":
		return ShellProfileFull
	case ShellProfileStrict, "":
		return ShellProfileStrict
	default:
		return ShellProfileStrict
	}
}

// StringShellAllowed 返回是否允许字符串 shell.run（仅 full 档为真）。
// strict/argv-only 档：默认永不启用无 opt-in 的自由字符串 shell。
func (c *Config) StringShellAllowed() bool {
	return c.ShellProfile() == ShellProfileFull
}

// Save 将当前配置持久化到配置文件
func Save() error {
	if globalConfig == nil {
		return nil
	}
	data, err := yaml.Marshal(globalConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath, data, 0644)
}
