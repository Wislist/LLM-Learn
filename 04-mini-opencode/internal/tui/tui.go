package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wislist/mini-opencode/internal/agent"
	"github.com/wislist/mini-opencode/internal/config"
)

type appState int

const (
	stateIdle appState = iota
	stateRunning
	statePermission
	stateKeyPrompt
	stateQuitting
	stateCompacting
)

// Messages bridging the synchronous runtime goroutine into Bubble Tea.
type runtimeEventMsg struct{ event agent.Event }
type runtimeDoneMsg struct{ err error }
type permissionRequestMsg struct {
	call   agent.ToolCall
	result *agent.ToolResult
	resp   chan bool
}
type compactDoneMsg struct {
	summary string
	err     error
}

// Callbacks the TUI needs from app.go.
type KeySaver func(key string) (config.Config, error)
type RuntimeFactory func(cfg config.Config) (*agent.Runtime, error)

type Model struct {
	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model
	keyInput textinput.Model

	state      appState
	runtime    *agent.Runtime
	program    *tea.Program
	cfg        *config.Config
	workingDir string
	version    string

	blocks []string
	width  int
	height int

	gitStatus GitStatus

	pendingPerm    *permissionRequestMsg
	keySaver       KeySaver
	runtimeFactory RuntimeFactory
	compactor      Compactor

	ctx    context.Context
	cancel context.CancelFunc
}

// Compactor summarizes the current conversation context. It returns the
// generated summary text.
type Compactor func(ctx context.Context) (string, error)

func New(cfg *config.Config, workingDir, ver string) *Model {
	vp := viewport.New(80, 20)

	ti := textinput.New()
	ti.Placeholder = "ask anything...  (/help for commands)"
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle

	ki := textinput.New()
	ki.Prompt = ""
	ki.EchoMode = textinput.EchoPassword
	ki.CharLimit = 0

	return &Model{
		viewport:   vp,
		input:      ti,
		spinner:    sp,
		keyInput:   ki,
		state:      stateIdle,
		cfg:        cfg,
		workingDir: workingDir,
		version:    ver,
	}
}

func (m *Model) SetRuntime(rt *agent.Runtime)        { m.runtime = rt }
func (m *Model) Runtime() *agent.Runtime             { return m.runtime }
func (m *Model) SetProgram(p *tea.Program)           { m.program = p }
func (m *Model) SetKeySaver(ks KeySaver)             { m.keySaver = ks }
func (m *Model) SetRuntimeFactory(rf RuntimeFactory) { m.runtimeFactory = rf }
func (m *Model) SetCompactor(c Compactor)            { m.compactor = c }
func (m *Model) MakeConfirmer() agent.PermissionConfirmer {
	return func(ctx context.Context, call agent.ToolCall, result agent.ToolResult) bool {
		resp := make(chan bool, 1)
		m.program.Send(permissionRequestMsg{call: call, result: &result, resp: resp})
		select {
		case <-ctx.Done():
			return false
		case ok := <-resp:
			return ok
		}
	}
}

func (m *Model) Init() tea.Cmd {
	m.addBlock(dimStyle.Render("welcome to mini-opencode") + "\n" +
		dimStyle.Render("type /help for commands, or just start typing."))
	m.gitStatus = collectGitStatus(m.workingDir)
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = max(1, msg.Height-7)
		m.refreshViewport()
		return m, nil

	case spinner.TickMsg:
		if m.state == stateRunning || m.state == stateCompacting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case runtimeEventMsg:
		m.handleRuntimeEvent(msg.event)

	case runtimeDoneMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.addBlock(errorStyle.Render("✗ " + msg.err.Error()))
		}
		m.gitStatus = collectGitStatus(m.workingDir)
		m.refreshViewport()
		m.input.Focus()
		cmds = append(cmds, textinput.Blink)

	case compactDoneMsg:
		m.state = stateIdle
		if msg.err != nil {
			m.addBlock(errorStyle.Render("✗ compact: " + msg.err.Error()))
		} else if msg.summary != "" {
			m.addBlock(toolArrow.Render("⟳ context compacted"))
			m.addBlock(dimStyle.Render(msg.summary))
		}
		m.gitStatus = collectGitStatus(m.workingDir)
		m.refreshViewport()
		m.input.Focus()
		cmds = append(cmds, textinput.Blink)

	case permissionRequestMsg:
		m.pendingPerm = &msg
		m.state = statePermission
		m.refreshViewport()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	if m.state == stateIdle || m.state == stateRunning {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.state == stateKeyPrompt {
		m.keyInput, cmd = m.keyInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// ── helpers ───────────────────────────────────────────

func (m *Model) addBlock(block string) {
	m.blocks = append(m.blocks, block)
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(strings.Join(m.blocks, "\n\n"))
	m.viewport.GotoBottom()
}
