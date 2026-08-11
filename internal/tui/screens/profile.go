package screens

import (
	"fmt"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/domain/stats"
	"github.com/BigJabber/dart-tui/internal/domain/x01"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// profileMode is what the profile screen is currently doing.
type profileMode int

const (
	profileView profileMode = iota
	profileRenaming
	profileRecolouring
	profileConfirmArchive
)

// PlayerProfile shows one player's lifetime record and offers the actions that
// change how they are displayed.
//
// The figures here are explicitly labelled lifetime, because the game screens
// show current-game averages and the two must never be confused.
type PlayerProfile struct {
	ctx      Context
	playerID int64

	player  player.Player
	stats   stats.PlayerStats
	recent  []app.RecentGame
	loadErr error

	mode         profileMode
	input        textinput.Model
	entryErr     string
	colourCursor components.Cursor
	viewport     viewport.Model
	ready        bool
}

// NewPlayerProfile builds the profile screen for a player.
func NewPlayerProfile(ctx Context, playerID int64) *PlayerProfile {
	s := &PlayerProfile{ctx: ctx, playerID: playerID}
	s.colourCursor.SetCount(len(player.Palette))
	s.reload()
	return s
}

// Init refreshes the profile.
func (s *PlayerProfile) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *PlayerProfile) reload() {
	s.loadErr = nil

	p, err := s.ctx.Services.Players.Get(s.ctx.Ctx, s.playerID)
	if err != nil {
		s.loadErr = err
		return
	}
	s.player = p

	if s.stats, err = s.ctx.Services.Stats.ForPlayer(s.ctx.Ctx, s.playerID); err != nil {
		s.loadErr = err
		return
	}
	if s.recent, err = s.ctx.Services.Players.RecentGames(s.ctx.Ctx, s.playerID, 8); err != nil {
		s.loadErr = err
		return
	}

	// Start the colour picker on the player's current colour.
	for i, c := range player.Palette {
		if strings.EqualFold(c, p.Colour) {
			s.colourCursor.Set(i)
			break
		}
	}
}

// Title names the screen.
func (s *PlayerProfile) Title() string {
	if s.player.Name == "" {
		return "Player"
	}
	return s.player.Name
}

// Hints lists the available keys for the current mode.
func (s *PlayerProfile) Hints() []components.KeyHint {
	switch s.mode {
	case profileRenaming:
		return []components.KeyHint{
			{Keys: "enter", Desc: "save name"},
			{Keys: "esc", Desc: "cancel"},
		}
	case profileRecolouring:
		return []components.KeyHint{
			{Keys: "←/→", Desc: "choose colour"},
			{Keys: "enter", Desc: "save"},
			{Keys: "esc", Desc: "cancel"},
		}
	case profileConfirmArchive:
		return []components.KeyHint{
			{Keys: "y", Desc: "confirm"},
			{Keys: "n/esc", Desc: "cancel"},
		}
	default:
		archiveLabel := "archive player"
		if s.player.Archived() {
			archiveLabel = "restore player"
		}
		return []components.KeyHint{
			{Keys: "↑/↓", Desc: "scroll"},
			{Keys: "e", Desc: "edit name"},
			{Keys: "c", Desc: "change colour"},
			{Keys: "a", Desc: archiveLabel},
			{Keys: "esc", Desc: "back"},
		}
	}
}

// Update handles profile keys and the edit modes.
func (s *PlayerProfile) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch s.mode {
	case profileRenaming:
		return s.updateRenaming(key)
	case profileRecolouring:
		return s.updateRecolouring(key)
	case profileConfirmArchive:
		return s.updateArchiveConfirm(key)
	}

	switch key.String() {
	case "esc":
		return s, Pop()
	case "e":
		s.mode = profileRenaming
		s.entryErr = ""
		s.input = newNameInput(s.player.Name)
		return s, s.input.Focus()
	case "c":
		s.mode = profileRecolouring
		return s, nil
	case "a":
		s.mode = profileConfirmArchive
		return s, nil
	case "up", "k":
		s.viewport.ScrollUp(1)
	case "down", "j":
		s.viewport.ScrollDown(1)
	case "pgup":
		s.viewport.HalfPageUp()
	case "pgdown":
		s.viewport.HalfPageDown()
	case "home", "g":
		s.viewport.GotoTop()
	case "end", "G":
		s.viewport.GotoBottom()
	}
	return s, nil
}

