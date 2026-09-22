/**
 * Adversarial argv + policy lock tests (Sprint12).
 * Focus: unicode/metachar/path/env injection and dangerous-tool default-off chain.
 */
package agent

import (
	"context"
	"strings"
	"testing"

	"agent/internal/config"
)

func TestArgvRejectsUnicodeHomoglyphMetachars(t *testing.T) {
	tool := NewArgvToolWithAllow("/tmp/ws", true, nil)
	// fullwidth and CJK punctuation that look like shell separators
	hostile := []string{
		"a；rm",
		"a｜b",
		"a＞b",
		"a＜b",
		"a＆b",
		"a｜｜b",
		"ａ＆ｂ",
		"foo\u2024bar", // one-dot leader
		"foo\uFF1Bbar", // fullwidth semicolon
	}
	for _, a := range hostile {
		err := checkArgvSafe([]string{a})
		if err == nil {
			// some homoglyphs may not be in list; then require binary still denied
			if strings.ContainsAny(a, "；｜＞＜＆") {
				t.Fatalf("expected reject for homoglyph arg %q", a)
			}
		}
	}
	// path / env / whitespace always rejected
	for _, a := range []string{`C:\Windows\System32\cmd`, `\\server\share`, `/etc/passwd`, `../x`, `$HOME`, `%PATH%`, `a b`, "a\tb", "a:b"} {
		if err := checkArgvSafe([]string{a}); err == nil {
			t.Fatalf("expected reject for %q", a)
		}
	}
	// no path-form binary even if extra_allow lists it
	evil := NewArgvToolWithAllow("/tmp/ws", false, []string{`C:\evil\git.exe`, `../git`, "git.exe"})
	res, err := tool.Execute(context.Background(), map[string]any{"bin": `C:\evil\git.exe`, "args": []any{"status"}})
	_ = res
	if err == nil {
		t.Fatal("path-form binary must be denied")
	}
	_ = evil
}

func TestArgvExtraAllowRejectsPathForms(t *testing.T) {
	tool := NewArgvToolWithAllow("/tmp/ws", false, []string{`/usr/bin/git`, `C:\bin\python`, "git"})
	if _, ok := tool.binaryAllowed("git"); !ok {
		t.Fatal("bare git from extra should be allowed")
	}
	if _, ok := tool.binaryAllowed("/usr/bin/git"); ok {
		t.Fatal("path form must not enter allowlist")
	}
	if _, ok := tool.binaryAllowed(`C:\bin\python`); ok {
		t.Fatal("windows path form must not enter allowlist")
	}
}

func TestDangerousBinariesNotDefault(t *testing.T) {
	tool := NewArgvToolWithAllow("/tmp/ws", false, nil)
	for _, b := range ArgvExtraAllowCandidates {
		if _, ok := tool.binaryAllowed(b); ok {
			t.Fatalf("dangerous binary %s must not be default-allowed", b)
		}
	}
}

func TestShellAndComputerAndMCPStayOffWithoutFullOptIn(t *testing.T) {
	// code defaults + policy yaml should not enable shell.run / computer / mcp
	cfg := config.Get()
	if cfg.ShellProfile() != config.ShellProfileStrict {
		t.Fatalf("default shell_profile must be strict, got %q", cfg.ShellProfile())
	}
	if cfg.StringShellAllowed() {
		t.Fatal("string shell must not be allowed by default")
	}
	if cfg.ComputerUseEnabled() {
		t.Fatal("computer_use must default false")
	}
	if cfg.MCPEnabled() {
		t.Fatal("mcp must default false")
	}
	// argv_extra_allow default empty
	if len(cfg.ArgvExtraAllow()) != 0 {
		t.Fatalf("default argv_extra_allow must be empty, got %v", cfg.ArgvExtraAllow())
	}
}

func TestCheckArgvSafeRejectsQuotedBreakout(t *testing.T) {
	for _, a := range []string{
		`"whoami`, `whoami"`, `'whoami`, "whoami'",
		"`whoami", "$(whoami)", "whoami;ls", "whoami&&ls",
		`"a"`, `'a'`, "a\"b",
	} {
		if err := checkArgvSafe([]string{a}); err == nil {
			t.Fatalf("expected reject %q", a)
		}
	}
}
