package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrispeterkins/codex-history/internal/config"
	"github.com/chrispeterkins/codex-history/internal/data"
)

const (
	panelProjects = iota
	panelSessions
	panelConversation
)

// Model is the top-level Bubble Tea model.
type Model struct {
	projects []data.Project
	sessions []data.Session
	messages []data.Message

	focus         int // which panel is active
	projectCursor int
	sessionCursor int
	viewport      viewport.Model
	ready         bool
	fullScreen    bool

	width  int
	height int

	renderer *glamour.TermRenderer

	collapsed           map[string]bool
	collapsibleSections []sectionLine

	showHelp bool

	spinner        spinner.Model
	loading        bool
	loadError      string
	follow         bool
	followRevision string
	loadSeq        uint64

	// Scroll position memory (sessionID → YOffset)
	scrollPositions map[string]int

	userMessageLines []int

	searchMode    bool
	searchInput   textinput.Model
	searchResults []SearchResult
	searchCursor  int
	searchSeq     uint64
	searching     bool

	// Vim marks
	marks             map[rune]markPosition
	awaitingMark      markMode
	pendingMarkOffset *int // offset to restore after cross-session mark jump

	// In-conversation search
	convSearchMode    bool
	convSearchInput   textinput.Model
	convSearchMatches []int    // line numbers with matches
	convSearchContent []string // cached content lines (set on search entry)
	convSearchIdx     int      // current match index

	sessionFilter int // index into sessionFilterTypes

	statusMessage string
	statusExpiry  time.Time

	version     string
	updateAvail string // non-empty if a newer version exists

	themeIndex int
}

// SearchResult represents a match from search.
type SearchResult struct {
	ProjectIdx int
	SessionIdx int
	SessionID  string
	Preview    string
	Project    string
	Date       string
}

// NewModel creates and returns an initialized Model with default settings.
func NewModel(version string) Model {
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	ti := textinput.New()
	ti.Placeholder = "Search conversations..."
	ti.CharLimit = searchCharLimit

	csi := textinput.New()
	csi.Placeholder = "Find in conversation..."
	csi.CharLimit = searchCharLimit

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#88C0D0"))

	themeIdx := 0
	cfg := config.Get()
	if strings.EqualFold(cfg.Theme, "custom") && cfg.CustomTheme != nil {
		custom := buildCustomTheme(cfg.CustomTheme)
		themes = append(themes, custom)
		themeIdx = len(themes) - 1
		applyTheme(custom)
	} else if cfg.Theme != "" {
		for i, t := range themes {
			if strings.EqualFold(t.Name, cfg.Theme) {
				themeIdx = i
				applyTheme(themes[i])
				break
			}
		}
	}

	filterIdx := 0
	filterName := config.DefaultFilterName()
	for i, ft := range sessionFilterTypes {
		if strings.EqualFold(ft.name, filterName) {
			filterIdx = i
			break
		}
	}

	return Model{
		renderer:        r,
		collapsed:       make(map[string]bool),
		searchInput:     ti,
		spinner:         s,
		scrollPositions: make(map[string]int),
		marks:           make(map[rune]markPosition),
		convSearchInput: csi,
		themeIndex:      themeIdx,
		sessionFilter:   filterIdx,
		version:         version,
	}
}

func (m *Model) rebuildRenderer() {
	wrapWidth := m.conversationWidth() - 12
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	style := "dark"
	if m.themeIndex < len(themes) && themes[m.themeIndex].Name == "Light" {
		style = "light"
	}

	m.renderer, _ = glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(wrapWidth),
	)
}

func (m Model) conversationViewportSize() (int, int) {
	return max(1, m.conversationWidth()-6), max(1, m.contentHeight()-3)
}

func (m *Model) syncConversationViewport() {
	m.viewport.Width, m.viewport.Height = m.conversationViewportSize()
}

func (m *Model) refreshConversationLayout() {
	offset := m.viewport.YOffset
	m.rebuildRenderer()
	m.syncConversationViewport()
	if len(m.messages) > 0 {
		m.updateConversationContent()
	}
	m.viewport.SetYOffset(offset)
}

