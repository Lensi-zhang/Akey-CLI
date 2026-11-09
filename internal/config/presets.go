package config

// ProviderPreset 描述无需联网即可使用的服务商接入预设。
type ProviderPreset struct {
	ID             string
	Name           string
	Provider       string
	BaseURL        string
	ConsoleURL     string
	CredentialType CredentialType
}

var providerPresets = [...]ProviderPreset{
	{ID: "volcengine", Name: "火山方舟（普通 API Key）", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", ConsoleURL: "https://ark.volcengine.com/region:cn-beijing/apiKey", CredentialType: CredentialAPI},
	{ID: "volcengine-agent-plan", Name: "火山方舟 Agent Plan", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3", ConsoleURL: "https://ark.volcengine.com/region:cn-beijing/openManagement?LLM=%7B%7D&OpenModelVisible=false&advancedActiveKey=agentPlan", CredentialType: CredentialAgentPlan},
	{ID: "volcengine-coding-plan", Name: "火山方舟 Coding Plan", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", ConsoleURL: "https://ark.volcengine.com/region:cn-beijing/openManagement?LLM=%7B%7D&advancedActiveKey=subscribe", CredentialType: CredentialCodingPlan},
	{ID: "openai", Name: "OpenAI", Provider: "openai", BaseURL: "https://api.openai.com/v1", ConsoleURL: "https://platform.openai.com/api-keys", CredentialType: CredentialAPI},
	{ID: "deepseek", Name: "DeepSeek", Provider: "deepseek", BaseURL: "https://api.deepseek.com", ConsoleURL: "https://platform.deepseek.com/api_keys", CredentialType: CredentialAPI},
	{ID: "moonshot", Name: "Moonshot AI", Provider: "moonshot", BaseURL: "https://api.moonshot.cn/v1", ConsoleURL: "https://platform.moonshot.cn/console/api-keys", CredentialType: CredentialAPI},
	{ID: "openrouter", Name: "OpenRouter", Provider: "openrouter", BaseURL: "https://openrouter.ai/api/v1", ConsoleURL: "https://openrouter.ai/settings/keys", CredentialType: CredentialAPI},
	{ID: "custom", Name: "Custom（其他 OpenAI 兼容服务）", Provider: "custom", CredentialType: CredentialAPI},
}

func ProviderPresets() []ProviderPreset {
	presets := make([]ProviderPreset, len(providerPresets))
	copy(presets, providerPresets[:])
	return presets
}
