package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/sftp"

	"github.com/Mald0r0r000/LOOTup/internal/config"
	"github.com/Mald0r0r000/LOOTup/internal/session"
	"github.com/Mald0r0r000/LOOTup/internal/ssh"
	"github.com/Mald0r0r000/LOOTup/internal/template"
	"github.com/Mald0r0r000/LOOTup/internal/transfer"
)

// State represents the current TUI state
type State int

const (
	StateHostInput      State = iota // Step 1: fill host/user/key/dest-path
	StateSessionMode                 // Step 2: new / merge / resume
	StateProjectName                 // Step 3a: type project name
	StateProjectBrowser              // Step 3b: browse existing projects
	StateSessionName                 // Step 4: type session name
	StateSourceBrowser               // Step 5: pick local source
	StateConfig                      // Step 6: summary before transfer
	StateConnecting
	StateTransfer
	StateDone
	StateError
)

// Session mode options
const (
	ModeNew    = "new"
	ModeMerge  = "merge"
	ModeResume = "resume"
)

var sessionModeLabels = []struct {
	mode  string
	label string
	desc  string
}{
	{ModeNew, "New session", "New project or new day on existing project"},
	{ModeMerge, "Add to session", "Merge — add files to existing session"},
	{ModeResume, "Resume transfer", "Continue interrupted transfer"},
}

// --- Tea Messages ---

type sshConnectedMsg struct {
	err       error
	sshClient *ssh.Client
}

type connectResultMsg struct {
	err        error
	transferer *transfer.Transferer
	sshClient  *ssh.Client
}

type transferStartMsg struct{}

type transferProgressMsg struct {
	msg transfer.ProgressMsg
}

type transferDoneMsg struct {
	err error
}

type postTransferMsg struct {
	output string
	err    error
}

type remoteProjectsMsg struct {
	dirs []dirEntry
	err  error
}

type sessionStateMsg struct {
	state *session.ProjectState
	err   error
}

// --- Model ---

// Model is the root Bubble Tea model for LOOTup
type Model struct {
	cfg     *config.Config
	state   State
	spinner spinner.Model
	err     error

	// Host input form
	hostForm hostInputModel

	// Session mode
	sessionModeCursor int

	// Project/Session name input
	projectNameInput textinput.Model
	sessionNameInput textinput.Model

	// Project browser (remote)
	projectBrowserPath   string
	projectBrowserCursor int
	projectBrowserDirs   []dirEntry
	projectState         *session.ProjectState
	sessionListCursor    int
	showSessionList      bool

	// Source browser (local)
	volumes       []string
	browserPath   string
	browserCursor int
	browserDirs   []dirEntry

	// Connections
	sshClient  *ssh.Client
	sftpClient *sftp.Client

	// Transfer state
	transferer  *transfer.Transferer
	fileCount   int
	totalBytes  int64
	bytesSent   int64
	filesDone   int
	currentFile string
	transferErr error

	// Post-transfer
	postOutput string

	// UI
	width  int
	height int
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = titleStyle

	m := Model{
		cfg:     cfg,
		spinner: s,
	}

	if cfg.IsInteractive() {
		// Interactive mode — start with host input
		m.state = StateHostInput
		m.hostForm = newHostInputModel(
			cfg.Host, cfg.User, cfg.KeyPath, cfg.DestPath,
			cfg.Template, cfg.Concurrency,
		)
	} else {
		// CLI mode — skip to config summary
		m.state = StateConfig
	}

	return m
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch m.state {
		case StateHostInput:
			return m.updateHostInput(msg)
		case StateSessionMode:
			return m.updateSessionMode(msg)
		case StateProjectName:
			return m.updateProjectName(msg)
		case StateProjectBrowser:
			return m.updateProjectBrowser(msg)
		case StateSessionName:
			return m.updateSessionName(msg)
		case StateSourceBrowser:
			return m.updateBrowser(msg)
		default:
			return m.updateDefault(msg)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case sshConnectedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			return m, nil
		}
		m.sshClient = msg.sshClient
		m.state = StateSessionMode
		return m, nil

	case remoteProjectsMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			return m, nil
		}
		m.projectBrowserDirs = msg.dirs
		m.projectBrowserCursor = 0
		m.state = StateProjectBrowser
		return m, nil

	case sessionStateMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			return m, nil
		}
		m.projectState = msg.state
		if len(msg.state.Sessions) > 0 {
			m.showSessionList = true
			m.sessionListCursor = 0
		}
		return m, nil

	case connectResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			return m, nil
		}
		m.transferer = msg.transferer
		if msg.sshClient != nil {
			m.sshClient = msg.sshClient
		}
		m.fileCount = len(m.transferer.Files())
		m.totalBytes = m.transferer.TotalBytes()
		m.state = StateTransfer
		return m, m.startTransfer()

	case transferStartMsg:
		return m, m.waitForProgress()

	case transferProgressMsg:
		m.bytesSent = msg.msg.BytesSent
		m.currentFile = msg.msg.File.RelPath
		if msg.msg.Done {
			m.filesDone++
		}
		if msg.msg.Err != nil {
			m.transferErr = msg.msg.Err
		}
		return m, m.waitForProgress()

	case transferDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			m.cleanup()
			return m, nil
		}
		// Save session state on completion
		m.saveSessionState()
		m.state = StateDone
		m.cleanup()
		return m, nil
	}

	return m, nil
}

