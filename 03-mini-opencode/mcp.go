package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/wislist/llmg"
)

// ---------- MCP stdio client ----------

// MCPClient 管理一个 MCP server 子进程，通过 JSON-RPC 2.0 over stdio 通信。
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Reader
	mu     sync.Mutex
	nextID int64
	// pending 保存等待响应的请求
	pending map[int64]chan jsonRPCResponse
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCErr     `json:"error,omitempty"`
}

type jsonRPCErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewMCPClient(command string, args ...string) (*MCPClient, error) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp start: %w", err)
	}
	c := &MCPClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: map[int64]chan jsonRPCResponse{},
	}
	go c.readLoop()
	return c, nil
}

func (c *MCPClient) Close() error {
	c.stdin.Close()
	return c.cmd.Wait()
}

// readLoop 持续读 stdout，把响应分发给 pending 的调用方。
func (c *MCPClient) readLoop() {
	sc := bufio.NewReader(c.stdout)
	dec := json.NewDecoder(sc)
	for {
		var resp jsonRPCResponse
		if err := dec.Decode(&resp); err != nil {
			return
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

// call 发一个 JSON-RPC 请求并等待响应。
func (c *MCPClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan jsonRPCResponse, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	c.mu.Lock()
	_, err = c.stdin.Write(data)
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mcp write: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Initialize 完成 MCP 握手。
func (c *MCPClient) Initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mini-opencode", "version": "0.3"},
	})
	return err
}

// mcpToolList 是 tools/list 的响应结构。
type mcpToolList struct {
	Tools []mcpToolDef `json:"tools"`
}

type mcpToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListTools 发现 server 暴露的工具。
func (c *MCPClient) ListTools(ctx context.Context) ([]mcpToolDef, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var list mcpToolList
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("mcp parse tools: %w", err)
	}
	return list.Tools, nil
}

// mcpCallResult 是 tools/call 的响应结构。
type mcpCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// CallTool 调用一个远程工具。
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	raw, err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var res mcpCallResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", fmt.Errorf("mcp parse result: %w", err)
	}
	out := ""
	for _, c := range res.Content {
		if c.Type == "text" {
			if out != "" {
				out += "\n"
			}
			out += c.Text
		}
	}
	if res.IsError {
		return "", fmt.Errorf("mcp tool error: %s", out)
	}
	return out, nil
}

// ---------- 把 MCP 工具适配成本地 Tool 接口 ----------

type mcpToolAdapter struct {
	client *MCPClient
	def    mcpToolDef
}

func (t *mcpToolAdapter) Name() string { return t.def.Name }
func (t *mcpToolAdapter) Description() string {
	if t.def.Description == "" {
		return "MCP tool: " + t.def.Name
	}
	return t.def.Description
}

func (t *mcpToolAdapter) Schema() llmg.Tool {
	var params any
	if len(t.def.InputSchema) > 0 {
		json.Unmarshal(t.def.InputSchema, &params)
	} else {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return llmg.Tool{
		Type: "function",
		Function: llmg.ToolFunction{
			Name:        t.def.Name,
			Description: t.def.Description,
			Parameters:  params,
		},
	}
}

func (t *mcpToolAdapter) Execute(args string) (string, error) {
	var raw json.RawMessage
	if args == "" {
		raw = json.RawMessage("{}")
	} else {
		raw = json.RawMessage(args)
	}
	return t.client.CallTool(context.Background(), t.def.Name, raw)
}

// LoadMCPFromConfig 从 config 加载所有 MCP server，启动子进程、握手、发现工具，
// 把工具注册进 registry。失败的服务端跳过并打印警告。
func LoadMCPFromConfig(registry *ToolRegistry, servers map[string]MCPServerConfig) {
	ctx := context.Background()
	for name, cfg := range servers {
		client, err := NewMCPClient(cfg.Command, cfg.Args...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] MCP %s 启动失败: %v\n", name, err)
			continue
		}
		if err := client.Initialize(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] MCP %s 握手失败: %v\n", name, err)
			client.Close()
			continue
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] MCP %s 发现工具失败: %v\n", name, err)
			client.Close()
			continue
		}
		for _, td := range tools {
			registry.Register(&mcpToolAdapter{client: client, def: td})
		}
		fmt.Fprintf(os.Stderr, "[mcp] %s: 加载 %d 个工具\n", name, len(tools))
	}
}
