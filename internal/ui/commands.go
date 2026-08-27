package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/chrispeterkins/codex-history/internal/data"
)

// --- Message types ---

type projectsLoaded struct {
	projects              []data.Project
	projectKey, sessionID string
	seq                   uint64
	preserve              bool
	err                   error
}

type sessionsLoaded struct {
	projectKey, sessionID string
	sessions              []data.Session
	seq                   uint64
	preserve              bool
	err                   error
}

type messagesLoaded struct {
	sessionID string
	messages  []data.Message
	seq       uint64
	preserve  bool
	follow    bool
	err       error
}

type followTickMsg struct{}

type followCheckMsg struct {
	sessionID, revision string
	err                 error
}

func followTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return followTickMsg{} })
}

func (m Model) followCheckCmd() tea.Cmd {
	if m.sessionCursor >= len(m.sessions) {
		return nil
	}
	s, sessionID := &m.sessions[m.sessionCursor], m.sessions[m.sessionCursor].ID
	return func() tea.Msg {
		revision, err := data.ConversationRevision(s)
		return followCheckMsg{sessionID: sessionID, revision: revision, err: err}
	}
}

// --- Commands ---

func loadProjects() tea.Msg {
	projects, err := data.LoadProjects()
	return projectsLoaded{projects: projects, err: err}
}

func (m *Model) refreshCmd() tea.Cmd {
	projectKey, sessionID := m.currentProjectKey(), m.currentSessionID()
	m.loadSeq++
	seq := m.loadSeq
	return func() tea.Msg {
		projects, err := data.LoadProjects()
		return projectsLoaded{projects: projects, projectKey: projectKey, sessionID: sessionID, seq: seq, preserve: true, err: err}
	}
}

func (m *Model) loadSessionsCmd(sessionID string, preserve bool) tea.Cmd {
	if m.projectCursor >= len(m.projects) {
		return nil
	}
	m.loadSeq++
	seq := m.loadSeq
	p, projectKey := &m.projects[m.projectCursor], m.projects[m.projectCursor].DirName
	return func() tea.Msg {
		sessions, err := data.LoadSessions(p)
		return sessionsLoaded{projectKey: projectKey, sessionID: sessionID, sessions: sessions, seq: seq, preserve: preserve, err: err}
	}
}

func (m *Model) loadMessagesCmd(preserve, follow bool) tea.Cmd {
	if m.sessionCursor >= len(m.sessions) {
		return nil
	}
	m.loadSeq++
	seq := m.loadSeq
	s, sessionID := &m.sessions[m.sessionCursor], m.sessions[m.sessionCursor].ID
	return func() tea.Msg {
		messages, err := data.LoadMessages(s)
		return messagesLoaded{sessionID: sessionID, messages: messages, seq: seq, preserve: preserve, follow: follow, err: err}
	}
}
