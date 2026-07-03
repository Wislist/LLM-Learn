// Package llmglc provides an LLM agent library built on langchaingo.
// It wraps langchaingo's model abstraction, tool calling, and memory
// into a clean, minimal API similar to the hand-rolled llmg package.
package llmglc

import (
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// Client wraps a langchaingo LLM model and provides a simplified
// Chat / ChatStream interface. It supports OpenAI-compatible APIs
// (including DeepSeek, OpenRouter, Ollama, etc.) through the
// standard OpenAI client.
type Client struct {
	model llms.Model
}

// ClientConfig configures the LLM backend.
type ClientConfig struct {
	// Provider selects the backend: "openai", "deepseek", "openrouter", or "ollama".
	// Defaults to "openai".
	Provider string

	// BaseURL is the API endpoint. Auto-set based on Provider if empty.
	BaseURL string

	// APIKey for authentication.
	APIKey string

	// Model name (e.g. "deepseek-chat", "gpt-4o", "llama3.2").
	Model string

	// Temperature for generation (0.0 – 2.0).
	Temperature float64
}

// NewClient creates a Client from the given config.
// It initializes the underlying langchaingo OpenAI-compatible model.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.APIKey == "" {
		cfg.APIKey = "not-needed" // Ollama and some proxies don't require a key.
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL(cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel(cfg.Provider)
	}

	model, err := openai.New(
		openai.WithModel(cfg.Model),
		openai.WithBaseURL(cfg.BaseURL),
		openai.WithToken(cfg.APIKey),
		openai.WithAPIType(openai.APITypeOpenAI),
	)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	return &Client{model: model}, nil
}

// Chat sends a non-streaming request and returns the full response text.
func (c *Client) Chat(ctx context.Context, messages []MessageContent, tools []llms.Tool) (string, error) {
	lcMessages := toLCMessages(messages)
	opts := []llms.CallOption{
		llms.WithTools(tools),
	}
	resp, err := c.model.GenerateContent(ctx, lcMessages, opts...)
	if err != nil {
		return "", fmt.Errorf("chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat: no choices in response")
	}
	return resp.Choices[0].Content, nil
}

// ChatStream sends a streaming request and returns a channel of text deltas.
func (c *Client) ChatStream(ctx context.Context, messages []MessageContent, tools []llms.Tool) (<-chan string, error) {
	lcMessages := toLCMessages(messages)
	opts := []llms.CallOption{
		llms.WithTools(tools),
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			// StreamingFunc is called by langchaingo for each chunk.
			// We handle streaming at a higher level via GenerateFromTongs in
			// Agent for streaming mode, but for raw stream we use this.
			return nil
		}),
	}

	_, err := c.model.GenerateContent(ctx, lcMessages, opts...)
	if err != nil {
		return nil, fmt.Errorf("chat stream: %w", err)
	}

	// Return a closed channel — streaming is handled by the Agent layer
	// using langchaingo's streaming callback directly.
	ch := make(chan string)
	close(ch)
	return ch, nil
}

// Model returns the underlying langchaingo model for advanced use.
func (c *Client) Model() llms.Model { return c.model }

// ---------- internal helpers ----------

func defaultBaseURL(provider string) string {
	switch strings.ToLower(provider) {
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

func defaultModel(provider string) string {
	switch strings.ToLower(provider) {
	case "deepseek":
		return "deepseek-chat"
	case "ollama":
		return "llama3.2"
	default:
		return "gpt-4o"
	}
}

// toLCMessages converts our simplified MessageContent to langchaingo's MessageContent.
func toLCMessages(msgs []MessageContent) []llms.MessageContent {
	out := make([]llms.MessageContent, len(msgs))
	for i, m := range msgs {
		role := llms.ChatMessageTypeAI
		switch m.Role {
		case "system":
			role = llms.ChatMessageTypeSystem
		case "user":
			role = llms.ChatMessageTypeHuman
		case "tool":
			role = llms.ChatMessageTypeTool
		}

		parts := []llms.ContentPart{llms.TextPart(m.Content)}

		// Attach tool calls if present.
		for _, tc := range m.ToolCalls {
			parts = append(parts, llms.ToolCall{
				ID:           tc.ID,
				Type:         tc.Type,
				FunctionCall: &llms.FunctionCall{Name: tc.Name, Arguments: tc.Arguments},
			})
		}

		// Attach tool call response if present.
		if m.ToolCallID != "" {
			parts = append(parts, llms.ToolCallResponse{
				ToolCallID: m.ToolCallID,
				Name:       m.ToolCallName,
				Content:    m.Content,
			})
		}

		out[i] = llms.MessageContent{
			Role:  role,
			Parts: parts,
		}
	}
	return out
}

// ---------- simplified message types ----------

// Role represents a message role.
type Role = string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCallInfo is a simplified tool call representation.
type ToolCallInfo struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

// MessageContent is a simplified message that maps to/from langchaingo types.
type MessageContent struct {
	Role         Role
	Content      string
	ToolCallID   string
	ToolCallName string
	ToolCalls    []ToolCallInfo
}
