package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// ---------- MCP resources ----------

// mcpResourceList 是 resources/list 的响应结构。
type mcpResourceList struct {
	Resources []mcpResource `json:"resources"`
}

type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ListResources 发现 server 暴露的资源。
func (c *MCPClient) ListResources(ctx context.Context) ([]mcpResource, error) {
	raw, err := c.call(ctx, "resources/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var list mcpResourceList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp parse resources: %w", err)
	}
	return list.Resources, nil
}

// mcpResourceContent 是 resources/read 的响应结构。
type mcpResourceContent struct {
	Contents []struct {
		URI      string `json:"uri"`
		Text     string `json:"text,omitempty"`
		Blob     string `json:"blob,omitempty"`
		MimeType string `json:"mimeType,omitempty"`
	} `json:"contents"`
}

// ReadResource 读取一个 MCP 资源的内容。
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	raw, err := c.call(ctx, "resources/read", map[string]any{"uri": uri})
	if err != nil {
		return "", err
	}
	var content mcpResourceContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return "", fmt.Errorf("mcp parse resource content: %w", err)
	}
	if len(content.Contents) == 0 {
		return "", fmt.Errorf("mcp resource %s: empty", uri)
	}
	return content.Contents[0].Text, nil
}

// ---------- MCP prompts ----------

// mcpPromptList 是 prompts/list 的响应结构。
type mcpPromptList struct {
	Prompts []mcpPrompt `json:"prompts"`
}

type mcpPrompt struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Arguments   []struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required,omitempty"`
	} `json:"arguments,omitempty"`
}

// ListPrompts 发现 server 暴露的 prompt 模板。
func (c *MCPClient) ListPrompts(ctx context.Context) ([]mcpPrompt, error) {
	raw, err := c.call(ctx, "prompts/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var list mcpPromptList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp parse prompts: %w", err)
	}
	return list.Prompts, nil
}

// mcpPromptResult 是 prompts/get 的响应结构。
type mcpPromptResult struct {
	Messages []struct {
		Role    string `json:"role"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

// GetPrompt 获取一个 prompt 模板渲染后的消息。
func (c *MCPClient) GetPrompt(ctx context.Context, name string, args map[string]string) (string, error) {
	params := map[string]any{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}
	raw, err := c.call(ctx, "prompts/get", params)
	if err != nil {
		return "", err
	}
	var result mcpPromptResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("mcp parse prompt result: %w", err)
	}
	if len(result.Messages) == 0 {
		return "", fmt.Errorf("mcp prompt %s: empty", name)
	}
	return result.Messages[0].Content.Text, nil
}
