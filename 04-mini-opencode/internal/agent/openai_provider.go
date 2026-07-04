package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type OpenAICompatibleProvider struct {
	config OpenAICompatibleConfig
	client *http.Client
}

func NewOpenAICompatibleProvider(config OpenAICompatibleConfig) (*OpenAICompatibleProvider, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	return &OpenAICompatibleProvider{
		config: config,
		client: &http.Client{Timeout: 120 * time.Second},
	}, nil
}

func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req Request) (AssistantResponse, error) {
	body := openAIChatRequest{
		Model:    p.config.Model,
		Messages: convertOpenAIMessages(req),
		Tools:    convertOpenAITools(req.Tools),
	}
	data, err := json.Marshal(body)
	if err != nil {
		return AssistantResponse{}, err
	}

	url := strings.TrimRight(p.config.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return AssistantResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return AssistantResponse{}, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return AssistantResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AssistantResponse{}, fmt.Errorf("provider error %d: %s", resp.StatusCode, strings.TrimSpace(string(respData)))
	}

	var out openAIChatResponse
	if err := json.Unmarshal(respData, &out); err != nil {
		return AssistantResponse{}, err
	}
	if len(out.Choices) == 0 {
		return AssistantResponse{}, fmt.Errorf("provider returned no choices")
	}
	msg := out.Choices[0].Message
	return AssistantResponse{
		Content:   msg.Content,
		ToolCalls: convertAgentToolCalls(msg.ToolCalls),
	}, nil
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAICallFunction `json:"function"`
}

type openAICallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func convertOpenAIMessages(req Request) []openAIMessage {
	messages := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, openAIMessage{Role: string(RoleSystem), Content: req.SystemPrompt})
	}
	for _, msg := range req.Messages {
		messages = append(messages, openAIMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			ToolCalls:  convertOpenAIToolCalls(msg.ToolCalls),
		})
	}
	return messages
}

func convertOpenAITools(specs []ToolSpec) []openAITool {
	tools := make([]openAITool, 0, len(specs))
	for _, spec := range specs {
		description := spec.Description
		if spec.Prompt != "" {
			description = strings.TrimSpace(description + "\n\n" + spec.Prompt)
		}
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        spec.Name,
				Description: description,
				Parameters:  spec.InputSchema,
			},
		})
	}
	return tools
}

func convertOpenAIToolCalls(calls []ToolCall) []openAIToolCall {
	out := make([]openAIToolCall, 0, len(calls))
	for _, call := range calls {
		args := string(call.Arguments)
		if args == "" {
			args = "{}"
		}
		out = append(out, openAIToolCall{
			ID:   call.ID,
			Type: "function",
			Function: openAICallFunction{
				Name:      call.Name,
				Arguments: args,
			},
		})
	}
	return out
}

func convertAgentToolCalls(calls []openAIToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		args := call.Function.Arguments
		if args == "" {
			args = "{}"
		}
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return out
}
