package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Init satisfies tea.Model.
func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C is a universal, always-available quit (with history
		// persisted), regardless of which area/mode currently has focus.
		if msg.String() == "ctrl+c" {
			return m, m.quitAndSave()
		}
		// Ctrl+H opens the run-history overlay from anywhere, as long as
		// no other exclusive overlay/prompt is already active.
		if msg.String() == "ctrl+h" && !m.profileSwitcherOpen && !m.confirmAddVarsOpen && !m.addVarMode && !m.historyOpen {
			m.openHistorySwitcher()
			return m, nil
		}
		switch {
		case m.profileSwitcherOpen:
			return m, m.handleSwitcherKey(msg)
		case m.historyOpen:
			return m, m.handleHistoryKey(msg)
		case m.confirmAddVarsOpen:
			return m, m.handleConfirmAddVarsKey(msg)
		case m.addVarMode:
			return m, m.handleAddVarKey(msg)
		case m.focus == FocusInput:
			return m, m.handleInputKey(msg)
		default:
			return m, m.handleNavKey(msg)
		}

	default:
		if ti := m.activeTextInput(); ti != nil {
			var cmd tea.Cmd
			*ti, cmd = ti.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

// activeTextInput returns whichever textinput.Model is currently "live"
// (should receive blink/tick messages), or nil.
func (m *Model) activeTextInput() *textinput.Model {
	switch {
	case m.profileSwitcherOpen:
		return nil
	case m.historyOpen:
		return nil
	case m.confirmAddVarsOpen:
		return nil
	case m.addVarMode:
		return &m.addVarInput
	case m.focus == FocusNav && m.filterMode:
		return &m.filterInput
	case m.focus == FocusInput && m.inputIndex >= 0 && m.inputIndex < len(m.fields):
		return &m.fields[m.inputIndex].Input
	default:
		return nil
	}
}

// ---- nav area ----

func (m *Model) handleNavKey(msg tea.KeyMsg) tea.Cmd {
	if m.filterMode {
		switch msg.String() {
		case "esc":
			m.filterMode = false
			m.filterInput.SetValue("")
			m.filterInput.Blur()
			m.syncSelectionAfterFilterChange()
			return nil
		case "enter":
			return m.tryExecute()
		case "tab":
			m.focus = FocusInput
			m.ensureInputFocusValid()
			return nil
		case "up":
			m.moveSelection(-1)
			return nil
		case "down":
			m.moveSelection(1)
			return nil
		case "left":
			m.switchTabKeepFilter(-1)
			return nil
		case "right":
			m.switchTabKeepFilter(1)
			return nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.syncSelectionAfterFilterChange()
			return cmd
		}
	}

	key := msg.String()
	if key == "esc" {
		if m.escQuitArmed {
			m.escQuitArmed = false
			return m.quitAndSave()
		}
		m.escQuitArmed = true
		return nil
	}
	m.escQuitArmed = false

	switch key {
	case "q":
		return m.quitAndSave()
	case "tab":
		m.focus = FocusInput
		m.ensureInputFocusValid()
	case "left":
		m.switchTab(-1)
	case "right":
		m.switchTab(1)
	case "up":
		m.moveSelection(-1)
	case "down":
		m.moveSelection(1)
	case " ":
		m.toggleFavorite()
	case "/":
		m.filterMode = true
		m.filterInput.SetValue("")
		m.filterInput.Focus()
	case "p":
		m.openProfileSwitcher()
	case "enter":
		return m.tryExecute()
	}
	return nil
}

// switchTabKeepFilter mirrors switchTab but preserves the active filter
// text across the tab change (switchTab itself clears it, which is what
// we want for the plain Left/Right-from-nav case; while actively typing a
// filter, though, clearing on tab-switch would be surprising, so this
// variant keeps it).
func (m *Model) switchTabKeepFilter(delta int) {
	n := len(m.profile.Libraries)
	if n == 0 {
		return
	}
	m.activeTab = ((m.activeTab+delta)%n + n) % n
	m.ensureCurrentSelectionValid()
}

// ---- input area ----

func (m *Model) handleInputKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if key == "esc" {
		if m.escQuitArmed {
			m.escQuitArmed = false
			return m.quitAndSave()
		}
		m.escQuitArmed = true
		return nil
	}
	m.escQuitArmed = false

	switch key {
	case "tab":
		m.moveInputFocus(1)
	case "shift+tab":
		m.moveInputFocus(-1)
	case "left", "right":
		cur := m.currentField()
		switch {
		case cur == nil:
		case cur.Kind == FieldDropdown && len(cur.Options) > 0:
			delta := 1
			if key == "left" {
				delta = -1
			}
			cur.SelectedIndex = clamp(cur.SelectedIndex+delta, 0, len(cur.Options)-1)
			cur.Input.SetValue(cur.Options[cur.SelectedIndex])
		case cur.Kind == FieldText:
			// Not a dropdown: let the text input handle its own
			// cursor movement as usual.
			var cmd tea.Cmd
			cur.Input, cmd = cur.Input.Update(msg)
			return cmd
		}
	case "ctrl+r":
		for i := range m.fields {
			if m.fields[i].Kind == FieldText {
				m.fields[i].Input.SetValue("")
			}
		}
		m.syncDerivedFields()
	case "ctrl+n":
		kept := m.fields[:0:0]
		for _, f := range m.fields {
			if f.MustHave {
				f.Input.SetValue("")
				kept = append(kept, f)
			}
		}
		m.fields = kept
		m.syncDerivedFields()
		m.ensureInputFocusValid()
	case "ctrl+a":
		m.addVarMode = true
		m.addVarInput.SetValue("")
		m.addVarInput.Focus()
	case "ctrl+d":
		if cur := m.currentField(); cur != nil && cur.Kind == FieldText && !cur.MustHave {
			m.fields = append(m.fields[:m.inputIndex], m.fields[m.inputIndex+1:]...)
			m.syncDerivedFields()
			if m.inputIndex >= len(m.fields) {
				m.inputIndex = len(m.fields) - 1
			}
			m.ensureInputFocusValid()
		}
	default:
		if cur := m.currentField(); cur != nil && cur.Kind == FieldText {
			var cmd tea.Cmd
			cur.Input, cmd = cur.Input.Update(msg)
			m.syncDerivedFields()
			return cmd
		}
	}
	return nil
}

// ---- add-variable prompt ----

func (m *Model) handleAddVarKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.addVarInput.Value())
		m.addVarMode = false
		m.addVarInput.Blur()
		if name == "" {
			return nil
		}
		if idx := m.fieldIndex(name); idx >= 0 {
			m.focusField(idx)
			return nil
		}
		m.addField(name, "", false)
		idx := len(m.fields) - 1
		m.syncDerivedFields()
		m.focusField(idx)
	case "esc":
		m.addVarMode = false
		m.addVarInput.SetValue("")
		m.addVarInput.Blur()
	default:
		var cmd tea.Cmd
		m.addVarInput, cmd = m.addVarInput.Update(msg)
		return cmd
	}
	return nil
}

