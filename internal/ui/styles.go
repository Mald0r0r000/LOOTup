package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	colorPrimary   = lipgloss.Color("#FF6F00") // Orange — matches LOOT's energy
	colorSecondary = lipgloss.Color("#4FC3F7")
	colorSuccess   = lipgloss.Color("#66BB6A")
	colorError     = lipgloss.Color("#EF5350")
	colorDim       = lipgloss.Color("#757575")
	colorWhite     = lipgloss.Color("#FAFAFA")

	// Title
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			PaddingLeft(1)

	// Header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite).
			Background(colorPrimary).
			Padding(0, 2)

	// Info label
	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary).
			Width(14)

	// Value
	valueStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	// Success
	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSuccess)

	// Error
	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorError)

	// Dim text
	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Progress bar
	progressFullStyle = lipgloss.NewStyle().
				Foreground(colorPrimary)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// Box
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			MarginBottom(1)

	// Status line
	statusStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			PaddingLeft(1)
)

// renderLogo returns the ASCII art banner for LOOTup
func renderLogo() string {
	lootAscii := `██╗      ██████╗  ██████╗ ████████╗
██║     ██╔═══██╗██╔═══██╗╚══██╔══╝
██║     ██║   ██║██║   ██║   ██║   
██║     ██║   ██║██║   ██║   ██║   
███████╗╚██████╔╝╚██████╔╝   ██║   
╚══════╝ ╚═════╝  ╚═════╝    ╚═╝`

	upAscii := `            
            
██╗ ██╗████╗
██║ ██║██╔═╝
╚████╔╝██║  
 ╚═══╝ ╚═╝  `

	orange := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6F00"))
	neon := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF2D9B")).Bold(true)

	lootRender := orange.Render(lootAscii)
	upRender := neon.Copy().MarginLeft(1).Render(upAscii)

	return lipgloss.JoinHorizontal(lipgloss.Bottom, lootRender, upRender)
}
