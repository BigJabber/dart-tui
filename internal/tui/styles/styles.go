// Package styles holds every Lip Gloss style the interface uses.
//
// Two rules run through it:
//
//   - Colours are adaptive, so the application is legible on both the default
//     dark Windows Terminal profile and a light one.
//   - Colour never carries meaning on its own. The current thrower has a "▶"
//     marker as well as a highlight, a bust says BUST, a finished player says
//     OUT. Anyone using a monochrome terminal, or who cannot distinguish the
//     palette, still gets the full picture.
package styles

import "github.com/charmbracelet/lipgloss"

// The colour scheme. Light values are chosen dark enough to read on white,
// dark values bright enough to read on black.
var (
	Accent    = lipgloss.AdaptiveColor{Light: "#005FAF", Dark: "#5FD7FF"}
	Muted     = lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#9E9E9E"}
	Subtle    = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#767676"}
	Danger    = lipgloss.AdaptiveColor{Light: "#AF0000", Dark: "#FF5F5F"}
	Success   = lipgloss.AdaptiveColor{Light: "#006B3C", Dark: "#5FD75F"}
	Warning   = lipgloss.AdaptiveColor{Light: "#875F00", Dark: "#FFD75F"}
	Border    = lipgloss.AdaptiveColor{Light: "#B2B2B2", Dark: "#4E4E4E"}
	Highlight = lipgloss.AdaptiveColor{Light: "#D7E8FF", Dark: "#1C3A4A"}
)

// Text styles.
var (
	Title = lipgloss.NewStyle().Bold(true).Foreground(Accent)

	Subtitle = lipgloss.NewStyle().Foreground(Muted)

	Faint = lipgloss.NewStyle().Foreground(Subtle)

	Bold = lipgloss.NewStyle().Bold(true)

	ErrorText = lipgloss.NewStyle().Bold(true).Foreground(Danger)

	SuccessText = lipgloss.NewStyle().Foreground(Success)

	WarningText = lipgloss.NewStyle().Foreground(Warning)

	// Header is the application banner at the top of every screen.
	Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(Accent).
		PaddingLeft(1)

	// Footer holds the contextual key hints.
	Footer = lipgloss.NewStyle().
		Foreground(Subtle).
		PaddingLeft(1)

	// Key is a keyboard shortcut in the footer.
	Key = lipgloss.NewStyle().Bold(true).Foreground(Muted)
)

// Layout styles.
var (
	// Panel frames a block of content.
	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Border).
		Padding(0, 1)

	// FocusPanel frames the block the operator is working in.
	FocusPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(0, 1)

	// Dialog frames a confirmation prompt.
	Dialog = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(Warning).
		Padding(1, 2)

	// SelectedRow marks the row under the cursor. It is always paired with a
	// leading marker so the selection is not signalled by colour alone.
	SelectedRow = lipgloss.NewStyle().
			Background(Highlight).
			Bold(true)

	// TableHeader styles a column heading row.
	TableHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(Muted).
			Underline(true)
)

// PlayerName renders a name in that player's colour.
//
// Archived players are dimmed and marked in the calling screen; the colour
// alone is never the signal.
func PlayerName(colour string, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(colour))
	if bold {
		s = s.Bold(true)
	}
	return s
}

// Swatch renders a solid block of a colour, for the colour picker.
func Swatch(colour string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colour))
}

// Marker returns the leading indicator for a list row: a caret when selected,
// otherwise blank space of the same width so rows do not shift.
func Marker(selected bool) string {
	if selected {
		return "▶ "
	}
	return "  "
}

// Checkbox renders a multi-select state as text, not just colour.
func Checkbox(checked bool) string {
	if checked {
		return "[x]"
	}
	return "[ ]"
}
