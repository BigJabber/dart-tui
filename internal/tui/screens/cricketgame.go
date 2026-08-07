package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/darts"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// cricketPhase is what the cricket screen is waiting for.
type cricketPhase int

const (
	// cricketEntry is the normal state. Which sort of entry it is — batting or
	// bowling — comes from the engine, never from the screen.
	cricketEntry cricketPhase = iota
	cricketConfirmAbandon
	cricketConfirmLeave
	cricketComplete
	// cricketCalculating adds up the individual darts of a batting visit. The
	// raw score is what the operator has to type, so the same arithmetic problem
	// applies here as in X01.
	cricketCalculating
)

// CricketGame is the in-play screen for the office cricket variant.
type CricketGame struct {
	ctx     Context
	session *app.CricketSession

	phase cricketPhase

	// battingEntry is the raw dartboard score being typed, and battingDarts the
	// number of darts it took.
	battingEntry string
	battingDarts int
	// bowlingEntry is the M/O/B notation typed so far.
	bowlingEntry string
	// battingFrom records the sum a calculated score came from, so the operator
	// can check the arithmetic before committing to it.
	battingFrom string

	calc    components.Calculator
	calcErr string

	entryErr  string
	lastVisit string
}

// NewCricketGame builds the play screen for a live or resumed cricket session.
func NewCricketGame(ctx Context, session *app.CricketSession) *CricketGame {
	s := &CricketGame{ctx: ctx, session: session, battingDarts: defaultDarts}
	if session.Game().Complete() {
		s.phase = cricketComplete
	}
	return s
}

// Init does nothing; the session is already loaded.
func (s *CricketGame) Init() tea.Cmd { return nil }

// Title names the screen.
func (s *CricketGame) Title() string {
	return fmt.Sprintf("Cricket — game %d", s.session.GameID())
}

// Hints lists the keys for the current phase.
func (s *CricketGame) Hints() []components.KeyHint {
	switch s.phase {
	case cricketConfirmAbandon:
		return []components.KeyHint{
			{Keys: "y", Desc: "abandon with no result"},
			{Keys: "n/esc", Desc: "keep playing"},
		}
	case cricketConfirmLeave:
		return []components.KeyHint{
			{Keys: "y", Desc: "leave (the game is saved)"},
			{Keys: "n/esc", Desc: "stay"},
		}
	case cricketComplete:
		return []components.KeyHint{
			{Keys: "u", Desc: "undo the last visit"},
			{Keys: "enter/esc", Desc: "back to the menu"},
		}
	case cricketCalculating:
		return []components.KeyHint{
			{Keys: "0-9", Desc: "a dart's score"},
			{Keys: "+", Desc: "add another"},
			{Keys: "enter", Desc: "use the total"},
			{Keys: "esc", Desc: "cancel"},
		}
	}

	turn, ok := s.session.Game().CurrentTurn()
	if ok && turn.Kind == cricket.Bowling {
		return []components.KeyHint{
			{Keys: "1", Desc: "no wicket"},
			{Keys: "2", Desc: "outer bull"},
			{Keys: "3", Desc: "inner bull"},
			{Keys: "enter", Desc: "record"},
			{Keys: "u", Desc: "undo"},
			{Keys: "f", Desc: "abandon"},
		}
	}
	return []components.KeyHint{
		{Keys: "0-9", Desc: "raw score"},
		{Keys: "=", Desc: "add darts up"},
		{Keys: "tab", Desc: "darts thrown"},
		{Keys: "enter", Desc: "record"},
		{Keys: "u", Desc: "undo"},
		{Keys: "f", Desc: "abandon"},
	}
}

// Update routes keys according to the phase and whose turn it is.
func (s *CricketGame) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch s.phase {
	case cricketConfirmAbandon:
		return s.updateAbandonConfirm(key)
	case cricketConfirmLeave:
		return s.updateLeaveConfirm(key)
	case cricketComplete:
		return s.updateComplete(key)
	case cricketCalculating:
		return s.updateCalculating(key)
	}

	turn, ok := s.session.Game().CurrentTurn()
	if !ok {
		s.phase = cricketComplete
		return s, nil
	}
	if turn.Kind == cricket.Bowling {
		return s.updateBowling(key, turn)
	}
	return s.updateBatting(key, turn)
}

