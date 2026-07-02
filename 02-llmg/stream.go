 package llmg
 
 import (
 	"bufio"
 	"encoding/json"
 	"io"
 	"strings"
 )
 
 // ---------- SSE 原始行类型 ----------
 
 type streamChunk struct {
 	ID      string         `json:"id"`
 	Object  string         `json:"object"`
 	Created int64          `json:"created"`
 	Model   string         `json:"model"`
 	Choices []streamChoice `json:"choices"`
 	Usage   *Usage         `json:"usage,omitempty"`
 }
 
 // toolCallDelta 处理流式模式下的增量 tool_call —— 不能用 ToolCall 套用。
 type toolCallDelta struct {
 	Index    *int   `json:"index"`
 	ID       string `json:"id,omitempty"`
 	Type     string `json:"type,omitempty"`
 	Function struct {
 		Name      *string `json:"name,omitempty"`
 		Arguments *string `json:"arguments,omitempty"`
 	} `json:"function,omitempty"`
 }
 
 type streamChoice struct {
 	Index int `json:"index"`
 	Delta struct {
 		Role           string          `json:"role,omitempty"`
 		Content        string          `json:"content,omitempty"`
 		ToolCallDeltas []toolCallDelta `json:"tool_calls,omitempty"`
 	} `json:"delta"`
 	FinishReason *string `json:"finish_reason"`
 }
 
 // ---------- SSE 逐行读取 ----------
 
 // parseSSE 从 body 读取 SSE 流，按行发到 rawCh。
 func parseSSE(body io.ReadCloser, rawCh chan<- string) {
 	defer body.Close()
 	defer close(rawCh)
 
 	scanner := bufio.NewScanner(body)
 	for scanner.Scan() {
 		line := scanner.Text()
 		if !strings.HasPrefix(line, "data: ") {
 			continue
 		}
 		data := strings.TrimPrefix(line, "data: ")
 		if data == "[DONE]" {
 			return
 		}
 		rawCh <- data
 	}
 }
 
 // ---------- 流式事件管道 ----------
 
 // AccumulateStreamEvents 将原始 SSE 行 channel 转为 StreamEvent channel。
 // 流式 tool_calls 是逐 chunk 增量推送的，按 index 归并后一次性发出完整 ToolCall。
 func AccumulateStreamEvents(rawCh <-chan string) <-chan StreamEvent {
 	eventCh := make(chan StreamEvent, 64)
 	go func() {
 		defer close(eventCh)
 		acc := &tcAccumulator{pending: map[int]*pendingTC{}}
 		for data := range rawCh {
 			for _, evt := range acc.feed(data) {
 				eventCh <- evt
 			}
 		}
 		for _, evt := range acc.flush() {
 			eventCh <- evt
 		}
 	}()
 	return eventCh
 }
 
 // ---------- tool call 累加器 ----------
 
 type pendingTC struct {
 	id   string
 	typ  string
 	name string
 	args strings.Builder
 }
 
 type tcAccumulator struct {
 	pending map[int]*pendingTC
 	textBuf strings.Builder
 }
 
 func (a *tcAccumulator) feed(data string) []StreamEvent {
 	var chunk streamChunk
 	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
 		return []StreamEvent{{Type: EventError, Err: err}}
 	}
 
 	if len(chunk.Choices) == 0 {
 		if chunk.Usage != nil {
 			return []StreamEvent{{Type: EventDone, Usage: chunk.Usage}}
 		}
 		return nil
 	}
 
 	choice := chunk.Choices[0]
 
 	if choice.FinishReason != nil {
 		return a.finish(chunk.Usage)
 	}
 
 	var events []StreamEvent
 
 	for _, d := range choice.Delta.ToolCallDeltas {
 		idx := 0
 		if d.Index != nil {
 			idx = *d.Index
 		}
 		tc, ok := a.pending[idx]
 		if !ok {
 			tc = &pendingTC{}
 			a.pending[idx] = tc
 		}
 		if d.ID != "" {
 			tc.id = d.ID
 		}
 		if d.Type != "" {
 			tc.typ = d.Type
 		}
 		if d.Function.Name != nil && *d.Function.Name != "" {
 			tc.name = *d.Function.Name
 		}
 		if d.Function.Arguments != nil {
 			tc.args.WriteString(*d.Function.Arguments)
 		}
 	}
 
 	if choice.Delta.Content != "" {
 		a.textBuf.WriteString(choice.Delta.Content)
 		events = append(events, StreamEvent{Type: EventText, Content: choice.Delta.Content})
 	}
 
 	return events
 }
 
func (a *tcAccumulator) finish(u *Usage) []StreamEvent {
 	var events []StreamEvent
 	for _, tc := range a.flushPending() {
 		events = append(events, StreamEvent{Type: EventToolCall, ToolCall: tc})
 	}
 	events = append(events, StreamEvent{Type: EventDone, Usage: u})
 	return events
 }
 
 func (a *tcAccumulator) flushPending() []*ToolCall {
 	out := make([]*ToolCall, 0, len(a.pending))
 	for _, tc := range a.pending {
 		if tc.name != "" {
 			out = append(out, &ToolCall{
 				ID:   tc.id,
 				Type: tc.typ,
 				Function: struct {
 					Name      string `json:"name"`
 					Arguments string `json:"arguments"`
 				}{
 					Name:      tc.name,
 					Arguments: tc.args.String(),
 				},
 			})
 		}
 	}
 	a.pending = map[int]*pendingTC{}
 	return out
 }
 
 func (a *tcAccumulator) flush() []StreamEvent {
 	var events []StreamEvent
 	for _, tc := range a.flushPending() {
 		events = append(events, StreamEvent{Type: EventToolCall, ToolCall: tc})
 	}
 	return events
 }
