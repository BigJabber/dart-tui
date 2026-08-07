package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BigJabber/dart-tui/internal/domain/cricket"
	"github.com/BigJabber/dart-tui/internal/domain/player"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// cricketStage is one step of the setup wizard.
type cricketStage int

const (
	stageTeamA cricketStage = iota
	stageTeamB
	stageFirstBat
	stageWickets
)

// defaultWickets is the office's usual innings length. It is only a default:
// the configured value is stored with the game.
const defaultWickets = 5

// CricketSetup walks through building two teams and configuring the innings.
//
// Player order matters in cricket — unlike X01 — so the order players are
// picked in becomes their batting and bowling order.
type CricketSetup struct {
	ctx    Context
	stage  cricketStage
	cursor components.Cursor

	players []player.Player
	loadErr error

	teamA []int64
	teamB []int64

	firstBatting cricket.Side
	wicketEntry  string
	entryErr     string
}

// NewCricketSetup builds the cricket setup wizard.
func NewCricketSetup(ctx Context) *CricketSetup {
	s := &CricketSetup{
		ctx:          ctx,
		firstBatting: cricket.TeamA,
		wicketEntry:  strconv.Itoa(defaultWickets),
	}
	s.reload()
	return s
}

// Init refreshes the player list.
func (s *CricketSetup) Init() tea.Cmd {
	s.reload()
	return nil
}

func (s *CricketSetup) reload() {
	players, err := s.ctx.Services.Players.List(s.ctx.Ctx, false)
	s.loadErr = err
	s.players = players
	s.cursor.SetCount(len(players))
}

// Title names the current step.
func (s *CricketSetup) Title() string {
	switch s.stage {
	case stageTeamA:
		return "Cricket — Team A"
	case stageTeamB:
		return "Cricket — Team B"
	case stageFirstBat:
		return "Cricket — who bats first?"
	default:
		return "Cricket — wickets per innings"
	}
}

// Hints lists the keys for the current step.
func (s *CricketSetup) Hints() []components.KeyHint {
	switch s.stage {
	case stageTeamA, stageTeamB:
		return []components.KeyHint{
			{Keys: "↑/↓", Desc: "move"},
			{Keys: "space", Desc: "add/remove (order = pick order)"},
			{Keys: "enter", Desc: "next"},
			{Keys: "esc", Desc: "back"},
		}
	case stageFirstBat:
		return []components.KeyHint{
			{Keys: "←/→", Desc: "choose team"},
			{Keys: "enter", Desc: "next"},
			{Keys: "esc", Desc: "back"},
		}
	default:
		return []components.KeyHint{
			{Keys: "0-9", Desc: "wickets"},
			{Keys: "enter", Desc: "start game"},
			{Keys: "esc", Desc: "back"},
		}
	}
}

// Update routes keys to the current step.
func (s *CricketSetup) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch s.stage {
	case stageTeamA, stageTeamB:
		return s.updateTeam(key)
	case stageFirstBat:
		return s.updateFirstBat(key)
	default:
		return s.updateWickets(key)
	}
}

func (s *CricketSetup) updateTeam(key tea.KeyMsg) (Screen, tea.Cmd) {
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
		return s, s.toggle()
	case "enter":
		return s, s.advanceFromTeam()
	case "esc":
		if s.stage == stageTeamB {
			s.stage = stageTeamA
			s.cursor.First()
			return s, nil
		}
		return s, Pop()
	}
	return s, nil
}

func (s *CricketSetup) currentTeam() *[]int64 {
	if s.stage == stageTeamB {
		return &s.teamB
	}
	return &s.teamA
}