// ----------------------------------------------------------------- batting

func (s *CricketGame) updateBatting(key tea.KeyMsg, turn cricket.Turn) (Screen, tea.Cmd) {
	str := key.String()

	if len(str) == 1 && str[0] >= '0' && str[0] <= '9' {
		if len(s.battingEntry) < 3 {
			s.battingEntry += str
			s.entryErr = ""
			s.battingFrom = ""
		}
		return s, nil
	}

	switch str {
	// "=" is what the help says; "+" is the one that exists on a numeric keypad.
	// Pressing "+" part-way through typing carries the digits across as the
	// first term of the sum.
	case "=", "+":
		return s, s.openCalculator(s.battingEntry, str == "+")
	case "backspace":
		if len(s.battingEntry) > 0 {
			s.battingEntry = s.battingEntry[:len(s.battingEntry)-1]
		}
		s.entryErr = ""
		s.battingFrom = ""
	case "tab", "d":
		s.battingDarts--
		if s.battingDarts < 1 {
			s.battingDarts = darts.MaxDartsPerVisit
		}
		s.entryErr = ""
	case "u":
		return s, s.undo()
	case "f":
		s.phase = cricketConfirmAbandon
	case "esc":
		s.phase = cricketConfirmLeave
	case "enter":
		return s, s.recordBatting(turn)
	}
	return s, nil
}

func (s *CricketGame) recordBatting(turn cricket.Turn) tea.Cmd {
	if s.battingEntry == "" {
		s.entryErr = "Enter the total scored with these darts — runs are worked out for you."
		return nil
	}
	raw, err := strconv.Atoi(s.battingEntry)
	if err != nil {
		s.entryErr = "That is not a number."
		return nil
	}

	event, err := s.session.RecordBatting(s.ctx.Ctx, turn.PlayerID, raw, s.battingDarts)
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	s.battingEntry = ""
	s.battingDarts = defaultDarts
	s.entryErr = ""
	s.battingFrom = ""

	name := s.session.Player(turn.PlayerID).Name
	switch event.Runs {
	case 0:
		s.lastVisit = fmt.Sprintf("%s scored %d — no runs (%d or less scores nothing)",
			name, event.RawPoints, cricket.RunsThreshold)
	case 1:
		s.lastVisit = fmt.Sprintf("%s scored %d — 1 run", name, event.RawPoints)
	default:
		s.lastVisit = fmt.Sprintf("%s scored %d — %d runs", name, event.RawPoints, event.Runs)
	}
	return s.afterVisit(s.lastVisit)
}

// -------------------------------------------------------------- calculator

// openCalculator starts a sum for the batting score, carrying across any digits
// already typed.
func (s *CricketGame) openCalculator(seed string, withPlus bool) tea.Cmd {
	s.calc.Reset()
	s.calc.SetExpression(seed)
	if withPlus {
		s.calc.Key("+")
	}
	s.calcErr = ""
	s.phase = cricketCalculating
	return nil
}

// updateCalculating handles the sum, then hands the total to the score box for
// the operator to confirm.
func (s *CricketGame) updateCalculating(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "esc":
		s.phase = cricketEntry
		s.calcErr = ""
		return s, nil
	case "enter":
		return s, s.applyCalculation()
	}

	if s.calc.Key(key.String()) {
		s.calcErr = ""
	}
	return s, nil
}

// applyCalculation checks the total against the same rule the engine applies
// when recording, so a mistyped sum is corrected where it was made.
func (s *CricketGame) applyCalculation() tea.Cmd {
	total, err := s.calc.Total()
	if err != nil {
		s.calcErr = capitalise(err.Error())
		return nil
	}
	if err := cricket.ValidateBattingScore(total, s.battingDarts); err != nil {
		s.calcErr = capitalise(err.Error())
		return nil
	}

	s.battingEntry = strconv.Itoa(total)
	s.battingFrom = s.calc.Display()
	s.entryErr = ""
	s.phase = cricketEntry
	return nil
}