func (s *PlayerProfile) updateRenaming(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.mode = profileView
		s.entryErr = ""
		s.input.Blur()
		return s, nil
	case "enter":
		name := strings.TrimSpace(s.input.Value())
		if err := s.ctx.Services.Players.Rename(s.ctx.Ctx, s.playerID, name); err != nil {
			s.entryErr = err.Error()
			return s, nil
		}
		s.mode = profileView
		s.input.Blur()
		s.reload()
		return s, Success("Name updated")
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(key)
	s.entryErr = ""
	return s, cmd
}

func (s *PlayerProfile) updateRecolouring(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.mode = profileView
		return s, nil
	case "left", "h", "up":
		s.colourCursor.Prev()
	case "right", "l", "down":
		s.colourCursor.Next()
	case "enter":
		i := s.colourCursor.Index()
		if i < 0 || i >= len(player.Palette) {
			s.mode = profileView
			return s, nil
		}
		if err := s.ctx.Services.Players.SetColour(s.ctx.Ctx, s.playerID, player.Palette[i]); err != nil {
			return s, Fail(err)
		}
		s.mode = profileView
		s.reload()
		return s, Success("Colour updated")
	}
	return s, nil
}

func (s *PlayerProfile) updateArchiveConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		archiving := !s.player.Archived()
		if err := s.ctx.Services.Players.SetArchived(s.ctx.Ctx, s.playerID, archiving); err != nil {
			s.mode = profileView
			return s, Fail(err)
		}
		s.mode = profileView
		s.reload()
		if archiving {
			return s, Success(s.player.Name + " archived — their statistics are kept")
		}
		return s, Success(s.player.Name + " restored to active play")
	case "n", "N", "esc":
		s.mode = profileView
	}
	return s, nil
}

// View renders the profile, or whichever edit mode is active.
func (s *PlayerProfile) View(width, height int) string {
	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load this player: " + s.loadErr.Error())
	}

	switch s.mode {
	case profileRenaming:
		return s.renameView()
	case profileRecolouring:
		return s.colourView(width)
	case profileConfirmArchive:
		return s.archiveConfirmView()
	}

	body := s.body(width)
	if !s.ready || s.viewport.Width != width || s.viewport.Height != height {
		s.viewport = viewport.New(width, maxInt(height, 3))
		s.ready = true
	}
	s.viewport.SetContent(body)

	out := s.viewport.View()
	if s.viewport.TotalLineCount() > s.viewport.VisibleLineCount() {
		out += "\n" + styles.Faint.Render(fmt.Sprintf("── %3.0f%% ── ↑/↓ to scroll",
			s.viewport.ScrollPercent()*100))
	}
	return out
}

func (s *PlayerProfile) renameView() string {
	var b strings.Builder
	b.WriteString(styles.Bold.Render("Edit name"))
	b.WriteString("\n\n")
	b.WriteString(s.input.View())
	b.WriteString("\n\n")
	if s.entryErr != "" {
		b.WriteString(styles.ErrorText.Render(s.entryErr))
	} else {
		b.WriteString(styles.Faint.Render("Renaming keeps every recorded game and statistic."))
	}
	return b.String()
}

