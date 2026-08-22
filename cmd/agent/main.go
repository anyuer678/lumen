package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"agent/internal/agent"
	agentDB "agent/internal/db"
	"agent/internal/config"
	"agent/internal/llm"
	agentLog "agent/internal/observability"
	"agent/internal/service"
)

var (
	version   = "0.1.0"
	buildDate = "unknown"
)

func main() {
	// 子命令
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := loadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "config error: %v\n", err)
				os.Exit(1)
			}
			if err := service.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "install error: %v\n", err)
				os.Exit(1)
			}
			return
		case "uninstall":
			if err := loadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "config error: %v\n", err)
				os.Exit(1)
			}
			if err := service.Uninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "uninstall error: %v\n", err)
				os.Exit(1)
			}
			return
		case "token":
			if err := runToken(); err != nil {
				fmt.Fprintf(os.Stderr, "token error: %v\n", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := loadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "config error: %v\n", err)
				os.Exit(1)
			}
			if err := service.Status(); err != nil {
				fmt.Fprintf(os.Stderr, "status error: %v\n", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Printf("agent v%s (built %s)\n", version, buildDate)
			return
		case "benchmark":
			if err := loadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "config error: %v\n", err)
				os.Exit(1)
			}
			if err := runBenchmark(); err != nil {
				fmt.Fprintf(os.Stderr, "benchmark error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// 前台运行
	configPath := flag.String("config", "./conf/config.yaml", "config file path")
	flag.Parse()

	if _, err := config.Load(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if err := service.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig 加载配置（默认 ./conf/config.yaml，可用第2个参数指定）
func loadConfig() error {
	configPath := "./conf/config.yaml"
	if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "-") {
		configPath = os.Args[2]
	}
	_, err := config.Load(configPath)
	return err
}

// runToken 生成一个 API token 并写入数据库（仅输出一次明文）。
// 用法：agent token [name]
func runToken() error {
	name := "default"
	if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "-") {
		name = os.Args[2]
	}

	if err := loadConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := initDB()
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	tokenStr := "agt_" + hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(tokenStr))

	_, err = db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, scopes, perm_level, enabled, created_at)
		 VALUES (?, ?, ?, ?, 3, 1, datetime('now'))`,
		"tok-"+hex.EncodeToString(raw[:4]), name, hex.EncodeToString(hash[:]),
		"admin")
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	fmt.Printf("✅ 已创建 API token（名称: %s，仅显示一次）\n", name)
	fmt.Printf("   %s\n\n", tokenStr)
	fmt.Println("使用方式：")
	fmt.Println("   export LUMEN_TOKEN=\"<token>\"   # 或")
	fmt.Println("   Authorization: Bearer <token>   # 或")
	fmt.Println("   X-API-Token: <token>")
	return nil
}

func runBenchmark() error {
	mode := "simple"
	benchVersion := "v3"
	for _, arg := range os.Args[2:] {
		if arg == "--llm" || arg == "-llm" {
			mode = "llm"
		}
		if arg == "--v1" || arg == "-v1" {
			benchVersion = "v1"
		}
		if arg == "--v2" || arg == "-v2" {
			benchVersion = "v2"
		}
	}

	fmt.Printf("=== Agent Benchmark %s (%s mode) ===\n", benchVersion, mode)
	fmt.Println("运行测试套件，找出真实失败点...")
	fmt.Println()

	db, err := initDB()
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	loop := initLoop(db)

	if benchVersion == "v1" {
		report, err := agent.RunBenchmark(context.Background(), loop, "BENCHMARK_REPORT.md", mode)
		if err != nil {
			return err
		}
		fmt.Printf("\n=== v1 完成 ===\n")
		fmt.Printf("通过: %d/%d (%.0f%%)\n", report.Passed, report.Total, float64(report.Passed)*100/float64(report.Total))
	} else if benchVersion == "v2" {
		report, err := agent.RunBenchmarkV2(context.Background(), loop, "BENCHMARK_V2_REPORT.md", mode)
		if err != nil {
			return err
		}
		fmt.Printf("\n=== v2 完成 ===\n")
		fmt.Printf("Overall: %.0f%% | Tool: %.0f%% | Safety: %.0f%% | Cost: $%.4f\n",
			report.Metrics.OverallSuccess, report.Metrics.ToolSelection,
			report.Metrics.SafetyRate, report.Metrics.TotalCostUSD)
	} else {
		// v3: 使用 v2 runner 但 v3 测试套件
		// 暂时先用 v2 runner + v3 套件数量
		report, err := agent.RunBenchmarkV3(context.Background(), loop, "BENCHMARK_V3_REPORT.md", mode)
		if err != nil {
			return err
		}
		fmt.Printf("\n=== v3 完成 ===\n")
		fmt.Printf("Overall: %.0f%% | Tool: %.0f%% | Safety: %.0f%% | Cost: $%.4f\n",
			report.Metrics.OverallSuccess, report.Metrics.ToolSelection,
			report.Metrics.SafetyRate, report.Metrics.TotalCostUSD)
	}
	fmt.Printf("报告已写入\n")
	return nil
}

func initDB() (*sql.DB, error) {
	dbPath := config.Get().DB.Path
	if dbPath == "" {
		dbPath = "./data/agent.db"
	}
	return agentDB.Init(dbPath)
}

func initLoop(db *sql.DB) *agent.Loop {
	cfg := config.Get()
	agentLog.Init(cfg.Observability.LogFile, cfg.Observability.LogLevel)
	logger := agentLog.Get()

	fmt.Printf("Config: default_provider=%q providers_count=%d\n", cfg.LLM.DefaultProvider, len(cfg.LLM.Providers))
	for name, p := range cfg.LLM.Providers {
		fmt.Printf("  Provider: %q base_url=%q model=%q key_env=%q\n", name, p.BaseURL, p.Model, p.APIKeyEnv)
	}

	// 直接用 config 构造 provider
	p, ok := cfg.LLM.Providers[cfg.LLM.DefaultProvider]
	if !ok || p.BaseURL == "" {
		fmt.Printf("WARNING: LLM provider %q not found (providers=%v), using simplified mode\n", cfg.LLM.DefaultProvider, cfg.LLM.Providers)
		return agent.NewLoop(db, logger, nil)
	}
	apiKey := p.APIKey
	if apiKey == "" && p.APIKeyEnv != "" {
		apiKey = os.Getenv(p.APIKeyEnv)
	}
	provider := llm.NewOpenAIProvider(llm.Config{
		Provider:  cfg.LLM.DefaultProvider,
		BaseURL:   p.BaseURL,
		APIKey:    apiKey,
		Model:     p.Model,
		MaxTokens: p.MaxTokens,
		Timeout:   120,
	})
	fmt.Printf("LLM provider: %s (model: %s, base_url: %s, key: %s)\n", cfg.LLM.DefaultProvider, p.Model, p.BaseURL, maskKey(apiKey))
	return agent.NewLoop(db, logger, provider)
}

func maskKey(k string) string {
	if k == "" {
		return "(empty)"
	}
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func createLLMProvider() llm.Provider {
	cfg := config.Get()
	p, ok := cfg.LLM.Providers[cfg.LLM.DefaultProvider]
	if !ok || p.BaseURL == "" {
		fmt.Printf("DEBUG: provider %q not found or empty base_url (providers=%v)\n", cfg.LLM.DefaultProvider, cfg.LLM.Providers)
		return nil
	}
	apiKey := p.APIKey
	if apiKey == "" && p.APIKeyEnv != "" {
		apiKey = os.Getenv(p.APIKeyEnv)
	}
	provider := llm.NewOpenAIProvider(llm.Config{
		Provider:  cfg.LLM.DefaultProvider,
		BaseURL:   p.BaseURL,
		APIKey:    apiKey,
		Model:     p.Model,
		MaxTokens: p.MaxTokens,
		Timeout:   120,
	})
	return provider
}
