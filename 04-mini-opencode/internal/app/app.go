package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wislist/mini-opencode/internal/agent"
	"github.com/wislist/mini-opencode/internal/agent/prompt"
	"github.com/wislist/mini-opencode/internal/config"
)

const version = "0.1.0"

func Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.Load("config.json")
	if err != nil {
		return err
	}
	if err := ensureProviderKey(scanner, out, workingDir, &cfg); err != nil {
		return err
	}
	runtime, err := newRuntime(workingDir, cfg)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "mini-opencode %s\n", version)
	fmt.Fprintln(out, "commands: /help /version /key /quit")

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
		case "/key":
			if err := configureDeepSeekKey(scanner, out, workingDir, &cfg, ""); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				continue
			}
			runtime, err = newRuntime(workingDir, cfg)
			if err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				continue
			}
			fmt.Fprintln(out, "[deepseek key saved]")
		case "/quit", "quit", "exit":
			return nil
		default:
			if strings.HasPrefix(input, "/key ") {
				key := strings.TrimSpace(strings.TrimPrefix(input, "/key "))
				if err := configureDeepSeekKey(scanner, out, workingDir, &cfg, key); err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					continue
				}
				runtime, err = newRuntime(workingDir, cfg)
				if err != nil {
					fmt.Fprintf(out, "error: %v\n", err)
					continue
				}
				fmt.Fprintln(out, "[deepseek key saved]")
				continue
			}
			if err := runtime.Run(ctx, input, renderEvent(out)); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		}
	}
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "mini-opencode is a fresh Go agent terminal project.")
	fmt.Fprintln(out, "commands: /key <deepseek-api-key> saves a local key and switches provider to DeepSeek.")
}

func newRuntime(workingDir string, cfg config.Config) (*agent.Runtime, error) {
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

	provider, err := newProvider(cfg.Provider, workingDir)
	if err != nil {
		return nil, err
	}

	return agent.NewRuntime(
		provider,
		agent.WithSystemPrompt(systemPrompt),
	), nil
}

func newProvider(cfg config.ProviderConfig, workingDir string) (agent.Provider, error) {
	switch strings.ToLower(cfg.Name) {
	case "", "echo":
		return agent.EchoProvider{}, nil
	case "deepseek", "openai-compatible", "openai_compatible":
		apiKey := cfg.ResolvedAPIKeyFrom(workingDir)
		return agent.NewOpenAICompatibleProvider(agent.OpenAICompatibleConfig{
			BaseURL: cfg.BaseURL,
			APIKey:  apiKey,
			Model:   cfg.Model,
		})
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Name)
	}
}

func ensureProviderKey(scanner *bufio.Scanner, out io.Writer, workingDir string, cfg *config.Config) error {
	if strings.ToLower(cfg.Provider.Name) != "deepseek" {
		return nil
	}
	if cfg.Provider.ResolvedAPIKeyFrom(workingDir) != "" {
		return nil
	}
	return configureDeepSeekKey(scanner, out, workingDir, cfg, "")
}

func configureDeepSeekKey(scanner *bufio.Scanner, out io.Writer, workingDir string, cfg *config.Config, key string) error {
	if key == "" {
		fmt.Fprint(out, "DeepSeek API key: ")
		if !scanner.Scan() {
			return scanner.Err()
		}
		key = strings.TrimSpace(scanner.Text())
	}
	if key == "" {
		return fmt.Errorf("deepseek api key is required")
	}
	provider := cfg.Provider
	if strings.ToLower(provider.Name) != "deepseek" {
		provider = config.DefaultDeepSeekProvider()
	}
	if provider.BaseURL == "" {
		provider.BaseURL = "https://api.deepseek.com"
	}
	if provider.Model == "" {
		provider.Model = "deepseek-chat"
	}
	if provider.APIKeyEnv == "" {
		provider.APIKeyEnv = "DEEPSEEK_API_KEY"
	}
	provider.APIKey = ""
	cfg.Provider = provider
	if err := config.SaveProviderKey(workingDir, provider.Name, key); err != nil {
		return err
	}
	return config.Save(filepath.Join(workingDir, "config.json"), *cfg)
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
		case agent.EventToolCallFailed, agent.EventToolPermissionRequired, agent.EventToolPermissionDenied:
			if event.ToolResult != nil && event.ToolResult.Error != "" {
				fmt.Fprintf(out, "tool error: %s\n", event.ToolResult.Error)
			}
		}
	}
}