// calculatorPanel renders the running sum of the individual darts.
func (s *CricketGame) calculatorPanel(width int) string {
	var b strings.Builder

	b.WriteString(styles.Bold.Render("Add up the darts"))
	if turn, ok := s.session.Game().CurrentTurn(); ok {
		p := s.session.Player(turn.PlayerID)
		b.WriteString(styles.Faint.Render("  for "))
		b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	}
	b.WriteString("\n\n")

	b.WriteString(s.calc.View())
	// The runs the total will earn, so the house rule stays visible here too.
	if total, err := s.calc.Total(); err == nil {
		b.WriteString("\n")
		b.WriteString("Runs   ")
		runs := cricket.RunsFor(total)
		switch runs {
		case 0:
			b.WriteString(styles.Faint.Render(fmt.Sprintf(
				"0  (%d or less scores nothing)", cricket.RunsThreshold)))
		case 1:
			b.WriteString(styles.SuccessText.Render("1"))
		default:
			b.WriteString(styles.SuccessText.Render(strconv.Itoa(runs)))
		}
	}
	b.WriteString("\n\n")

	if s.calcErr != "" {
		b.WriteString(styles.ErrorText.Render(s.calcErr))
	} else {
		b.WriteString(styles.Faint.Render(
			"Enter puts the total in the score box; press enter again to record it."))
	}

	panel := styles.FocusPanel
	if width > 4 {
		panel = panel.Width(minInt(width-2, 78))
	}
	return panel.Render(b.String())
}

// ----------------------------------------------------------------- bowling

func (s *CricketGame) updateBowling(key tea.KeyMsg, turn cricket.Turn) (Screen, tea.Cmd) {
	switch key.String() {
	// 1, 2 and 3 sit on the keypad beside the digits used for batting scores.
	// The old M/O/B letters still work for anyone who learnt those first.
	case "1", "2", "3", "m", "M", "o", "O", "b", "B":
		if len(s.bowlingEntry) >= darts.MaxDartsPerVisit {
			s.entryErr = fmt.Sprintf("A visit is at most %d darts.", darts.MaxDartsPerVisit)
			return s, nil
		}
		bowl, err := cricket.ParseBowl([]rune(key.String())[0])
		if err != nil {
			s.entryErr = capitalise(err.Error())
			return s, nil
		}
		// Store the canonical key so the display is consistent however it was
		// typed.
		s.bowlingEntry += bowl.Key()
		s.entryErr = ""
	case "backspace":
		if len(s.bowlingEntry) > 0 {
			s.bowlingEntry = s.bowlingEntry[:len(s.bowlingEntry)-1]
		}
		s.entryErr = ""
	case "u":
		return s, s.undo()
	case "f":
		s.phase = cricketConfirmAbandon
	case "esc":
		s.phase = cricketConfirmLeave
	case "enter":
		return s, s.recordBowling(turn)
	}
	return s, nil
}

func (s *CricketGame) recordBowling(turn cricket.Turn) tea.Cmd {
	bowls, err := cricket.ParseBowls(s.bowlingEntry)
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	event, err := s.session.RecordBowling(s.ctx.Ctx, turn.PlayerID, bowls)
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}

	s.bowlingEntry = ""
	s.entryErr = ""

	name := s.session.Player(turn.PlayerID).Name
	switch event.Wickets {
	case 0:
		s.lastVisit = fmt.Sprintf("%s bowled %d darts — no wickets", name, event.Darts)
	case 1:
		s.lastVisit = fmt.Sprintf("%s took 1 wicket", name)
	default:
		s.lastVisit = fmt.Sprintf("%s took %d wickets", name, event.Wickets)
	}
	// A visit truncated by the wicket limit is worth saying out loud.
	if event.Darts < len(bowls) {
		s.lastVisit += fmt.Sprintf(" — innings ended after %d dart(s)", event.Darts)
	}
	return s.afterVisit(s.lastVisit)
}

