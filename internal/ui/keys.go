package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chrispeterkins/codex-history/internal/config"
)

// handleKey dispatches keyboard input to focused handlers.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help overlay intercepts all keys
	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	// In-conversation search mode
	if m.convSearchMode {
		return m.handleConvSearchKey(msg)
	}

	// Mark input mode (waiting for a-z after m or ')
	if m.awaitingMark != markNone {
		return m.handleMarkKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	}

	// Try action keys, then nav keys, then panel keys
	if model, cmd, handled := m.handleActionKeys(msg); handled {
		return model, cmd
	}
	if model, cmd, handled := m.handleNavKeys(msg); handled {
		return model, cmd
	}
	if model, cmd, handled := m.handlePanelKeys(msg); handled {
		return model, cmd
	}

	// Forward unhandled keys to viewport when in conversation
	if m.focus == panelConversation {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleActionKeys handles global action keybindings (search, theme, copy, marks, etc.).
func (m Model) handleActionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "m":
		m.awaitingMark = markSet
		m.statusMessage = "Set mark: a-z"
		return m, nil, true

	case "'":
		m.awaitingMark = markJump
		m.statusMessage = "Jump to mark: a-z"
		return m, nil, true

	case "ctrl+f":
		if m.focus == panelConversation && len(m.messages) > 0 {
			m.convSearchMode = true
			m.convSearchInput.Focus()
			m.convSearchMatches = nil
			m.convSearchIdx = 0
			// Cache content lines once for fast searching
			result := m.renderConversation()
			m.convSearchContent = strings.Split(result.content, "\n")
			return m, textinput.Blink, true
		}

	case "/":
		m.searchMode = true
		m.searchInput.Focus()
		m.searchResults = nil
		m.searchCursor = 0
		return m, textinput.Blink, true

	case "t":
		m.themeIndex = (m.themeIndex + 1) % len(themes)
		applyTheme(themes[m.themeIndex])
		m.statusMessage = "Theme: " + themes[m.themeIndex].Name
		m.rebuildRenderer()
		if len(m.messages) > 0 {
			m.updateConversationContent()
		}
		// Persist theme choice
		cfg := config.Get()
		cfg.Theme = themes[m.themeIndex].Name
		config.Save(cfg)
		return m, clearStatusAfter(2 * time.Second), true

	case "f":
		m.fullScreen = !m.fullScreen
		if m.fullScreen {
			m.focus = panelConversation
		}
		m.refreshConversationLayout()
		return m, nil, true

	case "r":
		m.loading = true
		return m, tea.Batch(m.refreshCmd(), m.spinner.Tick), true

	case "l":
		m.follow = !m.follow
		if m.follow {
			m.statusMessage = "Live follow: on"
			m.followRevision = ""
			return m, tea.Batch(m.followCheckCmd(), followTick()), true
		}
		m.loadSeq++
		m.statusMessage = "Live follow: off"
		return m, clearStatusAfter(2 * time.Second), true

	case "y":
		if len(m.messages) > 0 {
			return m, m.copyConversationCmd(), true
		}
		return m, nil, true

	case "n":
		if m.focus == panelConversation {
			m.jumpToNextUserMessage(1)
		}
		return m, nil, true

	case "N":
		if m.focus == panelConversation {
			m.jumpToNextUserMessage(-1)
		}
		return m, nil, true

	case " ":
		if m.focus == panelConversation {
			m.toggleSelectedSection()
			return m, nil, true
		}

	case "[", "]":
		if m.focus == panelConversation {
			dir := 1
			if msg.String() == "[" {
				dir = -1
			}
			m.jumpToCollapsible(dir)
		}
		return m, nil, true

	case "F":
		m.sessionFilter = (m.sessionFilter + 1) % len(sessionFilterTypes)
		m.sessionCursor = 0
		m.statusMessage = "Filter: " + sessionFilterTypes[m.sessionFilter].label
		// Persist filter choice
		cfg := config.Get()
		cfg.DefaultFilter = sessionFilterTypes[m.sessionFilter].name
		config.Save(cfg)
		return m, clearStatusAfter(2 * time.Second), true

	case "a":
		if m.focus == panelConversation {
			offset := m.viewport.YOffset
			m.expandAll()
			m.updateConversationContent()
			m.viewport.SetYOffset(offset)
			return m, nil, true
		}

	case "A":
		if m.focus == panelConversation {
			offset := m.viewport.YOffset
			m.collapseAll()
			m.updateConversationContent()
			m.viewport.SetYOffset(offset)
			return m, nil, true
		}
	}

	return m, nil, false
}

