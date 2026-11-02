// record.go declares the record a caller keeps of one spawned fixer strand and the store seam it keeps it in.

package gatefixer

// The statuses a Record takes once its fixer ends; a live record has none.
const (
	// StatusRunning is what a wait reports when it elapsed before the fixer ended.
	StatusRunning = "running"
	// StatusDone is the status of a fixer that wrote its reply file.
	StatusDone = "done"
	// StatusDead is the status of a fixer that ended without a reply, for a DeadReason.
	StatusDead = "dead"
)

// The reasons a fixer ends dead.
const (
	// DeadReasonAsking is a fixer whose newest turn ended with no reply written.
	DeadReasonAsking = "asking"
	// DeadReasonTimeout is a fixer still without a reply when its timeout passed.
	DeadReasonTimeout = "timeout"
	// DeadReasonDied is a fixer whose strand is gone.
	DeadReasonDied = "died"
)

// Record is what a caller keeps of one spawned fixer strand.
// A record is live while Status is empty.
type Record struct {
	// StrandGUID identifies the reed strand the fixer runs in.
	StrandGUID string `json:"strandGuid"`
	// SessionID and Engine identify the fixer's session and the engine it ran under.
	SessionID string `json:"sessionId,omitempty"`
	Engine    string `json:"engine,omitempty"`
	// SpawnedAt is the spawn time in RFC 3339, from which the timeout runs.
	SpawnedAt string `json:"spawnedAt"`
	// ShuttleRunDir is the fixer's shuttle run directory, removed once the fixer is done.
	ShuttleRunDir string `json:"shuttleRunDir,omitempty"`
	// EventsPath and EventsOffset locate the fixer's turn events after its skill-load turns.
	EventsPath   string `json:"eventsPath,omitempty"`
	EventsOffset int64  `json:"eventsOffset,omitempty"`
	// ReportPath and ReplyPath are the report the fixer reads and the reply file it writes.
	ReportPath string `json:"reportPath,omitempty"`
	ReplyPath  string `json:"replyPath,omitempty"`
	// Status is empty while the fixer runs, then StatusDone or StatusDead.
	Status string `json:"status,omitempty"`
	// DeadReason is set only when Status is StatusDead.
	DeadReason string `json:"deadReason,omitempty"`
}

// Store is the caller's store of fixer records.
type Store interface {
	// Live returns the newest record with no terminal status, and nil when there is none.
	Live() (*Record, error)
	// Save appends rec, or rewrites the record of rec.StrandGUID.
	Save(rec *Record) error
}
