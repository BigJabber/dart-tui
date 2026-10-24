package sqlite

// migration is one versioned, forward-only schema change.
//
// Migrations are applied in order inside a transaction and recorded in
// schema_migrations, so a future release can add a migration without disturbing
// an existing database. Never edit a migration that has shipped; add a new one.
type migration struct {
	Version int
	Name    string
	SQL     string
	// WithoutForeignKeys runs the migration with foreign key enforcement off.
	//
	// SQLite cannot alter a CHECK constraint in place: the table has to be
	// rebuilt and the old one dropped. With enforcement on, dropping it would
	// cascade-delete every row that references it — which is the entire history
	// of every game. The documented procedure is to turn enforcement off around
	// the rebuild and verify the result before committing, which is what the
	// runner does when this is set.
	WithoutForeignKeys bool
}

// migrations is the ordered schema history.
//
// The model is deliberately event-shaped: x01_visits and cricket_events are the
// record of what happened at the board, and every statistic is derived from
// them. game_results holds the outcome the operator confirmed, which is a
// decision rather than a derivation and so genuinely has to be stored.
//
// Innings are NOT stored. They are a pure function of the cricket event log —
// the wicket limit decides where one ends — so persisting them would create a
// second source of truth that could drift from the events.
var migrations = []migration{
	{
		Version: 1,
		Name:    "initial schema",
		SQL: `
CREATE TABLE players (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    colour      TEXT    NOT NULL,
    created_at  TEXT    NOT NULL,
    archived_at TEXT
);

-- Names are unique case-insensitively so "Bob" and "bob" cannot both exist.
CREATE UNIQUE INDEX idx_players_name ON players (name COLLATE NOCASE);
CREATE INDEX idx_players_archived ON players (archived_at);

CREATE TABLE games (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    mode        TEXT    NOT NULL CHECK (mode IN ('x01', 'cricket')),
    status      TEXT    NOT NULL CHECK (status IN ('in_progress', 'finished')),
    created_at  TEXT    NOT NULL,
    finished_at TEXT,

    -- X01 configuration.
    start_score INTEGER,
    double_out  INTEGER,

    -- Cricket configuration.
    wicket_limit  INTEGER,
    first_batting TEXT CHECK (first_batting IS NULL OR first_batting IN ('A', 'B')),

    -- Outcome. X01 records a winning player; cricket records a winning side or
    -- a draw. Both may be absent for a game that was abandoned.
    winner_player_id INTEGER REFERENCES players (id),
    outcome          TEXT
);

CREATE INDEX idx_games_status ON games (status, created_at DESC);
CREATE INDEX idx_games_mode ON games (mode, status);

CREATE TABLE game_participants (
    game_id   INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players (id),
    position  INTEGER NOT NULL,
    team_side TEXT CHECK (team_side IS NULL OR team_side IN ('A', 'B')),
    PRIMARY KEY (game_id, player_id)
);

CREATE INDEX idx_participants_player ON game_participants (player_id);

CREATE TABLE teams (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    side    TEXT    NOT NULL CHECK (side IN ('A', 'B')),
    name    TEXT    NOT NULL,
    UNIQUE (game_id, side)
);

CREATE TABLE team_players (
    team_id   INTEGER NOT NULL REFERENCES teams (id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players (id),
    position  INTEGER NOT NULL,
    PRIMARY KEY (team_id, player_id)
);

-- One row per X01 visit. The derived columns (credited, bust, checkout, the
-- score either side) are recomputed on load by replaying the log; they are
-- stored so statistics can be queried directly without a replay.
CREATE TABLE x01_visits (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id          INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    seq              INTEGER NOT NULL,
    player_id        INTEGER NOT NULL REFERENCES players (id),
    points           INTEGER NOT NULL,
    darts            INTEGER NOT NULL CHECK (darts BETWEEN 1 AND 3),
    confirm_checkout INTEGER NOT NULL DEFAULT 0,
    credited         INTEGER NOT NULL,
    bust             INTEGER NOT NULL DEFAULT 0,
    checkout         INTEGER NOT NULL DEFAULT 0,
    score_before     INTEGER NOT NULL,
    score_after      INTEGER NOT NULL,
    UNIQUE (game_id, seq)
);

CREATE INDEX idx_x01_visits_player ON x01_visits (player_id);

-- One row per cricket visit, batting or bowling.
CREATE TABLE cricket_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id    INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    innings    INTEGER NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('bat', 'bowl')),
    side       TEXT    NOT NULL CHECK (side IN ('A', 'B')),
    player_id  INTEGER NOT NULL REFERENCES players (id),
    raw_points INTEGER NOT NULL DEFAULT 0,
    runs       INTEGER NOT NULL DEFAULT 0,
    darts      INTEGER NOT NULL DEFAULT 0,
    wickets    INTEGER NOT NULL DEFAULT 0,
    UNIQUE (game_id, seq)
);

CREATE INDEX idx_cricket_events_player ON cricket_events (player_id, kind);

-- Individual bowling darts, so outer and inner bulls stay distinguishable
-- rather than collapsing into a wicket count.
CREATE TABLE bowling_darts (
    event_id INTEGER NOT NULL REFERENCES cricket_events (id) ON DELETE CASCADE,
    idx      INTEGER NOT NULL,
    result   TEXT    NOT NULL CHECK (result IN ('M', 'O', 'B')),
    PRIMARY KEY (event_id, idx)
);

-- The confirmed outcome for each participant of a finished game. Everything
-- here except "won" could be recomputed; "won" is the operator's decision and
-- cannot be, which is why the table exists.
CREATE TABLE game_results (
    game_id   INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    player_id INTEGER NOT NULL REFERENCES players (id),
    team_side TEXT,
    completed INTEGER NOT NULL DEFAULT 0,
    won       INTEGER NOT NULL DEFAULT 0,
    remaining INTEGER,
    darts     INTEGER NOT NULL DEFAULT 0,
    visits    INTEGER NOT NULL DEFAULT 0,
    points    INTEGER NOT NULL DEFAULT 0,
    average   REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, player_id)
);

CREATE INDEX idx_game_results_player ON game_results (player_id);
`,
	},
	{
		Version: 2,
		Name:    "around the world",
		// The games table is rebuilt rather than altered because its mode is
		// constrained by a CHECK, and SQLite has no way to widen one in place.
		// Everything else about the table is carried over unchanged.
		//
		// The child tables reference "games" by name. Renaming the new table
		// into place with foreign keys disabled leaves those references pointing
		// at it, which is exactly what is wanted: the rows are untouched and
		// they now belong to the rebuilt table.
		WithoutForeignKeys: true,
		SQL: `
CREATE TABLE games_v2 (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    mode        TEXT    NOT NULL CHECK (mode IN ('x01', 'cricket', 'atw')),
    status      TEXT    NOT NULL CHECK (status IN ('in_progress', 'finished')),
    created_at  TEXT    NOT NULL,
    finished_at TEXT,

    start_score INTEGER,
    double_out  INTEGER,

    wicket_limit  INTEGER,
    first_batting TEXT CHECK (first_batting IS NULL OR first_batting IN ('A', 'B')),

    winner_player_id INTEGER REFERENCES players (id),
    outcome          TEXT
);

INSERT INTO games_v2
SELECT id, mode, status, created_at, finished_at, start_score, double_out,
       wicket_limit, first_batting, winner_player_id, outcome
FROM games;

DROP TABLE games;
ALTER TABLE games_v2 RENAME TO games;

CREATE INDEX idx_games_status ON games (status, created_at DESC);
CREATE INDEX idx_games_mode ON games (mode, status);

-- One row per Around the World visit. As with x01_visits the derived columns
-- are recomputed by replaying the darts on load; they are stored so statistics
-- can be queried directly without replaying every game a player has thrown.
--
-- The two target columns hold a position in the target sequence, 0 for "1" up
-- to 21 for the bullseye and 22 for a completed run. That numbering is an
-- implementation detail of the ordered sequence and is converted at the edge;
-- nothing outside the domain reasons about it, and nothing displays it.
CREATE TABLE atw_visits (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id       INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    seq           INTEGER NOT NULL,
    player_id     INTEGER NOT NULL REFERENCES players (id),
    darts         INTEGER NOT NULL CHECK (darts BETWEEN 1 AND 3),
    advanced      INTEGER NOT NULL DEFAULT 0,
    target_before INTEGER NOT NULL,
    target_after  INTEGER NOT NULL,
    completed     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (game_id, seq)
);

CREATE INDEX idx_atw_visits_player ON atw_visits (player_id);

-- Individual darts, holding the board segment struck rather than the keystroke
-- typed. "D14" stays true after the target has moved on; "double" does not.
-- A miss is stored as "-", because the operator is never asked which wrong
-- number was hit.
CREATE TABLE atw_darts (
    visit_id INTEGER NOT NULL REFERENCES atw_visits (id) ON DELETE CASCADE,
    idx      INTEGER NOT NULL,
    segment  TEXT    NOT NULL,
    PRIMARY KEY (visit_id, idx)
);
`,
	},
	{
		Version: 3,
		Name:    "automatic player swapping",
		// A plain added column with a default, so every existing game reads as
		// off — which is how they were played — and no table needs rebuilding.
		SQL: `
-- Whether the scoreboard moves on to the next player in sequence after each
-- visit. It belongs to the game rather than to the application so a resumed
-- game carries on the way it was being played. The sequence itself is the
-- participants' position, which the games already record.
ALTER TABLE games ADD COLUMN auto_swap INTEGER NOT NULL DEFAULT 0;
`,
	},
}

// SchemaVersion is the version a fresh database is created at.
func SchemaVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}
