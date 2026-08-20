package agent

import (
	"os"

	"gopkg.in/yaml.v3"
)

// PolicyConfig 策略配置（YAML 格式）
type PolicyConfig struct {
	Policies []PolicyRule `yaml:"policies"`
}

// PolicyRule 单条策略规则
type PolicyRule struct {
	Name        string           `yaml:"name"`
	EventType   string           `yaml:"event_type"`
	Enabled     bool             `yaml:"enabled"`
	QuietHours  QuietHoursConfig `yaml:"quiet_hours"`
	MaxPerHour  int              `yaml:"max_per_hour"`
	Keywords    []string         `yaml:"keywords"`
	MinPriority int              `yaml:"min_priority"`
	Action      PolicyAction     `yaml:"action"`
}

// QuietHoursConfig 静默时段配置
type QuietHoursConfig struct {
	Start int `yaml:"start"` // 24h 格式
	End   int `yaml:"end"`
}

// PolicyAction 策略动作
type PolicyAction struct {
	Notify      bool     `yaml:"notify"`
	AutoExecute bool     `yaml:"auto_execute"`
	Tools       []string `yaml:"tools"`
	RequireConfirm bool  `yaml:"require_confirm"`
}

// DefaultPolicyConfig 默认策略配置
func DefaultPolicyConfig() *PolicyConfig {
	return &PolicyConfig{
		Policies: []PolicyRule{
			{
				Name:      "file_organize",
				EventType: "file.created",
				Enabled:   true,
				QuietHours: QuietHoursConfig{Start: 23, End: 8},
				MaxPerHour: 5,
				Keywords:   []string{".pdf", ".docx", ".xlsx", ".png", ".jpg", ".zip"},
				MinPriority: 3,
				Action: PolicyAction{
					Notify:      true,
					AutoExecute: true,
					Tools:       []string{"fs"},
				},
			},
			{
				Name:      "task_auto_retry",
				EventType: "task.failed",
				Enabled:   true,
				QuietHours: QuietHoursConfig{Start: 23, End: 8},
				MaxPerHour: 3,
				MinPriority: 5,
				Action: PolicyAction{
					Notify:      false,
					AutoExecute: true,
					Tools:       []string{"shell.run"},
				},
			},
			{
				Name:      "system_alert",
				EventType: "system.alert",
				Enabled:   true,
				MaxPerHour: 10,
				MinPriority: 1,
				Action: PolicyAction{
					Notify:      true,
					AutoExecute: false,
				},
			},
		},
	}
}

// LoadPolicyConfig 从 YAML 文件加载策略配置
func LoadPolicyConfig(path string) (*PolicyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在时返回默认配置
		return DefaultPolicyConfig(), nil
	}

	var config PolicyConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SavePolicyConfig 保存策略配置到 YAML 文件
func SavePolicyConfig(config *PolicyConfig, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// ConvertToPolicyRules 将 YAML 配置转换为 PolicyEngine 可用的规则
func ConvertToPolicyRules(config *PolicyConfig) []Policy {
	var rules []Policy
	for _, r := range config.Policies {
		rules = append(rules, Policy{
			Name:              r.Name,
			EventType:         EventType(r.EventType),
			Enabled:           r.Enabled,
			QuietHoursStart:   r.QuietHours.Start,
			QuietHoursEnd:     r.QuietHours.End,
			MaxActionsPerHour: r.MaxPerHour,
			AllowedTools:      r.Action.Tools,
			RequireConfirm:    r.Action.RequireConfirm,
			MinPriority:       r.MinPriority,
			Keywords:          r.Keywords,
		})
	}
	return rules
}

// LoadPolicyFromYAML 从 YAML 加载并转换为 PolicyEngine 规则
func LoadPolicyFromYAML(path string) ([]Policy, error) {
	config, err := LoadPolicyConfig(path)
	if err != nil {
		return nil, err
	}
	return ConvertToPolicyRules(config), nil
}