// ------------------------------------------------------------------ shared

// afterVisit reports the visit and notices when the innings, the leg or the
// whole match has just changed underneath the operator.
func (s *CricketGame) afterVisit(message string) tea.Cmd {
	game := s.session.Game()
	if game.Complete() {
		s.phase = cricketComplete
		return Success(message + " — " + s.resultSentence())
	}

	// A fresh innings means the previous one just ended, which is a big enough
	// change of state to spell out.
	inn := game.CurrentInnings()
	if inn.Runs != 0 || inn.Wickets != 0 {
		return Status(message)
	}

	batting := game.Team(inn.BattingSide).Name
	if target, ok := game.Target(); ok {
		needed, _ := game.RunsNeeded()
		return Success(fmt.Sprintf(
			"%s — innings over. %s bat last, chasing %d: they need %d to win.",
			message, batting, target, needed))
	}
	if inn.Number == 3 {
		leader, by, ok := game.Lead()
		if ok {
			return Success(fmt.Sprintf(
				"%s — leg 1 over. %s lead by %d, so %s bat first in leg 2.",
				message, game.Team(leader).Name, by, batting))
		}
		return Success(fmt.Sprintf("%s — leg 1 over, level on aggregate. %s bat first.",
			message, batting))
	}
	return Success(fmt.Sprintf("%s — innings over. %s bat.", message, batting))
}

func (s *CricketGame) undo() tea.Cmd {
	event, err := s.session.Undo(s.ctx.Ctx)
	if err != nil {
		return Fail(err)
	}
	if !s.session.Game().Complete() {
		s.phase = cricketEntry
	}
	s.battingEntry = ""
	s.bowlingEntry = ""
	s.entryErr = ""
	s.lastVisit = ""

	name := s.session.Player(event.PlayerID).Name
	if event.Kind == cricket.Batting {
		return Success(fmt.Sprintf("Undid %s's batting visit of %d", name, event.RawPoints))
	}
	return Success(fmt.Sprintf("Undid %s's bowling visit", name))
}

func (s *CricketGame) updateAbandonConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		if err := s.session.Abandon(s.ctx.Ctx); err != nil {
			s.phase = cricketEntry
			return s, Fail(err)
		}
		return s, tea.Batch(PopToRoot(), Warn("Game abandoned — no result recorded."))
	case "n", "N", "esc":
		s.phase = cricketEntry
	}
	return s, nil
}

func (s *CricketGame) updateLeaveConfirm(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "y", "Y":
		return s, tea.Batch(PopToRoot(), Status("Game saved — resume it from the main menu."))
	case "n", "N", "esc":
		s.phase = cricketEntry
	}
	return s, nil
}

func (s *CricketGame) updateComplete(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "u":
		return s, s.undo()
	case "enter", "esc", "q":
		return s, PopToRoot()
	}
	return s, nil
}

// -------------------------------------------------------------------- view

// View renders the match state, the large panel and the current prompt.
func (s *CricketGame) View(width, height int) string {
	board := s.scoreboard(width)
	lower := s.lowerPanel(width)

	var b strings.Builder
	b.WriteString(board)
	b.WriteString("\n")

	// The prompt and the table get their rows first; the large panel takes what
	// is left and disappears when there is none, exactly as in X01.
	spare := height - components.LineCount(board) - components.LineCount(lower) - 2
	if spot := s.spotlight(width, spare); spot != "" {
		b.WriteString(spot)
		b.WriteString("\n")
	}

	b.WriteString(lower)
	return b.String()
}

// lowerPanel renders the prompt for the current phase.
func (s *CricketGame) lowerPanel(width int) string {
	switch s.phase {
	case cricketConfirmAbandon:
		return components.Confirm{
			Title: "Abandon this game?",
			Body: "No winner will be recorded and it will not count towards\n" +
				"anybody's cricket record. The visits already entered are kept.",
			Confirm:   "abandon",
			Cancel:    "keep playing",
			Dangerous: true,
		}.View()
	case cricketConfirmLeave:
		return components.Confirm{
			Title:   "Leave this game?",
			Body:    "It is saved automatically and will be waiting under Resume game.",
			Confirm: "leave",
			Cancel:  "stay",
		}.View()
	case cricketComplete:
		return s.completePanel()
	case cricketCalculating:
		return s.calculatorPanel(width)
	default:
		return s.entryPanel(width)
	}
}

