package config

// ProviderPreset 描述无需联网即可使用的服务商接入预设。
type ProviderPreset struct {
	ID             string
	Name           string
	Provider       string
	BaseURL        string
	CredentialType CredentialType
}

var providerPresets = [...]ProviderPreset{
	{ID: "volcengine", Name: "火山方舟（普通 API Key）", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/v3", CredentialType: CredentialAPI},
	{ID: "volcengine-agent-plan", Name: "火山方舟 Agent Plan", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3", CredentialType: CredentialAgentPlan},
	{ID: "volcengine-coding-plan", Name: "火山方舟 Coding Plan", Provider: "volcengine", BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", CredentialType: CredentialCodingPlan},
	{ID: "openai", Name: "OpenAI", Provider: "openai", BaseURL: "https://api.openai.com/v1", CredentialType: CredentialAPI},
	{ID: "deepseek", Name: "DeepSeek", Provider: "deepseek", BaseURL: "https://api.deepseek.com", CredentialType: CredentialAPI},
	{ID: "moonshot", Name: "Moonshot AI", Provider: "moonshot", BaseURL: "https://api.moonshot.cn/v1", CredentialType: CredentialAPI},
	{ID: "openrouter", Name: "OpenRouter", Provider: "openrouter", BaseURL: "https://openrouter.ai/api/v1", CredentialType: CredentialAPI},
	{ID: "custom", Name: "Custom（其他 OpenAI 兼容服务）", Provider: "custom", CredentialType: CredentialAPI},
}

func ProviderPresets() []ProviderPreset {
	presets := make([]ProviderPreset, len(providerPresets))
	copy(presets, providerPresets[:])
	return presets
}
