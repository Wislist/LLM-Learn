 package llmg
 
 import (
 	"context"
 	"fmt"
 	"net/http"
 )
 
 const openRouterBaseURL = "https://openrouter.ai/api/v1"
 const openRouterDefaultModel = "deepseek/deepseek-chat"
 
 type openRouterProvider struct {
 	apiKey string
 	client *http.Client
 }
 
 func WithOpenRouter(apiKey string) Provider {
 	return &openRouterProvider{
 		apiKey: apiKey,
 		client: http.DefaultClient,
 	}
 }
 
 func (p *openRouterProvider) Name() string { return "openrouter" }
 
 func (p *openRouterProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
 	if req.Model == "" {
 		req.Model = openRouterDefaultModel
 	}
 	req.Stream = false
 	return doRequest(ctx, p.client, openRouterBaseURL, p.apiKey, req)
 }
 
func (p *openRouterProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
 	if req.Model == "" {
 		req.Model = openRouterDefaultModel
 	}
 
 	rawCh, err := doStreamRequest(ctx, p.client, openRouterBaseURL, p.apiKey, req)
 	if err != nil {
 		return nil, fmt.Errorf("openrouter stream: %w", err)
 	}
 
 	return AccumulateStreamEvents(rawCh), nil
 }
