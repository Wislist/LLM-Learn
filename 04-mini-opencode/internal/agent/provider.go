package agent

import (
	"context"
	"fmt"
	"strings"
)

type Provider interface {
	Complete(ctx context.Context, req Request) (AssistantResponse, error)
}

type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolSpec
}

type EchoProvider struct{}

func (p EchoProvider) Complete(ctx context.Context, req Request) (AssistantResponse, error) {
	select {
	case <-ctx.Done():
		return AssistantResponse{}, ctx.Err()
	default:
	}

	last := lastUserMessage(req.Messages)
	if last == "" {
		return AssistantResponse{Content: "ready"}, nil
	}
	return AssistantResponse{
		Content: fmt.Sprintf("runtime ready. received: %s", last),
	}, nil
}

func lastUserMessage(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}
