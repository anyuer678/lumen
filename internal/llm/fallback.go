package llm

import (
	"context"
	"fmt"
	"strings"
)

// FallbackProvider 多模型回退包装器：主模型挂了自动切备用
// 实现 Provider 接口，调用方无需感知回退逻辑
type FallbackProvider struct {
	chain   []Provider // 按优先级排列
	lastErr error      // 上一次错误（用于日志）
}

// NewFallbackProvider 创建回退 provider
// chain 按优先级排列：第一个是主模型，后面是备用
func NewFallbackProvider(chain ...Provider) *FallbackProvider {
	// 过滤 nil
	var valid []Provider
	for _, p := range chain {
		if p != nil {
			valid = append(valid, p)
		}
	}
	return &FallbackProvider{chain: valid}
}

func (f *FallbackProvider) Name() string {
	if len(f.chain) == 0 {
		return "fallback(none)"
	}
	return fmt.Sprintf("fallback(%s)", f.chain[0].Name())
}

func (f *FallbackProvider) Chat(ctx context.Context, messages []Message, tools []ToolDef) (*Response, error) {
	var lastErr error
	for i, p := range f.chain {
		var resp *Response
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("provider panic: %v", r)
				}
			}()
			resp, err = p.Chat(ctx, messages, tools)
		}()
		if err == nil {
			if i > 0 {
				// 回退成功——记录切换
				f.lastErr = fmt.Errorf("fallback: %s → %s (after %s failed: %v)",
					f.chain[0].Name(), p.Name(), f.chain[i-1].Name(), lastErr)
			}
			return resp, nil
		}
		lastErr = fmt.Errorf("provider %s: %w", p.Name(), err)

		// 如果是上下文超限错误，不尝试下一个（换模型也没用）
		if isContextLimitError(err) {
			return nil, lastErr
		}
	}
	return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
}

// isContextLimitError 判断是否为上下文超限错误（不值得回退）
func isContextLimitError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "context_length_exceeded") ||
		strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "max_tokens") ||
		strings.Contains(msg, "context window")
}

// LastError 获取上一次回退记录（用于状态页展示）
func (f *FallbackProvider) LastError() error {
	return f.lastErr
}

// BoundedChat 把无工具、确定性的复核调用委托给回退链中第一个支持该能力的
// provider（目前为 OpenAIProvider），保证目标闭环在路由/回退模式下也可用。
func (f *FallbackProvider) BoundedChat(ctx context.Context, system, user string, maxTokens int, timeoutSec int) (string, error) {
	type boundedProvider interface {
		BoundedChat(ctx context.Context, system, user string, maxTokens int, timeoutSec int) (string, error)
	}
	var lastErr error
	for _, p := range f.chain {
		bp, ok := p.(boundedProvider)
		if !ok {
			continue
		}
		out, err := bp.BoundedChat(ctx, system, user, maxTokens, timeoutSec)
		if err == nil {
			return out, nil
		}
		lastErr = fmt.Errorf("provider %s: %w", p.Name(), err)
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no provider in fallback chain supports BoundedChat")
}