// ---- "add missing variables?" confirmation ----

// handleConfirmAddVarsKey answers the prompt tryExecute raises when the
// selected script references placeholders with no field in the pool yet:
// "y"/enter adds them (and focuses the first one); anything else cancels
// without adding or running.
func (m *Model) handleConfirmAddVarsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y", "enter":
		m.confirmAddMissingVars()
	default:
		m.cancelAddMissingVars()
	}
	return nil
}

// ---- profile switcher overlay ----

func (m *Model) handleSwitcherKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up":
		if len(m.profileNames) > 0 {
			m.profileSwitcherIndex = clamp(m.profileSwitcherIndex-1, 0, len(m.profileNames)-1)
		}
	case "down":
		if len(m.profileNames) > 0 {
			m.profileSwitcherIndex = clamp(m.profileSwitcherIndex+1, 0, len(m.profileNames)-1)
		}
	case "enter":
		if m.profileSwitcherIndex >= 0 && m.profileSwitcherIndex < len(m.profileNames) {
			m.switchProfile(m.profileNames[m.profileSwitcherIndex])
		}
		m.profileSwitcherOpen = false
	case "esc":
		m.profileSwitcherOpen = false
	}
	return nil
}

// ---- run history overlay ----

func (m *Model) handleHistoryKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up":
		if len(m.historyRuns) > 0 {
			m.historyIndex = clamp(m.historyIndex-1, 0, len(m.historyRuns)-1)
		}
	case "down":
		if len(m.historyRuns) > 0 {
			m.historyIndex = clamp(m.historyIndex+1, 0, len(m.historyRuns)-1)
		}
	case "enter":
		if m.historyIndex >= 0 && m.historyIndex < len(m.historyRuns) {
			m.restoreRun(m.historyRuns[m.historyIndex])
		}
		m.historyOpen = false
	case "esc":
		m.historyOpen = false
	}
	return nil
}
