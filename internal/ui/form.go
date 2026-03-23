package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	fieldHost = iota
	fieldUser
	fieldKeyPath
	fieldDestPath
	fieldTemplate
	fieldWorkers
	fieldCount
)

var fieldLabels = [fieldCount]string{
	"Host",
	"User",
	"Key Path",
	"Dest Path",
	"Template",
	"Workers",
}

var fieldPlaceholders = [fieldCount]string{
	"192.168.1.100",
	"admin",
	"~/.ssh/id_ed25519",
	"/data/projects/MyFilm",
	"film, photo, generic (optional)",
	"4",
}

// hostInputModel holds the form state for host configuration
type hostInputModel struct {
	inputs  [fieldCount]textinput.Model
	focused int
}

// newHostInputModel creates a form with pre-filled defaults
func newHostInputModel(host, user, keyPath, destPath, tmpl string, workers int) hostInputModel {
	var inputs [fieldCount]textinput.Model

	for i := 0; i < fieldCount; i++ {
		t := textinput.New()
		t.Placeholder = fieldPlaceholders[i]
		t.CharLimit = 256
		t.Width = 40
		inputs[i] = t
	}

	// Pre-fill from config defaults
	inputs[fieldHost].SetValue(host)
	inputs[fieldUser].SetValue(user)
	inputs[fieldKeyPath].SetValue(keyPath)
	inputs[fieldDestPath].SetValue(destPath)
	inputs[fieldTemplate].SetValue(tmpl)
	inputs[fieldWorkers].SetValue(fmt.Sprintf("%d", workers))

	// Focus first field
	inputs[fieldHost].Focus()

	return hostInputModel{
		inputs:  inputs,
		focused: 0,
	}
}

// Update handles key events for the form
func (h *hostInputModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			return h.focusNext()
		case "shift+tab", "up":
			return h.focusPrev()
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	h.inputs[h.focused], cmd = h.inputs[h.focused].Update(msg)
	return cmd
}

func (h *hostInputModel) focusNext() tea.Cmd {
	h.inputs[h.focused].Blur()
	h.focused = (h.focused + 1) % fieldCount
	return h.inputs[h.focused].Focus()
}

func (h *hostInputModel) focusPrev() tea.Cmd {
	h.inputs[h.focused].Blur()
	h.focused = (h.focused - 1 + fieldCount) % fieldCount
	return h.inputs[h.focused].Focus()
}

// IsOnLastField returns true if the cursor is on the last field
func (h *hostInputModel) IsOnLastField() bool {
	return h.focused == fieldCount-1
}

// Values returns the current form values
func (h *hostInputModel) Values() (host, user, keyPath, destPath, tmpl, workers string) {
	return h.inputs[fieldHost].Value(),
		h.inputs[fieldUser].Value(),
		h.inputs[fieldKeyPath].Value(),
		h.inputs[fieldDestPath].Value(),
		h.inputs[fieldTemplate].Value(),
		h.inputs[fieldWorkers].Value()
}

// View renders the form
func (h *hostInputModel) View() string {
	var b strings.Builder

	b.WriteString(boxStyle.Render(h.renderFields()))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Tab/Shift-Tab: navigate  •  Enter: confirm  •  Esc: back"))

	return b.String()
}

func (h *hostInputModel) renderFields() string {
	var b strings.Builder

	for i := 0; i < fieldCount; i++ {
		label := labelStyle.Render(fieldLabels[i] + ":")
		cursor := "  "
		if i == h.focused {
			cursor = successStyle.Render("▸ ")
		}
		b.WriteString(cursor + label + h.inputs[i].View())
		if i < fieldCount-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