// spotlight is the large panel: who is throwing, and the number the innings
// turns on, drawn big enough to read from the oche.
//
// The number belongs to the innings rather than to the thrower, so it does not
// change when the sides swap over mid-innings — which is the right answer for a
// bowler, whose job is defined by the total being defended. The supporting line
// therefore always names the side the number belongs to.
//
// It is hidden behind the two dialogs, and once the match is over: the finished
// game has no thrower and no live number, and completePanel already carries the
// result.
func (s *CricketGame) spotlight(width, rows int) string {
	switch s.phase {
	case cricketConfirmAbandon, cricketConfirmLeave, cricketComplete:
		return ""
	}

	game := s.session.Game()
	turn, ok := game.CurrentTurn()
	if !ok {
		return ""
	}
	p := s.session.Player(turn.PlayerID)
	inn := game.CurrentInnings()
	batting := game.Team(inn.BattingSide).Name

	banner := "▶ BOWLING"
	style := styles.WarningText
	if turn.Kind == cricket.Batting {
		banner = "▶ BATTING"
		style = styles.SuccessText
	}

	// A target exists only in the final innings, which is the one and only
	// chase. The same test decides it here as in the scoreboard above.
	number := strconv.Itoa(inn.Runs)
	caption := fmt.Sprintf("RUNS\nFOR %s", strings.ToUpper(plural(inn.Wickets, "wkt")))
	sub := fmt.Sprintf("%s BATTING  ·  %s ENDS THE INNINGS",
		strings.ToUpper(batting), strings.ToUpper(plural(game.Config().WicketLimit, "wicket")))

	if target, chasing := game.Target(); chasing {
		needed, _ := game.RunsNeeded()
		number = strconv.Itoa(needed)
		caption = "TO WIN"
		sub = fmt.Sprintf("%s NEED %d  ·  TARGET %d  ·  %s DOWN",
			strings.ToUpper(batting), needed, target, strings.ToUpper(plural(inn.Wickets, "wkt")))
	}

	return components.Spotlight{
		Banner:      banner,
		BannerStyle: &style,
		Name:        p.Name,
		Colour:      p.Colour,
		Number:      number,
		Caption:     caption,
		Sub:         sub,
		Rows:        rows,
	}.View(width)
}

