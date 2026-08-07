package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pourmoi/dart-tui/internal/tui/components"
	"github.com/pourmoi/dart-tui/internal/tui/styles"
)

// Menu is the home screen: everything is one keystroke away.
type Menu struct {
	ctx    Context
	cursor components.Cursor
	items  []menuItem

	playerCount   int
	resumableJobs int
	loadErr       error
}

type menuItem struct {
	key   string
	label string
	desc  string
	open  func(Context) tea.Cmd
}

// NewMenu builds the main menu.
func NewMenu(ctx Context) *Menu {
	m := &Menu{ctx: ctx}
	m.items = []menuItem{
		{
			key:   "s",
			label: "Start game",
			desc:  "501, 301 or Cricket",
			open:  func(c Context) tea.Cmd { return Push(NewGameModeSelect(c)) },
		},
		{
			key:   "r",
			label: "Resume game",
			desc:  "pick up an unfinished game",
			open:  func(c Context) tea.Cmd { return Push(NewResumeList(c)) },
		},
		{
			key:   "p",
			label: "Players",
			desc:  "add, edit and archive players",
			open:  func(c Context) tea.Cmd { return Push(NewPlayerList(c)) },
		},
		{
			key:   "t",
			label: "Statistics",
			desc:  "leaderboard and player records",
			open:  func(c Context) tea.Cmd { return Push(NewLeaderboard(c)) },
		},
		{
			key:   "q",
			label: "Exit",
			desc:  "quit the application",
			open:  func(Context) tea.Cmd { return Quit() },
		},
	}
	m.cursor.SetCount(len(m.items))
	m.reload()
	return m
}

// Init refreshes the counts shown beside the menu entries.
func (m *Menu) Init() tea.Cmd {
	m.reload()
	return nil
}

func (m *Menu) reload() {
	m.loadErr = nil
	players, err := m.ctx.Services.Players.List(m.ctx.Ctx, false)
	if err != nil {
		m.loadErr = err
		return
	}
	m.playerCount = len(players)

	resumable, err := m.ctx.Services.Games.Resumable(m.ctx.Ctx)
	if err != nil {
		m.loadErr = err
		return
	}
	m.resumableJobs = len(resumable)
}

// Title names the screen.
func (m *Menu) Title() string { return "Darts" }

// Hints lists the available keys.
func (m *Menu) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Keys: "↑/↓", Desc: "move"},
		{Keys: "enter", Desc: "select"},
		{Keys: "s/r/p/t", Desc: "jump straight to an entry"},
		{Keys: "q", Desc: "quit"},
	}
}

// Update handles menu navigation.
func (m *Menu) Update(msg tea.Msg) (Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		m.cursor.Prev()
		return m, nil
	case "down", "j":
		m.cursor.Next()
		return m, nil
	case "home", "g":
		m.cursor.First()
		return m, nil
	case "end", "G":
		m.cursor.Last()
		return m, nil
	case "enter", " ":
		return m, m.activate(m.cursor.Index())
	}

	// Direct shortcuts: the whole menu is reachable without arrow keys.
	for i, item := range m.items {
		if key.String() == item.key {
			m.cursor.Set(i)
			return m, m.activate(i)
		}
	}
	return m, nil
}

func (m *Menu) activate(index int) tea.Cmd {
	if index < 0 || index >= len(m.items) {
		return nil
	}
	m.reload()
	return m.items[index].open(m.ctx)
}

// View renders the menu.
func (m *Menu) View(width, height int) string {
	var b strings.Builder

	b.WriteString(styles.Subtitle.Render("Office darts scoring"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		selected := i == m.cursor.Index()
		line := fmt.Sprintf("%s%s  %s",
			styles.Marker(selected),
			styles.Key.Render("["+item.key+"]"),
			item.label)

		annotation := m.annotation(item)
		if annotation != "" {
			line += "  " + styles.Faint.Render(annotation)
		}
		if selected {
			line = styles.SelectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		b.WriteString(styles.Faint.Render("      " + item.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.loadErr != nil {
		b.WriteString(styles.ErrorText.Render("Could not read the database: " + m.loadErr.Error()))
		b.WriteString("\n")
	}
	b.WriteString(styles.Faint.Render("Data: " + components.Truncate(m.ctx.DBPath, maxInt(width-8, 20))))

	return b.String()
}

// annotation is the small note beside a menu entry, such as how many games can
// be resumed. It doubles as a nudge when there are no players yet.
func (m *Menu) annotation(item menuItem) string {
	switch item.key {
	case "r":
		switch m.resumableJobs {
		case 0:
			return "nothing to resume"
		case 1:
			return "1 game waiting"
		default:
			return fmt.Sprintf("%d games waiting", m.resumableJobs)
		}
	case "p":
		switch m.playerCount {
		case 0:
			return "none yet — add one to get started"
		case 1:
			return "1 active player"
		default:
			return fmt.Sprintf("%d active players", m.playerCount)
		}
	case "s":
		if m.playerCount == 0 {
			return "add a player first"
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
