package llm

import (
	"strings"
)

// Router 智能路由器：根据任务类型选择最佳模型
type Router struct {
	providers map[string]Provider
	config    RouterConfig
}

// RouterConfig 路由器配置
type RouterConfig struct {
	// 默认模型（当无法判断时使用）
	Default string
	// 简单任务模型（echo/list/status 等）
	Simple string
	// 复杂任务模型（规划/推理/多步骤）
	Complex string
	// 图像任务模型（截图分析等）
	Vision string
	// 本地模型（零成本）
	Local string
}

// NewRouter 创建路由器
func NewRouter(providers map[string]Provider, config RouterConfig) *Router {
	if config.Default == "" {
		config.Default = "zhipu"
	}
	if config.Simple == "" {
		config.Simple = "ollama"
	}
	if config.Complex == "" {
		config.Complex = "zhipu"
	}
	if config.Vision == "" {
		config.Vision = "openai"
	}
	if config.Local == "" {
		config.Local = "ollama"
	}
	return &Router{providers: providers, config: config}
}

// Route 根据任务内容选择最佳模型
func (r *Router) Route(task string, tools []string) string {
	lower := strings.ToLower(task)

	// 1. 简单任务：echo/list/status/time/date
	simplePatterns := []string{"echo", "list", "status", "time", "date", "whoami", "hostname", "pwd"}
	for _, p := range simplePatterns {
		if strings.Contains(lower, p) {
			if p, ok := r.providers[r.config.Simple]; ok && p != nil {
				return r.config.Simple
			}
		}
	}

	// 2. 图像任务：screenshot/分析图片
	imagePatterns := []string{"screenshot", "图片", "图像", "截图", "visual", "image"}
	for _, p := range imagePatterns {
		if strings.Contains(lower, p) {
			if p, ok := r.providers[r.config.Vision]; ok && p != nil {
				return r.config.Vision
			}
		}
	}

	// 3. 复杂任务：规划/推理/多步骤/代码
	complexPatterns := []string{"计划", "规划", "分析", "推理", "代码", "debug", "review", "compare", "总结", "报告"}
	for _, p := range complexPatterns {
		if strings.Contains(lower, p) {
			if p, ok := r.providers[r.config.Complex]; ok && p != nil {
				return r.config.Complex
			}
		}
	}

	// 4. 默认：仅当默认 provider 可用时返回，否则回退到任意可用 provider
	if p, ok := r.providers[r.config.Default]; ok && p != nil {
		return r.config.Default
	}
	for name, p := range r.providers {
		if p != nil {
			return name
		}
	}
	return ""
}

// GetProvider 获取指定 provider
func (r *Router) GetProvider(name string) Provider {
	if p, ok := r.providers[name]; ok {
		return p
	}
	return nil
}

// GetDefault 获取默认 provider
func (r *Router) GetDefault() Provider {
	return r.GetProvider(r.config.Default)
}