func (s *CricketSetup) toggle() tea.Cmd {
	i := s.cursor.Index()
	if i < 0 || i >= len(s.players) {
		return nil
	}
	id := s.players[i].ID

	team := s.currentTeam()
	for j, sel := range *team {
		if sel == id {
			*team = append((*team)[:j], (*team)[j+1:]...)
			return nil
		}
	}
	// A player cannot turn out for both sides.
	if s.stage == stageTeamB && contains(s.teamA, id) {
		return Warn(s.players[i].Name + " is already in Team A.")
	}
	if s.stage == stageTeamA && contains(s.teamB, id) {
		return Warn(s.players[i].Name + " is already in Team B.")
	}
	*team = append(*team, id)
	return nil
}

func (s *CricketSetup) advanceFromTeam() tea.Cmd {
	team := *s.currentTeam()
	if len(team) == 0 {
		return Warn("Pick at least one player for this team.")
	}
	if s.stage == stageTeamA {
		s.stage = stageTeamB
		s.cursor.First()
		return nil
	}
	s.stage = stageFirstBat
	return nil
}

func (s *CricketSetup) updateFirstBat(key tea.KeyMsg) (Screen, tea.Cmd) {
	switch key.String() {
	case "left", "h", "up", "k", "right", "l", "down", "j", "tab":
		s.firstBatting = s.firstBatting.Other()
	case "a", "A":
		s.firstBatting = cricket.TeamA
	case "b", "B":
		s.firstBatting = cricket.TeamB
	case "enter":
		s.stage = stageWickets
	case "esc":
		s.stage = stageTeamB
		s.cursor.First()
	}
	return s, nil
}

func (s *CricketSetup) updateWickets(key tea.KeyMsg) (Screen, tea.Cmd) {
	str := key.String()
	if len(str) == 1 && str[0] >= '0' && str[0] <= '9' {
		if len(s.wicketEntry) < 2 {
			s.wicketEntry += str
			s.entryErr = ""
		}
		return s, nil
	}
	switch str {
	case "backspace":
		if len(s.wicketEntry) > 0 {
			s.wicketEntry = s.wicketEntry[:len(s.wicketEntry)-1]
		}
		s.entryErr = ""
	case "esc":
		s.stage = stageFirstBat
	case "enter":
		return s, s.start()
	}
	return s, nil
}

func (s *CricketSetup) start() tea.Cmd {
	wickets, err := strconv.Atoi(s.wicketEntry)
	if err != nil {
		s.entryErr = "Enter how many wickets end an innings."
		return nil
	}
	if wickets < cricket.MinWickets || wickets > cricket.MaxWickets {
		s.entryErr = fmt.Sprintf("Wickets must be between %d and %d.",
			cricket.MinWickets, cricket.MaxWickets)
		return nil
	}

	cfg := cricket.Config{WicketLimit: wickets, FirstBatting: s.firstBatting}
	session, err := s.ctx.Services.Games.StartCricket(s.ctx.Ctx, cfg,
		cricket.Team{Side: cricket.TeamA, Name: "Team A", Players: s.teamA},
		cricket.Team{Side: cricket.TeamB, Name: "Team B", Players: s.teamB})
	if err != nil {
		s.entryErr = capitalise(err.Error())
		return nil
	}
	return Replace(NewCricketGame(s.ctx, session))
}

// View renders the current step.
func (s *CricketSetup) View(width, height int) string {
	if s.loadErr != nil {
		return styles.ErrorText.Render("Could not load players: " + s.loadErr.Error())
	}
	if len(s.players) == 0 {
		return styles.Faint.Render("No active players. Add some from the Players screen first.")
	}

	switch s.stage {
	case stageTeamA, stageTeamB:
		return s.teamView(width, height)
	case stageFirstBat:
		return s.firstBatView()
	default:
		return s.wicketsView()
	}
}

