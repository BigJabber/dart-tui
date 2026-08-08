package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout decides how a game screen arranges its three parts: the scoreboard,
// the large panel and the prompt.
//
// A wide terminal has far more width going spare than height. Fullscreen on a
// 1920×1080 display is something like 240 columns by 60 rows, and the prompt
// panels are written for 80 columns — so well over half the width is empty on
// every row while the panel and the prompt are still competing for the same
// rows. Given the width for it, the panel therefore moves into a column of its
// own on the right, where it can be about twice the size and where growing it
// costs the prompt nothing.
//
// The split also follows who is looking: the scoreboard and the prompt are for
// whoever is holding the keyboard, and the panel is for everyone else in the
// room. Below the threshold the three stack as before.
type Layout struct {
	// Body is the width for the scoreboard and the prompt.
	Body int
	// Panel is the width available to the large panel.
	Panel int
	// PanelRows is how many rows the panel may occupy. It is only meaningful
	// side by side, where the panel has a whole column; stacked, the caller
	// works the number out from what it has already rendered.
	PanelRows int
	// SideBySide reports which arrangement was chosen.
	SideBySide bool
}

// Column geometry.
const (
	// bodyColumnWidth is what the scoreboard and the prompt get side by side.
	// The prompt panels cap themselves at 78 and the scoreboard's flexible name
	// column stops widening at about the same point, so anything more would be
	// wasted on them and taken from the panel.
	bodyColumnWidth = 82

	// columnGutter separates the two columns.
	columnGutter = 2

	// stackedPanelWidth caps the panel when everything is stacked, so it does
	// not tower over the prompt below it.
	stackedPanelWidth = 98

	// minPanelColumn is the narrowest panel column worth moving to, and it is
	// the stacked width deliberately.
	//
	// A column buys height but costs width, and the number is usually limited
	// by width first — its caption sits beside it. At 160 columns a panel
	// column would be 76 wide, which is one short of the 77 the second-largest
	// size needs, so moving there would trade a fourteen-row number for a
	// seven-row one. Never move unless the column is at least as wide as
	// staying put would have been.
	minPanelColumn = stackedPanelWidth
)

// NewLayout chooses the arrangement for a body of the given size.
func NewLayout(width, height int) Layout {
	if panel := width - bodyColumnWidth - columnGutter; panel >= minPanelColumn {
		return Layout{
			Body:       bodyColumnWidth,
			Panel:      panel,
			PanelRows:  height,
			SideBySide: true,
		}
	}
	return Layout{Body: width, Panel: minInt(width, stackedPanelWidth)}
}

// Join puts the three rendered parts together. An empty panel is left out
// entirely rather than leaving a gap where it would have been.
func (l Layout) Join(board, panel, prompt string) string {
	stacked := board + "\n" + prompt
	if panel == "" {
		return stacked
	}
	if l.SideBySide {
		return lipgloss.JoinHorizontal(
			lipgloss.Top, stacked, strings.Repeat(" ", columnGutter), panel)
	}
	return board + "\n" + panel + "\n" + prompt
}
