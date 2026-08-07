package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pourmoi/dart-tui/internal/domain/player"
	"github.com/pourmoi/dart-tui/internal/tui/components"
	"github.com/pourmoi/dart-tui/internal/tui/styles"
)

// playerListMode is what the roster screen is currently doing.
type playerListMode int

const (
	playerListBrowse playerListMode = iota
	playerListAdding
)

// PlayerList is the roster: add a player, open a profile, or show the archive.
type PlayerList struct {
	ctx    Context
	cursor components.Cursor

	players         []player.Player
	includeArchived bool
	loadErr         error

	mode     playerListMode
	input    textinput.Model
	entryErr string
}

// NewPlayerList builds the roster screen.
func NewPlayerList(ctx Context) *PlayerList {
	s := &PlayerList{ctx: ctx, input: newNameInput("")}
	s.reload()
	return s
}

func newNameInput(initial string) textinput.Model {
	in := textinput.New()
	in.Placeholder = "player name"
	in.CharLimit = player.MaxNameLength
	in.Width = 30
	in.Prompt = "› "
	in.SetValue(initial)
	in.CursorEnd()
	// A static cursor rather than a blinking one. Blinking needs a repeating
	// timer redrawing the screen twice a second for as long as the field is
	// open, which is wasted work on a scoreboard that may sit on a wall-mounted
	// monitor all afternoon. A solid block marks the position just as well.
	in.Cursor.SetMode(cursor.CursorStatic)
	return in
}

// Init focuses nothing and simply refreshes the list.
func (s *PlayerList) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *PlayerList) reload() {
	players, err := s.ctx.Services.Players.List(s.ctx.Ctx, s.includeArchived)
	s.loadErr = err
	s.players = players
	s.cursor.SetCount(len(players))
}

// Title names the screen.
func (s *PlayerList) Title() string {
	if s.includeArchived {
		return "Players (including archived)"
	}
	return "Players"
}

// Hints lists the available keys.
func (s *PlayerList) Hints() []components.KeyHint {
	if s.mode == playerListAdding {
		return []components.KeyHint{
			{Keys: "enter", Desc: "add player"},
			{Keys: "esc", Desc: "cancel"},
		}
	}
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "enter", Desc: "open profile"},
		{Keys: "n", Desc: "new player"},
		{Keys: "a", Desc: "show/hide archived"},
		{Keys: "esc", Desc: "back"},
	}
}

// Update handles roster keys and the inline new-player field.
func (s *PlayerList) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	if s.mode == playerListAdding {
		return s.updateAdding(key)
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
	case "n":
		s.mode = playerListAdding
		s.entryErr = ""
		s.input = newNameInput("")
		return s, s.input.Focus()
	case "a":
		s.includeArchived = !s.includeArchived
		s.reload()
	case "enter":
		if p, ok := s.selected(); ok {
			return s, Push(NewPlayerProfile(s.ctx, p.ID))
		}
	case "esc":
		return s, Pop()
	}
	return s, nil
}

func (s *PlayerList) updateAdding(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.mode = playerListBrowse
		s.entryErr = ""
		s.input.Blur()
		return s, nil
	case "enter":
		name := strings.TrimSpace(s.input.Value())
		created, err := s.ctx.Services.Players.Create(s.ctx.Ctx, name)
		if err != nil {
			// Stay in the field so the operator can correct it immediately.
			s.entryErr = err.Error()
			return s, nil
		}
		s.mode = playerListBrowse
		s.entryErr = ""
		s.input.Blur()
		s.reload()
		s.selectPlayer(created.ID)
		return s, Success(fmt.Sprintf("Added %s", created.Name))
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(key)
	s.entryErr = ""
	return s, cmd
}

func (s *PlayerList) selected() (player.Player, bool) {
	i := s.cursor.Index()
	if i < 0 || i >= len(s.players) {
		return player.Player{}, false
	}
	return s.players[i], true
}

func (s *PlayerList) selectPlayer(id int64) {
	for i, p := range s.players {
		if p.ID == id {
			s.cursor.Set(i)
			return
		}
	}
}

// View renders the roster.
func (s *PlayerList) View(width, height int) string {
	var b strings.Builder

	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load players: " + s.loadErr.Error())
	}

	if s.mode == playerListAdding {
		b.WriteString(styles.Bold.Render("New player"))
		b.WriteString("\n")
		b.WriteString(s.input.View())
		b.WriteString("\n")
		if s.entryErr != "" {
			b.WriteString(styles.ErrorText.Render(s.entryErr))
		} else {
			b.WriteString(styles.Faint.Render("A colour is assigned automatically."))
		}
		b.WriteString("\n\n")
	}

	if len(s.players) == 0 {
		b.WriteString(styles.Faint.Render("No players yet. Press n to add the first one."))
		return b.String()
	}

	listHeight := height - strings.Count(b.String(), "\n") - 2
	start, end := s.cursor.Window(maxInt(listHeight, 3))

	nameWidth := minInt(maxInt(width-30, 12), player.MaxNameLength+2)
	for i := start; i < end && i < len(s.players); i++ {
		p := s.players[i]
		selected := i == s.cursor.Index() && s.mode == playerListBrowse

		name := styles.PlayerName(p.Colour, selected).Render(components.Truncate(p.Name, nameWidth))
		line := styles.Marker(selected) + name

		// Archived state is spelled out, never signalled by dimming alone.
		if p.Archived() {
			line += "  " + styles.WarningText.Render("[archived]")
		}
		if selected {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if end < len(s.players) || start > 0 {
		b.WriteString(styles.Faint.Render(
			fmt.Sprintf("\nshowing %d-%d of %d", start+1, end, len(s.players))))
	}
	return b.String()
}