func (s *CricketGame) scoreboard(width int) string {
	game := s.session.Game()
	inn := game.CurrentInnings()
	battingSide := inn.BattingSide

	var b strings.Builder

	b.WriteString(styles.Bold.Render(fmt.Sprintf(
		"Leg %d of %d  ·  innings %d", inn.Leg(), cricket.LegsPerGame, inn.Number)))
	if target, ok := game.Target(); ok {
		needed, _ := game.RunsNeeded()
		b.WriteString(styles.Faint.Render("  ·  "))
		b.WriteString(styles.WarningText.Render(fmt.Sprintf(
			"chasing %d on aggregate — needs %d more to win", target, needed)))
	}
	b.WriteString("\n\n")

	for _, side := range []cricket.Side{cricket.TeamA, cricket.TeamB} {
		team := game.Team(side)
		batting := side == battingSide

		role := "BOWLING"
		if batting {
			role = "BATTING"
		}
		roleLabel := styles.Faint.Render(role)
		if batting {
			roleLabel = styles.SuccessText.Render(role)
		}

		var names []string
		for _, id := range team.Players {
			p := s.session.Player(id)
			names = append(names, styles.PlayerName(p.Colour, false).Render(p.Name))
		}

		// The aggregate is what decides the match, so it sits beside each team.
		line := fmt.Sprintf("%-8s %-9s %4d agg  %s",
			team.Name, roleLabel, game.Aggregate(side), strings.Join(names, ", "))
		b.WriteString(components.Truncate(line, maxInt(width-2, 20)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Bold.Render(fmt.Sprintf("This innings — %s: %s for %s",
		game.Team(battingSide).Name,
		plural(inn.Runs, "run"), plural(inn.Wickets, "wicket"))))
	b.WriteString(styles.Faint.Render(fmt.Sprintf("  (%d ends the innings)", game.Config().WicketLimit)))
	b.WriteString("\n")
	b.WriteString(s.inningsHistory())
	return b.String()
}

// inningsHistory lists the completed innings so the aggregate is explainable at
// a glance rather than being a number the operator has to trust.
func (s *CricketGame) inningsHistory() string {
	game := s.session.Game()
	innings := game.Innings()
	if len(innings) < 2 {
		return styles.Faint.Render("Each team bats twice; the combined total wins.")
	}

	var parts []string
	for _, inn := range innings[:len(innings)-1] {
		parts = append(parts, fmt.Sprintf("%s %d/%d",
			game.Team(inn.BattingSide).Name, inn.Runs, inn.Wickets))
	}
	return styles.Faint.Render("Completed: " + strings.Join(parts, "  ·  "))
}

// plural renders a count with its noun, pluralised.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (s *CricketGame) entryPanel(width int) string {
	turn, ok := s.session.Game().CurrentTurn()
	if !ok {
		return ""
	}
	p := s.session.Player(turn.PlayerID)
	st := s.session.Game().StatsFor(turn.PlayerID)

	var b strings.Builder

	// The banner spells out which kind of entry is wanted, in words, at the top.
	if turn.Kind == cricket.Batting {
		b.WriteString(styles.SuccessText.Render("▶ BATTING"))
	} else {
		b.WriteString(styles.WarningText.Render("▶ BOWLING"))
	}
	b.WriteString("  ")
	b.WriteString(styles.PlayerName(p.Colour, true).Render(p.Name))
	b.WriteString(styles.Faint.Render(fmt.Sprintf("  (%s)", s.session.Game().Team(turn.Side).Name)))
	b.WriteString("\n")

	// This player's figures in this game, never their lifetime record.
	if turn.Kind == cricket.Batting {
		b.WriteString(styles.Faint.Render(fmt.Sprintf(
			"this game: %d batting visits, %d runs from %d raw points",
			st.BattingVisits, st.Runs, st.RawPoints)))
	} else {
		b.WriteString(styles.Faint.Render(fmt.Sprintf(
			"this game: %d bowling visits, %d wickets (%d outer, %d inner)",
			st.BowlingVisits, st.Wickets, st.OuterBulls, st.InnerBulls)))
	}
	b.WriteString("\n\n")

	if turn.Kind == cricket.Batting {
		b.WriteString(s.battingEntryFields())
	} else {
		b.WriteString(s.bowlingEntryFields())
	}

	if s.entryErr != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorText.Render(s.entryErr))
	} else if s.lastVisit != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.Faint.Render("Last: " + s.lastVisit))
	}

	panel := styles.FocusPanel
	if width > 4 {
		panel = panel.Width(minInt(width-2, 78))
	}
	return panel.Render(b.String())
}

func (s *CricketGame) battingEntryFields() string {
	var b strings.Builder

	entry := s.battingEntry
	preview := ""
	if entry == "" {
		entry = styles.Faint.Render("___")
	} else {
		entry = styles.Bold.Render(entry)
		if raw, err := strconv.Atoi(s.battingEntry); err == nil {
			runs := cricket.RunsFor(raw)
			switch runs {
			case 0:
				preview = styles.Faint.Render(fmt.Sprintf(
					"  → 0 runs (%d or less scores nothing)", cricket.RunsThreshold))
			case 1:
				preview = styles.SuccessText.Render("  → 1 run")
			default:
				preview = styles.SuccessText.Render(fmt.Sprintf("  → %d runs", runs))
			}
		}
	}

	b.WriteString("Raw score  " + entry + preview)
	if s.battingFrom != "" {
		b.WriteString(styles.Faint.Render("   = " + s.battingFrom))
	}
	b.WriteString("\n")
	b.WriteString("Darts      ")
	for d := 1; d <= darts.MaxDartsPerVisit; d++ {
		if d == s.battingDarts {
			b.WriteString(styles.SelectedRow.Render("[" + strconv.Itoa(d) + "]"))
		} else {
			b.WriteString(styles.Faint.Render(" " + strconv.Itoa(d) + " "))
		}
	}
	b.WriteString(styles.Faint.Render("  (tab to change)"))
	b.WriteString("\n\n")
	b.WriteString(components.CalculatorHint())
	return b.String()
}

