 package main
 
 import (
 	"bufio"
 	"bytes"
 	"encoding/json"
 	"fmt"
 	"io"
 	"net/http"
 	"os"
 	"strings"
 )
 
 const deepseekURL = "https://api.deepseek.com/chat/completions"
 
 // ---------- request types ----------
 
type Message struct {
 	Role    string `json:"role"`
 	Content string `json:"content"`
 	ToolCallID string    `json:"tool_call_id,omitempty"`
 	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
 }
 
 type Tool struct {
 	Type     string       `json:"type"`
 	Function ToolFunction `json:"function"`
 }
 
 type ToolFunction struct {
 	Name        string `json:"name"`
 	Description string `json:"description"`
 	Parameters  any    `json:"parameters"`
 }
 
 type ChatRequest struct {
 	Model    string    `json:"model"`
 	Messages []Message `json:"messages"`
 	Stream   bool      `json:"stream"`
 	Tools    []Tool    `json:"tools,omitempty"`
 }
 
 // ---------- response types ----------
 
 type ChatResponse struct {
 	ID      string   `json:"id"`
 	Choices []Choice `json:"choices"`
 }
 
 type Choice struct {
 	Delta        Message     `json:"delta"`
 	Message      Message     `json:"message"`
 	FinishReason *string     `json:"finish_reason"`
 	Index        int         `json:"index"`
 }
 
 // ---------- tool call types ----------
 
 type ToolCall struct {
 	ID       string         `json:"id"`
 	Type     string         `json:"type"`
 	Function ToolCallFunc   `json:"function"`
 }
 
 type ToolCallFunc struct {
 	Name      string `json:"name"`
 	Arguments string `json:"arguments"`
 }
 
 type ToolCallMessage struct {
 	Role      string     `json:"role"`
 	Content   *string    `json:"content"`
 	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
 }
 
 type ToolResultMessage struct {
 	Role       string `json:"role"`
 	ToolCallID string `json:"tool_call_id"`
 	Content    string `json:"content"`
 }
 
 // ---------- main ----------
 
 func main() {
 	key := os.Getenv("DEEPSEEK_API_KEY")
 	if key == "" {
 		fmt.Fprintln(os.Stderr, "请设置 DEEPSEEK_API_KEY 环境变量")
 		fmt.Fprintln(os.Stderr, "  export DEEPSEEK_API_KEY=sk-xxxxxxxx")
 		os.Exit(1)
 	}
 
 	fmt.Println("=== 1. 流式 Chat (Streaming) ===\n")
 	streamChat(key, []Message{
 		{Role: "system", Content: "你是一个 Go 导师，回答简洁"},
 		{Role: "user", Content: "用一句话解释 Go 的 goroutine"},
 	})
 
 	fmt.Println("\n\n=== 2. 非流式 + Tool Calling ===\n")
 	toolCallDemo(key)
 }
 
 // ---------- 1. streaming ----------
 
 func streamChat(apiKey string, messages []Message) {
 	body := ChatRequest{
 		Model:    "deepseek-chat",
 		Messages: messages,
 		Stream:   true,
 	}
 
 	reqBody, _ := json.Marshal(body)
 	req, _ := http.NewRequest("POST", deepseekURL, bytes.NewReader(reqBody))
 	req.Header.Set("Authorization", "Bearer "+apiKey)
 	req.Header.Set("Content-Type", "application/json")
 	req.Header.Set("Accept", "text/event-stream")
 
 	resp, err := http.DefaultClient.Do(req)
 	if err != nil {
 		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
 		return
 	}
 	defer resp.Body.Close()
 
 	if resp.StatusCode != 200 {
 		bodyBytes, _ := io.ReadAll(resp.Body)
 		fmt.Fprintf(os.Stderr, "API 错误 %d: %s\n", resp.StatusCode, bodyBytes)
 		return
 	}
 
 	// SSE 解析: "data: {json}\n\n"
 	scanner := bufio.NewScanner(resp.Body)
 	for scanner.Scan() {
 		line := scanner.Text()
 
 		if !strings.HasPrefix(line, "data: ") {
 			continue
 		}
 		data := strings.TrimPrefix(line, "data: ")
 
 		if data == "[DONE]" {
 			break
 		}
 
 		var chunk ChatResponse
 		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
 			continue
 		}
 
 		for _, ch := range chunk.Choices {
 			fmt.Print(ch.Delta.Content)
 		}
 	}
 
 	if err := scanner.Err(); err != nil {
 		fmt.Fprintf(os.Stderr, "\n读取错误: %v\n", err)
 	}
 }
 
 // ---------- 2. tool calling demo ----------
 
 func toolCallDemo(apiKey string) {
 	// 定义两个工具
 	tools := []Tool{
 		{
 			Type: "function",
 			Function: ToolFunction{
 				Name:        "get_weather",
 				Description: "查询指定城市的天气",
 				Parameters: map[string]any{
 					"type": "object",
 					"properties": map[string]any{
 						"city": map[string]any{"type": "string", "description": "城市名"},
 					},
 					"required": []string{"city"},
 				},
 			},
 		},
 		{
 			Type: "function",
 			Function: ToolFunction{
 				Name:        "get_date",
 				Description: "获取今天的日期",
 				Parameters: map[string]any{
 					"type":       "object",
 					"properties": map[string]any{},
 				},
 			},
 		},
 	}
 
 	// Agent Loop — 一次 tool call
 	messages := []Message{
 		{Role: "system", Content: "你是一个助手。如果需要查询信息，请使用工具。回答要简洁。"},
 		{Role: "user", Content: "北京今天天气怎么样？今天几号？"},
 	}
 
 	// 第一轮：问 LLM
 	resp := nonStreamChat(apiKey, messages, tools)
 
 	// 检查是否有 tool_calls
 	var toolCallMsg ToolCallMessage
 	json.Unmarshal([]byte(resp), &toolCallMsg)
 
 	if len(toolCallMsg.ToolCalls) == 0 {
 		fmt.Println("LLM 没有调工具，直接回复:", resp)
 		return
 	}
 
 	fmt.Printf("LLM 调用了 %d 个工具:\n", len(toolCallMsg.ToolCalls))
 
 	// 关键修复 1：把 assistant 的 tool_calls 消息原样加入对话历史
 	messages = append(messages, Message{
 		Role:      "assistant",
 		Content:   "",
 		ToolCalls: toolCallMsg.ToolCalls,
 	})
 
 	for _, tc := range toolCallMsg.ToolCalls {
 		fmt.Printf("  → %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
 
 		// 模拟执行工具
 		var result string
 		switch tc.Function.Name {
 		case "get_weather":
 			result = `{"temperature": 28, "condition": "晴", "city": "北京"}`
 		case "get_date":
 			result = `{"date": "2026-07-02"}`
 		}
 
 		// 关键修复 2：tool 结果必须带 tool_call_id，与 assistant 发出的调用对应
 		messages = append(messages, Message{
 			Role:       "tool",
 			ToolCallID: tc.ID,
 			Content:    result,
 		})
 	}
 
 	fmt.Println("\n工具执行完毕，把结果发回 LLM...\n")
 
 	// 第二轮：把工具结果发给 LLM
 	finalResp := nonStreamChat(apiKey, messages, nil)
 	var finalMsg ToolCallMessage
 	json.Unmarshal([]byte(finalResp), &finalMsg)
 
 	if finalMsg.Content != nil {
 		fmt.Println("最终回复:", *finalMsg.Content)
 	}
 }
 
 // nonStreamChat 非流式调用（Tool Calling 需要非流式）
 func nonStreamChat(apiKey string, messages []Message, tools []Tool) string {
 	body := ChatRequest{
 		Model:    "deepseek-chat",
 		Messages: messages,
 		Stream:   false,
 		Tools:    tools,
 	}
 
 	reqBody, _ := json.Marshal(body)
 	req, _ := http.NewRequest("POST", deepseekURL, bytes.NewReader(reqBody))
 	req.Header.Set("Authorization", "Bearer "+apiKey)
 	req.Header.Set("Content-Type", "application/json")
 
 	resp, err := http.DefaultClient.Do(req)
 	if err != nil {
 		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
 		return ""
 	}
 	defer resp.Body.Close()
 
 	bodyBytes, _ := io.ReadAll(resp.Body)
 
 	if resp.StatusCode != 200 {
 		fmt.Fprintf(os.Stderr, "API 错误 %d: %s\n", resp.StatusCode, bodyBytes)
 		return ""
 	}
 
 	var chatResp ChatResponse
 	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
 		fmt.Fprintf(os.Stderr, "解析失败: %v\n", err)
 		return ""
 	}
 
 	if len(chatResp.Choices) == 0 {
 		return ""
 	}
 
 	// 把完整响应序列化成 JSON 返回，方便上一层判断 tool_calls
 	msgBytes, _ := json.Marshal(chatResp.Choices[0].Message)
 	return string(msgBytes)
 }
 
