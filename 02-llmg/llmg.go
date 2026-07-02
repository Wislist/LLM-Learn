 package llmg
 
 import (
 	"bytes"
 	"context"
 	"encoding/json"
 	"fmt"
 	"io"
 	"net/http"
 )
 
 // Provider 统一接口。
 // 所有 LLM 服务商（DeepSeek、OpenRouter、Ollama 等）实现这个接口。
 type Provider interface {
 	// Name 返回服务商标识，如 "deepseek"
 	Name() string
 
 	// Chat 非流式调用，返回完整响应。
 	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
 
 	// ChatStream 流式调用，返回事件 channel。
 	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
 }
 
 // Client 是面向用户的入口。
 type Client struct {
 	provider Provider
 }
 
 // New 创建一个带指定 Provider 的客户端。
 // provider 用 WithDeepSeek / WithOpenRouter 等辅助函数创建。
 func New(provider Provider) *Client {
 	return &Client{provider: provider}
 }
 
 func (c *Client) ProviderName() string { return c.provider.Name() }
 
 func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
 	return c.provider.Chat(ctx, req)
 }
 
 func (c *Client) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
 	return c.provider.ChatStream(ctx, req)
 }
 
 // ---------- 底层 HTTP 请求封装 ----------
 
 // doRequest 发送 HTTP 请求并解析 JSON 响应。
 func doRequest(ctx context.Context, client *http.Client, baseURL, apiKey string, req *ChatRequest) (*ChatResponse, error) {
 	body, err := json.Marshal(req)
 	if err != nil {
 		return nil, fmt.Errorf("marshal request: %w", err)
 	}
 
 	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
 	if err != nil {
 		return nil, fmt.Errorf("create request: %w", err)
 	}
 	httpReq.Header.Set("Content-Type", "application/json")
 	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
 
 	resp, err := client.Do(httpReq)
 	if err != nil {
 		return nil, fmt.Errorf("http do: %w", err)
 	}
 	defer resp.Body.Close()
 
 	if resp.StatusCode != 200 {
 		bodyBytes, _ := io.ReadAll(resp.Body)
 		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, bodyBytes)
 	}
 
 	var chatResp ChatResponse
 	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
 		return nil, fmt.Errorf("decode response: %w", err)
 	}
 	return &chatResp, nil
 }
 
 // doStreamRequest 发送流式请求，返回 SSE 行 channel。
 func doStreamRequest(ctx context.Context, client *http.Client, baseURL, apiKey string, req *ChatRequest) (chan string, error) {
 	req.Stream = true
 
 	body, err := json.Marshal(req)
 	if err != nil {
 		return nil, fmt.Errorf("marshal request: %w", err)
 	}
 
 	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
 	if err != nil {
 		return nil, fmt.Errorf("create request: %w", err)
 	}
 	httpReq.Header.Set("Content-Type", "application/json")
 	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
 	httpReq.Header.Set("Accept", "text/event-stream")
 	httpReq.Header.Set("Cache-Control", "no-cache")
 	httpReq.Header.Set("Connection", "keep-alive")
 
 	resp, err := client.Do(httpReq)
 	if err != nil {
 		return nil, fmt.Errorf("http do: %w", err)
 	}
 
 	if resp.StatusCode != 200 {
 		bodyBytes, _ := io.ReadAll(resp.Body)
 		resp.Body.Close()
 		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, bodyBytes)
 	}
 
 	ch := make(chan string, 64)
 	go parseSSE(resp.Body, ch)
 	return ch, nil
 }