// handleNavKeys handles cursor movement and scrolling within panels.
func (m Model) handleNavKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "up", "k":
		switch m.focus {
		case panelProjects:
			if m.projectCursor > 0 {
				return m, m.selectProject(m.projectCursor - 1), true
			}
		case panelSessions:
			if m.sessionCursor > 0 {
				return m, m.selectSession(m.sessionCursor - 1), true
			}
		case panelConversation:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd, true
		}

	case "down", "j":
		switch m.focus {
		case panelProjects:
			if m.projectCursor < len(m.projects)-1 {
				return m, m.selectProject(m.projectCursor + 1), true
			}
		case panelSessions:
			if m.sessionCursor < len(m.sessions)-1 {
				return m, m.selectSession(m.sessionCursor + 1), true
			}
		case panelConversation:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd, true
		}

	case "g":
		switch m.focus {
		case panelProjects:
			if m.projectCursor != 0 {
				return m, m.selectProject(0), true
			}
		case panelSessions:
			if m.sessionCursor != 0 {
				return m, m.selectSession(0), true
			}
		case panelConversation:
			m.viewport.GotoTop()
		}
		return m, nil, true

	case "G":
		switch m.focus {
		case panelProjects:
			last := len(m.projects) - 1
			if last >= 0 && m.projectCursor != last {
				return m, m.selectProject(last), true
			}
		case panelSessions:
			last := len(m.sessions) - 1
			if last >= 0 && m.sessionCursor != last {
				return m, m.selectSession(last), true
			}
		case panelConversation:
			m.viewport.GotoBottom()
		}
		return m, nil, true

	case "pgup":
		switch m.focus {
		case panelProjects:
			return m, m.selectProject(clamp(m.projectCursor-m.contentHeight()/2, 0, max(0, len(m.projects)-1))), true
		case panelSessions:
			return m, m.selectSession(clamp(m.sessionCursor-m.contentHeight()/2, 0, max(0, len(m.sessions)-1))), true
		case panelConversation:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd, true
		}

	case "pgdown":
		switch m.focus {
		case panelProjects:
			return m, m.selectProject(clamp(m.projectCursor+m.contentHeight()/2, 0, max(0, len(m.projects)-1))), true
		case panelSessions:
			return m, m.selectSession(clamp(m.sessionCursor+m.contentHeight()/2, 0, max(0, len(m.sessions)-1))), true
		case panelConversation:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd, true
		}
	}

	return m, nil, false
}

// handlePanelKeys handles panel focus switching and sliding.
func (m Model) handlePanelKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "tab":
		if m.fullScreen {
			return m, nil, true
		}
		oldFocus := m.focus
		switch m.visiblePanelCount() {
		case 1:
			// Single panel: tab does nothing
		case 2:
			if m.focus == panelProjects {
				m.focus = panelSessions
			} else if m.focus == panelSessions {
				if m.width < breakpointMedium && m.isShowingProjectsSessions() {
					m.focus = panelProjects
				} else {
					m.focus = panelConversation
				}
			} else {
				m.focus = panelSessions
			}
		default:
			m.focus = (m.focus + 1) % 3
		}
		if m.focus != oldFocus {
			m.rebuildRendererIfNeeded()
		}
		return m, nil, true

	case "shift+tab":
		if m.fullScreen {
			return m, nil, true
		}
		oldFocus := m.focus
		switch m.visiblePanelCount() {
		case 1:
			// Single panel: shift+tab does nothing
		case 2:
			if m.focus == panelConversation {
				m.focus = panelSessions
			} else if m.focus == panelSessions {
				if m.width < breakpointMedium && !m.isShowingProjectsSessions() {
					m.focus = panelConversation
				} else {
					m.focus = panelProjects
				}
			} else {
				m.focus = panelSessions
			}
		default:
			m.focus = (m.focus + 2) % 3
		}
		if m.focus != oldFocus {
			m.rebuildRendererIfNeeded()
		}
		return m, nil, true

	case "enter":
		if m.focus == panelConversation {
			m.toggleSelectedSection()
		} else {
			m.focus++
			m.rebuildRendererIfNeeded()
		}
		return m, nil, true

	case "esc":
		if m.fullScreen {
			m.fullScreen = false
			m.refreshConversationLayout()
			return m, nil, true
		}
		if m.focus > panelProjects {
			m.focus--
			m.rebuildRendererIfNeeded()
		}
		return m, nil, true
	}

	return m, nil, false
}