func (s *CricketGame) bowlingEntryFields() string {
	var b strings.Builder

	b.WriteString(styles.Faint.Render("Aiming at the bull. Type one letter per dart, up to three."))
	b.WriteString("\n\n")

	entry := s.bowlingEntry
	if entry == "" {
		b.WriteString("Darts  " + styles.Faint.Render("___"))
	} else {
		b.WriteString("Darts  " + styles.Bold.Render(entry))
		if bowls, err := cricket.ParseBowls(entry); err == nil {
			wickets := 0
			var described []string
			for _, bowl := range bowls {
				wickets += bowl.Wickets()
				described = append(described, bowl.Label())
			}
			b.WriteString(styles.Faint.Render("  (" + strings.Join(described, ", ") + ")"))
			b.WriteString("\n       ")
			if wickets == 1 {
				b.WriteString(styles.SuccessText.Render("→ 1 wicket"))
			} else {
				b.WriteString(styles.SuccessText.Render(fmt.Sprintf("→ %d wickets", wickets)))
			}
		}
	}
	b.WriteString("\n\n")
	b.WriteString(styles.Faint.Render(
		"1 = no wicket    2 = outer bull (1 wkt)    3 = inner bull (2 wkts)"))
	return b.String()
}

func (s *CricketGame) completePanel() string {
	var b strings.Builder
	b.WriteString(styles.Title.Render("Game over"))
	b.WriteString("\n\n")
	b.WriteString(styles.Bold.Render(s.resultSentence()))
	b.WriteString("\n\n")

	game := s.session.Game()
	for _, inn := range game.Innings() {
		b.WriteString(fmt.Sprintf("Leg %d, innings %d — %s: %d for %d\n",
			inn.Leg(), inn.Number, game.Team(inn.BattingSide).Name, inn.Runs, inn.Wickets))
	}

	// If the match ended in three innings, say why the fourth was not played —
	// otherwise it looks like a game that was cut short.
	if len(game.Innings()) < cricket.InningsPerGame && game.Result() != cricket.ResultAbandoned {
		if leader, _, ok := game.Lead(); ok {
			b.WriteString(styles.Faint.Render(fmt.Sprintf(
				"%s did not need to bat again.\n", game.Team(leader).Name)))
		}
	}

	b.WriteString("\n")
	for _, side := range []cricket.Side{cricket.TeamA, cricket.TeamB} {
		b.WriteString(styles.Bold.Render(fmt.Sprintf("%s aggregate: %d\n",
			game.Team(side).Name, game.Aggregate(side))))
	}

	b.WriteString("\n")
	b.WriteString(styles.Faint.Render("Press enter to return to the menu, or u to undo the last visit."))
	return styles.Panel.Render(b.String())
}

// resultSentence describes the outcome in plain words.
func (s *CricketGame) resultSentence() string {
	game := s.session.Game()
	switch game.Result() {
	case cricket.ResultDraw:
		return fmt.Sprintf("Draw — both sides finished level on %d.", game.Aggregate(cricket.TeamA))
	case cricket.ResultAbandoned:
		return "Abandoned — no result recorded."
	case cricket.ResultTeamA, cricket.ResultTeamB:
		side, _ := game.WinningSide()
		_, by, _ := game.Lead()
		return fmt.Sprintf("%s win by %s on aggregate.", game.Team(side).Name, plural(by, "run"))
	default:
		return "In progress."
	}
}
