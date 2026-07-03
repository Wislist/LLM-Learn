// Package rag wires retrieval + prompt assembly + streaming chat.
//
// Two retrieval paths (Hybrid RAG):
//   - documents/quotes (Milvus vector search) for narrative & quote questions
//   - sales (DuckDB text-to-sql) for chart/stat questions
//
// Routing is keyword-based for the MVP: questions containing chart/stats
// keywords go to the SQL path; everything else goes to the vector path.
package rag

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/embedder"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/llm"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/milvus"
	"github.com/wislist/llm-learn/05-rag-langchaingo/internal/store"
)

//go:embed prompts/quote_predict.md
var quotePredictPrompt string

//go:embed prompts/sales_sql.md
var salesSQLPrompt string

// Engine combines all the pieces needed to answer a user question.
type Engine struct {
	LLM      *llm.Client
	Embedder *embedder.Embedder
	Milvus   *milvus.Store
	DB       *store.Store
	TopK     int
}

// NewEngine constructs an Engine with sensible defaults.
func NewEngine(lc *llm.Client, e *embedder.Embedder, m *milvus.Store, db *store.Store) *Engine {
	return &Engine{LLM: lc, Embedder: e, Milvus: m, DB: db, TopK: 6}
}

// ChartSpec is returned to the frontend when the question routes to the SQL path.
type ChartSpec struct {
	Title   string             `json:"title"`
	SQL     string             `json:"sql"`
	Series  []store.ChartPoint `json:"series"`
	ChartType string           `json:"chart_type"` // "bar" | "line"
}

// Answer is the full result of a question.
type Answer struct {
	Text      string     `json:"text"`
	Context   []milvus.SearchHit `json:"context,omitempty"`
	Chart     *ChartSpec `json:"chart,omitempty"`
	Route     string     `json:"route"` // "vector" | "sql"
}

// Ask is the non-streaming entrypoint. For streaming use AskStream.
func (e *Engine) Ask(ctx context.Context, question string) (*Answer, error) {
	if isChartQuestion(question) {
		return e.answerViaSQL(ctx, question)
	}
	return e.answerViaVector(ctx, question, nil)
}

// AskStream streams the LLM answer via onDelta. The returned Answer holds
// the final text and any chart/context metadata (chart metadata is computed
// before streaming for the SQL route).
func (e *Engine) AskStream(ctx context.Context, question string, onDelta func(string)) (*Answer, error) {
	if isChartQuestion(question) {
		return e.answerViaSQL(ctx, question)
	}
	return e.answerViaVector(ctx, question, onDelta)
}

// ---------- vector path ----------

func (e *Engine) answerViaVector(ctx context.Context, question string, onDelta func(string)) (*Answer, error) {
	qvec, err := e.Embedder.Embed(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("rag embed query: %w", err)
	}

	docs, err := e.Milvus.SearchDocs(ctx, qvec, e.TopK)
	if err != nil {
		return nil, fmt.Errorf("rag search docs: %w", err)
	}
	quotes, err := e.Milvus.SearchQuotes(ctx, qvec, e.TopK)
	if err != nil {
		return nil, fmt.Errorf("rag search quotes: %w", err)
	}

	ctxBlock := buildContextBlock(docs, quotes)
	system := strings.ReplaceAll(quotePredictPrompt, "{{CONTEXT}}", ctxBlock)
	system = strings.ReplaceAll(system, "{{QUESTION}}", question)

	messages := []llms.MessageContent{
		{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextPart(system)}},
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart(question)}},
	}

	var full string
	if onDelta != nil {
		full, err = e.LLM.Stream(ctx, messages, onDelta)
	} else {
		full, err = e.LLM.Chat(ctx, messages)
	}
	if err != nil {
		return nil, fmt.Errorf("rag llm: %w", err)
	}

	hits := append(docs, quotes...)
	return &Answer{Text: full, Context: hits, Route: "vector"}, nil
}

func buildContextBlock(docs, quotes []milvus.SearchHit) string {
	var b strings.Builder
	if len(docs) > 0 {
		b.WriteString("【文档上下文】\n")
		for i, h := range docs {
			fmt.Fprintf(&b, "[%d] (来源: %s, 类型: %s)\n%s\n\n", i+1, h.Source, h.Fields["doc_type"], h.Content)
		}
	}
	if len(quotes) > 0 {
		b.WriteString("【历史报价记录】\n")
		for i, h := range quotes {
			fmt.Fprintf(&b, "[%d] 项目:%s 品名:%s 数量:%s 单价:%s 总价:%s (来源:%s)\n",
				i+1, h.Fields["project_name"], h.Fields["item_name"],
				h.Fields["qty"], h.Fields["unit_price"], h.Fields["total"], h.Source)
		}
	}
	if b.Len() == 0 {
		b.WriteString("（未检索到相关历史数据）")
	}
	return b.String()
}

// ---------- sql path ----------

func (e *Engine) answerViaSQL(ctx context.Context, question string) (*Answer, error) {
	messages := []llms.MessageContent{
		{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextPart(salesSQLPrompt)}},
		{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart(question)}},
	}
	sqlText, err := e.LLM.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("rag sql gen: %w", err)
	}
	sqlText = sanitizeSQL(sqlText)
	if sqlText == "" {
		return &Answer{Text: "无法生成有效查询，请换个问法。", Route: "sql"}, nil
	}

	points, qerr := e.DB.RunAggSQL(ctx, sqlText)
	chartType := "bar"
	if isTrendQuestion(question) {
		chartType = "line"
	}

	// Generate a short narrative summary alongside the chart.
	summaryPrompt := fmt.Sprintf("用户问题: %s\nSQL: %s\n结果数据: %v\n请用 2-3 句中文概括这组数据的要点，不要列表。",
		question, sqlText, points)
	var summary string
	if qerr == nil && len(points) > 0 {
		smsg := []llms.MessageContent{
			{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextPart("你是数据分析助手，用简洁中文概括数据要点。")}},
			{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart(summaryPrompt)}},
		}
		summary, _ = e.LLM.Chat(ctx, smsg, llms.WithTemperature(0.2))
	} else {
		summary = fmt.Sprintf("查询执行失败: %v", qerr)
	}

	spec := &ChartSpec{
		Title:     question,
		SQL:       sqlText,
		Series:    points,
		ChartType: chartType,
	}
	return &Answer{Text: summary, Chart: spec, Route: "sql"}, nil
}

// ---------- routing heuristics ----------

func isChartQuestion(q string) bool {
	q = strings.ToLower(q)
	keywords := []string{"图表", "趋势", "统计", "汇总", "销量", "销售额", "对比", "占比", "分布", "排名", "柱状", "折线", "画图"}
	for _, k := range keywords {
		if strings.Contains(q, k) {
			return true
		}
	}
	return false
}

func isTrendQuestion(q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(q, "趋势") || strings.Contains(q, "折线") || strings.Contains(q, "随时间") || strings.Contains(q, "月份")
}

// sanitizeSQL strips markdown fences and trailing prose, keeping only the
// first SELECT ... statement. It also forbids dangerous verbs.
func sanitizeSQL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```sql")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	// take up to the first semicolon
	if idx := strings.Index(s, ";"); idx >= 0 {
		s = s[:idx]
	}
	lower := strings.ToLower(s)
	for _, bad := range []string{"delete", "update", "drop", "insert", "alter", "create", "truncate"} {
		if strings.Contains(lower, bad) {
			return ""
		}
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "select") {
		return ""
	}
	return s
}

// EnsureCtxTimeout applies a sane deadline if none is set.
func EnsureCtxTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, 90*time.Second)
}
