package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/wislist/mini-opencode/internal/agent"
)

// ── View ──────────────────────────────────────────────

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.viewport.View())
	if m.state == statePermission && m.pendingPerm != nil {
		sections = append(sections, m.renderPermissionPrompt())
	} else if m.state == stateKeyPrompt {
		sections = append(sections, m.renderKeyPrompt())
	} else {
		sections = append(sections, m.renderInputBar())
	}
	sections = append(sections, m.renderHelpBar())
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *Model) renderHeader() string {
	left := headerStyle.Render("◆ mini-opencode")
	left += m.renderGitSegment()
	right := m.renderContextSegment() + "  " + dimStyle.Render(fmt.Sprintf("v%s · %s", m.version, m.cfg.Provider.Name))
	space := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return left + strings.Repeat(" ", space) + right
}

// renderGitSegment renders the branch and dirty-file counts for the header.
func (m *Model) renderGitSegment() string {
	if !m.gitStatus.Available {
		return ""
	}
	branch := gitBranchStyle.Render(" " + m.gitStatus.Branch)
	if !m.gitStatus.IsDirty() {
		return branch + gitCleanStyle.Render(" ✓")
	}
	parts := []string{branch}
	if m.gitStatus.Staged > 0 {
		parts = append(parts, gitStagedStyle.Render(fmt.Sprintf(" +%d", m.gitStatus.Staged)))
	}
	if m.gitStatus.Modified > 0 {
		parts = append(parts, gitModifiedStyle.Render(fmt.Sprintf(" ~%d", m.gitStatus.Modified)))
	}
	if m.gitStatus.Untracked > 0 {
		parts = append(parts, gitUntrackedStyle.Render(fmt.Sprintf(" ?%d", m.gitStatus.Untracked)))
	}
	return strings.Join(parts, "")
}

// renderContextSegment renders an approximate token usage indicator.
func (m *Model) renderContextSegment() string {
	if m.runtime == nil {
		return dimStyle.Render("ctx 0%")
	}
	tokens := m.runtime.ContextEstimate()
	pct := contextPercent(tokens, m.cfg.Provider.EffectiveContextWindow())
	return renderContextBar(pct)
}

func (m *Model) renderInputBar() string {
	return inputBorder.Render(promptStyle.Render("❯") + " " + m.input.View())
}

func (m *Model) renderKeyPrompt() string {
	return permBox.Render(keyLabel.Render("DeepSeek API Key:") + " " + m.keyInput.View())
}

func (m *Model) renderPermissionPrompt() string {
	if m.pendingPerm == nil {
		return ""
	}
	call := m.pendingPerm.call
	content := toolName.Render(call.Name) + "\n" +
		dimStyle.Render(extractToolDetail(call)) + "\n" +
		permAsk.Render("allow? [y/N]")
	return permBox.Render(content)
}

func (m *Model) renderHelpBar() string {
	if m.state == stateRunning {
		return spinnerStyle.Render(m.spinner.View()) + " " + dimStyle.Render("thinking...  ctrl+c to interrupt")
	}
	if m.state == stateCompacting {
		return spinnerStyle.Render(m.spinner.View()) + " " + dimStyle.Render("compacting...  ctrl+c to interrupt")
	}
	left := dimStyle.Render("/help /version /tools /status /key /compact /quit")
	right := dimStyle.Render("↑↓ scroll")
	space := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", space) + right
}

func (m *Model) renderUserMessage(text string) string {
	return userLabel.Render("▸ you") + "\n" + userText.Render(text)
}

func (m *Model) renderAssistantMessage(text string) string {
	return assistantLabel.Render("◂ assistant") + "\n" + assistantText.Render(text)
}

func (m *Model) renderToolCall(call *agent.ToolCall) string {
	return toolBox.Render(toolName.Render(call.Name) + "\n" + dimStyle.Render(extractToolDetail(*call)))
}

func (m *Model) renderTools() string {
	if m.runtime == nil {
		return dimStyle.Render("no runtime")
	}
	var lines []string
	lines = append(lines, toolName.Render("tools:"))
	for _, t := range m.runtime.Tools() {
		lines = append(lines, fmt.Sprintf("  %-12s %s", t.Name, t.Description))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderStatus() string {
	var lines []string
	lines = append(lines, toolName.Render("status:"))
	if m.gitStatus.Available {
		lines = append(lines, "  "+cmdStyle.Render("git")+"  branch: "+m.gitStatus.Branch)
		lines = append(lines, fmt.Sprintf("  staged: %d  modified: %d  untracked: %d",
			m.gitStatus.Staged, m.gitStatus.Modified, m.gitStatus.Untracked))
		if !m.gitStatus.IsDirty() {
			lines = append(lines, "  "+gitCleanStyle.Render("working tree clean"))
		}
	} else {
		lines = append(lines, "  "+dimStyle.Render("not a git repository"))
	}
	if m.runtime != nil {
		tokens := m.runtime.ContextEstimate()
		window := m.cfg.Provider.EffectiveContextWindow()
		pct := contextPercent(tokens, window)
		lines = append(lines, fmt.Sprintf("  %s  ~%s tokens  %.0f%% of %s  (%d messages)",
			cmdStyle.Render("ctx"), formatTokens(tokens), pct, formatTokens(window), len(m.runtime.Messages())))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderHelp() string {
	return toolName.Render("mini-opencode") + "\n" +
		dimStyle.Render("  a fresh Go agent terminal") + "\n\n" +
		"  " + cmdStyle.Render("/help") + "    show this help\n" +
		"  " + cmdStyle.Render("/version") + " show version\n" +
		"  " + cmdStyle.Render("/tools") + "   list registered tools\n" +
		"  " + cmdStyle.Render("/status") + "  show git status and context usage\n" +
		"  " + cmdStyle.Render("/compact") + " summarize and replace the conversation context\n" +
		"  " + cmdStyle.Render("/key") + "     set DeepSeek API key\n" +
		"  " + cmdStyle.Render("/quit") + "    exit"
}

func extractToolDetail(call agent.ToolCall) string {
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return string(call.Arguments)
	}
	for _, key := range []string{"command", "path", "file_path", "pattern", "query", "content"} {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%s: %v", key, v)
		}
	}
	var parts []string
	count := 0
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
		count++
		if count >= 3 {
			break
		}
	}
	return strings.Join(parts, "  ")
}

// formatTokens renders a token count in a human-friendly compact form.
func formatTokens(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1000000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

// contextPercent returns the context usage as a percentage of window.
func contextPercent(used, window int) float64 {
	if window <= 0 {
		return 0
	}
	return float64(used) / float64(window) * 100
}

// renderContextBar renders the context percentage with a compact bar and
// color-coded by usage tier (green < 60%, yellow < 85%, red otherwise).
func renderContextBar(pct float64) string {
	label := fmt.Sprintf("ctx %.0f%%", pct)
	switch {
	case pct < 60:
		return ctxLowStyle.Render(label)
	case pct < 85:
		return ctxMidStyle.Render(label)
	default:
		return ctxHighStyle.Render(label)
	}
}
