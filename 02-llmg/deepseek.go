 package llmg
 
 import (
 	"context"
 	"fmt"
 	"net/http"
 )
 
 const deepseekBaseURL = "https://api.deepseek.com"
 const deepseekModel = "deepseek-chat"
 
 type deepseekProvider struct {
 	apiKey string
 	client *http.Client
 }
 
 func WithDeepSeek(apiKey string) Provider {
 	return &deepseekProvider{
 		apiKey: apiKey,
 		client: http.DefaultClient,
 	}
 }
 
 func (p *deepseekProvider) Name() string { return "deepseek" }
 
 func (p *deepseekProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
 	if req.Model == "" {
 		req.Model = deepseekModel
 	}
 	req.Stream = false
 	return doRequest(ctx, p.client, deepseekBaseURL, p.apiKey, req)
 }
 
func (p *deepseekProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
 	if req.Model == "" {
 		req.Model = deepseekModel
 	}
 
 	rawCh, err := doStreamRequest(ctx, p.client, deepseekBaseURL, p.apiKey, req)
 	if err != nil {
 		return nil, fmt.Errorf("deepseek stream: %w", err)
 	}
 
 	return AccumulateStreamEvents(rawCh), nil
 }
