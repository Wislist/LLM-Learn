 package llmg
 
 // ---------- 请求类型 ----------
 
 type Role string
 
 const (
 	RoleSystem    Role = "system"
 	RoleUser      Role = "user"
 	RoleAssistant Role = "assistant"
 	RoleTool      Role = "tool"
 )
 
type Message struct {
 	Role    Role   `json:"role"`
 	Content string `json:"content"`
 	ToolCallID string    `json:"tool_call_id,omitempty"`
 	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
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
 	Model       string    `json:"model"`
 	Messages    []Message `json:"messages"`
 	Stream      bool      `json:"stream"`
 	Tools       []Tool    `json:"tools,omitempty"`
 	Temperature *float64  `json:"temperature,omitempty"`
 	MaxTokens   *int      `json:"max_tokens,omitempty"`
 }
 
 // ---------- 响应类型 ----------
 
 type ToolCall struct {
 	ID       string `json:"id"`
 	Type     string `json:"type"`
 	Function struct {
 		Name      string `json:"name"`
 		Arguments string `json:"arguments"`
 	} `json:"function"`
 }
 
 type ChatResponse struct {
 	ID      string  `json:"id"`
 	Model   string  `json:"model"`
 	Created int64   `json:"created"`
 	Choices []Choice `json:"choices"`
 	Usage  *Usage  `json:"usage,omitempty"`
 }
 
 type Choice struct {
 	Index        int      `json:"index"`
 	Message      ResponseMessage `json:"message"`
 	FinishReason string          `json:"finish_reason"`
 }
 
 type ResponseMessage struct {
 	Role      Role       `json:"role"`
 	Content   string     `json:"content"`
 	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
 }
 
 type Usage struct {
 	PromptTokens     int `json:"prompt_tokens"`
 	CompletionTokens int `json:"completion_tokens"`
 	TotalTokens      int `json:"total_tokens"`
 }
 
 // ---------- 流式事件 ----------
 
 type StreamEventType int
 
 const (
 	EventText     StreamEventType = iota // 文本增量
 	EventToolCall                        // tool call 信息
 	EventDone                            // 流结束
 	EventError                           // 错误
 )
 
 type StreamEvent struct {
 	Type     StreamEventType
 	Content  string     // 文本增量（EventText）
 	ToolCall *ToolCall  // 工具调用（EventToolCall）
 	Err      error      // 错误（EventError）
 	Usage    *Usage     // 用量（EventDone）
 }
