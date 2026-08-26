package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chrispeterkins/codex-history/internal/data"
)

func TestLayoutProbe(t *testing.T) {
	for _, size := range [][2]int{{50, 20}, {80, 24}, {100, 30}, {120, 36}, {140, 42}, {180, 50}} {
		m := NewModel("test")
		m.ready, m.width, m.height = true, size[0], size[1]
		m.projects = []data.Project{{Name: "Large Project", SessionCount: 100}}
		for i := 0; i < 100; i++ {
			m.sessions = append(m.sessions, data.Session{ID: fmt.Sprint(i), Preview: "A representative conversation title that may be long", StartedAt: time.Now().Add(-time.Duration(i) * 24 * time.Hour), MessageCount: 42, TotalTokensOut: 123456})
		}
		for focus := panelProjects; focus <= panelConversation; focus++ {
			m.focus = focus
			view := m.View()
			if width, height := lipgloss.Width(view), lipgloss.Height(view); width != size[0] || height != size[1] {
				t.Errorf("%dx%d focus %d rendered as %dx%d", size[0], size[1], focus, width, height)
			}
		}
	}
}