// --- Navigation helpers ---

// jumpToNextUserMessage scrolls viewport to the next (dir=1) or previous (dir=-1) user message.
func (m *Model) jumpToNextUserMessage(dir int) {
	if len(m.userMessageLines) == 0 {
		return
	}

	currentLine := m.viewport.YOffset

	if dir > 0 {
		for _, line := range m.userMessageLines {
			if line > currentLine+1 {
				m.viewport.SetYOffset(line)
				return
			}
		}
	} else {
		for i := len(m.userMessageLines) - 1; i >= 0; i-- {
			if m.userMessageLines[i] < currentLine-1 {
				m.viewport.SetYOffset(m.userMessageLines[i])
				return
			}
		}
	}
}

func (m *Model) toggleSelectedSection() {
	if section, ok := m.activeCollapsibleSection(); ok {
		offset := m.viewport.YOffset
		m.collapsed[section.key] = !m.isCollapsed(section.key)
		m.updateConversationContent()
		m.viewport.SetYOffset(offset)
	}
}

func (m *Model) jumpToCollapsible(dir int) {
	if len(m.collapsibleSections) == 0 {
		return
	}
	idx := len(m.collapsibleSections)
	if dir > 0 {
		idx = -1
	}
	if active, ok := m.activeCollapsibleSection(); ok {
		for i, section := range m.collapsibleSections {
			if section == active {
				idx = i
				break
			}
		}
	}
	idx = (idx + dir + len(m.collapsibleSections)) % len(m.collapsibleSections)
	m.viewport.SetYOffset(max(0, m.collapsibleSections[idx].line-m.viewport.Height/2))
}

func (m Model) activeCollapsibleSection() (sectionLine, bool) {
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height
	target, bestDist := top+m.viewport.Height/2, 0
	var best sectionLine
	found := false
	for _, section := range m.collapsibleSections {
		if section.line < top {
			continue
		}
		if section.line >= bottom {
			break
		}
		dist := target - section.line
		if dist < 0 {
			dist = -dist
		}
		if !found || dist < bestDist {
			best, bestDist, found = section, dist, true
		}
	}
	return best, found
}

// expandAll expands all collapsible sections in the current conversation.
func (m *Model) expandAll() {
	for k := range m.collapsed {
		m.collapsed[k] = false
	}
}

// collapseAll collapses all collapsible sections in the current conversation.
func (m *Model) collapseAll() {
	for _, msg := range m.messages {
		for _, block := range msg.ContentBlocks {
			switch block.Type {
			case "thinking":
				m.collapsed["thinking:"+msg.UUID] = true
			case "tool_use":
				m.collapsed["tool:"+block.ToolID] = true
			}
		}
	}
}

// --- Layout helpers ---

// visiblePanelCount returns how many panels are shown at the current width.
func (m Model) visiblePanelCount() int {
	if m.fullScreen || m.width < breakpointNarrow {
		return 1
	}
	if m.width < breakpointMedium {
		return 2
	}
	return 3
}

// isShowingProjectsSessions returns true if the 2-panel view is showing
// projects+sessions (as opposed to sessions+conversation).
func (m Model) isShowingProjectsSessions() bool {
	return m.focus == panelProjects
}

func (m *Model) rebuildRendererIfNeeded() {
	if m.visiblePanelCount() < 3 {
		m.refreshConversationLayout()
	}
}
