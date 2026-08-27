package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type markMode int

const (
	markNone markMode = iota
	markSet
	markJump
)

type markPosition struct {
	sessionID string
	yOffset   int
}

func (m Model) handleMarkKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Only accept a-z
	if len(key) != 1 || key[0] < 'a' || key[0] > 'z' {
		m.awaitingMark = markNone
		m.statusMessage = ""
		return m, nil
	}

	r := rune(key[0])

	if m.awaitingMark == markSet {
		sessionID := ""
		if m.sessionCursor < len(m.sessions) {
			sessionID = m.sessions[m.sessionCursor].ID
		}
		m.marks[r] = markPosition{
			sessionID: sessionID,
			yOffset:   m.viewport.YOffset,
		}
		m.statusMessage = fmt.Sprintf("Mark '%c' set", r)
		m.awaitingMark = markNone
		return m, clearStatusAfter(2 * time.Second)
	}

	if m.awaitingMark == markJump {
		mark, ok := m.marks[r]
		if !ok {
			m.statusMessage = fmt.Sprintf("Mark '%c' not set", r)
			m.awaitingMark = markNone
			return m, clearStatusAfter(2 * time.Second)
		}

		m.awaitingMark = markNone
		m.statusMessage = ""

		// Same session — just jump
		if m.sessionCursor < len(m.sessions) && m.sessions[m.sessionCursor].ID == mark.sessionID {
			m.viewport.SetYOffset(mark.yOffset)
			m.focus = panelConversation
			return m, nil
		}

		// Different session — find and load it
		for i, s := range m.sessions {
			if s.ID == mark.sessionID {
				offset := mark.yOffset
				m.pendingMarkOffset = &offset
				m.focus = panelConversation
				return m, m.selectSession(i)
			}
		}

		m.statusMessage = fmt.Sprintf("Mark '%c': session not found", r)
		return m, clearStatusAfter(2 * time.Second)
	}

	m.awaitingMark = markNone
	return m, nil
}