// ========== STATE HANDLERS ==========

// --- Host Input ---

func (m Model) updateHostInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "enter":
		host, user, keyPath, destPath, tmpl, workersStr := m.hostForm.Values()
		m.cfg.Host = host
		m.cfg.User = user
		if keyPath != "" {
			m.cfg.KeyPath = keyPath
		}
		m.cfg.DestPath = destPath
		m.cfg.Template = tmpl
		if w, err := strconv.Atoi(workersStr); err == nil && w > 0 {
			m.cfg.Concurrency = w
		}
		// Connect SSH silently, then go to session mode
		m.state = StateConnecting
		return m, tea.Batch(m.spinner.Tick, m.connectSSH())

	default:
		cmd := m.hostForm.Update(msg)
		return m, cmd
	}
}

// --- Session Mode ---

func (m Model) updateSessionMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.cleanup()
		return m, tea.Quit

	case "up":
		if m.sessionModeCursor > 0 {
			m.sessionModeCursor--
		}

	case "down":
		if m.sessionModeCursor < len(sessionModeLabels)-1 {
			m.sessionModeCursor++
		}

	case "enter":
		selected := sessionModeLabels[m.sessionModeCursor]
		m.cfg.SessionMode = selected.mode

		switch selected.mode {
		case ModeNew:
			// New session → project name input
			m.projectNameInput = textinput.New()
			m.projectNameInput.Placeholder = "YYYYMMDD_PROJECTNAME"
			m.projectNameInput.SetValue(time.Now().Format("20060102") + "_")
			m.projectNameInput.CharLimit = 128
			m.projectNameInput.Width = 40
			m.projectNameInput.Focus()
			m.state = StateProjectName
			return m, textinput.Blink

		case ModeMerge, ModeResume:
			// Browse existing projects on remote
			return m, m.listRemoteProjects()
		}
	}

	return m, nil
}

// --- Project Name ---

func (m Model) updateProjectName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit

	case "esc":
		m.state = StateSessionMode
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.projectNameInput.Value())
		if name == "" {
			break
		}
		m.cfg.ProjectName = name
		// Go to session name input
		m.sessionNameInput = textinput.New()
		m.sessionNameInput.Placeholder = "YYYYMMDD_SESSIONNAME"
		m.sessionNameInput.SetValue(time.Now().Format("20060102") + "_")
		m.sessionNameInput.CharLimit = 128
		m.sessionNameInput.Width = 40
		m.sessionNameInput.Focus()
		m.state = StateSessionName
		return m, textinput.Blink

	default:
		var cmd tea.Cmd
		m.projectNameInput, cmd = m.projectNameInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Project Browser (remote) ---

