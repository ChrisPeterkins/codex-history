package ui

import (
	"errors"
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
			if w, h := m.conversationViewportSize(); m.viewport.Width != w || m.viewport.Height != h {
				t.Errorf("%dx%d viewport is %dx%d, want %dx%d", size[0], size[1], m.viewport.Width, m.viewport.Height, w, h)
			}
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

func TestCollapsibleSectionsAreStructuralAndDeterministic(t *testing.T) {
	m := NewModel("test")
	one := data.ContentBlock{Type: "tool_use", ToolID: "one", ToolName: "Bash", Input: map[string]interface{}{"command": "first"}}
	two := data.ContentBlock{Type: "tool_use", ToolID: "two", ToolName: "Bash", Input: map[string]interface{}{"command": "second"}}
	m.messages = []data.Message{{Type: "assistant", ContentBlocks: []data.ContentBlock{one, two}, ToolPairs: []data.ToolInteraction{{Use: one, Result: data.ContentBlock{Content: "▸ output is not a section"}}}}}
	m.collapsed["tool:one"] = false
	result := m.renderConversation()
	if len(result.sections) != 2 || result.sections[0].key != "tool:one" || result.sections[1].key != "tool:two" || !strings.Contains(strings.Split(result.content, "\n")[result.sections[1].line], "second") {
		t.Fatalf("incorrect structural sections: %#v", result.sections)
	}
	m.viewport.Height, m.viewport.YOffset = 5, 0
	m.collapsibleSections = []sectionLine{{key: "upper", line: 1}, {key: "lower", line: 3}, {key: "offscreen", line: 5}}
	if section, ok := m.activeCollapsibleSection(); !ok || section.key != "upper" {
		t.Fatalf("tie selected %#v, %v", section, ok)
	}
	m.collapsibleSections = []sectionLine{{key: "offscreen", line: 5}}
	if _, ok := m.activeCollapsibleSection(); ok {
		t.Fatal("bottom boundary was treated as visible")
	}
}

func TestLoadsRejectStaleResultsAndOwnTheirScroll(t *testing.T) {
	m := NewModel("test")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	m.projects = []data.Project{{Name: "project", DirName: "project"}}
	m.sessions = []data.Session{{ID: "old"}, {ID: "new"}}
	m.viewport.SetContent(strings.Repeat("line\n", 80))
	m.viewport.SetYOffset(7)
	cmd := m.selectSession(1)
	if cmd == nil || m.scrollPositions["old"] != 7 || m.sessionCursor != 1 {
		t.Fatalf("selection lost outgoing scroll: %#v", m.scrollPositions)
	}
	updated, _ = m.Update(projectsLoaded{seq: m.loadSeq - 1, projects: []data.Project{{DirName: "stale"}}})
	m = updated.(Model)
	updated, _ = m.Update(sessionsLoaded{projectKey: "project", seq: m.loadSeq - 1, sessions: []data.Session{{ID: "stale"}}})
	m = updated.(Model)
	if m.currentProjectKey() != "project" || len(m.sessions) != 2 {
		t.Fatal("stale project or session result was accepted")
	}
	m.messages = []data.Message{{UUID: "kept"}}
	updated, _ = m.Update(messagesLoaded{sessionID: "new", seq: m.loadSeq - 1, messages: []data.Message{{UUID: "stale"}}})
	m = updated.(Model)
	if m.messages[0].UUID != "kept" {
		t.Fatal("stale message result was accepted")
	}
	updated, _ = m.Update(messagesLoaded{sessionID: "new", seq: m.loadSeq, err: errors.New("database unavailable")})
	m = updated.(Model)
	if m.loadError == "" || !strings.Contains(m.renderConversationPanel(), "database unavailable") {
		t.Fatal("load error was not visible")
	}
}

func TestToolSelectionEnterAndMouseGeometry(t *testing.T) {
	m := NewModel("test")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, m.focus = updated.(Model), panelConversation
	tool := data.ContentBlock{Type: "tool_use", ToolID: "enter", ToolName: "Bash", Input: map[string]interface{}{"command": "true"}}
	m.messages = []data.Message{{Type: "assistant", ContentBlocks: []data.ContentBlock{tool}}}
	m.updateConversationContent()
	updated, _, _ = m.handlePanelKeys(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.isCollapsed("tool:enter") {
		t.Fatal("enter did not expand the selected tool")
	}
	m.viewport.SetContent(strings.Repeat("line\n", 30))
	m.viewport.Height, m.viewport.YOffset = 5, 0
	m.collapsibleSections = []sectionLine{{key: "first", line: 1}, {key: "second", line: 15}}
	m.jumpToCollapsible(1)
	if section, ok := m.activeCollapsibleSection(); !ok || section.key != "second" {
		t.Fatalf("next tool selected %#v", section)
	}
	m.collapsibleSections, m.viewport.YOffset = []sectionLine{{key: "mouse", line: 0}}, 0
	updated, _ = m.handleMouseClick(panelConversation, 0, screenHeaderHeight+panelTopChrome)
	m = updated.(Model)
	if m.isCollapsed("mouse") {
		t.Fatal("layout-derived mouse click missed line zero")
	}
	m.sessions = []data.Session{{ID: "first", StartedAt: time.Now()}, {ID: "second", StartedAt: time.Now()}}
	m.sessionCursor = 1
	updated, _ = m.handleMouseClick(panelSessions, 0, screenHeaderHeight+panelTopChrome+1)
	m = updated.(Model)
	if m.sessionCursor != 0 {
		t.Fatal("session mouse hit-testing ignored the date-header row")
	}
}

func TestSearchAndRefreshRequestsAreCurrent(t *testing.T) {
	m := NewModel("test")
	m.searchMode, m.searchSeq = true, 2
	m.searchResults = []SearchResult{{SessionID: "current"}}
	updated, _ := m.Update(searchResultsMsg{seq: 1, results: []SearchResult{{SessionID: "stale"}}})
	m = updated.(Model)
	if m.searchResults[0].SessionID != "current" {
		t.Fatal("stale search result was accepted")
	}
	m.searchInput.Focus()
	next, cmd := m.handleSearchKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("xy")})
	m = next.(Model)
	if cmd == nil || !m.searching || m.searchSeq != 3 {
		t.Fatal("search input was not debounced")
	}
	m.searchMode = false
	m.projects = []data.Project{{DirName: "project"}}
	m.sessions = []data.Session{{ID: "session"}}
	before := m.loadSeq
	updated, cmd, _ = m.handleActionKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated.(Model)
	if cmd == nil || !m.loading || m.loadSeq <= before {
		t.Fatal("refresh did not start a request-aware reload")
	}
	updated, cmd, _ = m.handleActionKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updated.(Model)
	if cmd == nil || !m.follow {
		t.Fatal("live follow did not start")
	}
	m.followRevision, before = "same", m.loadSeq
	updated, cmd = m.Update(followCheckMsg{sessionID: "session", revision: "same"})
	m = updated.(Model)
	if cmd != nil || m.loadSeq != before {
		t.Fatal("unchanged follow check reloaded the conversation")
	}
	updated, cmd = m.Update(followCheckMsg{sessionID: "session", revision: "new"})
	m = updated.(Model)
	if cmd == nil || m.loadSeq <= before {
		t.Fatal("changed follow check did not reload the conversation")
	}
}
