package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/sftp"

	"github.com/Mald0r0r000/LOOTup/internal/config"
	"github.com/Mald0r0r000/LOOTup/internal/ssh"
	"github.com/Mald0r0r000/LOOTup/internal/template"
	"github.com/Mald0r0r000/LOOTup/internal/transfer"
)

// State represents the current TUI state
type State int

const (
	StateConfig State = iota
	StateTemplate
	StateConnecting
	StateTransfer
	StatePostTransfer
	StateDone
	StateError
)

// --- Tea Messages ---

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

// --- Model ---

// Model is the root Bubble Tea model for LOOTup
type Model struct {
	cfg     *config.Config
	state   State
	spinner spinner.Model
	err     error

	// Connections
	sshClient *ssh.Client

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

	return Model{
		cfg:     cfg,
		state:   StateConfig,
		spinner: s,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cleanup()
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case connectResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
			return m, nil
		}
		m.transferer = msg.transferer
		m.sshClient = msg.sshClient
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
		m.state = StateDone
		m.cleanup()
		return m, nil

	case postTransferMsg:
		m.postOutput = msg.output
		if msg.err != nil {
			m.postOutput = fmt.Sprintf("⚠ %v", msg.err)
		}
		m.state = StateDone
		m.cleanup()
		return m, nil
	}

	return m, nil
}

// View implements tea.Model
func (m Model) View() string {
	var b strings.Builder

	// Banner
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(" 🚀 LOOTup "))
	b.WriteString(dimStyle.Render(fmt.Sprintf(" v%s", m.cfg.Version)))
	b.WriteString("\n\n")

	switch m.state {

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
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(statusStyle.Render("Press q to quit"))
	b.WriteString("\n")

	return b.String()
}

// --- View helpers ---

func (m Model) viewConfig() string {
	var b strings.Builder
	b.WriteString(boxStyle.Render(
		labelStyle.Render("Source:") + valueStyle.Render(m.cfg.Source) + "\n" +
			labelStyle.Render("Host:") + valueStyle.Render(m.cfg.Host) + "\n" +
			labelStyle.Render("User:") + valueStyle.Render(m.cfg.User) + "\n" +
			labelStyle.Render("Dest Path:") + valueStyle.Render(m.cfg.DestPath) + "\n" +
			labelStyle.Render("Template:") + valueStyle.Render(m.cfg.Template) + "\n" +
			labelStyle.Render("Workers:") + valueStyle.Render(fmt.Sprintf("%d", m.cfg.Concurrency)),
	))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Press Enter to start transfer..."))
	return b.String()
}

func (m Model) viewTransfer() string {
	var b strings.Builder

	// File progress
	if m.currentFile != "" {
		b.WriteString(fmt.Sprintf("  %s Transferring: %s\n", m.spinner.View(), m.currentFile))
	}

	// Overall progress
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

// --- Actions ---

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateConfig:
		m.state = StateConnecting
		return m, tea.Batch(m.spinner.Tick, m.connect())
	case StateDone:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) connect() tea.Cmd {
	return func() tea.Msg {
		if err := m.cfg.Validate(); err != nil {
			return connectResultMsg{err: err}
		}
		sshClient, err := ssh.Connect(m.cfg.Host, m.cfg.User, m.cfg.KeyPath)
		if err != nil {
			return connectResultMsg{err: err}
		}
		t, err := transfer.New(sshClient.Conn(), m.cfg.Source,
			m.cfg.DestPath, m.cfg.Concurrency)
		if err != nil {
			sshClient.Close()
			return connectResultMsg{err: err}
		}
		if err := t.Walk(); err != nil {
			sshClient.Close()
			return connectResultMsg{err: err}
		}
		return connectResultMsg{transferer: t, sshClient: sshClient}
	}
}

func (m Model) startTransfer() tea.Cmd {
	return func() tea.Msg {
		// Apply template if specified
		if m.cfg.Template != "" {
			tmpl, err := template.Get(m.cfg.Template)
			if err != nil {
				return transferDoneMsg{err: err}
			}
			sftpClient, err := sftp.NewClient(m.sshClient.Conn())
			if err != nil {
				return transferDoneMsg{err: err}
			}
			if err := tmpl.Apply(sftpClient, m.cfg.DestPath); err != nil {
				sftpClient.Close()
				return transferDoneMsg{err: err}
			}
			sftpClient.Close()
		}

		// Start transfer in background goroutine
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
			// Channel closed — transfer is done
			return transferDoneMsg{}
		}
		return transferProgressMsg{msg: msg}
	}
}

// cleanup closes SSH and SFTP connections
func (m *Model) cleanup() {
	if m.transferer != nil {
		m.transferer.Close()
	}
	if m.sshClient != nil {
		m.sshClient.Close()
	}
}
