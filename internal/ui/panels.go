package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrispeterkins/codex-history/internal/data"
)

type sessionListItem struct {
	isGroupHeader bool
	groupLabel    string
	session       data.Session
	origIdx       int
}

func (m Model) sessionListItems() []sessionListItem {
	var items []sessionListItem
	for _, group := range GroupSessionsByDate(m.filterSessions(m.sessions)) {
		items = append(items, sessionListItem{isGroupHeader: true, groupLabel: group.Label})
		for _, session := range group.Sessions {
			items = append(items, sessionListItem{session: session.Session, origIdx: session.OriginalIndex})
		}
	}
	return items
}

func (m Model) visibleSessionRange(items []sessionListItem) (int, int) {
	cursor := 0
	for i, item := range items {
		if !item.isGroupHeader && item.origIdx == m.sessionCursor {
			cursor = i
			break
		}
	}
	return m.visibleRange(cursor, len(items), max(1, (m.contentHeight()-3)/2))
}

func (m Model) renderProjectsPanel() string {
	w := m.projectsWidth()
	h := m.contentHeight()

	title := panelTitleStyle.Render("Projects")
	if m.focus == panelProjects {
		title = panelTitleActiveStyle.Render(" Projects ")
	}

	var items []string
	items = append(items, title)

	if len(m.projects) == 0 {
		noData := "\n" + emptyStyle.Width(w-4).Render("No Codex\nhistory found.")
		if m.loadError != "" {
			noData = "\n" + toolErrorStyle.Width(w-4).Render("Could not load history:\n"+m.loadError)
		} else {
			noData += "\n\n" + timestampStyle.Render("  Start a conversation\n  with Codex to\n  see it here.")
		}
		items = append(items, noData)
	}

	visibleStart, visibleEnd := m.visibleRange(m.projectCursor, len(m.projects), h-3)
	for i := visibleStart; i < visibleEnd; i++ {
		p := m.projects[i]
		dot := activityDot(p.LastActive)
		suffix := ""
		if dot != "" {
			suffix = " " + dot
		}
		if p.SessionCount > 0 {
			suffix += tokenStyle.Render(fmt.Sprintf(" (%d)", p.SessionCount))
		}
		if p.HistoryOnly {
			suffix += " ○"
		}
		suffixWidth := lipgloss.Width(suffix)
		maxNameWidth := w - 6 - suffixWidth // panel padding(4) + item border/padding(2)
		if maxNameWidth < 4 {
			maxNameWidth = 4
		}
		name := truncateStr(p.Name, maxNameWidth)
		focused := m.focus == panelProjects
		if i == m.projectCursor {
			style := selectedItemStyle
			if !focused {
				style = dimSelectedItemStyle
			}
			items = append(items, style.Width(w-4).MaxWidth(w-4).Render(name+suffix))
		} else {
			style := itemStyle
			if !focused {
				style = dimItemStyle
			}
			items = append(items, style.Width(w-4).MaxWidth(w-4).Render(name+suffix))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)

	return m.panelStyleFor(panelProjects).Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(content)
}

func (m Model) renderSessionsPanel() string {
	w := m.sessionsWidth()
	h := m.contentHeight()

	filterLabel := ""
	if m.sessionFilter > 0 {
		filterLabel = " (" + sessionFilterTypes[m.sessionFilter].label + ")"
	}
	title := panelTitleStyle.Render("Sessions" + filterLabel)
	if m.focus == panelSessions {
		title = panelTitleActiveStyle.Render(" Sessions" + filterLabel + " ")
	}

	var items []string
	items = append(items, title)

	if len(m.sessions) == 0 {
		message := "No sessions"
		if m.loadError != "" {
			message = "Could not load sessions:\n" + m.loadError
		}
		items = append(items, "\n"+emptyLogoStyle.Width(w-4).Render("◈")+"\n"+emptyStyle.Width(w-4).Render(message))
	} else {
		flatItems := m.sessionListItems()
		visibleStart, visibleEnd := m.visibleSessionRange(flatItems)
		for i := visibleStart; i < visibleEnd; i++ {
			item := flatItems[i]
			if item.isGroupHeader {
				items = append(items, dateGroupStyle.Width(w-4).Render("  "+item.groupLabel))
				continue
			}

			s := item.session
			date := relativeTime(s.StartedAt)
			preview := truncateStr(s.Preview, w-6)
			if preview == "" {
				preview = "(empty session)"
			}
			stats := sessionStatsLine(s)

			focused := m.focus == panelSessions
			if item.origIdx == m.sessionCursor {
				s1 := selectedItemStyle
				if !focused {
					s1 = dimSelectedItemStyle
				}
				line1 := s1.Width(w - 4).MaxWidth(w - 4).Render(truncateStr(date+"  "+stats, w-6))
				line2 := s1.Width(w - 4).MaxWidth(w - 4).Render(preview)
				items = append(items, line1, line2)
			} else {
				s1, s2 := itemStyle, itemDescStyle
				if !focused {
					s1, s2 = dimItemStyle, dimItemDescStyle
				}
				line1 := s1.Width(w - 4).MaxWidth(w - 4).Render(truncateStr(date+"  "+stats, w-6))
				line2 := s2.Width(w - 4).MaxWidth(w - 4).Render(preview)
				items = append(items, line1, line2)
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)

	return m.panelStyleFor(panelSessions).Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(content)
}

func (m Model) renderConversationPanel() string {
	w := m.conversationWidth()
	h := m.contentHeight()

	var title string
	if m.convSearchMode {
		matchInfo := ""
		if len(m.convSearchMatches) > 0 {
			matchInfo = fmt.Sprintf(" %d/%d", m.convSearchIdx+1, len(m.convSearchMatches))
		}
		title = panelTitleActiveStyle.Render(" Find"+matchInfo+" ") + " " + m.convSearchInput.View()
	} else if m.focus == panelConversation {
		title = panelTitleActiveStyle.Render(" Conversation ")
	} else {
		title = panelTitleStyle.Render("Conversation")
	}

	scrollInfo := ""
	if m.follow {
		scrollInfo = tokenStyle.Render(" LIVE")
	}
	if m.viewport.TotalLineCount() > 0 {
		pct := int(m.viewport.ScrollPercent() * 100)
		scrollInfo += tokenStyle.Render(fmt.Sprintf(" %d%%", pct))
	}

	header := lipgloss.JoinHorizontal(lipgloss.Center, title, scrollInfo)

	var body string
	if m.loading {
		body = "\n\n" + emptyStyle.Width(w-6).Render(m.spinner.View()+" Loading session...")
	} else if m.loadError != "" {
		body = "\n\n" + toolErrorStyle.Width(w-6).Render("Could not load conversation:\n"+m.loadError)
	} else if m.focus != panelConversation && len(m.messages) == 0 && m.sessionCursor < len(m.sessions) {
		s := m.sessions[m.sessionCursor]
		peek := "\n\n" + timestampStyle.Render("  Preview") + "\n\n"
		if s.Preview != "" {
			preview := s.Preview
			if len(preview) > 300 {
				preview = preview[:297] + "..."
			}
			peek += emptyStyle.Width(w - 6).Render(preview)
		}
		peek += "\n\n" + tokenStyle.Render("  "+sessionStatsLine(s))
		peek += "\n\n" + timestampStyle.Render("  press enter to load full conversation")
		body = peek
	} else {
		body = m.applyLineHighlight(m.viewport.View(), w-6)
	}

	scrollbar := m.renderScrollbar(h - 3)
	bodyWithScroll := lipgloss.JoinHorizontal(lipgloss.Top, body, " ", scrollbar)

	content := lipgloss.JoinVertical(lipgloss.Left, header, bodyWithScroll)

	return m.panelStyleFor(panelConversation).Width(w - 2).Height(h - 2).MaxWidth(w).MaxHeight(h).Render(content)
}

func (m Model) projectsWidth() int {
	if (m.fullScreen || m.width < breakpointNarrow) && m.focus == panelProjects {
		return m.width
	}
	if m.fullScreen || m.width < breakpointNarrow {
		return 0
	}
	if m.width < breakpointMedium {
		if m.focus == panelProjects {
			return max(20, m.width*2/5)
		}
		return 0 // hidden when focus is sessions or conversation
	}
	return max(20, m.width/5)
}

func (m Model) sessionsWidth() int {
	if (m.fullScreen || m.width < breakpointNarrow) && m.focus == panelSessions {
		return m.width
	}
	if m.fullScreen || m.width < breakpointNarrow {
		return 0
	}
	if m.width < breakpointMedium {
		if m.focus == panelProjects {
			return m.width - m.projectsWidth()
		}
		return max(24, m.width/3)
	}
	return max(30, m.width*3/10)
}

func (m Model) conversationWidth() int {
	if (m.fullScreen || m.width < breakpointNarrow) && m.focus == panelConversation {
		return m.width
	}
	if m.fullScreen || m.width < breakpointNarrow {
		return 0
	}
	w := m.width - m.projectsWidth() - m.sessionsWidth()
	if w < 30 {
		return 30
	}
	return w
}

func (m Model) contentHeight() int {
	h := m.height - screenHeaderHeight - screenFooterHeight
	if h < 5 {
		return 5
	}
	return h
}

func (m Model) visibleRange(cursor, total, height int) (int, int) {
	if total <= height {
		return 0, total
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
		start = end - height
	}
	return start, end
}

func sessionStatsLine(s data.Session) string {
	var parts []string

	if s.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d msgs", s.MessageCount))
	}
	if s.TotalTokensOut > 0 {
		parts = append(parts, formatTokenCount(s.TotalTokensOut)+" tok")
	}
	if s.TotalDurationMs > 0 {
		dur := time.Duration(s.TotalDurationMs) * time.Millisecond
		if dur >= time.Minute {
			parts = append(parts, fmt.Sprintf("%dm", int(dur.Minutes())))
		} else {
			parts = append(parts, fmt.Sprintf("%ds", int(dur.Seconds())))
		}
	}

	if len(parts) == 0 {
		return formatSize(s.FileSize)
	}
	return strings.Join(parts, " · ")
}

func (m Model) panelStyleFor(panel int) lipgloss.Style {
	if m.focus == panel {
		return activePanelStyle
	}
	return panelStyle
}

func (m Model) renderScrollbar(height int) string {
	if height <= 0 || m.viewport.TotalLineCount() <= m.viewport.Height {
		return strings.TrimSuffix(strings.Repeat(" \n", height), "\n")
	}

	pct := m.viewport.ScrollPercent()
	thumbPos := int(pct * float64(height-1))

	var lines []string
	for i := 0; i < height; i++ {
		if i == thumbPos {
			lines = append(lines, scrollThumbStyle.Render("█"))
		} else {
			lines = append(lines, scrollTrackStyle.Render("│"))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderHeader() string {
	logo := headerStyle.Render(" ◈ Codex History")
	if lipgloss.Width(logo)+2 >= m.width {
		return ansi.Truncate(logo, m.width, "")
	}

	right := m.renderBreadcrumb()
	if m.updateAvail != "" {
		updateBadge := lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Render("Update " + m.updateAvail + " available")
		if right != "" {
			right = headerBreadcrumbStyle.Render(right) + "  " + updateBadge + " "
		} else {
			right = updateBadge + " "
		}
	} else if right != "" {
		right = headerBreadcrumbStyle.Render(right) + " "
	}

	logoLen := lipgloss.Width(logo)
	rightLen := lipgloss.Width(right)
	maxRight := m.width - logoLen - 5
	if maxRight <= 0 {
		right, rightLen = "", 0
	} else if rightLen > maxRight {
		right = ansi.TruncateLeft(right, rightLen-maxRight+1, "…")
		rightLen = lipgloss.Width(right)
	}
	fillLen := m.width - logoLen - rightLen - 2
	fill := headerLineStyle.Render(" " + strings.Repeat("─", fillLen) + " ")

	return logo + fill + right
}

func (m Model) applyLineHighlight(viewOutput string, maxWidth int) string {
	if m.focus != panelConversation || viewOutput == "" {
		return viewOutput
	}
	highlight := func(style lipgloss.Style, line string) string {
		line = ansi.Truncate(strings.TrimRight(line, " "), max(1, maxWidth-2), "")
		return style.Width(maxWidth).MaxWidth(maxWidth).Render(line)
	}

	if m.convSearchMode && len(m.convSearchMatches) > 0 {
		lines := strings.Split(viewOutput, "\n")
		matchSet := make(map[int]bool)
		for _, absLine := range m.convSearchMatches {
			matchSet[absLine-m.viewport.YOffset] = true
		}
		currentRel := -1
		if m.convSearchIdx < len(m.convSearchMatches) {
			currentRel = m.convSearchMatches[m.convSearchIdx] - m.viewport.YOffset
		}
		for i := range lines {
			if i == currentRel {
				lines[i] = highlight(selectedItemStyle, lines[i])
			} else if matchSet[i] {
				lines[i] = highlight(dimSelectedItemStyle, lines[i])
			}
		}
		return strings.Join(lines, "\n")
	}

	section, ok := m.activeCollapsibleSection()
	if !ok {
		return viewOutput
	}
	relativeLine := section.line - m.viewport.YOffset
	if relativeLine < 0 {
		return viewOutput
	}

	lines := strings.Split(viewOutput, "\n")
	if relativeLine >= len(lines) {
		return viewOutput
	}
	lines[relativeLine] = highlight(selectedItemStyle, lines[relativeLine])
	return strings.Join(lines, "\n")
}

func activityDot(lastActive time.Time) string {
	if lastActive.IsZero() {
		return ""
	}
	age := time.Since(lastActive)
	switch {
	case age < 7*24*time.Hour:
		return lipgloss.NewStyle().Foreground(colorAccent).Render("●") // green - active this week
	case age < 30*24*time.Hour:
		return lipgloss.NewStyle().Foreground(colorWarm).Render("●") // yellow - active this month
	default:
		return lipgloss.NewStyle().Foreground(colorSubtle).Render("○") // dim - older
	}
}

func (m Model) filterSessions(sessions []data.Session) []data.Session {
	if m.sessionFilter == 0 {
		return sessions // "all" — no filtering
	}

	filterName := sessionFilterTypes[m.sessionFilter].name
	now := time.Now()

	var filtered []data.Session
	for _, s := range sessions {
		switch filterName {
		case "code":
			if s.MessageCount > 10 {
				filtered = append(filtered, s)
			}
		case "long":
			if s.MessageCount >= filterLongMinMessages {
				filtered = append(filtered, s)
			}
		case "recent":
			if now.Sub(s.StartedAt).Hours() < float64(filterRecentDays*24) {
				filtered = append(filtered, s)
			}
		}
	}
	return filtered
}
