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
