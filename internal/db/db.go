package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"
)

var globalDB *sql.DB

const schema = `
-- 任务主表
CREATE TABLE IF NOT EXISTS tasks (
    id             TEXT PRIMARY KEY,
    type           TEXT NOT NULL DEFAULT 'manual',
    goal           TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    progress       REAL DEFAULT 0,
    current_step   INTEGER DEFAULT 0,
    priority       INTEGER DEFAULT 5,
    perm_level     INTEGER DEFAULT 0,
    owner          TEXT NOT NULL DEFAULT '',
    session_id     TEXT,
    retry_count    INTEGER DEFAULT 0,
    max_retries    INTEGER DEFAULT 3,
    result         TEXT,
    error          TEXT,
    plan_json      TEXT,
    pause_reason   TEXT,
    created_at     DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at     DATETIME,
    finished_at    DATETIME,
    updated_at     DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);

-- 步骤表
CREATE TABLE IF NOT EXISTS steps (
    id           TEXT PRIMARY KEY,
    task_id      TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    description  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    tool         TEXT,
    args_json    TEXT,
    result       TEXT,
    summary      TEXT,
    retries      INTEGER DEFAULT 0,
    started_at   DATETIME,
    finished_at  DATETIME
);
CREATE INDEX IF NOT EXISTS idx_steps_task ON steps(task_id, seq);

-- 调度任务
CREATE TABLE IF NOT EXISTS jobs (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    trigger_type  TEXT NOT NULL,
    cron_expr     TEXT,
    interval_secs INTEGER,
    watch_path    TEXT,
    goal_template TEXT NOT NULL,
    priority      INTEGER DEFAULT 5,
    enabled       INTEGER DEFAULT 1,
    catch_up      INTEGER DEFAULT 0,
    concurrency   TEXT DEFAULT 'skip',
    last_run_at   DATETIME,
    next_run_at   DATETIME,
    last_status   TEXT,
    miss_count    INTEGER DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 人工确认
CREATE TABLE IF NOT EXISTS confirmations (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES tasks(id),
    step_seq      INTEGER NOT NULL,
    operation     TEXT NOT NULL,
    tool          TEXT NOT NULL,
    args_json     TEXT,
    risk_level    INTEGER NOT NULL,
    reason        TEXT,
    status        TEXT NOT NULL DEFAULT 'pending',
    requester     TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT (datetime('now')),
    decided_at    DATETIME,
    decided_by    TEXT,
    timeout_secs  INTEGER DEFAULT 60
);
CREATE INDEX IF NOT EXISTS idx_conf_pending ON confirmations(status);

-- 长期记忆
CREATE TABLE IF NOT EXISTS memories (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    content     TEXT NOT NULL,
    tags        TEXT,
    embedding   BLOB,
    source_task TEXT,
    confirmed   INTEGER DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_mem_kind ON memories(kind);

-- 审计日志（append-only）
CREATE TABLE IF NOT EXISTS audit_logs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         DATETIME NOT NULL DEFAULT (datetime('now')),
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    target     TEXT,
    detail     TEXT,
    result     TEXT,
    ip         TEXT
);

-- API Token
CREATE TABLE IF NOT EXISTS api_tokens (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    scopes     TEXT NOT NULL,
    perm_level INTEGER DEFAULT 1,
    enabled    INTEGER DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at DATETIME
);

-- 工作记忆（恢复点）
CREATE TABLE IF NOT EXISTS working_memory (
    task_id      TEXT PRIMARY KEY REFERENCES tasks(id),
    resume_step  INTEGER NOT NULL DEFAULT 0,
    last_summary TEXT,
    interrupted_at DATETIME,
    restore_count  INTEGER DEFAULT 0
);

-- 聊天会话
CREATE TABLE IF NOT EXISTS chat_sessions (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '新对话',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- 聊天消息
CREATE TABLE IF NOT EXISTS chat_messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES chat_sessions(id),
    role        TEXT NOT NULL, -- user/assistant/system
    content     TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, created_at);

-- 知识库
CREATE TABLE IF NOT EXISTS knowledge (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    content     TEXT NOT NULL,
    tags        TEXT,
    source      TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_knowledge_created ON knowledge(created_at);

-- Token 用量追踪
CREATE TABLE IF NOT EXISTS token_usage (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',  -- chat/task/tool_call/review
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    cost_usd    REAL DEFAULT 0,
    duration_ms INTEGER DEFAULT 0,
    task_id     TEXT,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_token_usage_provider ON token_usage(provider, created_at);
CREATE INDEX IF NOT EXISTS idx_token_usage_task ON token_usage(task_id);

-- 事件总线
CREATE TABLE IF NOT EXISTS events (
    id         TEXT PRIMARY KEY,
    source     TEXT NOT NULL,
    event_type TEXT NOT NULL,
    timestamp  DATETIME NOT NULL,
    payload    TEXT,
    priority   INTEGER DEFAULT 5,
    processed  INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_processed ON events(processed);

-- 工作流
CREATE TABLE IF NOT EXISTS workflows (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    steps_json  TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

func Init(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// 使用纯 Go SQLite 驱动，无需 CGO
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	// 建表
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables: %w", err)
	}

	globalDB = db
	return db, nil
}

func Get() *sql.DB {
	return globalDB
}

func Close() error {
	if globalDB != nil {
		return globalDB.Close()
	}
	return nil
}
