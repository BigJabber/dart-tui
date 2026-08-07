package screens

import (
	"fmt"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// Leaderboard ranks the active players.
//
// There is no single "best player" column, because a three-dart X01 average and
// a cricket batting average measure different things. The screen offers several
// orderings instead and names the one in use.
type Leaderboard struct {
	ctx    Context
	cursor components.Cursor

	rows    []app.LeaderboardRow
	sortKey app.SortKey
	loadErr error
}

// NewLeaderboard builds the statistics screen.
func NewLeaderboard(ctx Context) *Leaderboard {
	s := &Leaderboard{ctx: ctx}
	s.reload()
	return s
}

// Init refreshes the leaderboard.
func (s *Leaderboard) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *Leaderboard) reload() {
	rows, err := s.ctx.Services.Stats.Leaderboard(s.ctx.Ctx, s.sortKey)
	s.loadErr = err
	s.rows = rows
	s.cursor.SetCount(len(rows))
}

// Title names the screen.
func (s *Leaderboard) Title() string {
	return "Statistics — by " + s.sortKey.Label()
}

// Hints lists the available keys.
func (s *Leaderboard) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "enter", Desc: "open profile"},
		{Keys: "s", Desc: "change sort"},
		{Keys: "esc", Desc: "back"},
	}
}

// Update handles sorting and navigation.
func (s *Leaderboard) Update(msg tea.Msg) (Screen, tea.Cmd) {
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
	case "s", "tab":
		s.sortKey = s.sortKey.Next()
		s.reload()
		return s, Status("Sorted by " + s.sortKey.Label())
	case "enter":
		i := s.cursor.Index()
		if i >= 0 && i < len(s.rows) {
			return s, Push(NewPlayerProfile(s.ctx, s.rows[i].Player.ID))
		}
	case "esc":
		return s, Pop()
	}
	return s, nil
}

// View renders the leaderboard table.
func (s *Leaderboard) View(width, height int) string {
	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load statistics: " + s.loadErr.Error())
	}
	if len(s.rows) == 0 {
		return styles.Faint.Render("No active players yet.")
	}

	var b strings.Builder
	b.WriteString(styles.Subtitle.Render(
		"X01 and cricket figures are kept apart — they are not comparable."))
	b.WriteString("\n\n")

	nameWidth := maxInt(minInt(width-52, 18), 8)
	table := components.Table{
		Columns: []components.Column{
			{Title: "", Width: 2},
			{Title: "Player", Width: nameWidth},
			{Title: "Won", Width: 5, Right: true},
			{Title: "Played", Width: 6, Right: true},
			{Title: "X01 avg", Width: 8, Right: true},
			{Title: "Runs", Width: 6, Right: true},
			{Title: "Wkts", Width: 5, Right: true},
		},
	}

	start, end := s.cursor.Window(maxInt(height-8, 3))
	for i := start; i < end && i < len(s.rows); i++ {
		row := s.rows[i]
		selected := i == s.cursor.Index()

		average := "—"
		if row.Stats.X01.Darts > 0 {
			average = fmt.Sprintf("%.2f", row.Stats.X01.Average)
		}
		runs := "—"
		if row.Stats.Cricket.Batting.Visits > 0 {
			runs = fmt.Sprintf("%d", row.Stats.Cricket.Batting.Runs)
		}
		wickets := "—"
		if row.Stats.Cricket.Bowling.Visits > 0 {
			wickets = fmt.Sprintf("%d", row.Stats.Cricket.Bowling.Wickets)
		}

		table.Rows = append(table.Rows, components.Row{
			Selected: selected,
			Cells: []string{
				styles.Marker(selected),
				styles.PlayerName(row.Player.Colour, selected).Render(
					components.Truncate(row.Player.Name, nameWidth)),
				fmt.Sprintf("%d", row.Stats.TotalGamesWon()),
				fmt.Sprintf("%d", row.Stats.TotalGamesPlayed()),
				average,
				runs,
				wickets,
			},
		})
	}
	b.WriteString(table.View())

	b.WriteString("\n\n")
	b.WriteString(styles.Faint.Render(
		"Won and Played count both game modes. Averages, runs and wickets do not mix."))
	return b.String()
}
