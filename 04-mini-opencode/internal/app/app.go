package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wislist/mini-opencode/internal/agent"
	"github.com/wislist/mini-opencode/internal/agent/prompt"
)

const version = "0.1.0"

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	runtime, err := newRuntime()
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "mini-opencode %s\n", version)
	fmt.Fprintln(out, "commands: /help /version /quit")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Fprint(out, "\n> ")
		if !scanner.Scan() {
			return scanner.Err()
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case "/help":
			printHelp(out)
		case "/version":
			fmt.Fprintf(out, "mini-opencode %s\n", version)
		case "/quit", "quit", "exit":
			return nil
		default:
			if err := runtime.Run(ctx, input, renderEvent(out)); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		}
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "mini-opencode is a fresh Go agent terminal project.")
	fmt.Fprintln(out, "next steps: provider abstraction, event-driven runtime, tools, TUI, MCP.")
}

func newRuntime() (*agent.Runtime, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	promptContext := prompt.DefaultPromptContext(workingDir)
	contextFiles, err := prompt.DiscoverContextFiles(workingDir, nil)
	if err != nil {
		return nil, err
	}
	promptContext.ContextFiles = contextFiles

	systemPrompt, err := prompt.BuildSystemPrompt(prompt.PromptCoder, promptContext)
	if err != nil {
		return nil, err
	}

	return agent.NewRuntime(
		agent.EchoProvider{},
		agent.WithSystemPrompt(systemPrompt),
	), nil
}

func renderEvent(out io.Writer) func(agent.Event) {
	return func(event agent.Event) {
		switch event.Type {
		case agent.EventTurnStarted:
			fmt.Fprintf(out, "[turn %d]\n", event.Turn)
		case agent.EventAssistantResponse:
			if event.Message != nil && event.Message.Content != "" {
				fmt.Fprintln(out, event.Message.Content)
			}
		case agent.EventToolCallStarted:
			if event.ToolCall != nil {
				fmt.Fprintf(out, "tool: %s\n", event.ToolCall.Name)
			}
		case agent.EventToolCallFinished:
			if event.ToolResult == nil {
				return
			}
			if event.ToolResult.Error != "" {
				fmt.Fprintf(out, "tool error: %s\n", event.ToolResult.Error)
				return
			}
			fmt.Fprintf(out, "tool result: %s\n", event.ToolResult.Content)
		}
	}
}
