// Package tui is the Bubble Tea root: it owns the screen stack, the header and
// footer, and the terminal size. Individual screens live in tui/screens.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/BigJabber/dart-tui/internal/app"
	"github.com/BigJabber/dart-tui/internal/tui/components"
	"github.com/BigJabber/dart-tui/internal/tui/screens"
	"github.com/BigJabber/dart-tui/internal/tui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// Minimum usable terminal size. Below this the application says so plainly
// rather than drawing a corrupted layout.
const (
	minWidth  = 52
	minHeight = 14
)

// Model is the root Bubble Tea model.
type Model struct {
	ctx   screens.Context
	stack []screens.Screen

	width  int
	height int

	status     string
	statusKind screens.StatusKind
}

// New builds the root model with the main menu on the stack.
func New(ctx context.Context, services *app.Services, dbPath string) Model {
	sctx := screens.Context{Services: services, Ctx: ctx, DBPath: dbPath}
	return Model{
		ctx:   sctx,
		stack: []screens.Screen{screens.NewMenu(sctx)},
		// Sensible defaults until the first WindowSizeMsg arrives.
		width:  80,
		height: 24,
	}
}

// numericKeypadMode is the DECKPNM escape sequence.
//
// A terminal left in application keypad mode sends escape sequences for the
// keypad instead of ordinary characters: Enter arrives as ESC O M rather than a
// carriage return, and the digits arrive as ESC O q and friends. Neither is
// recognised as a normal keystroke, which would make the keypad useless for
// scoring. Asking for numeric mode up front means the keypad sends plain digits
// and a plain Enter on every terminal.
const numericKeypadMode = "\x1b>"

// Run starts the Bubble Tea program.
func Run(ctx context.Context, services *app.Services, dbPath string) error {
	fmt.Fprint(os.Stdout, numericKeypadMode)

	p := tea.NewProgram(New(ctx, services, dbPath), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// normaliseKey folds the several ways a terminal can report "Enter" into one.
//
// The scoreboard is operated from the numeric keypad, and its Enter key does not
// always arrive as a carriage return: some terminals and multiplexers send a
// line feed instead, which would otherwise be reported as ctrl+j and ignored.
// Nothing in the application binds ctrl+j to anything else, so mapping it here
// costs nothing and makes every Enter key work everywhere.
func normaliseKey(k tea.KeyMsg) tea.KeyMsg {
	if k.Type == tea.KeyCtrlJ {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return k
}

// Init starts the first screen.
func (m Model) Init() tea.Cmd {
	if len(m.stack) == 0 {
		return nil
	}
	return m.current().Init()
}

func (m Model) current() screens.Screen {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// Update handles global keys, navigation messages and terminal resizes, and
// otherwise delegates to the visible screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Resizing is handled by simply re-rendering at the new size; every
		// screen computes its layout from the width and height it is given.
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C always works, everywhere, immediately. Games are saved as they
		// are played, so quitting can never lose a leg.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Normalise once, here, so every screen sees the same "enter" however
		// the terminal reported it.
		return m.dispatch(normaliseKey(msg))

	case screens.PushMsg:
		m.stack = append(m.stack, msg.Screen)
		m.status = ""
		return m, msg.Screen.Init()

	case screens.PopMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
			m.status = ""
			return m, m.current().Init()
		}
		return m, nil

	case screens.PopToRootMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:1]
		}
		return m, m.current().Init()

	case screens.ReplaceMsg:
		if len(m.stack) == 0 {
			m.stack = []screens.Screen{msg.Screen}
		} else {
			m.stack[len(m.stack)-1] = msg.Screen
		}
		m.status = ""
		return m, msg.Screen.Init()

	case screens.StatusMsg:
		m.status, m.statusKind = msg.Text, msg.Kind
		return m, nil

	case screens.QuitMsg:
		return m, tea.Quit
	}

	return m.dispatch(msg)
}

// dispatch hands a message to the visible screen.
func (m Model) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	screen := m.current()
	if screen == nil {
		return m, tea.Quit
	}

	// A keystroke aimed at a screen clears the previous status line, so an old
	// message never lingers over a new action.
	if _, isKey := msg.(tea.KeyMsg); isKey {
		m.status = ""
	}

	next, cmd := screen.Update(msg)
	if next != nil {
		m.stack[len(m.stack)-1] = next
	}
	return m, cmd
}

// View renders the header, the current screen and the contextual footer.
func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmallView()
	}

	screen := m.current()
	if screen == nil {
		return ""
	}

	// The header and the footer are measured rather than assumed a fixed size,
	// because both of them grow: the header gains a row when there is a status
	// message, and the footer gains one whenever the key hints wrap. A screen
	// that lays itself out from this number needs it to be true, and a view
	// that comes out too tall loses its top — the header and the scoreboard —
	// rather than its bottom.
	header := m.headerView(screen.Title())
	footer := m.footerView(screen)

	// The two newlines written either side of the body are rows as well.
	bodyHeight := m.height - components.LineCount(header) - components.LineCount(footer) - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(screen.View(m.width, bodyHeight))
	b.WriteString("\n")
	b.WriteString(footer)
	return b.String()
}

func (m Model) headerView(title string) string {
	line := styles.Header.Render("🎯 " + title)
	if len(m.stack) > 1 {
		line += styles.Faint.Render("   esc to go back")
	}

	// The status row is always there, blank when there is nothing to say. It
	// used to appear and disappear, which shifted the whole screen down a row
	// after every visit and could cost the large panel a size at the same time.
	// On a wall-mounted display that flicker is far more distracting than one
	// unused row.
	return line + "\n" + components.Bar(m.width) + "\n" + m.statusView()
}

// statusView renders the status line. Each kind is prefixed with a word so the
// severity survives on a monochrome terminal.
func (m Model) statusView() string {
	if m.status == "" {
		return ""
	}
	text := components.Truncate(m.status, maxInt(m.width-12, 20))
	switch m.statusKind {
	case screens.StatusError:
		return styles.ErrorText.Render("Error: " + text)
	case screens.StatusWarning:
		return styles.WarningText.Render("Note: " + text)
	case screens.StatusSuccess:
		return styles.SuccessText.Render("✓ " + text)
	default:
		return styles.Subtitle.Render(text)
	}
}

func (m Model) footerView(screen screens.Screen) string {
	hints := screen.Hints()
	// Ctrl+C is always available, so it is always advertised.
	hints = append(hints, components.KeyHint{Keys: "ctrl+c", Desc: "quit"})

	return components.Bar(m.width) + "\n" +
		styles.Footer.Render(components.RenderHints(hints, m.width-2))
}

func (m Model) tooSmallView() string {
	return styles.WarningText.Render(fmt.Sprintf(
		"Terminal too small.\n\nPlease resize to at least %d×%d.\nCurrently %d×%d.\n\nctrl+c to quit.",
		minWidth, minHeight, m.width, m.height))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
