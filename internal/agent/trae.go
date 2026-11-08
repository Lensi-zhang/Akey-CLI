package agent

// Trae 管理 Trae 顶层 mcpServers 配置。
type Trae struct {
	jsonAdapter
}

// NewTrae 创建 Trae 适配器；configPath 为空时使用 ~/.trae/mcp.json。
func NewTrae(configPath string) *Trae {
	return &Trae{jsonAdapter: jsonAdapter{
		agentName:  "trae",
		executable: "trae",
		path:       configPath,
	}}
}
