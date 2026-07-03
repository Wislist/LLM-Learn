// Package llm wraps an OpenAI-compatible chat model (DeepSeek by default)
// using langchaingo. Mirrors the style of 02-llmg-with-langchaingo/client.go.
package llm

import (
	"context"
	"errors"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// Client wraps a langchaingo LLM model.
type Client struct {
	model llms.Model
}

// Config configures the LLM backend.
type Config struct {
	Provider    string // "deepseek" | "openai" | "openrouter" | "ollama"
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float32
}

// NewClient creates a Client from config.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		cfg.APIKey = "not-needed"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL(cfg.Provider)
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel(cfg.Provider)
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}

	model, err := openai.New(
		openai.WithModel(cfg.Model),
		openai.WithBaseURL(cfg.BaseURL),
		openai.WithToken(cfg.APIKey),
		openai.WithAPIType(openai.APITypeOpenAI),
	)
	if err != nil {
		return nil, fmt.Errorf("llm new client: %w", err)
	}
	return &Client{model: model}, nil
}

// Chat sends a non-streaming request and returns the full response text.
func (c *Client) Chat(ctx context.Context, messages []llms.MessageContent, opts ...llms.CallOption) (string, error) {
	resp, err := c.model.GenerateContent(ctx, messages, opts...)
	if err != nil {
		return "", fmt.Errorf("llm chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("llm chat: no choices")
	}
	return resp.Choices[0].Content, nil
}

// Stream sends a streaming request. The callback is invoked for each token chunk.
// Returns the full accumulated text once the stream ends.
func (c *Client) Stream(ctx context.Context, messages []llms.MessageContent, onDelta func(chunk string), opts ...llms.CallOption) (string, error) {
	streamOpts := append(opts, llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		if onDelta != nil {
			onDelta(string(chunk))
		}
		return nil
	}))
	resp, err := c.model.GenerateContent(ctx, messages, streamOpts...)
	if err != nil {
		return "", fmt.Errorf("llm stream: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("llm stream: no choices")
	}
	return resp.Choices[0].Content, nil
}

// Model returns the underlying langchaingo model for advanced use.
func (c *Client) Model() llms.Model { return c.model }

func defaultBaseURL(provider string) string {
	switch provider {
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
	switch provider {
	case "deepseek":
		return "deepseek-chat"
	case "ollama":
		return "qwen2.5"
	default:
		return "gpt-4o"
	}
}
