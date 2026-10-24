package screens

import (
	"strings"

	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
)

// The X01 and Around the World screens share the throwing sequence and the
// automatic player swap that follows it, so the keys and the wording live here
// once rather than drifting apart in each screen.

// moveInOrder moves a selected player one place earlier (delta < 0) or later
// (delta > 0) in the throwing order. It reports whether anything moved; a
// player who is not selected, or is already at that end, stays where they are.
func moveInOrder(order []int64, id int64, delta int) bool {
	for i, sel := range order {
		if sel != id {
			continue
		}
		j := i + delta
		if j < 0 || j >= len(order) {
			return false
		}
		order[i], order[j] = order[j], order[i]
		return true
	}
	return false
}

// sequenceLine spells out the throwing order, for the setup screens.
func sequenceLine(order []int64, byID map[int64]player.Player) string {
	names := make([]string, 0, len(order))
	for _, id := range order {
		if p, ok := byID[id]; ok {
			names = append(names, styles.PlayerName(p.Colour, true).Render(p.Name))
		}
	}
	return styles.Faint.Render("Throwing order  ") + strings.Join(names, styles.Faint.Render(" → "))
}

// autoSwapLine describes the auto-swap setting and how to change it.
func autoSwapLine(on bool) string {
	if on {
		return styles.Faint.Render("Auto-swap ") + styles.SuccessText.Render("ON") +
			styles.Faint.Render("  — the next player in order is selected after every visit (s to turn off)")
	}
	return styles.Faint.Render("Auto-swap ") + styles.Bold.Render("OFF") +
		styles.Faint.Render(" — choose who threw after every visit (s to turn on)")
}

// autoSwapStatus is the footer message after the setting is changed mid-game.
func autoSwapStatus(on bool) string {
	if on {
		return "Auto-swap on — the next player in order is selected after every visit."
	}
	return "Auto-swap off — choose who threw after every visit."
}

// autoSwapHint is the footer's description of the auto-swap key, which names
// the current setting so it can be read without trying it.
func autoSwapHint(on bool) string {
	if on {
		return "auto-swap: on"
	}
	return "auto-swap: off"
}
