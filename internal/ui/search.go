package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/chrispeterkins/codex-history/internal/data"
)

type searchResultsMsg struct {
	results []SearchResult
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchInput.Blur()
		m.searchInput.SetValue("")
		m.searchResults = nil
		return m, nil

	case "enter":
		if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
			result := m.searchResults[m.searchCursor]
			m.projectCursor = result.ProjectIdx
			m.sessionCursor = result.SessionIdx
			m.searchMode = false
			m.searchInput.Blur()
			m.searchInput.SetValue("")
			m.searchResults = nil
			m.focus = panelConversation
			return m, tea.Batch(m.loadSessionsForSearch(), m.loadMessagesForSearch(result))
		}
		return m, nil

	case "up", "ctrl+p":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil

	case "down", "ctrl+n":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)

	query := m.searchInput.Value()
	if query != "" && len(query) >= minSearchQueryLen {
		return m, tea.Batch(cmd, m.searchCmd(query))
	}

	if query == "" {
		m.searchResults = nil
	}

	return m, cmd
}

func (m Model) searchCmd(query string) tea.Cmd {
	projects := m.projects
	return func() tea.Msg {
		query = strings.ToLower(query)
		var results []SearchResult

		for pi, project := range projects {
			sessions, err := data.LoadSessions(&projects[pi])
			if err != nil {
				continue
			}
			for si, session := range sessions {
				searchText := strings.ToLower(session.Preview + " " + project.Name)
				if fuzzyMatch(searchText, query) {
					results = append(results, SearchResult{
						ProjectIdx: pi,
						SessionIdx: si,
						Preview:    session.Preview,
						Project:    project.Name,
						Date:       session.StartedAt.Format("Jan 02 15:04"),
					})
				}
				if len(results) >= maxSearchResults {
					return searchResultsMsg{results: results}
				}
			}
		}

		return searchResultsMsg{results: results}
	}
}

func (m Model) loadSessionsForSearch() tea.Cmd {
	if m.projectCursor >= len(m.projects) {
		return nil
	}
	p := &m.projects[m.projectCursor]
	return func() tea.Msg {
		sessions, err := data.LoadSessions(p)
		if err != nil {
			return sessionsLoaded{}
		}
		return sessionsLoaded{sessions: sessions}
	}
}

func (m Model) loadMessagesForSearch(result SearchResult) tea.Cmd {
	if result.ProjectIdx >= len(m.projects) {
		return nil
	}
	p := &m.projects[result.ProjectIdx]
	return func() tea.Msg {
		sessions, err := data.LoadSessions(p)
		if err != nil || result.SessionIdx >= len(sessions) {
			return messagesLoaded{}
		}
		s := &sessions[result.SessionIdx]
		messages, err := data.LoadMessages(s)
		if err != nil {
			return messagesLoaded{}
		}
		return messagesLoaded{messages: messages}
	}
}

func (m Model) renderSearchView() string {
	w := m.width - 4
	h := m.height

	title := panelTitleActiveStyle.Render("Search")
	input := m.searchInput.View()

	header := title + "\n\n" + "  " + input + "\n"

	var resultLines []string
	for i, r := range m.searchResults {
		prefix := "  "
		style := itemStyle
		descStyle := itemDescStyle

		if i == m.searchCursor {
			prefix = "▸ "
			style = selectedItemStyle
			descStyle = selectedItemDescStyle
		}

		line1 := style.Width(w).MaxWidth(w).Render(prefix + r.Project + "  " + r.Date)
		line2 := descStyle.Width(w).MaxWidth(w).Render("  " + truncateStr(r.Preview, w-4))
		resultLines = append(resultLines, line1, line2)

		if len(resultLines) > h-8 {
			break
		}
	}

	if len(m.searchResults) == 0 && m.searchInput.Value() != "" && len(m.searchInput.Value()) >= 2 {
		resultLines = append(resultLines, emptyStyle.Width(w).Render("\n  No results found"))
	}

	content := header + strings.Join(resultLines, "\n")

	help := helpKeyStyle.Render("↑/↓") + " " + helpDescStyle.Render("navigate") +
		statusBarStyle.Render("  ·  ") +
		helpKeyStyle.Render("enter") + " " + helpDescStyle.Render("open") +
		statusBarStyle.Render("  ·  ") +
		helpKeyStyle.Render("esc") + " " + helpDescStyle.Render("close")

	return lipgloss.JoinVertical(lipgloss.Left,
		activePanelStyle.Width(m.width-2).Height(h-3).MaxWidth(m.width).MaxHeight(h-1).Render(content),
		statusBarStyle.Width(m.width).MaxWidth(m.width).MaxHeight(1).Render("  "+help),
	)
}

// fuzzyMatch checks if all characters in query appear in text in order.
func fuzzyMatch(text, query string) bool {
	qi := 0
	for ti := 0; ti < len(text) && qi < len(query); ti++ {
		if text[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}