func (m *Model) updateConversationContent() {
	result := m.renderConversation()
	m.viewport.SetContent(result.content)
	m.userMessageLines = result.userLines
	m.collapsibleSections = result.sections
}

func (m Model) currentProjectKey() string {
	if m.projectCursor < len(m.projects) {
		return m.projects[m.projectCursor].DirName
	}
	return ""
}

func (m Model) currentSessionID() string {
	if m.sessionCursor < len(m.sessions) {
		return m.sessions[m.sessionCursor].ID
	}
	return ""
}

func (m *Model) saveScroll() {
	if id := m.currentSessionID(); id != "" {
		m.scrollPositions[id] = m.viewport.YOffset
	}
}

func (m *Model) selectProject(index int) tea.Cmd {
	m.saveScroll()
	m.followRevision = ""
	m.projectCursor, m.sessionCursor, m.loading = index, 0, true
	return m.loadSessionsCmd("", false)
}

func (m *Model) selectSession(index int) tea.Cmd {
	m.saveScroll()
	m.followRevision = ""
	m.sessionCursor, m.loading = index, true
	return tea.Batch(m.loadMessagesCmd(false, false), m.spinner.Tick)
}

func (m *Model) setLoadError(err error) {
	m.loading = false
	m.loadError = err.Error()
	m.statusMessage = "Error: " + m.loadError
}

