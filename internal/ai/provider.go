package ai

// ProviderClient is the interface all providers should implement.
type ProviderClient interface {
	SendPrompt(payload PromptPayload) (string, error)
}

// ProviderType is an enum-like type for different providers.
type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderClaude ProviderType = "claude"
)

// PromptPayload represents the structured input for an LLM provider.
type PromptPayload struct {
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
}
