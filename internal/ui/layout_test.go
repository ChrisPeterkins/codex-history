package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chrispeterkins/codex-history/internal/data"
)

func TestLayoutStatesAndConversationScrolling(t *testing.T) {
	base := NewModel("test")
	base.projects = []data.Project{{Name: strings.Repeat("Long project ", 10), SessionCount: 100}}
	for i := 0; i < 100; i++ {
		base.sessions = append(base.sessions, data.Session{ID: fmt.Sprint(i), Preview: strings.Repeat("Conversation title ", 10), StartedAt: time.Now().Add(-time.Duration(i) * time.Hour)})
	}
	base.sessionCursor = 50
	for i := 0; i < 40; i++ {
		base.messages = append(base.messages, data.Message{UUID: fmt.Sprint(i), Type: "user", RawText: strings.Repeat("Scrollable message content ", 8), Timestamp: time.Now()})
	}
	base.messages = append(base.messages, data.Message{UUID: "tool", Type: "assistant", ContentBlocks: []data.ContentBlock{{Type: "tool_use", ToolID: "tool", ToolName: "Bash", Input: map[string]interface{}{"command": strings.Repeat("long command ", 10)}}}})
	states := []func(*Model){
		func(m *Model) { m.focus = panelProjects }, func(m *Model) { m.focus = panelSessions },
		func(m *Model) { m.focus = panelConversation }, func(m *Model) { m.showHelp = true },
		func(m *Model) {
			m.searchMode = true
			m.searchResults = []SearchResult{{Project: strings.Repeat("Result ", 30), Preview: strings.Repeat("Preview ", 30)}}
		},
		func(m *Model) { m.focus, m.fullScreen = panelConversation, true },
		func(m *Model) { m.focus, m.convSearchMode, m.convSearchMatches = panelConversation, true, []int{0} },
		func(m *Model) { m.updateAvail, m.statusMessage = "v123.456.789", strings.Repeat("Status ", 30) },
	}
	sizes := [][2]int{{20, 6}, {40, 12}, {59, 24}, {60, 24}, {99, 30}, {100, 30}, {159, 40}, {160, 40}, {200, 55}, {100, 30}, {40, 12}}
	for state, set := range states {
		m := base
		set(&m)
		for _, size := range sizes {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = updated.(Model)
			if got := m.View(); lipgloss.Width(got) != size[0] || lipgloss.Height(got) != size[1] {
				t.Errorf("%dx%d state %d rendered as %dx%d", size[0], size[1], state, lipgloss.Width(got), lipgloss.Height(got))
			}
		}
	}
	m := base
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, m.focus = updated.(Model), panelConversation
	m.viewport.GotoBottom()
	if last := strings.Split(m.renderConversationPanel(), "\n")[m.contentHeight()-1]; !strings.ContainsAny(last, "╯┘") {
		t.Fatalf("conversation bottom border was clipped: %q", last)
	}
	projects, sessions := m.renderProjectsPanel(), m.renderSessionsPanel()
	for range 20 {
		updated, _ = m.handleMouseScroll(panelConversation, 1)
		m = updated.(Model)
		if m.renderProjectsPanel() != projects || m.renderSessionsPanel() != sessions {
			t.Fatal("left panels changed while scrolling the conversation")
		}
	}
}
