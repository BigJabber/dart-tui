// Package player holds the player entity and the display-colour palette.
package player

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MaxNameLength keeps names short enough to fit the in-game table columns.
const MaxNameLength = 24

// Player is a person who throws darts. Players are never hard-deleted; taking a
// player out of circulation archives them so their historic visits, and every
// statistic derived from those visits, remain intact.
type Player struct {
	ID         int64
	Name       string
	Colour     string // "#RRGGBB"
	CreatedAt  time.Time
	ArchivedAt *time.Time
}

// Archived reports whether the player has been retired from active play.
func (p Player) Archived() bool { return p.ArchivedAt != nil }

// Palette is the set of display colours assigned to players.
//
// The colours are chosen to stay legible on both the default dark and light
// Windows Terminal profiles: nothing near-black, nothing near-white, and no two
// entries so close that they read as the same colour at a glance. Colour is
// always paired with a text or symbol cue elsewhere in the UI so that the
// application remains usable without it.
var Palette = []string{
	"#FF5F87", // rose
	"#5FD7FF", // sky
	"#87D75F", // green
	"#FFAF5F", // amber
	"#AF87FF", // violet
	"#5FD7AF", // teal
	"#FF87D7", // pink
	"#D7D75F", // citron
	"#5F87FF", // indigo
	"#FF875F", // coral
	"#87FFD7", // mint
	"#D78700", // bronze
	"#00D7AF", // jade
	"#D75FD7", // magenta
	"#87AFFF", // periwinkle
	"#FFD75F", // gold
}

// DefaultColour is used when a stored colour is missing or unreadable, so a
// corrupt row can never leave a player invisible.
const DefaultColour = "#C0C0C0"

var colourPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var (
	// ErrEmptyName is returned when a player name is blank.
	ErrEmptyName = errors.New("player name cannot be empty")
	// ErrNameTooLong is returned when a player name exceeds MaxNameLength.
	ErrNameTooLong = fmt.Errorf("player name cannot be longer than %d characters", MaxNameLength)
	// ErrBadColour is returned when a colour is not a #RRGGBB string.
	ErrBadColour = errors.New("colour must be a hex value such as #FF5F87")
)

// NormaliseName trims surrounding whitespace and collapses internal runs of
// whitespace, so " Bob   Smith " and "Bob Smith" are stored identically.
func NormaliseName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidateName checks a player name and returns the normalised form.
func ValidateName(name string) (string, error) {
	n := NormaliseName(name)
	if n == "" {
		return "", ErrEmptyName
	}
	if len([]rune(n)) > MaxNameLength {
		return "", ErrNameTooLong
	}
	return n, nil
}

// ValidateColour checks a colour and returns it upper-cased for consistency.
func ValidateColour(colour string) (string, error) {
	c := strings.TrimSpace(colour)
	if !colourPattern.MatchString(c) {
		return "", ErrBadColour
	}
	return strings.ToUpper(c), nil
}

// PickColour chooses a display colour for a new player.
//
// It prefers a palette entry nobody is using yet. Once every colour is in use it
// falls back to the least-used colour, breaking ties by palette order so the
// choice stays deterministic and testable.
func PickColour(used []string) string {
	counts := make(map[string]int, len(Palette))
	for _, u := range used {
		counts[strings.ToUpper(strings.TrimSpace(u))]++
	}
	best := Palette[0]
	bestCount := -1
	for _, c := range Palette {
		n := counts[strings.ToUpper(c)]
		if n == 0 {
			return c
		}
		if bestCount == -1 || n < bestCount {
			best, bestCount = c, n
		}
	}
	return best
}

// SafeColour returns a usable colour for rendering, falling back to
// DefaultColour rather than letting a bad stored value break the display.
func SafeColour(colour string) string {
	if c, err := ValidateColour(colour); err == nil {
		return c
	}
	return DefaultColour
}
