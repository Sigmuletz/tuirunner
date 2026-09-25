package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorViolet = lipgloss.Color("99")  // border
	colorGray   = lipgloss.Color("245") // default text
	colorYellow = lipgloss.Color("220") // selected/focused highlight, favorite star
)

// No Background() is set anywhere here — the box is transparent and shows
// the terminal's own background color, not a forced one.
var (
	outerBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorViolet).
				Foreground(colorGray)

	lineStyle = lipgloss.NewStyle().
			Foreground(colorGray)

	dividerStyle = lipgloss.NewStyle().
			Foreground(colorViolet)

	highlightStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	// relevantStyle marks a field whose name is referenced by the
	// currently selected script's placeholders — recomputed on every
	// render, so it tracks selection live rather than being persisted.
	relevantStyle = lipgloss.NewStyle().
			Foreground(colorViolet).
			Bold(true)

	starStyle = lipgloss.NewStyle().
			Foreground(colorYellow)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			Faint(true)

	headingStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			Underline(true)
)

// padLine pads/truncates s (which may already contain nested ANSI style
// sequences) to exactly width printable cells, so every row of the layout
// consistently fills the box.
func padLine(s string, width int) string {
	if width < 0 {
		width = 0
	}
	return lineStyle.Width(width).MaxWidth(width).Render(s)
}