func (m Model) updateProjectBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit

	case "esc":
		if m.showSessionList {
			m.showSessionList = false
			return m, nil
		}
		m.state = StateSessionMode
		return m, nil

	case "up":
		if m.showSessionList {
			if m.sessionListCursor > 0 {
				m.sessionListCursor--
			}
		} else {
			if m.projectBrowserCursor > 0 {
				m.projectBrowserCursor--
			}
		}

	case "down":
		if m.showSessionList {
			if m.projectState != nil && m.sessionListCursor < len(m.projectState.Sessions)-1 {
				m.sessionListCursor++
			}
		} else {
			if m.projectBrowserCursor < len(m.projectBrowserDirs)-1 {
				m.projectBrowserCursor++
			}
		}

	case "enter", " ":
		if m.showSessionList {
			// Select session
			if m.projectState != nil && len(m.projectState.Sessions) > 0 {
				sess := m.projectState.Sessions[m.sessionListCursor]
				m.cfg.SessionName = sess.Name
				if m.cfg.SessionMode == ModeResume {
					// Resume: go to source browser
					m.initSourceBrowser()
					m.state = StateSourceBrowser
				} else {
					// Merge: go to session name (pre-filled)
					m.sessionNameInput = textinput.New()
					m.sessionNameInput.SetValue(sess.Name)
					m.sessionNameInput.CharLimit = 128
					m.sessionNameInput.Width = 40
					m.sessionNameInput.Focus()
					m.state = StateSessionName
					return m, textinput.Blink
				}
			}
			return m, nil
		}

		// Select project → load session state
		if len(m.projectBrowserDirs) > 0 {
			selected := m.projectBrowserDirs[m.projectBrowserCursor]
			m.cfg.ProjectName = selected.Name
			m.showSessionList = true
			return m, m.loadProjectSessions(selected.Name)
		}
	}

	return m, nil
}

// --- Session Name ---

func (m Model) updateSessionName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit

	case "esc":
		if m.cfg.SessionMode == ModeNew {
			m.state = StateProjectName
		} else {
			m.state = StateProjectBrowser
		}
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.sessionNameInput.Value())
		if name == "" {
			break
		}
		m.cfg.SessionName = name
		// Go to source browser
		m.initSourceBrowser()
		m.state = StateSourceBrowser
		return m, nil

	default:
		var cmd tea.Cmd
		m.sessionNameInput, cmd = m.sessionNameInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- Source Browser ---

func (m Model) updateBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.cleanup()
		return m, tea.Quit

	case "esc":
		m.state = StateSessionName
		return m, nil

	case "up":
		if m.browserCursor > 0 {
			m.browserCursor--
		}

	case "down":
		if m.browserCursor < len(m.browserDirs)-1 {
			m.browserCursor++
		}

	case "right", "enter":
		if len(m.browserDirs) == 0 {
			break
		}
		selected := m.browserDirs[m.browserCursor]
		m.browserPath = selected.Path
		m.browserDirs = listDirEntries(selected.Path)
		m.browserCursor = 0

	case "left":
		parent := filepath.Dir(m.browserPath)
		if parent != m.browserPath {
			m.browserPath = parent
			m.browserDirs = listDirEntries(parent)
			m.browserCursor = 0
		}

	case " ":
		m.cfg.Source = m.browserPath
		m.state = StateConfig
		return m, nil
	}

	return m, nil
}

// --- Default key handling (Config, Done, Error) ---

func (m Model) updateDefault(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.cleanup()
		return m, tea.Quit
	case "enter":
		return m.handleEnter()
	}
	return m, nil
}

