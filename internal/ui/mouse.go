package ui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x := msg.X
	y := msg.Y

	panel := m.panelAtX(x)

	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		return m.handleMouseClick(panel, x, y)
	case msg.Button == tea.MouseButtonWheelUp:
		return m.handleMouseScroll(panel, -1)
	case msg.Button == tea.MouseButtonWheelDown:
		return m.handleMouseScroll(panel, 1)
	}

	return m, nil
}

func (m Model) panelAtX(x int) int {
	if m.fullScreen || m.width < breakpointNarrow {
		return m.focus
	}
	if m.width < breakpointMedium {
		if m.focus == panelProjects {
			if x < m.projectsWidth() {
				return panelProjects
			}
			return panelSessions
		}
		if x < m.sessionsWidth() {
			return panelSessions
		}
		return panelConversation
	}
	if x < m.projectsWidth() {
		return panelProjects
	}
	if x < m.projectsWidth()+m.sessionsWidth() {
		return panelSessions
	}
	return panelConversation
}

func (m Model) handleMouseClick(panel, x, y int) (tea.Model, tea.Cmd) {
	oldFocus := m.focus
	m.focus = panel
	if m.focus != oldFocus {
		m.rebuildRendererIfNeeded()
	}
	contentY := y - screenHeaderHeight - panelTopChrome

	switch panel {
	case panelProjects:
		if contentY < 0 {
			return m, nil
		}
		visibleStart, _ := m.visibleRange(m.projectCursor, len(m.projects), m.contentHeight()-3)
		idx := visibleStart + contentY
		if idx >= 0 && idx < len(m.projects) && idx != m.projectCursor {
			return m, m.selectProject(idx)
		}

	case panelSessions:
		if contentY < 0 {
			return m, nil
		}
		items := m.sessionListItems()
		start, end := m.visibleSessionRange(items)
		row := 0
		for _, item := range items[start:end] {
			height := 2
			if item.isGroupHeader {
				height = 1
			}
			if contentY < row+height {
				if contentY >= row && !item.isGroupHeader && item.origIdx != m.sessionCursor {
					return m, m.selectSession(item.origIdx)
				}
				break
			}
			row += height
		}

	case panelConversation:
		clickedRelLine := contentY
		if clickedRelLine >= 0 {
			clickedAbsLine := m.viewport.YOffset + clickedRelLine
			for _, section := range m.collapsibleSections {
				if section.line == clickedAbsLine {
					m.collapsed[section.key] = !m.isCollapsed(section.key)
					offset := m.viewport.YOffset
					m.updateConversationContent()
					m.viewport.SetYOffset(offset)
					return m, nil
				}
			}
		}
	}

	return m, nil
}

func (m Model) handleMouseScroll(panel, dir int) (tea.Model, tea.Cmd) {
	switch panel {
	case panelProjects:
		newCursor := m.projectCursor + dir
		if newCursor >= 0 && newCursor < len(m.projects) {
			return m, m.selectProject(newCursor)
		}
	case panelSessions:
		newCursor := m.sessionCursor + dir
		if newCursor >= 0 && newCursor < len(m.sessions) {
			return m, m.selectSession(newCursor)
		}
	case panelConversation:
		if dir < 0 {
			m.viewport.LineUp(3)
		} else {
			m.viewport.LineDown(3)
		}
	}
	return m, nil
}
