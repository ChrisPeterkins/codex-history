package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/chrispeterkins/codex-history/internal/data"
)

// --- Message types ---

type projectsLoaded struct {
	projects []data.Project
}

type sessionsLoaded struct {
	sessions []data.Session
}

type messagesLoaded struct {
	messages []data.Message
}

// --- Commands ---

func loadProjects() tea.Msg {
	projects, err := data.LoadProjects()
	if err != nil {
		return projectsLoaded{}
	}
	return projectsLoaded{projects: projects}
}

func (m Model) loadSessionsCmd() tea.Cmd {
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

func (m Model) loadMessagesCmd() tea.Cmd {
	if m.sessionCursor >= len(m.sessions) {
		return nil
	}
	s := &m.sessions[m.sessionCursor]
	return func() tea.Msg {
		messages, err := data.LoadMessages(s)
		if err != nil {
			return messagesLoaded{}
		}
		return messagesLoaded{messages: messages}
	}
}
