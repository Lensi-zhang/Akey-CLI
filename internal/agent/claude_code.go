package agent

// ClaudeCode 管理 Claude Code 顶层 mcpServers 配置。
type ClaudeCode struct {
	jsonAdapter
}

// NewClaudeCode 创建 Claude Code 适配器；configPath 为空时使用 ~/.claude.json。
func NewClaudeCode(configPath string) *ClaudeCode {
	return &ClaudeCode{jsonAdapter: jsonAdapter{
		agentName:  "claude-code",
		executable: "claude",
		stdio:      true,
		path:       configPath,
	}}
}
