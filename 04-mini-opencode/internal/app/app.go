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
	"github.com/wislist/mini-opencode/internal/agent/tools"
	"github.com/wislist/mini-opencode/internal/config"
	"github.com/wislist/mini-opencode/internal/skills"
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
	runtime, err := newRuntime(workingDir, cfg, scanner, out)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "mini-opencode %s\n", version)
	fmt.Fprintln(out, "commands: /help /version /tools /skills /key /compact /quit")

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
		case "/tools":
			for _, tool := range runtime.Tools() {
				fmt.Fprintf(out, "%s\t%s\n", tool.Name, tool.Description)
			}
		case "/skills":
			printSkills(out, workingDir)
		case "/compact":
			if err := runCompact(ctx, out, workingDir, runtime); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
			}
		case "/key":
			if err := configureDeepSeekKey(scanner, out, workingDir, &cfg, ""); err != nil {
				fmt.Fprintf(out, "error: %v\n", err)
				continue
			}
			runtime, err = newRuntime(workingDir, cfg, scanner, out)
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
				runtime, err = newRuntime(workingDir, cfg, scanner, out)
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

func runCompact(ctx context.Context, out io.Writer, workingDir string, runtime *agent.Runtime) error {
	if len(runtime.Messages()) == 0 {
		fmt.Fprintln(out, "[nothing to compact yet]")
		return nil
	}
	before := len(runtime.Messages())
	summaryPrompt, err := prompt.SummarySystemPrompt(workingDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "[compacting context...]")
	summary, err := runtime.Compact(ctx, summaryPrompt)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[context compacted: %d messages -> 1]\n", before)
	fmt.Fprintln(out, summary)
	return nil
}

func toPromptSkills(installed []skills.Skill) []prompt.Skill {
	out := make([]prompt.Skill, 0, len(installed))
	for _, s := range installed {
		out = append(out, prompt.Skill{Name: s.Name, Description: s.Description, Location: s.Location})
	}
	return out
}

func printSkills(out io.Writer, workingDir string) {
	installed, err := skills.LoadSkills(workingDir)
	if err != nil {
		fmt.Fprintf(out, "error loading skills: %v\n", err)
		return
	}
	if len(installed) == 0 {
		fmt.Fprintln(out, "no skills installed")
	} else {
		fmt.Fprintln(out, "installed skills:")
		for _, s := range installed {
			fmt.Fprintf(out, "  %s\t%s\n", s.Name, s.Description)
		}
	}
	fmt.Fprintf(out, "curated available: %s\n", strings.Join(skills.CuratedNames(), ", "))
}

func newRuntime(workingDir string, cfg config.Config, scanner *bufio.Scanner, out io.Writer) (*agent.Runtime, error) {
	promptContext := prompt.DefaultPromptContext(workingDir)
	contextFiles, err := prompt.DiscoverContextFiles(workingDir, nil)
	if err != nil {
		return nil, err
	}
	promptContext.ContextFiles = contextFiles

	installed, err := skills.LoadSkills(workingDir)
	if err != nil {
		return nil, err
	}
	promptContext.Skills = toPromptSkills(installed)

	systemPrompt, err := prompt.BuildSystemPrompt(prompt.PromptCoder, promptContext)
	if err != nil {
		return nil, err
	}

	provider, err := newProvider(cfg.Provider, workingDir)
	if err != nil {
		return nil, err
	}

	options := []agent.RuntimeOption{
		agent.WithSystemPrompt(systemPrompt),
		agent.WithPermissionPolicy(agent.NewDefaultPermissionPolicy(workingDir)),
		agent.WithPermissionConfirmer(confirmTool(scanner, out)),
		agent.WithHook(agent.NewSafetyHook(workingDir)),
		agent.WithHook(agent.NewLoopGuardHook()),
	}
	for _, tool := range tools.CodingTools(tools.CodingToolOptions{WorkDir: workingDir}) {
		options = append(options, agent.WithTool(tool))
	}

	return agent.NewRuntime(
		provider,
		options...,
	), nil
}

func confirmTool(scanner *bufio.Scanner, out io.Writer) agent.PermissionConfirmer {
	return func(ctx context.Context, call agent.ToolCall, result agent.ToolResult) bool {
		fmt.Fprintf(out, "allow tool %s? [y/N]: ", call.Name)
		if !scanner.Scan() {
			return false
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return answer == "y" || answer == "yes" || answer == "允许"
	}
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
		case agent.EventHookDenied:
			if event.ToolResult != nil && event.ToolResult.Error != "" {
				fmt.Fprintf(out, "hook blocked: %s\n", event.ToolResult.Error)
			}
		case agent.EventHookStopped:
			if event.Error != nil {
				fmt.Fprintf(out, "hook stopped run: %v\n", event.Error)
			}
		}
	}
}
