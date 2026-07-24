package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateIdle:
		return m.handleIdleKey(msg)
	case stateRunning:
		return m.handleRunningKey(msg)
	case statePermission:
		return m.handlePermissionKey(msg)
	case stateKeyPrompt:
		return m.handleKeyPromptKey(msg)
	case stateQuitting:
		return m, tea.Quit
	case stateCompacting:
		return m.handleCompactingKey(msg)
	}
	return m, nil
}

func (m *Model) handleIdleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		m.state = stateQuitting
		return m, tea.Quit
	case tea.KeyEnter:
		input := strings.TrimSpace(m.input.Value())
		if input == "" {
			return m, nil
		}
		m.input.Reset()
		return m.handleInput(input)
	case tea.KeyUp:
		m.viewport.LineUp(1)
		return m, nil
	case tea.KeyDown:
		m.viewport.LineDown(1)
		return m, nil
	case tea.KeyPgUp:
		m.viewport.HalfViewUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleRunningKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.cancel != nil {
			m.cancel()
		}
		m.state = stateIdle
		m.addBlock(errorStyle.Render("✗ interrupted"))
		m.refreshViewport()
		return m, textinput.Blink
	case tea.KeyUp:
		m.viewport.LineUp(1)
	case tea.KeyDown:
		m.viewport.LineDown(1)
	}
	return m, nil
}

func (m *Model) handleCompactingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.cancel != nil {
			m.cancel()
		}
		m.state = stateIdle
		m.addBlock(errorStyle.Render("✗ compact interrupted"))
		m.refreshViewport()
		return m, textinput.Blink
	case tea.KeyUp:
		m.viewport.LineUp(1)
	case tea.KeyDown:
		m.viewport.LineDown(1)
	}
	return m, nil
}

func (m *Model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.pendingPerm != nil {
			m.pendingPerm.resp <- true
		}
		m.pendingPerm = nil
		m.state = stateRunning
		m.refreshViewport()
	case "n", "N", "esc":
		if m.pendingPerm != nil {
			m.pendingPerm.resp <- false
		}
		m.pendingPerm = nil
		m.state = stateRunning
		m.refreshViewport()
	}
	return m, nil
}

func (m *Model) handleKeyPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		key := strings.TrimSpace(m.keyInput.Value())
		m.keyInput.Reset()
		if key == "" {
			m.state = stateIdle
			m.addBlock(errorStyle.Render("key input cancelled"))
			m.refreshViewport()
			return m, textinput.Blink
		}
		return m.saveKey(key)
	case tea.KeyCtrlC, tea.KeyEsc:
		m.keyInput.Reset()
		m.state = stateIdle
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}