func (s *CricketSetup) teamView(width, height int) string {
	var b strings.Builder

	teamName := "Team A"
	if s.stage == stageTeamB {
		teamName = "Team B"
	}
	b.WriteString(styles.Subtitle.Render(fmt.Sprintf(
		"Pick the players for %s. The order you pick them in is their throwing order.", teamName)))
	b.WriteString("\n\n")

	team := *s.currentTeam()
	start, end := s.cursor.Window(maxInt(height-9, 3))
	for i := start; i < end && i < len(s.players); i++ {
		p := s.players[i]
		cursorHere := i == s.cursor.Index()

		position := "   "
		picked := false
		for j, id := range team {
			if id == p.ID {
				position = fmt.Sprintf("%2d.", j+1)
				picked = true
				break
			}
		}

		// A player already on the other side is shown as unavailable in words.
		otherTeam := ""
		if s.stage == stageTeamB && contains(s.teamA, p.ID) {
			otherTeam = "  " + styles.WarningText.Render("(Team A)")
		}
		if s.stage == stageTeamA && contains(s.teamB, p.ID) {
			otherTeam = "  " + styles.WarningText.Render("(Team B)")
		}

		name := styles.PlayerName(p.Colour, picked).Render(
			components.Truncate(p.Name, maxInt(width-24, 10)))
		line := fmt.Sprintf("%s%s %s %s%s",
			styles.Marker(cursorHere), styles.Checkbox(picked), position, name, otherTeam)
		if cursorHere {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(s.teamSummary())
	return b.String()
}

func (s *CricketSetup) teamSummary() string {
	var b strings.Builder
	b.WriteString(styles.Bold.Render("Team A: "))
	b.WriteString(s.namesOf(s.teamA))
	b.WriteString("\n")
	b.WriteString(styles.Bold.Render("Team B: "))
	b.WriteString(s.namesOf(s.teamB))
	return b.String()
}

func (s *CricketSetup) namesOf(ids []int64) string {
	if len(ids) == 0 {
		return styles.Faint.Render("nobody yet")
	}
	var parts []string
	for _, id := range ids {
		for _, p := range s.players {
			if p.ID == id {
				parts = append(parts, styles.PlayerName(p.Colour, false).Render(p.Name))
				break
			}
		}
	}
	return strings.Join(parts, styles.Faint.Render(" → "))
}

func (s *CricketSetup) firstBatView() string {
	var b strings.Builder
	b.WriteString(styles.Subtitle.Render("Which team bats first? The batter always throws before the bowler."))
	b.WriteString("\n\n")

	for _, side := range []cricket.Side{cricket.TeamA, cricket.TeamB} {
		selected := side == s.firstBatting
		ids := s.teamA
		label := "Team A"
		if side == cricket.TeamB {
			ids = s.teamB
			label = "Team B"
		}
		line := fmt.Sprintf("%s%s  %s", styles.Marker(selected),
			styles.Bold.Render(label), s.namesOf(ids))
		if selected {
			line = styles.SelectedRow.Render(line) + "  " + styles.SuccessText.Render("bats first")
		}
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	b.WriteString(s.teamSummary())
	return b.String()
}

func (s *CricketSetup) wicketsView() string {
	var b strings.Builder
	b.WriteString(styles.Subtitle.Render(
		"How many wickets end an innings? Stored with the game."))
	b.WriteString("\n\n")

	entry := s.wicketEntry
	if entry == "" {
		entry = styles.Faint.Render("__")
	} else {
		entry = styles.Bold.Render(entry)
	}
	b.WriteString("Wickets  " + entry)
	b.WriteString("\n\n")

	if s.entryErr != "" {
		b.WriteString(styles.ErrorText.Render(s.entryErr))
	} else {
		b.WriteString(styles.Faint.Render(fmt.Sprintf(
			"Outer bull takes one wicket, inner bull takes two. Between %d and %d.",
			cricket.MinWickets, cricket.MaxWickets)))
	}
	b.WriteString("\n\n")

	first := "Team A"
	if s.firstBatting == cricket.TeamB {
		first = "Team B"
	}
	b.WriteString(styles.Faint.Render(first + " bats first."))
	b.WriteString("\n")
	b.WriteString(s.teamSummary())
	return b.String()
}

func contains(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
