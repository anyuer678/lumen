package agent

import (
	"context"
	"math"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2.0,
	}
}

// RetryableFunc 可重试的函数
type RetryableFunc func(ctx context.Context) error

// RetryWithBackoff 带指数退避的重试
func RetryWithBackoff(ctx context.Context, config RetryConfig, fn RetryableFunc) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// 检查上下文
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 执行函数
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// 如果是最后一次尝试，返回错误
		if attempt == config.MaxRetries {
			break
		}

		// 计算退避时间
		delay := calculateBackoff(attempt, config)

		// 等待
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// calculateBackoff 计算退避时间
func calculateBackoff(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.BaseDelay) * math.Pow(config.Multiplier, float64(attempt))

	// 限制最大延迟
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// 添加随机抖动（±20%）
	jitter := delay * 0.2
	delay = delay - jitter + (2 * jitter * (float64(time.Now().UnixNano()%1000) / 1000.0))

	return time.Duration(delay)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 网络错误、超时错误可重试
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"dial tcp",
		"no such host",
		"network is unreachable",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// contains 字符串包含检查
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
