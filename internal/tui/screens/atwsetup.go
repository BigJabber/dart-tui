package screens

import (
	"fmt"
	"strings"

	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// ATWSetup selects who is playing Around the World.
//
// It works exactly as the X01 selection screen does, down to the keys, because
// the operator should not have to learn a second way of picking players. The
// order players are chosen in is recorded for display only — during the game
// anybody may throw at any time.
type ATWSetup struct {
	ctx    Context
	cursor components.Cursor

	players []player.Player
	// selected preserves the order players were picked in.
	selected []int64
	loadErr  error
}

// NewATWSetup builds the player-selection screen for an Around the World game.
func NewATWSetup(ctx Context) *ATWSetup {
	s := &ATWSetup{ctx: ctx}
	s.reload()
	return s
}

// Init refreshes the player list.
func (s *ATWSetup) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *ATWSetup) reload() {
	players, err := s.ctx.Services.Players.List(s.ctx.Ctx, false)
	s.loadErr = err
	s.players = players
	s.cursor.SetCount(len(players))
}

// Title names the screen.
func (s *ATWSetup) Title() string { return "Around the World — choose players" }

// Hints lists the available keys.
func (s *ATWSetup) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "space", Desc: "select/deselect"},
		{Keys: "a", Desc: "select all"},
		{Keys: "enter", Desc: "start game"},
		{Keys: "esc", Desc: "back"},
	}
}

// Update handles selection keys.
func (s *ATWSetup) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch key.String() {
	case "up", "k":
		s.cursor.Prev()
	case "down", "j":
		s.cursor.Next()
	case "home", "g":
		s.cursor.First()
	case "end", "G":
		s.cursor.Last()
	case " ", "x":
		s.toggle()
	case "a":
		s.selectAll()
	case "c":
		s.selected = nil
	case "enter":
		return s, s.start()
	case "esc":
		return s, Pop()
	}
	return s, nil
}

func (s *ATWSetup) toggle() {
	i := s.cursor.Index()
	if i < 0 || i >= len(s.players) {
		return
	}
	id := s.players[i].ID
	for j, sel := range s.selected {
		if sel == id {
			s.selected = append(s.selected[:j], s.selected[j+1:]...)
			return
		}
	}
	s.selected = append(s.selected, id)
}

func (s *ATWSetup) selectAll() {
	if len(s.selected) == len(s.players) {
		s.selected = nil
		return
	}
	s.selected = nil
	for _, p := range s.players {
		s.selected = append(s.selected, p.ID)
	}
}

func (s *ATWSetup) isSelected(id int64) (int, bool) {
	for i, sel := range s.selected {
		if sel == id {
			return i, true
		}
	}
	return 0, false
}

func (s *ATWSetup) start() tea.Cmd {
	if len(s.selected) == 0 {
		return Warn("Select at least one player before starting.")
	}
	session, err := s.ctx.Services.Games.StartATW(s.ctx.Ctx, s.selected)
	if err != nil {
		return Fail(err)
	}
	return Replace(NewATWGame(s.ctx, session))
}

// View renders the selection list.
func (s *ATWSetup) View(width, height int) string {
	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load players: " + s.loadErr.Error())
	}
	if len(s.players) == 0 {
		return styles.Faint.Render(
			"No active players. Go back and add some from the Players screen.")
	}

	var b strings.Builder
	b.WriteString(styles.Subtitle.Render(
		"Everyone selected starts on 1 and works round to the bullseye. Fewest darts wins."))
	b.WriteString("\n\n")

	start, end := s.cursor.Window(maxInt(height-6, 3))
	for i := start; i < end && i < len(s.players); i++ {
		p := s.players[i]
		cursorHere := i == s.cursor.Index()
		order, picked := s.isSelected(p.ID)

		// The checkbox and the pick number carry the state; colour is decoration.
		box := styles.Checkbox(picked)
		position := "   "
		if picked {
			position = fmt.Sprintf("%2d.", order+1)
		}
		name := styles.PlayerName(p.Colour, picked).Render(
			components.Truncate(p.Name, maxInt(width-20, 10)))

		line := fmt.Sprintf("%s%s %s %s", styles.Marker(cursorHere), box, position, name)
		if cursorHere {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	switch len(s.selected) {
	case 0:
		b.WriteString(styles.Faint.Render("No players selected yet."))
	case 1:
		b.WriteString(styles.SuccessText.Render("1 player selected — press enter to start."))
		b.WriteString("\n")
		b.WriteString(styles.Faint.Render(
			"On their own this is a training session: statistics are recorded as usual, but no win."))
	default:
		b.WriteString(styles.SuccessText.Render(fmt.Sprintf(
			"%d players selected — press enter to start.", len(s.selected))))
	}
	return b.String()
}