// ========== VIEW ==========

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(renderLogo())
	b.WriteString(dimStyle.Render(fmt.Sprintf("  v%s", m.cfg.Version)))
	b.WriteString("\n\n")

	switch m.state {

	case StateHostInput:
		b.WriteString(titleStyle.Render("  Remote Configuration"))
		b.WriteString("\n\n")
		b.WriteString(m.hostForm.View())

	case StateSessionMode:
		b.WriteString(m.viewSessionMode())

	case StateProjectName:
		b.WriteString(titleStyle.Render("  Project Name"))
		b.WriteString("\n\n")
		b.WriteString("  " + m.projectNameInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Type project name  •  Enter: confirm  •  Esc: back"))

	case StateProjectBrowser:
		b.WriteString(m.viewProjectBrowser())

	case StateSessionName:
		b.WriteString(titleStyle.Render("  Session Name"))
		b.WriteString("\n\n")
		b.WriteString("  " + m.sessionNameInput.View())
		// Show existing sessions as hints
		if m.projectState != nil && len(m.projectState.Sessions) > 0 {
			b.WriteString("\n\n")
			b.WriteString(dimStyle.Render("  Existing sessions:"))
			b.WriteString("\n")
			for _, s := range m.projectState.Sessions {
				status := dimStyle.Render(s.Status)
				if s.Status == "complete" {
					status = successStyle.Render("✓")
				}
				b.WriteString(dimStyle.Render(fmt.Sprintf("    %s %s (%d files)", status, s.Name, s.Files)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  Type session name  •  Enter: confirm  •  Esc: back"))

	case StateSourceBrowser:
		b.WriteString(m.viewBrowser())

	case StateConfig:
		b.WriteString(m.viewConfig())

	case StateConnecting:
		b.WriteString(fmt.Sprintf("  %s Connecting to %s@%s...\n",
			m.spinner.View(), m.cfg.User, m.cfg.Host))

	case StateTransfer:
		b.WriteString(m.viewTransfer())

	case StateDone:
		b.WriteString(m.viewDone())

	case StateError:
		b.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ Error: %v", m.err)))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Press q to quit"))
	}

	b.WriteString("\n")
	return b.String()
}

func (m Model) viewSessionMode() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("  Session Mode"))
	b.WriteString("\n\n")

	for i, opt := range sessionModeLabels {
		cursor := "  "
		if i == m.sessionModeCursor {
			cursor = successStyle.Render("▸ ")
		}

		label := valueStyle.Render(fmt.Sprintf("%d. %s", i+1, opt.label))
		if i == m.sessionModeCursor {
			label = successStyle.Render(fmt.Sprintf("%d. %s", i+1, opt.label))
		}

		b.WriteString(fmt.Sprintf("  %s%s\n", cursor, label))
		b.WriteString(fmt.Sprintf("      %s\n", dimStyle.Render(opt.desc)))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓: navigate  •  Enter: select  •  Esc: quit"))
	return b.String()
}

func (m Model) viewProjectBrowser() string {
	var b strings.Builder

	if m.showSessionList {
		b.WriteString(titleStyle.Render(fmt.Sprintf("  Sessions — %s", m.cfg.ProjectName)))
		b.WriteString("\n\n")

		if m.projectState == nil || len(m.projectState.Sessions) == 0 {
			b.WriteString(dimStyle.Render("  No sessions found in this project."))
			b.WriteString("\n")
		} else {
			for i, s := range m.projectState.Sessions {
				cursor := "  "
				if i == m.sessionListCursor {
					cursor = successStyle.Render("▸ ")
				}

				status := dimStyle.Render("●")
				if s.Status == "complete" {
					status = successStyle.Render("✓")
				} else {
					status = errorStyle.Render("…")
				}

				name := valueStyle.Render(s.Name)
				if i == m.sessionListCursor {
					name = successStyle.Render(s.Name)
				}

				info := dimStyle.Render(fmt.Sprintf("  %s — %d files, %s",
					s.Date, s.Files, transfer.FormatBytes(s.Bytes)))

				b.WriteString(fmt.Sprintf("  %s%s %s%s\n", cursor, status, name, info))
			}
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  Enter/Space: select  •  Esc: back"))
		return b.String()
	}

	b.WriteString(titleStyle.Render("  Select Project"))
	b.WriteString("\n\n")

	b.WriteString(labelStyle.Render("  📂 " + m.cfg.DestPath))
	b.WriteString("\n\n")

	if len(m.projectBrowserDirs) == 0 {
		b.WriteString(dimStyle.Render("  No projects found."))
		b.WriteString("\n")
	} else {
		for i, entry := range m.projectBrowserDirs {
			cursor := "  "
			name := valueStyle.Render(entry.Name)
			if i == m.projectBrowserCursor {
				cursor = successStyle.Render("▸ ")
				name = successStyle.Render(entry.Name)
			}
			b.WriteString(fmt.Sprintf("  %s📁 %s\n", cursor, name))
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓: navigate  •  Enter: open  •  Space: select  •  Esc: back"))
	return b.String()
}

func (m Model) viewBrowser() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("  Select Source Directory"))
	b.WriteString("\n\n")

	if len(m.volumes) > 0 {
		b.WriteString(dimStyle.Render("  Volumes:"))
		b.WriteString("\n")
		for _, v := range m.volumes {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    💾 %s", v)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(labelStyle.Render("  📂 " + m.browserPath))
	b.WriteString("\n\n")

	maxVisible := 15
	if m.height > 0 {
		maxVisible = m.height - 14
		if maxVisible < 5 {
			maxVisible = 5
		}
	}

	start := 0
	if m.browserCursor >= maxVisible {
		start = m.browserCursor - maxVisible + 1
	}

	end := start + maxVisible
	if end > len(m.browserDirs) {
		end = len(m.browserDirs)
	}

	for i := start; i < end; i++ {
		entry := m.browserDirs[i]
		cursor := "  "
		name := valueStyle.Render(entry.Name)

		if i == m.browserCursor {
			cursor = successStyle.Render("▸ ")
			name = successStyle.Render(entry.Name)
		}

		b.WriteString(fmt.Sprintf("  %s📁 %s\n", cursor, name))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑/↓: navigate  •  →/Enter: open  •  ←: back  •  Space: select  •  Esc: back"))

	return b.String()
}

func (m Model) viewConfig() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("  Transfer Summary"))
	b.WriteString("\n\n")

	destDisplay := m.effectiveDest()

	b.WriteString(boxStyle.Render(
		labelStyle.Render("Source:") + valueStyle.Render(m.cfg.Source) + "\n" +
			labelStyle.Render("Host:") + valueStyle.Render(m.cfg.Host) + "\n" +
			labelStyle.Render("User:") + valueStyle.Render(m.cfg.User) + "\n" +
			labelStyle.Render("Key:") + valueStyle.Render(m.cfg.KeyPath) + "\n" +
			labelStyle.Render("Project:") + valueStyle.Render(m.cfg.ProjectName) + "\n" +
			labelStyle.Render("Session:") + valueStyle.Render(m.cfg.SessionName) + "\n" +
			labelStyle.Render("Dest:") + valueStyle.Render(destDisplay) + "\n" +
			labelStyle.Render("Template:") + valueStyle.Render(m.cfg.Template) + "\n" +
			labelStyle.Render("Workers:") + valueStyle.Render(fmt.Sprintf("%d", m.cfg.Concurrency)),
	))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Press Enter to start transfer...  •  q to quit"))
	return b.String()
}

func (m Model) viewTransfer() string {
	var b strings.Builder

	if m.currentFile != "" {
		b.WriteString(fmt.Sprintf("  %s Transferring: %s\n", m.spinner.View(), m.currentFile))
	}

	pct := float64(0)
	if m.totalBytes > 0 {
		pct = float64(m.bytesSent) / float64(m.totalBytes)
	}
	barWidth := 40
	filled := int(pct * float64(barWidth))
	empty := barWidth - filled

	bar := progressFullStyle.Render(strings.Repeat("█", filled)) +
		progressEmptyStyle.Render(strings.Repeat("░", empty))

	b.WriteString(fmt.Sprintf("  [%s] %.1f%%\n", bar, pct*100))
	b.WriteString(fmt.Sprintf("  %s / %s — %d/%d files\n",
		transfer.FormatBytes(m.bytesSent),
		transfer.FormatBytes(m.totalBytes),
		m.filesDone, m.fileCount))

	return b.String()
}

func (m Model) viewDone() string {
	var b strings.Builder
	b.WriteString(successStyle.Render("  ✓ Transfer complete!"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %d files, %s transferred\n",
		m.filesDone, transfer.FormatBytes(m.totalBytes)))

	if m.postOutput != "" {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("  Remote:"))
		b.WriteString(dimStyle.Render(m.postOutput))
		b.WriteString("\n")
	}
	return b.String()
}

// ========== ACTIONS ==========

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateConfig:
		m.state = StateConnecting
		return m, tea.Batch(m.spinner.Tick, m.connectAndPrepare())
	case StateDone:
		return m, tea.Quit
	}
	return m, nil
}

