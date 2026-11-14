package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"agent/internal/agent"
	"agent/internal/config"
	agentDB "agent/internal/db"
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
			auditServiceAction(dbIfAvailable(), "service.install", "Windows 服务注册")
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
			auditServiceAction(dbIfAvailable(), "service.uninstall", "Windows 服务移除")
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
		case "decisions":
			if err := runDecisions(); err != nil {
				fmt.Fprintf(os.Stderr, "decisions error: %v\n", err)
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

// loadConfig 加载配置（默认 ./conf/config.yaml，可用第2个参数指定）。
// 注意：仅适用于 install/uninstall/status 等没有位置参数的子命令；
// token 子命令必须走 parseTokenArgs（位置参数是 token 名，不是配置路径）。
func loadConfig() error {
	configPath := "./conf/config.yaml"
	if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "-") {
		configPath = os.Args[2]
	}
	_, err := config.Load(configPath)
	return err
}

// parseTokenArgs 解析 token 子命令参数，返回 (name, level, configPath, err)。
// token 名是第一个位置参数；配置路径只能用 --config 指定。
// 修复背景：此前 loadConfig 会把 os.Args[2]（token 名）误当配置路径，
// 导致 `agent token <名称>` 以名称为路径读取/静默创建默认配置，
// 并在错误的配置（含默认数据库）上签发 token。
func parseTokenArgs(args []string) (string, int, string, error) {
	name := "default"
	level := 1
	configPath := "./conf/config.yaml"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--level":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--level 需要一个取值（0-3）")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 || n > 3 {
				return "", 0, "", fmt.Errorf("--level 取值须为 0-3 的整数")
			}
			level = n
		case arg == "--config":
			if i+1 >= len(args) {
				return "", 0, "", fmt.Errorf("--config 需要一个配置文件路径")
			}
			i++
			configPath = args[i]
		case !strings.HasPrefix(arg, "-") && name == "default":
			name = arg
		}
	}
	return name, level, configPath, nil
}