func (m *Model) clearLoadError() {
	if strings.HasPrefix(m.statusMessage, "Error: ") {
		m.statusMessage = ""
	}
	m.loadError = ""
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadProjects, checkForUpdate(m.version))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.searchMode {
			return m.handleSearchKey(msg)
		}
		return m.handleKey(msg)

	case tea.MouseMsg:
		if !m.searchMode && !m.showHelp {
			return m.handleMouse(msg)
		}

	case tea.WindowSizeMsg:
		offset, initialized := m.viewport.YOffset, m.ready
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.rebuildRenderer()
		if !initialized {
			m.viewport = viewport.New(1, 1)
			m.viewport.Style = lipgloss.NewStyle()
		}
		m.syncConversationViewport()
		if len(m.messages) > 0 {
			m.updateConversationContent()
		}
		m.viewport.SetYOffset(offset)
		return m, nil

	case projectsLoaded:
		if msg.seq != m.loadSeq || msg.projectKey != "" && m.currentProjectKey() != msg.projectKey {
			return m, nil
		}
		if msg.err != nil {
			m.setLoadError(msg.err)
			return m, nil
		}
		m.projects = msg.projects
		m.projectCursor = 0
		for i := range m.projects {
			if m.projects[i].DirName == msg.projectKey {
				m.projectCursor = i
				break
			}
		}
		m.clearLoadError()
		if len(m.projects) > 0 {
			m.loading = true
			return m, m.loadSessionsCmd(msg.sessionID, msg.preserve)
		}
		return m, nil

	case sessionsLoaded:
		if msg.seq != m.loadSeq || msg.projectKey != m.currentProjectKey() {
			return m, nil
		}
		if msg.err != nil {
			m.setLoadError(msg.err)
			return m, nil
		}
		m.sessions = msg.sessions
		m.sessionCursor = 0
		for i := range m.sessions {
			if m.sessions[i].ID == msg.sessionID {
				m.sessionCursor = i
				break
			}
		}
		m.clearLoadError()
		if len(m.sessions) > 0 {
			m.loading = true
			return m, tea.Batch(m.loadMessagesCmd(msg.preserve, false), m.spinner.Tick)
		}
		m.messages = nil
		m.loading = false
		m.viewport.SetContent(emptyStyle.Render("No sessions found"))
		return m, nil

	case messagesLoaded:
		if msg.seq != m.loadSeq || msg.sessionID != m.currentSessionID() {
			return m, nil
		}
		if msg.err != nil {
			m.setLoadError(msg.err)
			return m, nil
		}
		offset, atBottom := m.viewport.YOffset, m.viewport.AtBottom()
		m.messages = msg.messages
		m.loading = false
		m.clearLoadError()
		if !msg.preserve {
			m.collapsed = make(map[string]bool)
		}
		m.updateConversationContent()
		if msg.follow && atBottom {
			m.viewport.GotoBottom()
		} else if msg.preserve {
			m.viewport.SetYOffset(offset)
		} else if m.pendingMarkOffset != nil {
			m.viewport.SetYOffset(*m.pendingMarkOffset)
			m.pendingMarkOffset = nil
		} else if m.sessionCursor < len(m.sessions) {
			if offset, ok := m.scrollPositions[m.sessions[m.sessionCursor].ID]; ok {
				m.viewport.SetYOffset(offset)
			} else {
				m.viewport.GotoTop()
			}
		} else {
			m.viewport.GotoTop()
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case followTickMsg:
		if m.follow && m.currentSessionID() != "" {
			return m, tea.Batch(m.followCheckCmd(), followTick())
		}
		return m, nil

	case followCheckMsg:
		if !m.follow || msg.sessionID != m.currentSessionID() {
			return m, nil
		}
		if msg.err != nil {
			m.statusMessage = "Follow error: " + msg.err.Error()
			return m, nil
		}
		if strings.HasPrefix(m.statusMessage, "Follow error: ") {
			m.statusMessage = ""
		}
		if msg.revision != m.followRevision {
			m.followRevision = msg.revision
			return m, m.loadMessagesCmd(true, true)
		}
		return m, nil

	case updateAvailableMsg:
		m.updateAvail = msg.version
		return m, nil

	case searchDelayMsg:
		if m.searchMode && msg.seq == m.searchSeq && msg.query == m.searchInput.Value() {
			return m, m.searchCmd(msg.query, msg.seq)
		}
		return m, nil

	case searchResultsMsg:
		if m.searchMode && msg.seq == m.searchSeq {
			m.searchResults, m.searchCursor = msg.results, 0
			m.searching = false
			if msg.err != nil {
				m.statusMessage = "Search content unavailable: " + msg.err.Error()
			} else if strings.HasPrefix(m.statusMessage, "Search content unavailable: ") {
				m.statusMessage = ""
			}
		}
		return m, nil

	case clipboardCopiedMsg:
		if msg.err != nil {
			m.statusMessage = "Copy failed: " + msg.err.Error()
		} else {
			m.statusMessage = "Copied to clipboard!"
		}
		return m, clearStatusAfter(2 * time.Second)

	case statusClearMsg:
		m.statusMessage = ""
		return m, nil
	}

	if m.focus == panelConversation {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	if m.showHelp {
		return fitToScreen(m.renderHelpOverlay(), m.width, m.height)
	}

	if m.searchMode {
		return fitToScreen(m.renderSearchView(), m.width, m.height)
	}

	var main string
	if m.fullScreen || m.width < breakpointNarrow {
		switch m.focus {
		case panelProjects:
			main = m.renderProjectsPanel()
		case panelSessions:
			main = m.renderSessionsPanel()
		default:
			main = m.renderConversationPanel()
		}
	} else if m.width < breakpointMedium {
		switch m.focus {
		case panelProjects:
			main = lipgloss.JoinHorizontal(lipgloss.Top,
				m.renderProjectsPanel(), m.renderSessionsPanel())
		default:
			main = lipgloss.JoinHorizontal(lipgloss.Top,
				m.renderSessionsPanel(), m.renderConversationPanel())
		}
	} else {
		projectsPanel := m.renderProjectsPanel()
		sessionsPanel := m.renderSessionsPanel()
		convoPanel := m.renderConversationPanel()
		main = lipgloss.JoinHorizontal(lipgloss.Top, projectsPanel, sessionsPanel, convoPanel)
	}

	header := m.renderHeader()
	help := m.renderHelp()

	return fitToScreen(lipgloss.JoinVertical(lipgloss.Left, header, main, help), m.width, m.height)
}

func fitToScreen(view string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
		lines[i] += strings.Repeat(" ", width-lipgloss.Width(lines[i]))
	}
	return strings.Join(lines, "\n")
}

type statusClearMsg struct{}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return statusClearMsg{}
	})
}