// connectSSH establishes SSH connection only
func (m Model) connectSSH() tea.Cmd {
	return func() tea.Msg {
		client, err := ssh.Connect(m.cfg.Host, m.cfg.User, m.cfg.KeyPath)
		if err != nil {
			return sshConnectedMsg{err: err}
		}
		return sshConnectedMsg{sshClient: client}
	}
}

// connectAndPrepare sets up SSH + SFTP + transfer for the final step
func (m Model) connectAndPrepare() tea.Cmd {
	return func() tea.Msg {
		if err := m.cfg.Validate(); err != nil {
			return connectResultMsg{err: err}
		}

		// Reuse existing SSH if available, otherwise connect
		sshClient := m.sshClient
		var err error
		if sshClient == nil {
			sshClient, err = ssh.Connect(m.cfg.Host, m.cfg.User, m.cfg.KeyPath)
			if err != nil {
				return connectResultMsg{err: err}
			}
		}

		dest := m.effectiveDest()

		t, err := transfer.New(sshClient.Conn(), m.cfg.Source, dest, m.cfg.Concurrency)
		if err != nil {
			return connectResultMsg{err: err}
		}
		if err := t.Walk(); err != nil {
			return connectResultMsg{err: err}
		}
		return connectResultMsg{transferer: t, sshClient: sshClient}
	}
}