// runToken 生成一个 API token 并写入数据库（仅输出一次明文）。
// 用法：agent token [name] [--level 0-3]
// 权限级别：0=只读 1=普通(默认) 2=危险(可批准高危确认) 3=管理员。
// 此前默认签发 L3/admin token，一个泄漏的 token 即等于无确认的整机控制。
func runToken() error {
	name, level, configPath, err := parseTokenArgs(os.Args[2:])
	if err != nil {
		return err
	}

	if _, err := config.Load(configPath); err != nil {
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

	scopes := "tools,chat,tasks,confirm"
	if level >= 3 {
		scopes = "admin"
	} else if level == 0 {
		scopes = "read"
	}

	_, err = db.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, scopes, perm_level, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, 1, datetime('now'))`,
		"tok-"+hex.EncodeToString(raw[:4]), name, hex.EncodeToString(hash[:]),
		scopes, level)
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}

	_recordTokenAudit(db, name, level)

	fmt.Printf("✅ 已创建 API token（名称: %s，级别 L%d，仅显示一次）\n", name, level)
	fmt.Printf("   %s\n\n", tokenStr)
	fmt.Println("使用方式：")
	fmt.Println("   export LUMEN_TOKEN=\"<token>\"   # 或")
	fmt.Println("   Authorization: Bearer <token>   # 或")
	fmt.Println("   X-API-Token: <token>")
	fmt.Println("级别说明：0=只读 1=普通（默认） 2=危险（可批准 L2 高危确认） 3=管理员（--level 3）")
	return nil
}

// parseDecisionsArgs 解析 decisions 子命令参数（无位置参数）。
func parseDecisionsArgs(args []string) (limit int, jsonOut bool, actor, action, since string, err error) {
	limit = 50
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--limit":
			if i+1 >= len(args) {
				return 0, false, "", "", "", fmt.Errorf("--limit 需要一个取值")
			}
			i++
			n, e := strconv.Atoi(args[i])
			if e != nil || n <= 0 {
				return 0, false, "", "", "", fmt.Errorf("--limit 取值须为正整数")
			}
			limit = n
		case arg == "--json":
			jsonOut = true
		case arg == "--actor":
			if i+1 >= len(args) {
				return 0, false, "", "", "", fmt.Errorf("--actor 需要一个取值")
			}
			i++
			actor = args[i]
		case arg == "--action":
			if i+1 >= len(args) {
				return 0, false, "", "", "", fmt.Errorf("--action 需要一个取值")
			}
			i++
			action = args[i]
		case arg == "--since":
			if i+1 >= len(args) {
				return 0, false, "", "", "", fmt.Errorf("--since 需要一个取值")
			}
			i++
			since = args[i]
		case !strings.HasPrefix(arg, "-"):
			return 0, false, "", "", "", fmt.Errorf("decisions 不接受位置参数：%q", arg)
		}
	}
	return limit, jsonOut, actor, action, since, nil
}

// runDecisions 查询并展示决策/审计日志（DESIGN Phase C：决策日志可查询）。
// 用法：agent decisions [--limit N] [--json] [--actor X] [--action X] [--since YYYY-MM-DD]
// 库内 ts 为 UTC；--since 亦按 UTC 比较。
func runDecisions() error {
	limit, jsonOut, actor, action, since, err := parseDecisionsArgs(os.Args[2:])
	if err != nil {
		return err
	}
	if err := loadConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	db, err := initDB()
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()

	logger := agentLog.NewAuditLogger(db)
	entries, err := logger.Query(limit, actor, action, since)
	if err != nil {
		return err
	}
	if jsonOut {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if len(entries) == 0 {
		fmt.Println("（无匹配的决策/审计记录）")
		return nil
	}
	fmt.Printf("%-20s  %-12s  %-18s  %-22s  %s\n", "UTC 时间", "actor", "action", "target", "result")
	for _, e := range entries {
		target := e.Target
		if len(target) > 22 {
			target = target[:19] + "..."
		}
		result := e.Result
		if len(result) > 10 {
			result = result[:10]
		}
		fmt.Printf("%-20s  %-12s  %-18s  %-22s  %s\n",
			e.TS.Format("2006-01-02 15:04:05"), e.Actor, e.Action, target, result)
		if e.Detail != "" {
			fmt.Printf("%20s  ↳ %s\n", "", e.Detail)
		}
	}
	fmt.Printf("\n共 %d 条\n", len(entries))
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

	tmpDir, err := os.MkdirTemp("", "lumen-bench-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	db, err := agentDB.Init(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer db.Close()
	fmt.Println("（隔离运行：临时数据库 " + tmpDir + "）")

	loop := initLoop(db)

	if benchVersion == "v1" {
		report, err := agent.RunBenchmark(context.Background(), loop, filepath.Join(tmpDir, "BENCHMARK_REPORT.md"), mode)
		if err != nil {
			return err
		}
		fmt.Printf("\n=== v1 完成 ===\n")
		fmt.Printf("通过: %d/%d (%.0f%%)\n", report.Passed, report.Total, float64(report.Passed)*100/float64(report.Total))
	} else if benchVersion == "v2" {
		report, err := agent.RunBenchmarkV2(context.Background(), loop, filepath.Join(tmpDir, "BENCHMARK_V2_REPORT.md"), mode)
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
		report, err := agent.RunBenchmarkV3(context.Background(), loop, filepath.Join(tmpDir, "BENCHMARK_V3_REPORT.md"), mode)
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

// _recordTokenAudit 把 token 签发写入审计日志（安全相关动作必须留痕，DESIGN Phase C）。
// 审计失败不阻断已完成的签发，但要在 stderr 可见。
func _recordTokenAudit(db *sql.DB, name string, level int) {
	_audit(db, "cli", "token.create", name, fmt.Sprintf("level=L%d", level), "ok")
}

// _audit 通用审计写入：失败仅 stderr 可见，不阻断主流程。
func _audit(db *sql.DB, actor, action, target, detail, result string) {
	logger := agentLog.NewAuditLogger(db)
	err := logger.Record(&agentLog.AuditEntry{
		Actor: actor, Action: action, Target: target, Detail: detail, Result: result,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit warning: 记录 %s 失败：%v\n", action, err)
	}
}

// dbIfAvailable 尽力打开本地数据库用于审计写入；失败返回 nil（审计静默跳过）。
// 前置条件：调用方必须已执行 loadConfig()（install/uninstall 路径均满足），
// 未加载配置时 config.Get() 会 panic——不在测试中直接调用本函数。
func dbIfAvailable() *sql.DB {
	db, err := initDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit warning: 数据库不可用，本次动作不入审计：%v\n", err)
		return nil
	}
	return db
}

// auditServiceAction 记录服务安装/移除审计（AuditLogger 对 nil db 静默跳过）。
func auditServiceAction(db *sql.DB, action, detail string) {
	_audit(db, "cli", action, "lumen-service", detail, "ok")
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
