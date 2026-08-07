package screens

import (
	"fmt"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/persistence/sqlite"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// ResumeList shows games that were left unfinished.
//
// Games are written to the database as they are played, so anything here is a
// genuine snapshot of the board — nothing is lost by closing the terminal.
type ResumeList struct {
	ctx    Context
	cursor components.Cursor

	games   []app.ResumableGame
	loadErr error

	confirmingDiscard bool
}

// NewResumeList builds the resume screen.
func NewResumeList(ctx Context) *ResumeList {
	s := &ResumeList{ctx: ctx}
	s.reload()
	return s
}

// Init refreshes the list.
func (s *ResumeList) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *ResumeList) reload() {
	games, err := s.ctx.Services.Games.Resumable(s.ctx.Ctx)
	s.loadErr = err
	s.games = games
	s.cursor.SetCount(len(games))
}

// Title names the screen.
func (s *ResumeList) Title() string { return "Resume game" }

// Hints lists the available keys.
func (s *ResumeList) Hints() []components.KeyHint {
	if s.confirmingDiscard {
		return []components.KeyHint{
			{Keys: "y", Desc: "discard"},
			{Keys: "n/esc", Desc: "keep"},
		}
	}
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "enter", Desc: "resume"},
		{Keys: "d", Desc: "discard game"},
		{Keys: "esc", Desc: "back"},
	}
}

// Update handles resume and discard.
func (s *ResumeList) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	if s.confirmingDiscard {
		switch key.String() {
		case "y", "Y":
			return s, s.discard()
		case "n", "N", "esc":
			s.confirmingDiscard = false
		}
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
	case "enter":
		return s, s.resume()
	case "d":
		if _, ok := s.selected(); ok {
			s.confirmingDiscard = true
		}
	case "esc":
		return s, Pop()
	}
	return s, nil
}

func (s *ResumeList) selected() (app.ResumableGame, bool) {
	i := s.cursor.Index()
	if i < 0 || i >= len(s.games) {
		return app.ResumableGame{}, false
	}
	return s.games[i], true
}

func (s *ResumeList) resume() tea.Cmd {
	g, ok := s.selected()
	if !ok {
		return nil
	}
	switch g.Mode {
	case sqlite.ModeCricket:
		session, err := s.ctx.Services.Games.ResumeCricket(s.ctx.Ctx, g.ID)
		if err != nil {
			return Fail(err)
		}
		return Replace(NewCricketGame(s.ctx, session))
	default:
		session, err := s.ctx.Services.Games.ResumeX01(s.ctx.Ctx, g.ID)
		if err != nil {
			return Fail(err)
		}
		return Replace(NewX01Game(s.ctx, session))
	}
}

func (s *ResumeList) discard() tea.Cmd {
	g, ok := s.selected()
	if !ok {
		s.confirmingDiscard = false
		return nil
	}
	s.confirmingDiscard = false
	if err := s.ctx.Services.Games.Discard(s.ctx.Ctx, g.ID); err != nil {
		return Fail(err)
	}
	s.reload()
	return Success("Game discarded.")
}

// View renders the resumable games.
func (s *ResumeList) View(width, height int) string {
	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load games: " + s.loadErr.Error())
	}
	if len(s.games) == 0 {
		return styles.Faint.Render("No unfinished games. Anything you start is saved here automatically.")
	}

	if s.confirmingDiscard {
		g, _ := s.selected()
		return components.Confirm{
			Title: fmt.Sprintf("Discard the %s game from %s?",
				g.Label, g.StartedAt.Local().Format("2 Jan, 15:04")),
			Body:      "Every visit recorded in it is removed.",
			Confirm:   "discard",
			Cancel:    "keep",
			Dangerous: true,
		}.View()
	}

	var b strings.Builder
	start, end := s.cursor.Window(maxInt(height-4, 3))

	for i := start; i < end && i < len(s.games); i++ {
		g := s.games[i]
		selected := i == s.cursor.Index()

		var names []string
		for _, p := range g.Players {
			names = append(names, styles.PlayerName(p.Colour, false).Render(p.Name))
		}

		header := fmt.Sprintf("%s%-9s %s  %s",
			styles.Marker(selected),
			styles.Bold.Render(g.Label),
			g.StartedAt.Local().Format("2 Jan 15:04"),
			styles.Faint.Render(fmt.Sprintf("%d visits recorded", g.VisitCount)))
		if selected {
			header = styles.SelectedRow.Render(header)
		}
		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(components.Truncate("    "+strings.Join(names, ", "), maxInt(width-2, 20)))
		b.WriteString("\n\n")
	}
	return b.String()
}
