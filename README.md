# Darts

A keyboard-driven darts scoreboard that runs in a terminal window. It is built
for scoring real games while standing next to a dartboard: everything is one or
two keystrokes, mistakes are easy to take back, and every game is saved as it is
played so closing the window by accident costs nothing.

It is a single `.exe`. There is no installer, no server, no browser and no
database to set up.

- **501 / 301** — any number of players, thrown in **any order**, with recommended checkouts
- **Cricket** — the office's own variant over two legs, **not** standard darts cricket
- Persistent player statistics, derived from every visit ever recorded

---

## Contents

- [Installation](#installation)
- [Using it on Windows](#using-it-on-windows)
- [Keyboard controls](#keyboard-controls)
- [Screens](#screens)
- [501 and 301](#501-and-301)
- [Cricket — the office rules](#cricket--the-office-rules)
- [Statistics](#statistics)
- [Where your data is stored](#where-your-data-is-stored)
- [Building from source](#building-from-source)
- [Development](#development)
- [Design decisions and assumptions](#design-decisions-and-assumptions)

---

## Installation

Download `darts-windows-amd64.exe` from the
[releases page](../../releases), put it wherever you like, and run it.

Nothing else is required. Specifically, it does **not** need:

- Go installed
- SQLite installed
- Visual Studio build tools or any C runtime
- A web server or network connection
- Administrator rights

The executable imports exactly one system library (`kernel32.dll`), because the
SQLite engine is compiled into the program itself as pure Go.

| Platform | File |
| --- | --- |
| Windows, most PCs | `darts-windows-amd64.exe` |
| Windows on ARM | `darts-windows-arm64.exe` |
| Linux | `darts-linux-amd64` |

## Using it on Windows

Use **Windows Terminal** if you have it — the colours and box drawing look best
there. The application also works in the older `conhost` console window, though
colours will be approximated.

You can start it three ways:

- Double-click `darts.exe`
- From Windows Terminal: `.\darts.exe`
- Pin it to the taskbar for the office darts machine

The window should be at least **52 × 14** characters. Below that the application
says so plainly rather than drawing a broken layout. It redraws correctly when
you resize the window at any time.

Command-line flags, all optional:

```
darts.exe -version     print the version
darts.exe -where       print the database location and exit
darts.exe -portable    keep the database beside the .exe (for a USB stick)
darts.exe -db PATH     use a specific database file
```

## Keyboard controls

The application is designed to be used without a mouse. Every screen shows its
own available keys along the bottom, so this table is a reference rather than
something to memorise.

### Everywhere

| Key | Action |
| --- | --- |
| `↑` `↓` or `k` `j` | Move the selection |
| `Enter` | Confirm / choose |
| `Esc` | Go back or cancel |
| `Ctrl+C` | Quit immediately (always safe — games are already saved) |

### Main menu

| Key | Action |
| --- | --- |
| `s` | Start a game |
| `r` | Resume an unfinished game |
| `p` | Players |
| `t` | Statistics and leaderboard |
| `q` | Exit |

### Playing 501 / 301

| Key | Action |
| --- | --- |
| `0`–`9` | Start typing the score for the highlighted player |
| `↑` `↓` | Choose which player threw |
| `=` or `+` | Add the darts up (see below) |
| — | Checkout suggestions appear on their own; there is no key to press |
| `Tab` | Change the darts thrown (3 → 2 → 1) |
| `Enter` | Record the visit |
| `y` / `n` | Answer the "finished on a double?" question |
| `u` | **Undo the last visit** |
| `f` | Finish the game and confirm the winner (a solo game just saves) |
| `Esc` | Leave (the game is saved and can be resumed) |

### Playing cricket

| Key | Action |
| --- | --- |
| `0`–`9` | Batting: the raw score thrown |
| `=` or `+` | Batting: add the darts up |
| `Tab` | Change the darts thrown |
| `1` `2` `3` | Bowling: no wicket / outer bull / inner bull |
| `Enter` | Record the visit |
| `u` | Undo the last visit |
| `f` | Abandon the game with no result |

### Players

| Key | Action |
| --- | --- |
| `n` | Add a player |
| `a` | Show or hide archived players |
| `Enter` | Open a player's profile |

On a profile: `e` edit name, `c` change colour, `a` archive or restore.

## Screens

> Screenshots to be added. Drop PNGs into `docs/` and link them here.

<!--
![Main menu](docs/menu.png)
![Playing 501](docs/x01.png)
![Cricket](docs/cricket.png)
![Player profile](docs/profile.png)
-->

**Main menu** — start, resume, players, statistics, exit. Shows how many games
are waiting to be resumed and where your data lives.

Both game screens have three parts: the table, the **large panel**, and the entry
box. The table and the entry box are for whoever is holding the keyboard; the
large panel is for everyone else in the room, and is sized to be read from the
oche rather than from the desk. It shrinks in stages as the window gets smaller
and disappears altogether on a short terminal — nothing in it is information you
cannot also find in the table.

From about 182 columns — which a fullscreen window on a 1080p display comfortably
exceeds — the panel moves into a **column of its own** on the right, with the
table and the entry box stacked on the left. That is where the panel is at its
biggest: it no longer shares its rows with the entry box, so the number can be
about half again as tall. Narrower than that and the three stack as below.

```
  Player    Score  Status       │  ╭────────────────────────────╮
▶ Alice       361  ← throwing   │  │        NOW THROWING        │
  Bob         501  yet to throw │  │                            │
                                │  │      █▀▀ █▀▀▄ ▀█▀          │
╭──────────────────────────────╮│  │      ▀▀█ █▀▀▄  █           │
│ Ready for Alice              ││  │                            │
│ Scored  ___                  ││  │   ███   ███  ███           │
╰──────────────────────────────╯│  │  █   █ █       █    LEFT   │
                                │  │   ███   ███  ███           │
                                │  ╰────────────────────────────╯
```

**501 in play** — a table of every player with their score, darts, visits,
current-game average and status, then the thrower and their remaining score,
then the entry box for the visit being recorded.

```
  Player      Score  Darts Visits     Avg  Status
▶ Alice         361      3      1   140.0  ← throwing
  Bob           501      0      0       —  yet to throw
  Carol           0     28     10    53.7  OUT · 28 darts

╭──────────────────────────────────────────────────────╮
│ NOW THROWING                                         │
│ ▄▀▀▀▄ █      ▀█▀  ▄▀▀▀▄ █▀▀▀▀                        │
│ █▄▄▄█ █       █   █     █▄▄▄                         │
│ █   █ █       █   █   ▄ █                            │
│ ▀   ▀ ▀▀▀▀▀  ▀▀▀   ▀▀▀  ▀▀▀▀▀                        │
│                                                      │
│ ▄▀▀▀▄  ▄▀▀   ▄█                                      │
│   ▄▄▀ █▄▄▄    █                                      │
│ ▄   █ █   █   █     LEFT                             │
│  ▀▀▀   ▀▀▀   ▀▀▀                                     │
│                                                      │
│ CHECKOUT  T20  T7  D60                               │
╰──────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────╮
│ Alice  ·  361 left  ·  this game: 140.00 avg over 3 darts
│
│ Scored  100
│ Darts    1  2 [3]   (tab to change — 3 is the default)
╰──────────────────────────────────────────────────────╯
```

**Cricket in play** — the leg and innings, both teams with their roles and running
aggregates, the wickets standing, and the target once the chase begins. A banner
spells out whether a **batting** or **bowling** entry is wanted.

The large panel shows the innings total for the first three innings and switches
to the runs needed for the fourth, which is the only one that is a chase. That
number belongs to the innings rather than to whoever is throwing, so it does not
change when the sides swap over — which is the right answer for a bowler, whose
job is defined by the total being defended. The supporting line always names the
side the number belongs to.

```
Leg 2 of 2  ·  innings 4  ·  chasing 120 on aggregate — needs 61 more to win

Team A   BOWLING    120 agg  Alice, Bob
Team B   BATTING     60 agg  Charlie, David, Eve

This innings — Team B: 0 runs for 0 wickets  (2 ends the innings)
Completed: Team A 20/2  ·  Team B 60/2  ·  Team A 100/2

╭──────────────────────────────────────────────────────╮
│ ▶ BOWLING                                            │
│ ▄▀▀▀▄ █      ▀█▀  ▄▀▀▀▄ █▀▀▀▀                        │
│ █▄▄▄█ █       █   █     █▄▄▄                         │
│ █   █ █       █   █   ▄ █                            │
│ ▀   ▀ ▀▀▀▀▀  ▀▀▀   ▀▀▀  ▀▀▀▀▀                        │
│                                                      │
│  ▄▀▀   ▄█                                            │
│ █▄▄▄    █                                            │
│ █   █   █    TO WIN                                  │
│  ▀▀▀   ▀▀▀                                           │
│                                                      │
│ TEAM B NEED 61  ·  TARGET 120  ·  0 WKTS DOWN        │
╰──────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────╮
│ ▶ BOWLING  Alice  (Team A)
│ this game: 2 bowling visits, 3 wickets (1 outer, 1 inner)
│
│ Darts  23   (outer bull, inner bull)
│        → 3 wickets
│
│ 1 = no wicket    2 = outer bull (1 wkt)    3 = inner bull (2 wkts)
╰──────────────────────────────────────────────────────╯
```

**Player profile** — lifetime summary, then 501, 301, cricket batting and cricket
bowling figures kept separately, then recent games.

## 501 and 301

Both use the same engine and differ only in the starting score.

### Players throw in any order

This is the most important thing about how this application works, and it is
deliberate. **The game never advances to "the next player" by itself.**

The office does not play strict alternating turns. One player may play their
entire leg from 501 down to a checkout before anyone else has thrown a single
dart, and players may swap back and forth in any order at all. So after every
visit you simply choose who threw next with `↑`/`↓`, or leave the highlight where
it is and keep typing scores for the same player.

Selection order is recorded so the table has a stable order, but it never
restricts who may throw.

### Recording a visit

1. Highlight whoever threw.
2. Type their total for that visit.
3. Press `Enter`.

Darts thrown defaults to **3**, because it nearly always is. Press `Tab` to
change it to 2 or 1.

### Adding the darts up

You usually know what each dart scored — 60, 30, 21 — but adding them up in your
head while someone waits at the board is exactly where scoring goes wrong. Press
`=` (or `+`, which is on the numeric keypad) and type the sum.

The same calculator is on the cricket batting screen, and both screens show a
one-line reminder that it is there:

```
Add up the darts  for Alice  ·  501 left

Sum    60 + 30 + 21
Total  111   (60 + 30 + 21)

Enter puts the total in the score box; press enter again to record it.
```

- `0`–`9` enter a number, `+` starts the next one, `Backspace` corrects.
- A space works as a separator too, so `60 30 21` is the same as `60+30+21`.
- The running total updates as you type.
- `Enter` puts the total in the score box — it does **not** record the visit.
  The score box then shows `111   = 60 + 30 + 21` so you can check the
  arithmetic, and a second `Enter` records it as normal.
- `Esc` cancels without changing anything.

Terms do not have to be single darts. `120+50+21` is a perfectly good way to
enter a visit where you have already added two darts together yourself.

If the total could not actually be thrown, the error appears **in the
calculator** so you can fix the sum where you made it, rather than after it has
been dismissed.

Pressing `+` part-way through typing a score turns what you have already entered
into the first term, so `140` then `+` continues naturally into a sum.

The calculator does not change the dart count. Three darts stays the default and
`Tab` still sets it, because the number of terms in a sum does not reliably tell
you how many darts were thrown — `120+50+21` is three terms but could be four
darts, and a dart that missed scores nothing and may not be typed at all.

**Record the dart count accurately, especially on a checkout.** It is what
separates a good leg from a great one:

| | Visits | Final visit | Total darts |
| --- | --- | --- | --- |
| Player A | 9 full + checkout | 3 darts | **30 darts** |
| Player B | 9 full + checkout | 1 dart | **28 darts** |

Both scored 501 in ten visits, but Player B's leg is clearly stronger. The
application stores the exact dart count, ranks B ahead of A, and suggests B as
the winner.

### Recommended checkouts

When the player at the board can finish, the suggested routes appear under their
name:

```
Ready for James — type their score, or ↑/↓ to pick someone else.

Checkout   T20  T20  Bull
```

Where several finishes are genuinely used, up to three are offered, preferred
first:

```
Checkout   1. T20  D10
           2. T16  D16
           3. D20  D20
```

Nothing appears unless the score can really be finished with the three darts of
the visit. **The panel is hidden rather than showing something misleading** — so
you will see nothing at 501, at 171 or above, at 1, or on the seven bogey
numbers that cannot be taken out in three darts:

```
169  168  166  165  163  162  159
```

Setup shots are not suggested. If a score cannot be checked out, the application
says nothing rather than pretending otherwise.

#### How the routes are worked out

Two layers, deliberately separate:

- A **solver** enumerates real dartboard segments to find every legal way to
  finish: the darts must total the score exactly, the last must be a double or
  the bull, and nothing may take the score below zero. It is the authority on
  what is possible, and it works for any score rather than depending on a table
  being complete. It derives the bogey numbers on its own — nobody typed that
  list into the code.
- A **curated chart** of the conventional routes, following the published
  checkout charts, decides which of the legal finishes is worth suggesting. There
  are many ways to take 100; only `T20 D20` is worth showing.

The chart is data in source, never a download — the application makes no network
requests and works entirely offline. It is also **not trusted**: every curated
route is checked by the solver, so a typo in the table fails a test rather than
reaching a player at the board.

Curated routes may deliberately use more darts than strictly necessary. 101 goes
in two as `T17 Bull`, but the chart prefers `T17 10 D20` because leaving D20
beats throwing at the bull. If only two darts are left, that route no longer
fits and the solver's answer is used instead.

The suggestion assumes all three darts of the visit are available, because scores
are recorded a visit at a time rather than dart by dart. The underlying API takes
the number of darts as a parameter, so a future dart-by-dart mode can ask for
fresh advice after every dart without any change to the engine.

### Scoring rules

- Scores are subtracted from the remaining total.
- **Double out** is the default. Leaving exactly 1 is a bust.
- A **bust** scores nothing, restores the score to what it was at the start of
  the visit, and **still counts the darts thrown** — because they were.
- Impossible entries are refused: a negative score, more than 180, or a total
  that cannot physically be thrown with that many darts (179 with three darts,
  61 with one).

### The double-out question

Because the application records the **total for a visit** rather than each
individual dart, it cannot tell whether the last dart was a double. So whenever
an entry would land exactly on zero it asks:

> **Did they finish on a double?**

- `y` — records the checkout and the exact dart count.
- `n` — records a bust, which is what an illegal finish is at the board.

### Undo

Press `u`. It is a first-class feature, because manual scoring goes wrong often.

Undo removes the last visit and rebuilds the entire game from the remaining
visits, so scores, dart counts, averages and completed legs all unwind together —
including undoing a checkout, which puts that player back in the game.

### Finishing

**Reaching zero does not end the game.** Other players may still want to play
their own legs out. When the office is done, press `f`.

The finish screen shows every player's result and suggests a winner: only a
completed leg can win, and among finishers the fewest darts wins. You confirm or
override it. You can also record **no winner** for a game that was simply
abandoned.

Players who never threw are stored honestly as incomplete, with zeroes — they are
never given a fabricated finish.

### Training — playing 301 or 501 alone

Selecting a single player starts a **training session**. The setup screen says so
before the game begins, and the header says so throughout.

Everything about the throwing is recorded exactly as it would be in a contest:
every visit, dart, bust and checkout, the three-dart average, legs completed, and
personal bests such as fewest darts to finish and best game average. Practice
counts.

What is *not* recorded is a victory. There was nobody to beat, so the finish
screen offers no winner to choose and simply saves the session. A win is only
ever awarded when at least two players took part.

Training sessions are also kept out of **games played** and **games won**, and so
out of the win percentage. Counting a solo leg as a game played with no win would
make it a recorded defeat, and practising would drag a player's win rate down.
They are shown separately on the profile as "Training sessions", and appear in
recent games as `Training` rather than as a game that failed to produce a result.

The rule is applied on every read, from the number of participants stored against
the game. A solo game recorded by an earlier version with a winner against it is
therefore already treated as training, with no repair step and no migration.

Cricket has no training mode: it needs two sides to be playable at all.

## Cricket — the office rules

> ⚠️ **This is not standard darts cricket.** There are no 15–20 numbers and
> nothing is "closed". It borrows its shape from cricket the bat-and-ball sport:
> two teams, batting and bowling, runs, wickets, and a match played over two
> legs and decided on the combined total. If you know the normal darts game by
> this name, read this section.

### Setup

Two teams, each with one or more players. Pick each team's players in the order
they will throw — **cricket enforces turn order**, unlike 501/301. Then choose
which team bats first and how many wickets end an innings (default 5, stored with
the game).

Teams do not need to be the same size. The wicket limit applies to every innings,
and each team bats twice, so a short limit makes for a quick match.

### Turn order

The **batter always throws first**, then a bowler, alternating. Each team cycles
through its own players independently.

With Team A = Alice, Bob and Team B = Charlie, David, Eve, with A batting:

| | Batting | Bowling |
| --- | --- | --- |
| 1 | Alice | Charlie |
| 2 | Bob | David |
| 3 | Alice | Eve |
| 4 | Bob | Charlie |

### Batting

Enter the **raw score** thrown — not the runs. Runs are worked out for you:

> **40 or below scores nothing. Every point above 40 is one run.**

| Raw score | Runs |
| --- | --- |
| 26 | 0 |
| 40 | 0 |
| 41 | 1 |
| 57 | 17 |
| 100 | 60 |

Both the raw score and the runs are stored, along with the darts thrown.

The same calculator as 501/301 is available here — press `=` or `+` and enter
`60+41` rather than working out 101 in your head. It also shows the runs the
total will earn, so the house rule holds no surprises:

```
Add up the darts  for Alice

Sum    60 + 41
Total  101   (60 + 41)
Runs   61
```

It applies to batting only. Bowling is entered as `1`, `2` and `3`, which needs
no arithmetic.

### Bowling

Bowlers throw at the bull. Press one key per dart, up to three:

| Key | Meaning | Wickets |
| --- | --- | --- |
| `1` | Anything else | 0 |
| `2` | Outer bull | 1 |
| `3` | Inner bull (bullseye) | 2 |

The digits ascend with the number of wickets, and sit on the numeric keypad
beside the digits used for batting scores and the keypad's own Enter key, so a
whole match can be scored without moving your hand. (`m`, `o` and `b` still work
for anyone who learnt those first.)

If the wicket limit is reached part-way through a visit, only the darts actually
thrown are recorded — the rest never happened.

### Two legs and four innings

A match is played over **two legs**. Each team bats once per leg, so there are
four innings in all, and **the winner is decided on the combined total of both
legs** — not on either leg alone.

| | Innings | Who bats |
| --- | --- | --- |
| **Leg 1** | 1 | the team chosen to bat first |
| | 2 | the other team |
| **Leg 2** | 3 | whichever team is **behind** on aggregate |
| | 4 | the team that is **ahead** — only if it still needs to bat |

**Leg 1 is played out in full.** Neither innings ends early for passing the other
side's score, because the match continues into leg 2 regardless. An innings in
leg 1 ends only when the batting side loses its wickets.

**In leg 2 the trailing team bats first**, which leaves the leading team batting
last. That ordering is what makes the next rule possible.

### One team may never bat again

Because the trailing team bats first in leg 2, the leaders may not need their
final innings at all.

If the trailing team is bowled out with a **combined total from both legs still
short of the opposition's**, the match is already decided and the leading team
wins without throwing another dart. The screen says so explicitly rather than
looking like a game that was cut short.

```
Leg 1   Team A ...... 60      Leg 2   Team B ...... 20 more
        Team B ...... 20              Team B aggregate 40

        Team A lead, so Team B bat first in leg 2.
        Bowled out on 40, short of 60 → Team A win. They never bat again.
```

If instead the trailing team **overtakes**, the leaders bat the final innings as
a genuine chase.

### Winning the final innings

Only the fourth innings is a chase. It ends the moment the batting side's
aggregate **exceeds** its opponent's.

- Passing the opponent's aggregate wins immediately.
- Being bowled out **below** it loses.
- Being bowled out **exactly level** is a **draw**.

Matching the total is not enough to win, in leg 2 as in leg 1. If the two sides
are level at the end of the third innings, the fourth is still played so the tie
can be broken.

This ending logic is deliberately kept in one place (`endInnings` in
`internal/domain/cricket`) so the office can extend it later — a super-over, a
tie-break, or a different number of legs.

## Statistics

Statistics are **derived from the recorded visits**, not accumulated into stored
totals. Correcting a mistake with undo therefore corrects every figure that
depends on it, with no repair step.

Only **finished** games count towards lifetime figures. A leg still in progress is
not a result yet, and counting it would make "games played" disagree with "total
darts". While you are playing, the game screen shows **current-game** figures
instead, which is what you actually want at the board — and the profile screen
labels its numbers "lifetime" so the two are never confused.

X01 and cricket figures are kept strictly apart. Averaging a three-dart average
together with a cricket batting average would produce a meaningless number.

### The three-dart average

```
average = credited points ÷ darts thrown × 3
```

Note the divisor: **darts thrown**, not visits × 3. A checkout visit may contain
one or two darts, and using visits would quietly misreport exactly the legs that
matter most.

"Credited points" excludes busted visits, which score nothing — but the darts
thrown in a bust **are** counted, so a bust correctly drags an average down.

### X01 figures

Games played, games won, win percentage, training sessions, legs completed, total
darts, total points, lifetime three-dart average, averages split by 301 and 501,
best average in a completed leg, fewest darts to finish 301 and 501, highest
visit, and counts of 100+, 140+ and 180 visits.

Games played, games won and the win percentage cover contested games only.
Everything else includes solo practice, because the darts were thrown either way.

High-score counts use credited points, so a visit that busts counts towards
nothing — which is what happened at the board.

### Cricket figures

**Batting:** visits, raw points, average raw score per visit, runs, average runs
per visit, highest raw visit, highest runs from a visit.

**Bowling:** visits, darts bowled, wickets, outer bulls, inner bulls, wickets per
visit.

Plus cricket games played and won. A match counts once however many
innings it took, so a three-innings win is worth the same as a four-innings one. Nothing is reported that cannot be derived
from what was actually captured.

## Where your data is stored

A single SQLite file, in your per-user application data directory:

| Platform | Location |
| --- | --- |
| **Windows** | `%LOCALAPPDATA%\DartsTUI\darts.db` |
| Linux | `$XDG_DATA_HOME/darts-tui/darts.db`, or `~/.local/share/darts-tui/darts.db` |
| macOS | `~/Library/Application Support/DartsTUI/darts.db` |

Run `darts.exe -where` to print the exact path.

`%LOCALAPPDATA%` rather than `%APPDATA%` is deliberate: on a domain-joined office
machine the roaming profile is synced at login, and a growing game history does
not belong there.

To move your history to another PC, copy `darts.db` (and any `darts.db-wal` file
next to it). To run from a USB stick, use `-portable`, which keeps the database
beside the executable instead.

The schema is versioned with migrations, so a future version of the application
will upgrade an existing database rather than replace it.

## Building from source

Requires Go 1.24 or newer. Nothing else — no C compiler.

```bash
git clone <this repository>
cd dart-tui

go build ./cmd/darts       # build for the machine you are on
go run ./cmd/darts         # build and run
go test ./...              # run the tests
```

### Windows builds

From any platform, including Linux and macOS:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o darts.exe ./cmd/darts
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o darts-arm64.exe ./cmd/darts
```

### Release build

Optimised, stripped, with the version stamped in:

```bash
make release
```

which produces, in `dist/`:

```
darts-windows-amd64.exe
darts-windows-arm64.exe
darts-linux-amd64
```

Or by hand:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -trimpath -ldflags "-s -w -X main.version=v1.0.0" \
  -o dist/darts-windows-amd64.exe ./cmd/darts
```

`CGO_ENABLED=0` is what keeps the result dependency-free. The SQLite driver is
[`modernc.org/sqlite`](https://modernc.org/sqlite), a pure-Go implementation, so
cross-compiling to Windows needs no MinGW and the executable needs no DLLs.

### GitHub Actions

- `.github/workflows/ci.yml` — tests on Linux **and** Windows, checks formatting
  and `go vet`, builds all three targets, then runs the resulting `.exe` on a
  clean Windows runner to prove it needs nothing installed.
- `.github/workflows/release.yml` — push a tag like `v1.0.0` to get a draft
  release with the executables and their checksums attached.

## Development

```bash
make check       # gofmt, go vet and the full test suite
make test        # tests only
make cover       # coverage per package
make test-race   # tests under the race detector
```

### Layout

```
cmd/darts/                 entry point and command-line flags
internal/
  domain/                  the rules — no terminal, no SQL
    darts/                 dartboard facts shared by all game modes
    player/                the player entity and colour palette
    x01/                   501 / 301 engine, and the checkout solver,
                           curated chart and route ranker
    cricket/               the office cricket engine
    stats/                 statistics derived from recorded visits
  app/                     application services the interface talks to
  persistence/
    sqlite/                all SQL, migrations, the store
  tui/                     Bubble Tea root, screen stack
    screens/               one file per screen
    components/            reusable widgets: the dart calculator, the block
                           font, the large "now throwing" panel and the
                           one-or-two-column game layout
    styles/                Lip Gloss styles
```

The dependency direction is one-way: `tui` → `app` → `domain` and `persistence`.

Four rules hold this together:

1. **Bubble Tea models do not own the rules.** A screen asks the engine what a
   visit did and displays the answer.
2. **SQLite does not own the rules.** The store records what happened and hands
   it back; it never decides what a bust is.
3. **X01 contains no cricket branching, and cricket contains no X01 branching.**
   They share only dartboard facts such as "no single dart scores 23".
4. **The engines are testable with no terminal.** Most of the test suite never
   starts a UI.

### Tests

```
internal/domain/darts        99%   board segments, reachability, averages
internal/domain/player      100%   names, palette, colour assignment
internal/domain/stats       100%   every derived statistic
internal/domain/x01          89%   scoring, busts, undo, finishing, and the
                                   checkout solver, chart and ranking
internal/domain/cricket      98%   runs, wickets, innings, chase, undo
internal/persistence/sqlite         migrations, save and resume, statistics
internal/app                        sessions, undo across storage, leaderboard
internal/tui/components             the dart calculator, the block font, the
                                   large panel at every size, and the choice
                                   between one column and two
internal/tui                        every screen driven headlessly by keystroke
```

The TUI tests drive the real screens through the real database with synthetic key
presses and assert on the rendered text, so navigation and rendering are covered
without a terminal.

Both engines are **event-sourced**: state is derived by replaying an ordered log
of visits, never mutated in place. That is what makes undo reliable — it drops
the last entry and replays, so there is no per-field reversal logic to get wrong —
and it is why a resumed game cannot drift from the one that was saved.

## Design decisions and assumptions

Where the specification left something open, a decision was made and recorded
here so it can be changed easily.

### Game rules

| Assumption | Where it lives |
| --- | --- |
| Double out is the X01 default, and is configurable for future variants. | `x01.Config.DoubleOut` |
| A visit landing on zero that the operator does not confirm is recorded as a bust, matching what happens at the board. | `x01.evaluate` |
| Winner ranking: completed legs first, then fewest darts, then whoever finished first. Never throwing order. | `x01.Game.better` |
| A game needs two players before a win means anything; alone it is training. | `x01.MinPlayersForWinner` |
| Training is refused a winner outright rather than having one silently dropped, so a caller that has not handled solo play is told. | `x01.Game.Finish` |
| Training counts towards every throwing figure, but not towards games played, games won or the win percentage — a solo leg is not a defeat. | `stats.ComputeX01` |
| Whether a game was training is derived from its stored participants on every read, so old solo games need no migration. | `sqlite.X01LegsForPlayer` |
| Cricket: both teams restart their player rotation at the top of the order at the start of every innings. | `cricket.startInnings` |
| X01 checkouts finish on D1-D20 or the inner bull; the outer bull scores 25 but cannot finish. | `darts.Segment.IsFinish` |
| Curated checkout routes are preferred over the solver's own ranking, and are the whole answer when they exist. | `x01.CheckoutAdvisor.Recommend` |
| Reordering the setup darts is the same route, not an alternative worth showing. | `x01.CheckoutRoute.Key` |
| Checkout suggestions assume three darts, since scoring is per visit rather than per dart. | `screens.dartsAtVisitStart` |
| Cricket: a match is two legs of two innings, decided on the combined total. | `cricket.LegsPerGame` |
| Cricket: in leg 2 the side behind on aggregate bats first, so the leaders bat last and may not bat at all. | `cricket.trailingSide` |
| Cricket: level aggregates after leg 1 keep the leg 1 order, so the choice never depends on chance. | `cricket.trailingSide` |
| Cricket: only the fourth innings is a chase; leg 1 is played out in full. | `cricket.step` |
| Cricket: bowled out exactly level on aggregate is a draw. Matching does not win. | `cricket.endInnings` |
| Cricket: the runs threshold is one named constant (`RunsThreshold = 40`). | `cricket.RunsFor` |
| Cricket has no manual "finish" — it ends itself — but a game can be **abandoned** with no result, because office games do stop early. | `cricket.Game.Abandon` |

### Interface

| Decision | Why |
| --- | --- |
| `Tab` changes the dart count, not `1`/`2`/`3`. | The digit keys are needed for score entry. |
| Bowling uses `1`/`2`/`3` rather than letters, ascending with the wickets taken. | Keeps every key on the numeric keypad, so scoring never needs the main keyboard. |
| `=` **and** `+` open the X01 calculator. | `=` is what was asked for; `+` is the one that exists on a numeric keypad. |
| The calculator fills the score box rather than recording the visit. | The total is a suggestion to check, not a decision — the second `Enter` is the commitment. |
| The calculator never changes the dart count. | Terms are not darts: `120+50+21` is three terms and could be four darts. Guessing would corrupt the statistic that matters most on a checkout. |
| The calculator appears on X01 and cricket batting, but not cricket bowling. | Bowling is three keypresses of `1`/`2`/`3`; there is nothing to add up. |
| Both game screens carry a permanent one-line reminder that the calculator exists. | A feature nobody knows about is no feature at all, and the reminder is worded once in `components.CalculatorHint`. |
| The keypad Enter key is accepted anywhere Return is. | The application asks the terminal for numeric keypad mode at startup and also treats a line feed as Enter, since terminals disagree on what that key sends. |
| Typing a digit jumps straight into score entry. | The common case should take no setup keystrokes. |
| Cricket pick order is throwing order. | One action instead of a separate ordering step. |
| The cursor stays put after a visit in X01. | The application must never choose the next thrower. |
| Leaving a game in progress asks for confirmation; recording a score never does. | Ordinary repetitive entry must stay fast. |
| Discarding a game and archiving a player are confirmed; archiving explains that nothing is deleted. | They are the only destructive actions. |
| Data is read from SQLite synchronously rather than through commands. | A local single-user file read costs microseconds; async plumbing would add a lot of code for no perceptible gain. |
| The text-entry cursor does not blink. | Blinking needs a timer redrawing twice a second all afternoon on a wall-mounted screen. |
| Both game screens have a third, always-visible section between the table and the entry box. | The scoreboard is watched from the oche, where the table is a grey blur and the entry box is none of the room's business. A tab would hide it from exactly the people it is for. |
| Given the width, the panel moves into a column of its own on the right. | A wide terminal has far more width going spare than height: fullscreen at 1920×1080 is around 240 columns, and the prompt panels are written for 80. A column also stops the panel and the prompt competing for the same rows. The split follows who is looking — keyboard on the left, room on the right. |
| The panel only moves to a column at least as wide as it would have had stacked, which works out at 182 columns. | A column buys height but costs width, and the number is limited by width first because its caption sits beside it. At 160 columns the column would be one short of what the second-largest size needs, so the move would trade a fourteen-row number for a seven-row one. |
| The parts inside the panel are centred as blocks, not line by line. | The rows of a number are not independent: a stroke has to stay in the same column from the top of the digit to the bottom. Centring each line on its own would shear it. |
| The name is drawn about a quarter the height of the number. | The number is read continuously all through a visit; the name is read once, when the thrower changes. The proportion is one map, `components.NameSizes` — change it there. |
| The name falls back to spaced capitals when there is no room for blocks, or when it contains a letter the font has no glyph for. | Drawing it in blocks costs the number the same rows, and a name pocked with blank cells where the accents should be is worse than one in ordinary text. |
| The block font is a 5×7 bitmap in the repository rather than a dependency. | The application ships as one static executable that needs no build tools; a figlet library for one panel is not a fair trade. Every size is generated from the one bitmap, so a character is edited in a single place. |
| `0` and `O` are the same shape. | The number is always numeric and the name always letters, so the two never share a field. Slashing the zero to tell them apart would make every score look odd for a distinction nothing depends on. |
| The large panel shrinks through five sizes and then disappears, and the entry box is never squeezed. | Bubble Tea keeps the *last* lines of a view that is too tall, so overflow silently removes the header and the table. The panel is a reminder and can go; the entry box is what is being typed into. |
| The largest size is only reachable when the panel has a column to itself. | Stacked it would never have the rows for it, so offering it there would only mean the panel could not draw anything at all on some windows. |
| The header always reserves a row for the status line, even when there is nothing to say. | It used to appear and disappear, shifting the whole screen down a row after every visit and sometimes costing the large panel a size at the same time. On a wall display that flicker is worse than one unused row. |
| The panel is hidden while a confirmation is on screen. | A modal question should not compete with it, and beside "did they finish on a double?" it would show the score from *before* the visit being asked about. |
| Cricket shows the innings runs in innings 1–3 and the runs needed in innings 4. | Only the fourth innings is a chase, so only then is there a number to chase. The number belongs to the innings, not the thrower, so it does not move when the sides swap over. |
| The best checkout appears in the large panel as well as in the prompt, but without the alternatives. | The two have different readers at different distances: a numbered list of three routes is noise from across the room. |
| Every number in the large panel also appears as ordinary text in the table above it. | Block glyphs can be read by an eye and nothing else, and the panel is gone entirely on a short terminal. |
| A large "who won" panel at the end of a cricket match was left out. | It is a different panel — no thrower and no live number — and the result screen already carries the result. Deliberately deferred, not overlooked. |

### Data model

The schema follows the suggested shape with two deliberate changes:

- **X01 visits and cricket events are separate tables** rather than one generic
  `visits` table. The two record genuinely different things, and separating them
  keeps the statistics queries honest and the constraints meaningful.
- **Innings are not stored.** They are a pure function of the cricket event log
  and the configured wicket limit, so storing them would create a second source
  of truth that could drift. They are recomputed on load.

Derived columns on `x01_visits` (credited, bust, checkout, the score either side)
are stored so statistics can be queried directly, but they are always recomputed
by replay on load and never trusted as the source of truth.

`game_results` stores the confirmed outcome of a finished game. Almost all of it
could be recalculated — but **who won** is the operator's decision, not a
derivation, so it has to be recorded.

### Colour

Player colours are assigned automatically from a 16-colour palette, preferring
one nobody is using. All colours are chosen to stay legible on both light and
dark terminal profiles.

**Colour never carries meaning on its own.** The current thrower has a `▶` marker
as well as a highlight, a bust says `BUST`, a finished player says `OUT · 28
darts`, an archived player says `[archived]`, and selections use `[x]`
checkboxes. The large panel says `NOW THROWING`, `▶ BATTING` or `▶ BOWLING` in
words, and labels its number `LEFT` or `TO WIN`. The application is fully usable
in monochrome.

The number in the large panel is drawn in the ordinary foreground rather than in
the player's colour, for the same reason the checkout routes are: it has to stay
legible from across the room whatever the palette and whatever the terminal
theme. The name above it carries the colour.

### Players are never deleted

Archiving hides a player from game setup. Every visit, result and statistic is
kept, they still appear correctly in the games they played, and they can be
restored at any time.