func (m Model) startTransfer() tea.Cmd {
	return func() tea.Msg {
		// Apply template if specified
		if m.cfg.Template != "" && m.cfg.ProjectName != "" {
			tmpl, err := template.Get(m.cfg.Template)
			if err != nil {
				return transferDoneMsg{err: err}
			}
			sftpClient, err := sftp.NewClient(m.sshClient.Conn())
			if err != nil {
				return transferDoneMsg{err: err}
			}
			if err := tmpl.Apply(sftpClient, m.cfg.DestPath, m.cfg.ProjectName, m.cfg.SessionName); err != nil {
				sftpClient.Close()
				return transferDoneMsg{err: err}
			}
			sftpClient.Close()
		}

		// Write session state: in_progress
		m.writeSessionState("in_progress", 0, 0)

		// Start transfer in background
		go func() {
			m.transferer.Run()
		}()

		return transferStartMsg{}
	}
}

func (m Model) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-m.transferer.Progress()
		if !ok {
			return transferDoneMsg{}
		}
		return transferProgressMsg{msg: msg}
	}
}

// listRemoteProjects lists subdirectories of DestPath on remote via SFTP
func (m Model) listRemoteProjects() tea.Cmd {
	return func() tea.Msg {
		sftpClient, err := sftp.NewClient(m.sshClient.Conn())
		if err != nil {
			return remoteProjectsMsg{err: err}
		}
		defer sftpClient.Close()

		entries, err := sftpClient.ReadDir(m.cfg.DestPath)
		if err != nil {
			return remoteProjectsMsg{err: fmt.Errorf("list %s: %w", m.cfg.DestPath, err)}
		}

		var dirs []dirEntry
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, dirEntry{
					Name: e.Name(),
					Path: filepath.Join(m.cfg.DestPath, e.Name()),
				})
			}
		}
		return remoteProjectsMsg{dirs: dirs}
	}
}

// loadProjectSessions reads the session JSON for a given project
func (m Model) loadProjectSessions(projectName string) tea.Cmd {
	return func() tea.Msg {
		sftpClient, err := sftp.NewClient(m.sshClient.Conn())
		if err != nil {
			return sessionStateMsg{err: err}
		}
		defer sftpClient.Close()

		remotePath := session.RemotePath(m.cfg.DestPath, projectName)
		state, err := session.Load(sftpClient, remotePath)
		if err != nil {
			return sessionStateMsg{err: err}
		}
		return sessionStateMsg{state: state}
	}
}

// ========== HELPERS ==========

func (m Model) effectiveDest() string {
	if m.cfg.ProjectName != "" && m.cfg.SessionName != "" {
		return filepath.Join(m.cfg.DestPath, m.cfg.ProjectName,
			"FILM-DATAS", m.cfg.SessionName)
	}
	return m.cfg.DestPath
}

func (m *Model) initSourceBrowser() {
	m.volumes = detectVolumes()
	startDir := "/Volumes"
	if _, err := os.Stat(startDir); err != nil {
		startDir = "/"
	}
	m.browserPath = startDir
	m.browserDirs = listDirEntries(startDir)
	m.browserCursor = 0
}

func (m *Model) writeSessionState(status string, files int, bytes int64) {
	if m.sshClient == nil || m.cfg.ProjectName == "" {
		return
	}
	sftpClient, err := sftp.NewClient(m.sshClient.Conn())
	if err != nil {
		return
	}
	defer sftpClient.Close()

	remotePath := session.RemotePath(m.cfg.DestPath, m.cfg.ProjectName)
	state, _ := session.Load(sftpClient, remotePath)
	if state.Project == "" {
		state = session.NewProjectState(m.cfg.ProjectName)
	}

	state.AddSession(session.SessionEntry{
		Name:         m.cfg.SessionName,
		Date:         time.Now().Format("2006-01-02"),
		Status:       status,
		Files:        files,
		Bytes:        bytes,
		HashVerified: status == "complete",
	})

	session.Save(sftpClient, remotePath, state)
}

func (m *Model) saveSessionState() {
	m.writeSessionState("complete", m.filesDone, m.totalBytes)
}

func (m *Model) cleanup() {
	if m.transferer != nil {
		m.transferer.Close()
	}
	if m.sftpClient != nil {
		m.sftpClient.Close()
	}
	if m.sshClient != nil {
		m.sshClient.Close()
	}
}