func (s *PlayerProfile) colourView(width int) string {
	var b strings.Builder
	b.WriteString(styles.Bold.Render("Choose a display colour"))
	b.WriteString("\n\n")

	perRow := maxInt(minInt((width-2)/16, 4), 1)
	for i, colour := range player.Palette {
		selected := i == s.colourCursor.Index()
		swatch := styles.Swatch(colour).Render("████")
		label := styles.PlayerName(colour, selected).Render(s.player.Name)
		cell := fmt.Sprintf("%s%s %s", styles.Marker(selected), swatch, components.Truncate(label, 10))
		if selected {
			cell = styles.SelectedRow.Render(cell)
		}
		b.WriteString(cell)
		if (i+1)%perRow == 0 {
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(styles.Faint.Render("The preview shows how the name will appear during a game."))
	return b.String()
}

func (s *PlayerProfile) archiveConfirmView() string {
	if s.player.Archived() {
		return components.Confirm{
			Title:   "Restore " + s.player.Name + "?",
			Body:    "They will appear again when selecting players for a game.",
			Confirm: "restore",
		}.View()
	}
	return components.Confirm{
		Title: "Archive " + s.player.Name + "?",
		Body: "They will be hidden when selecting players for a new game.\n" +
			"Every visit, result and statistic is kept, and they can be restored\n" +
			"at any time. Nothing is deleted.",
		Confirm: "archive",
	}.View()
}

// body renders the whole profile as scrollable content.
func (s *PlayerProfile) body(width int) string {
	var b strings.Builder

	name := styles.PlayerName(s.player.Colour, true).Render(s.player.Name)
	b.WriteString(name)
	if s.player.Archived() {
		b.WriteString("  " + styles.WarningText.Render("[archived]"))
	}
	b.WriteString("\n")
	b.WriteString(styles.Faint.Render("Joined " + s.player.CreatedAt.Local().Format("2 Jan 2006")))
	b.WriteString("\n\n")

	b.WriteString(s.lifetimeSection())
	b.WriteString("\n")
	b.WriteString(s.x01Section(x01.Start501))
	b.WriteString("\n")
	b.WriteString(s.x01Section(x01.Start301))
	b.WriteString("\n")
	b.WriteString(s.cricketBattingSection())
	b.WriteString("\n")
	b.WriteString(s.cricketBowlingSection())
	b.WriteString("\n")
	b.WriteString(s.atwSection())
	b.WriteString("\n")
	b.WriteString(s.recentSection(width))

	return b.String()
}

func (s *PlayerProfile) lifetimeSection() string {
	var b strings.Builder
	b.WriteString(sectionTitle("Lifetime summary"))
	b.WriteString("\n")
	b.WriteString(field("Games played", fmt.Sprintf("%d", s.stats.TotalGamesPlayed())))
	b.WriteString(field("Games won", fmt.Sprintf("%d (%.0f%%)",
		s.stats.TotalGamesWon(), s.stats.TotalWinPercent())))
	if training := s.stats.TotalTrainingGames(); training > 0 {
		b.WriteString(field("Training sessions", fmt.Sprintf("%d", training)))
		// The newline goes outside Render: a trailing newline inside makes
		// Lip Gloss pad an empty final line to the block's width, which the next
		// line then starts after.
		b.WriteString(styles.Faint.Render(
			"  Solo practice is not counted above — there was nobody to beat."))
		b.WriteString("\n")
	}
	b.WriteString(styles.Faint.Render(
		"  X01 and cricket are counted together here but never averaged together."))
	b.WriteString("\n")
	return b.String()
}

func (s *PlayerProfile) x01Section(startScore int) string {
	var b strings.Builder
	b.WriteString(sectionTitle(fmt.Sprintf("%d", startScore)))
	b.WriteString("\n")

	v, ok := s.stats.X01.Variant(startScore)
	if !ok || v.GamesPlayed+v.TrainingGames == 0 {
		b.WriteString(styles.Faint.Render("  No completed games yet.\n"))
		return b.String()
	}

	b.WriteString(field("Games played", fmt.Sprintf("%d", v.GamesPlayed)))
	b.WriteString(field("Games won", fmt.Sprintf("%d", v.GamesWon)))
	if v.TrainingGames > 0 {
		b.WriteString(field("Training sessions", fmt.Sprintf("%d", v.TrainingGames)))
	}
	b.WriteString(field("Legs completed", fmt.Sprintf("%d", v.LegsCompleted)))
	b.WriteString(field("Darts thrown", fmt.Sprintf("%d", v.Darts)))
	b.WriteString(field("Points scored", fmt.Sprintf("%d", v.Points)))
	b.WriteString(field("Lifetime 3-dart average", fmt.Sprintf("%.2f", v.Average)))
	if v.BestAverage > 0 {
		b.WriteString(field("Best game average", fmt.Sprintf("%.2f", v.BestAverage)))
	}
	if v.FewestDarts > 0 {
		b.WriteString(field("Fewest darts to finish", fmt.Sprintf("%d", v.FewestDarts)))
	} else {
		b.WriteString(field("Fewest darts to finish", "—"))
	}
	return b.String()
}

func (s *PlayerProfile) cricketBattingSection() string {
	var b strings.Builder
	b.WriteString(sectionTitle("Cricket — batting"))
	b.WriteString("\n")

	bat := s.stats.Cricket.Batting
	if bat.Visits == 0 {
		b.WriteString(styles.Faint.Render("  No batting visits yet.\n"))
		return b.String()
	}
	b.WriteString(field("Batting visits", fmt.Sprintf("%d", bat.Visits)))
	b.WriteString(field("Raw points scored", fmt.Sprintf("%d", bat.RawPoints)))
	b.WriteString(field("Average raw visit", fmt.Sprintf("%.1f", bat.AverageRawPerVisit)))
	b.WriteString(field("Runs scored", fmt.Sprintf("%d", bat.Runs)))
	b.WriteString(field("Average runs per visit", fmt.Sprintf("%.1f", bat.AverageRunsPerVisit)))
	b.WriteString(field("Highest raw visit", fmt.Sprintf("%d", bat.HighestVisitRaw)))
	b.WriteString(field("Highest runs from a visit", fmt.Sprintf("%d", bat.HighestVisitRuns)))
	return b.String()
}

func (s *PlayerProfile) cricketBowlingSection() string {
	var b strings.Builder
	b.WriteString(sectionTitle("Cricket — bowling"))
	b.WriteString("\n")

	bowl := s.stats.Cricket.Bowling
	if bowl.Visits == 0 {
		b.WriteString(styles.Faint.Render("  No bowling visits yet.\n"))
		return b.String()
	}
	b.WriteString(field("Bowling visits", fmt.Sprintf("%d", bowl.Visits)))
	b.WriteString(field("Darts bowled", fmt.Sprintf("%d", bowl.Darts)))
	b.WriteString(field("Wickets", fmt.Sprintf("%d", bowl.Wickets)))
	b.WriteString(field("Outer bulls", fmt.Sprintf("%d", bowl.OuterBulls)))
	b.WriteString(field("Inner bulls", fmt.Sprintf("%d", bowl.InnerBulls)))
	b.WriteString(field("Wickets per visit", fmt.Sprintf("%.2f", bowl.WicketsPerVisit)))
	b.WriteString(field("Cricket games played", fmt.Sprintf("%d", s.stats.Cricket.GamesPlayed)))
	b.WriteString(field("Cricket games won", fmt.Sprintf("%d", s.stats.Cricket.GamesWon)))
	return b.String()
}

// atwSection reports the Around the World record.
//
// There is deliberately no average here that could be confused with a three-
// dart average. The number that matters is how few darts a run took, and the
// supporting figures describe how the darts landed rather than what they
// scored: an Around the World dart scores nothing in the X01 sense.
func (s *PlayerProfile) atwSection() string {
	var b strings.Builder
	b.WriteString(sectionTitle("Around the World"))
	b.WriteString("\n")

	a := s.stats.ATW
	if a.GamesPlayed+a.TrainingGames == 0 {
		b.WriteString(styles.Faint.Render("  No completed games yet.\n"))
		return b.String()
	}

	b.WriteString(field("Games played", fmt.Sprintf("%d", a.GamesPlayed)))
	b.WriteString(field("Games won", fmt.Sprintf("%d", a.GamesWon)))
	if a.TrainingGames > 0 {
		b.WriteString(field("Training sessions", fmt.Sprintf("%d", a.TrainingGames)))
	}
	b.WriteString(field("Runs completed", fmt.Sprintf("%d of %d (%.0f%%)",
		a.RunsCompleted, a.GamesPlayed+a.TrainingGames, a.CompletionPercent)))

	if a.BestDarts > 0 {
		b.WriteString(field("Fewest darts to go round", fmt.Sprintf("%d", a.BestDarts)))
		b.WriteString(field("Average darts per run", fmt.Sprintf("%.1f", a.AverageDartsPerRun)))
	} else {
		b.WriteString(field("Fewest darts to go round", "—"))
	}

	b.WriteString(field("Darts thrown", fmt.Sprintf("%d", a.Darts)))
	b.WriteString(field("Targets hit", fmt.Sprintf("%d of %d darts (%.0f%%)",
		a.Hits, a.Darts, a.HitPercent)))
	b.WriteString(field("Doubles / trebles hit", fmt.Sprintf("%d / %d", a.Doubles, a.Trebles)))
	b.WriteString(field("Targets per dart", fmt.Sprintf("%.2f", a.ProgressPerDart)))
	return b.String()
}

func (s *PlayerProfile) recentSection(width int) string {
	var b strings.Builder
	b.WriteString(sectionTitle("Recent games"))
	b.WriteString("\n")

	if len(s.recent) == 0 {
		b.WriteString(styles.Faint.Render("  Nothing played yet.\n"))
		return b.String()
	}

	for _, g := range s.recent {
		outcome := g.Outcome
		switch {
		case g.Won:
			outcome = styles.SuccessText.Render(outcome)
		case g.Outcome == "Draw":
			outcome = styles.WarningText.Render(outcome)
		default:
			outcome = styles.Faint.Render(outcome)
		}

		var opponents []string
		for _, o := range g.Opponents {
			opponents = append(opponents, styles.PlayerName(o.Colour, false).Render(o.Name))
		}
		versus := "solo"
		if len(opponents) > 0 {
			versus = "v " + strings.Join(opponents, ", ")
		}

		line := fmt.Sprintf("  %-8s %-10s %-8s %s",
			g.PlayedAt.Local().Format("2 Jan"), g.Label, outcome, versus)
		b.WriteString(components.Truncate(line, maxInt(width-2, 20)))
		if g.Detail != "" {
			b.WriteString(styles.Faint.Render("  (" + g.Detail + ")"))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func sectionTitle(title string) string {
	return styles.Title.Render(title)
}

func field(label, value string) string {
	return fmt.Sprintf("  %-28s %s\n", label, styles.Bold.Render(value))
}
