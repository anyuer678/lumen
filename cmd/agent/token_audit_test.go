package main

import (
	"path/filepath"
	"testing"

	agentDB "agent/internal/db"
	agentLog "agent/internal/observability"
)

func TestRecordTokenAudit(t *testing.T) {
	db, err := agentDB.Init(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer db.Close()

	_recordTokenAudit(db, "ops", 2)

	logger := agentLog.NewAuditLogger(db)
	entries, err := logger.Query(10, "cli", "token.create", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 audit entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Target != "ops" {
		t.Fatalf("target = %q, want ops", e.Target)
	}
	if e.Detail != "level=L2" {
		t.Fatalf("detail = %q, want level=L2", e.Detail)
	}
	if e.Result != "ok" {
		t.Fatalf("result = %q, want ok", e.Result)
	}
}

func TestRecordTokenAuditDifferentLevels(t *testing.T) {
	db, err := agentDB.Init(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer db.Close()

	_recordTokenAudit(db, "a", 0)
	_recordTokenAudit(db, "b", 3)

	logger := agentLog.NewAuditLogger(db)
	entries, err := logger.Query(10, "", "token.create", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	found := map[string]string{}
	for _, e := range entries {
		found[e.Target] = e.Detail
	}
	if found["a"] != "level=L0" || found["b"] != "level=L3" {
		t.Fatalf("unexpected details: %v", found)
	}
}

func TestAuditGeneric(t *testing.T) {
	db, err := agentDB.Init(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	defer db.Close()

	_audit(db, "cli", "service.install", "lumen-service", "Windows 服务注册", "ok")

	logger := agentLog.NewAuditLogger(db)
	entries, err := logger.Query(10, "cli", "service.install", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Target != "lumen-service" || entries[0].Detail != "Windows 服务注册" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestAuditNilDbSilent(t *testing.T) {
	// nil db：AuditLogger 静默跳过，不得 panic
	_audit(nil, "cli", "service.install", "x", "d", "ok")
	_recordTokenAudit(nil, "x", 1)
}
