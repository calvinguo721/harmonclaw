// Package providers defines a unified LLM provider interface and shared request/response types.
package providers

import "context"

// ChatMessage is OpenAI-compatible (including tool roles).
type ChatMessage struct {
	Role         string     `json:"role"`
	Content      string     `json:"content,omitempty"`
	Name         string     `json:"name,omitempty"`
	ToolCallID   string     `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
}

// ChatRequest is a unified chat completion request.
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Model       string        `json:"model,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []Tool        `json:"tools,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// ChatResponse is a unified non-streaming chat response.
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Model        string     `json:"model"`
	Usage        Usage      `json:"usage"`
	FinishReason string     `json:"finish_reason"`
}

// Tool is an OpenAI-style tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ToolCall is a model-emitted tool invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc holds the function name and JSON arguments.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage reports token counts from the provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider is implemented by each LLM backend (DeepSeek, Kimi, Qwen, ...).
type Provider interface {
	Name() string
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Available() bool
	// ChatStream streams completion tokens (optional per backend; DeepSeek implements it).
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan string, error)
}
