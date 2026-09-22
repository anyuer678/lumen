package agent

import (
	"context"
	"strings"
	"testing"
)

// Exhaustive rejection table + property checks for exec.argv argument validation.
// Covers shell metachars, absolute paths, env-var styles, and whitespace/spacing.

func TestArgvRejectsHostileArgTable(t *testing.T) {
	tool := NewArgvToolWithAllow("./ws", true, nil)
	ctx := context.Background()
	cases := []struct {
		name string
		arg  string
	}{
		// shell metachars
		{"semi", "a;b"},
		{"and", "a&&b"},
		{"or", "a||b"},
		{"pipe", "a|b"},
		{"redir-out", "a>b"},
		{"redir-in", "a<b"},
		{"backtick", "a`b`c"},
		{"dollar-paren", "a$(id)"},
		{"newline", "a\nb"},
		{"cr", "a\rb"},
		{"nul", "a\x00b"},
		// absolute paths
		{"unix-abs", "/etc/passwd"},
		{"unix-abs-usr", "/usr/bin/id"},
		{"win-abs", `C:\Windows\System32\cmd.exe`},
		{"win-abs-lower", `c:\windows\system32\cmd.exe`},
		{"unc", `\\evil\share\payload`},
		{"unc-forward", `//evil/share/payload`},
		// relative path separators
		{"dotdot", "../../etc/passwd"},
		{"dotdot-win", `..\..\secret`},
		{"mixed-sep", `foo/bar`},
		{"mixed-sep-win", `foo\bar`},
		{"drive-relative", `C:foo`},
		// env var styles
		{"dollar", "$PATH"},
		{"dollar-home", "$HOME"},
		{"dollar-brace", "${PATH}"},
		{"percent", "%PATH%"},
		{"percent-home", "%USERPROFILE%"},
		{"percent-mid", "pre%PATH%post"},
		// spaces / whitespace (must reject — no spaced tokens)
		{"space", "hello world"},
		{"leading-space", " lead"},
		{"trailing-space", "trail "},
		{"tab", "a\tb"},
		{"multi-space", "a  b"},
		// mixed attack shapes
		{"space-plus-semi", "foo bar; id"},
		{"space-plus-dollar", "foo $HOME"},
		{"space-plus-path", "foo /etc/passwd"},
		{"utf8-fullwidth-semi", "a；b"},
	}
	for _, c := range cases {
		_, err := tool.Execute(ctx, map[string]any{"bin": "echo", "args": []any{c.arg}})
		if err == nil {
			t.Errorf("case %q arg=%q: expected rejection, got success", c.name, c.arg)
			continue
		}
	}
}

// Property: any arg containing a metachar / path sep / $ / % / whitespace / colon must be rejected.
func TestArgvSafeRejectsForbiddenClasses(t *testing.T) {
	forbidden := []string{"\t", "\n", "\r", " ", ";", "&", "|", ">", "<", "`", "$", "%", "/", "\\", ":", "；", "｜"}
	prefixes := []string{"x", "foo", "a", "pre"}
	suffixes := []string{"y", "bar", "b", "post"}
	for _, f := range forbidden {
		for i := range prefixes {
			arg := prefixes[i] + f + suffixes[i]
			if err := checkArgvSafe([]string{arg}); err == nil {
				t.Errorf("checkArgvSafe should reject %q (forbidden %q)", arg, f)
			}
		}
		// bare forbidden char
		if err := checkArgvSafe([]string{f}); err == nil {
			t.Errorf("checkArgvSafe should reject bare %q", f)
		}
	}
	// control chars
	if err := checkArgvSafe([]string{"a\x00b"}); err == nil {
		t.Error("checkArgvSafe should reject NUL")
	}
}

// Property: clean basename args must pass.
func TestArgvSafeAcceptsCleanArgs(t *testing.T) {
	ok := [][]string{
		{},
		{"--version"},
		{"-n", "4"},
		{"hello"},
		{"中文参数"},
		{"a-b_c.d"},
		{"123"},
	}
	for _, args := range ok {
		if err := checkArgvSafe(args); err != nil {
			t.Errorf("checkArgvSafe should accept %v, got %v", args, err)
		}
	}
}

// Fuzz checkArgvSafe: never panics; any input with a forbidden class is rejected.
func FuzzCheckArgvSafe(f *testing.F) {
	seeds := []string{
		"", "ok", "hello world", "/etc/passwd", `C:\Windows`, "$PATH", "%PATH%",
		"a;b", "a&&b", "$(id)", "`id`", "a\nb", "a|b", "a>b", "../x", `..\x`, "a\tb",
		"；", "foo/bar", "C:foo", "x\x00y",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, arg string) {
		err := checkArgvSafe([]string{arg})
		bad := false
		if strings.ContainsAny(arg, " \t\n\r;&|><`$%/\\:") {
			bad = true
		}
		if strings.ContainsAny(arg, "；｜＞＜＆＊") {
			bad = true
		}
		if strings.ContainsAny(arg, "\x00\x01\x1b") {
			bad = true
		}
		if bad && err == nil {
			t.Fatalf("checkArgvSafe(%q) accepted hostile arg", arg)
		}
		if !bad && err != nil && arg != "" {
			t.Fatalf("checkArgvSafe(%q) rejected clean arg: %v", arg, err)
		}
	})
}

// Fuzz binary allowlist: path-form bins always denied even with extra_allow.
func FuzzArgvBinaryAllowed(f *testing.F) {
	seeds := []string{"echo", "ls", "python", "/bin/ls", `C:\evil\python.exe`, "..\\python", "cmd.exe", ""}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, bin string) {
		tool := NewArgvToolWithAllow("./ws", true, []string{"python", "git"})
		_, ok := tool.binaryAllowed(bin)
		if ok {
			if strings.ContainsAny(bin, `/\`) || strings.Contains(bin, ":") {
				t.Fatalf("path-form bin %q must be denied", bin)
			}
			base := normalizeArgvBin(bin)
			allowed := base == "python" || base == "git"
			for _, b := range defaultArgvAllowlist {
				if b == base {
					allowed = true
				}
			}
			if !allowed {
				t.Fatalf("bin %q unexpectedly allowed", bin)
			}
		}
	})
}

// Execute-level property: every hostile arg shape must fail before spawn.
func TestArgvExecuteNeverSpawnsHostileArgs(t *testing.T) {
	tool := NewArgvToolWithAllow("./ws", true, nil)
	ctx := context.Background()
	hostile := []string{
		"hello world", "/etc/passwd", `C:\Windows`, "$PATH", "%PATH%",
		"a;b", "$(id)", "a\nb", "a|b", "foo/bar", `foo\bar`, "a\tb",
	}
	for _, arg := range hostile {
		_, err := tool.Execute(ctx, map[string]any{"bin": "true", "args": []any{arg}})
		if err == nil {
			t.Errorf("Execute must reject hostile arg %q", arg)
		}
	}
}
