package observability

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

func newAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/audit.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts DATETIME NOT NULL DEFAULT (datetime('now')),
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		target TEXT,
		detail TEXT,
		result TEXT,
		ip TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	l := NewAuditLogger(db)
	seed := []*AuditEntry{
		{TS: time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC), Actor: "user", Action: "login", Target: "web", Result: "ok"},
		{TS: time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC), Actor: "agent", Action: "run_tool", Target: "shell:ls", Detail: "列出目录", Result: "ok"},
		{TS: time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC), Actor: "agent", Action: "confirm", Target: "task:12", Detail: "高危确认", Result: "approved"},
	}
	for _, e := range seed {
		if err := l.Record(e); err != nil {
			t.Fatalf("seed record: %v", err)
		}
	}
	return db
}

func TestAuditQueryAll(t *testing.T) {
	l := NewAuditLogger(newAuditTestDB(t))
	entries, err := l.Query(50, "", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	// 按 ts 降序：最新在前
	if entries[0].Action != "confirm" {
		t.Fatalf("latest first, got %q", entries[0].Action)
	}
}

func TestAuditQueryActorAndAction(t *testing.T) {
	l := NewAuditLogger(newAuditTestDB(t))
	entries, err := l.Query(50, "agent", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("actor=agent: want 2, got %d", len(entries))
	}
	entries, err = l.Query(50, "", "login", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 || entries[0].Actor != "user" {
		t.Fatalf("action=login: want 1 user entry, got %+v", entries)
	}
}

func TestAuditQuerySinceDateOnly(t *testing.T) {
	l := NewAuditLogger(newAuditTestDB(t))
	// 仅 10:00 及以后的记录
	entries, err := l.Query(50, "", "", "2026-11-14 09:30:00")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "confirm" {
		t.Fatalf("since filter: want 1 confirm, got %d", len(entries))
	}
	// 日期补全为当天 00:00:00
	entries, err = l.Query(50, "", "", "2026-11-14")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("since=day: want 3, got %d", len(entries))
	}
}

func TestAuditQueryLimitAndJSON(t *testing.T) {
	l := NewAuditLogger(newAuditTestDB(t))
	entries, err := l.Query(2, "", "", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("limit: want 2, got %d", len(entries))
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) == "null" {
		t.Fatal("json should not be null")
	}
}

func TestNormalizeSince(t *testing.T) {
	cases := map[string]string{
		"2026-11-14":          "2026-11-14 00:00:00",
		" 2026-11-14 ":        "2026-11-14 00:00:00",
		"2026-11-14 09:30:00": "2026-11-14 09:30:00",
	}
	for in, want := range cases {
		got, err := NormalizeSince(in)
		if err != nil {
			t.Fatalf("NormalizeSince(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("NormalizeSince(%q)=%q, want %q", in, got, want)
		}
	}
	if _, err := NormalizeSince("昨天"); err == nil {
		t.Fatal("invalid since should error")
	}
}
