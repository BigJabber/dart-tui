package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pourmoi/dart-tui/internal/domain/x01"
	"github.com/pourmoi/dart-tui/internal/tui/components"
	"github.com/pourmoi/dart-tui/internal/tui/styles"
)

// GameModeSelect asks which game is being played.
//
// Adding a mode means adding an entry here and a setup screen. The menu makes
// no assumption about how many modes exist or what they need to be configured.
type GameModeSelect struct {
	ctx    Context
	cursor components.Cursor
	modes  []gameMode
}

type gameMode struct {
	key   string
	name  string
	desc  string
	setup func(Context) Screen
}

// NewGameModeSelect builds the game-type chooser.
func NewGameModeSelect(ctx Context) *GameModeSelect {
	s := &GameModeSelect{ctx: ctx}
	s.modes = []gameMode{
		{
			key:  "5",
			name: "501",
			desc: "double out · throw in any order",
			setup: func(c Context) Screen {
				return NewX01Setup(c, x01.DefaultConfig(x01.Start501))
			},
		},
		{
			key:  "3",
			name: "301",
			desc: "double out · throw in any order",
			setup: func(c Context) Screen {
				return NewX01Setup(c, x01.DefaultConfig(x01.Start301))
			},
		},
		{
			key:   "c",
			name:  "Cricket",
			desc:  "office rules · two teams · batting and bowling",
			setup: func(c Context) Screen { return NewCricketSetup(c) },
		},
	}
	s.cursor.SetCount(len(s.modes))
	return s
}

// Init does nothing; the chooser holds no loaded state.
func (s *GameModeSelect) Init() tea.Cmd { return nil }

// Title names the screen.
func (s *GameModeSelect) Title() string { return "Start game" }

// Hints lists the available keys.
func (s *GameModeSelect) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "enter", Desc: "choose"},
		{Keys: "5/3/c", Desc: "jump to a game"},
		{Keys: "esc", Desc: "back"},
	}
}

// Update handles the mode choice.
func (s *GameModeSelect) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "up", "k":
		s.cursor.Prev()
		return s, nil
	case "down", "j":
		s.cursor.Next()
		return s, nil
	case "esc":
		return s, Pop()
	case "enter", " ":
		return s, s.choose(s.cursor.Index())
	}
	for i, m := range s.modes {
		if key.String() == m.key {
			s.cursor.Set(i)
			return s, s.choose(i)
		}
	}
	return s, nil
}

func (s *GameModeSelect) choose(index int) tea.Cmd {
	if index < 0 || index >= len(s.modes) {
		return nil
	}
	return Replace(s.modes[index].setup(s.ctx))
}

// View renders the chooser.
func (s *GameModeSelect) View(width, height int) string {
	var b strings.Builder
	b.WriteString(styles.Subtitle.Render("Which game?"))
	b.WriteString("\n\n")

	for i, m := range s.modes {
		selected := i == s.cursor.Index()
		line := fmt.Sprintf("%s%s  %s",
			styles.Marker(selected), styles.Key.Render("["+m.key+"]"), styles.Bold.Render(m.name))
		if selected {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		b.WriteString(styles.Faint.Render("      " + m.desc))
		b.WriteString("\n\n")
	}
	return b.String()
}
