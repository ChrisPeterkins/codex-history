package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleConvSearchKey handles keyboard input during in-conversation search.
func (m Model) handleConvSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.convSearchMode = false
		m.convSearchInput.Blur()
		m.convSearchInput.SetValue("")
		m.convSearchMatches = nil
		m.convSearchContent = nil
		return m, nil

	case "enter", "ctrl+n":
		if len(m.convSearchMatches) > 0 {
			m.convSearchIdx = (m.convSearchIdx + 1) % len(m.convSearchMatches)
			m.viewport.SetYOffset(m.convSearchMatches[m.convSearchIdx])
		}
		return m, nil

	case "ctrl+p":
		if len(m.convSearchMatches) > 0 {
			m.convSearchIdx--
			if m.convSearchIdx < 0 {
				m.convSearchIdx = len(m.convSearchMatches) - 1
			}
			m.viewport.SetYOffset(m.convSearchMatches[m.convSearchIdx])
		}
		return m, nil
	}

	// Update text input
	var cmd tea.Cmd
	m.convSearchInput, cmd = m.convSearchInput.Update(msg)

	// Recompute matches from cached content (set on search entry, no re-rendering)
	query := strings.ToLower(m.convSearchInput.Value())
	m.convSearchMatches = nil
	m.convSearchIdx = 0

	if query != "" && len(m.convSearchContent) > 0 {
		for i, line := range m.convSearchContent {
			if strings.Contains(strings.ToLower(line), query) {
				m.convSearchMatches = append(m.convSearchMatches, i)
			}
		}
		if len(m.convSearchMatches) > 0 {
			m.viewport.SetYOffset(m.convSearchMatches[0])
		}
	}

	return m, cmd
}
