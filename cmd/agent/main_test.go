package main

import "testing"

func TestParseTokenArgsDefaults(t *testing.T) {
	name, level, cfgPath, err := parseTokenArgs(nil)
	if err != nil {
		t.Fatalf("nil args should parse: %v", err)
	}
	if name != "default" || level != 1 || cfgPath != "./conf/config.yaml" {
		t.Fatalf("got name=%q level=%d cfgPath=%q", name, level, cfgPath)
	}
}

func TestParseTokenArgsNameAndLevel(t *testing.T) {
	// README 记载的用法：agent token <名称> --level 2
	name, level, cfgPath, err := parseTokenArgs([]string{"交互用", "--level", "2"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if name != "交互用" || level != 2 || cfgPath != "./conf/config.yaml" {
		t.Fatalf("got name=%q level=%d cfgPath=%q", name, level, cfgPath)
	}
}

func TestParseTokenArgsConfigFlagNotName(t *testing.T) {
	// 回归关键：--config 的值不能被当成 token 名；token 名也不能被当成配置路径
	name, level, cfgPath, err := parseTokenArgs([]string{"自定义名", "--config", "conf/other.yaml"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if name != "自定义名" || level != 1 || cfgPath != "conf/other.yaml" {
		t.Fatalf("got name=%q level=%d cfgPath=%q", name, level, cfgPath)
	}
}

func TestParseTokenArgsLevelBeforeName(t *testing.T) {
	name, level, _, err := parseTokenArgs([]string{"--level", "3", "ops"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if name != "ops" || level != 3 {
		t.Fatalf("got name=%q level=%d", name, level)
	}
}

func TestParseTokenArgsInvalidLevel(t *testing.T) {
	for _, args := range [][]string{
		{"a", "--level", "9"},
		{"a", "--level", "-1"},
		{"a", "--level", "abc"},
	} {
		if _, _, _, err := parseTokenArgs(args); err == nil {
			t.Fatalf("args %v should error", args)
		}
	}
}

func TestParseTokenArgsMissingFlagValue(t *testing.T) {
	if _, _, _, err := parseTokenArgs([]string{"a", "--level"}); err == nil {
		t.Fatal("--level without value should error")
	}
	if _, _, _, err := parseTokenArgs([]string{"--config"}); err == nil {
		t.Fatal("--config without value should error")
	}
}

func TestParseTokenArgsFlagsIgnoredAsName(t *testing.T) {
	// 未知 flag 不占用 token 名
	name, _, _, err := parseTokenArgs([]string{"--future-flag", "realname"})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if name != "realname" {
		t.Fatalf("got name=%q", name)
	}
}
