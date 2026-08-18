// audit_test.go table-drives webster's own fork-audit policy over the full violation taxonomy
// CheckFork/CheckParent enforce, the warning-only ForkWarnings case, the fabricengine.RefScanner
// matcher (built from a fake lyxcwd.Location, never a hardcoded geometry token), and the
// attribution pipeline (NewTranscripts, SettleRetry with a recording fake Sleeper, and
// ClassifyAttribution's pinned check order).
// Every case here is a pure fact-in/verdict-out table, per the discussion's TDD-centre framing: no
// git spawn, no real sleeping, no filesystem I/O.

package websterengine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeLayout returns a lyxcwd.Location that resolves fabricengine.WeftWorktree() without spawning git.
func fakeLayout() *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: "/hub", WorktreeName: filepath.Base("/hub/master-builder")}
}

// TestRefScannerMatches matrixes fabricengine.NewRefScanner against every Bash command shape
// CheckFork/CheckParent must classify: `lyx fabric` invocations (the live spelling the Fabric Git
// Invariant bans), the pre-cutover `lyx weft`/`lyx warp` spellings, a command referencing the fabric
// worktree path directly (e.g. `git -C <fabric-worktree> add`), and a set of fabric-free commands that
// must never match.
// The `lyx fabric` rows are the regression guard: the fabric cutover deleted `lyx weft`/`lyx warp`
// and renamed every fabric-touching verb under `lyx fabric`, so a matcher that knows only the old
// spellings bans nothing an agent can actually run today.
// The `lyx.exe` rows are the same guard for the Windows spelling — lyx's primary platform, where an
// agent writing the extension out would otherwise slip the whole audit — paired with a `lyx.exe
// board` row proving the extension did not widen the match to every lyx invocation.
func TestRefScannerMatches(t *testing.T) {
	layout := fakeLayout()
	fabricRef := fabricengine.NewRefScanner(layout)
	fabricWorktree := fabricengine.WeftWorktree(layout)

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"lyx fabric sync", "lyx fabric sync", true},
		{"lyx fabric commit", "lyx fabric commit", true},
		{"lyx fabric push", "lyx fabric push", true},
		{"lyx fabric checkout", "lyx fabric checkout feature", true},
		{"lyx fabric with leading prose", "cd /hub/warp && lyx fabric sync", true},
		{"lyx weft sync", "lyx weft sync", true},
		{"lyx warp checkout", "lyx warp checkout feature", true},
		{"lyx.exe fabric sync", "lyx.exe fabric sync", true},
		{"lyx.exe weft push", "lyx.exe weft push", true},
		{"absolute lyx.exe fabric push", `C:\bin\lyx.exe fabric push`, true},
		{"git -C fabric-worktree add", "git -C " + fabricWorktree + " add -A", true},
		{"cd into fabric worktree", "cd " + fabricWorktree + " && git status", true},
		{"warp git commit is not a fabric reference", "git commit -am wip", false},
		{"plain read", "cat notes.txt", false},
		{"warp status", "git status", false},
		{"unrelated path", "cat /hub/other-repo/README.md", false},
		{"a fabric-named file is not a lyx fabric invocation", "cat fabric-notes.md", false},
		{"lyx board is not a fabric reference", "lyx board list", false},
		{"lyx.exe board is not a fabric reference either", "lyx.exe board list", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fabricRef.Matches(tt.cmd); got != tt.want {
				t.Errorf("fabricRef.Matches(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// Compile-time assertions that both suppliers satisfy RefMatcher: a later signature drift on either
// side fails at compile time rather than at the first standalone run.
var (
	_ RefMatcher = NeverMatches{}
	_ RefMatcher = (*fabricengine.RefScanner)(nil)
)

// TestNeverMatches_AlwaysFalse pins NeverMatches's whole contract: it never matches, not even for a
// command spelling a real *fabricengine.RefScanner would match.
func TestNeverMatches_AlwaysFalse(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"a real RefScanner would match this fabric command", "lyx fabric sync"},
		{"empty string", ""},
		{"ordinary non-fabric command", "git status"},
	}

	var m NeverMatches
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Matches(tt.cmd); got != false {
				t.Errorf("NeverMatches{}.Matches(%q) = %v; want false", tt.cmd, got)
			}
		})
	}
}

// cleanForkReport returns a ForkReport that violates none of CheckFork's
// rules — tests mutate a copy to trigger exactly one violation at a time.
func cleanForkReport(path string) shuttleengine.ForkReport {
	return shuttleengine.ForkReport{TranscriptPath: path, ReportReturned: true}
}

// TestCheckFork covers every violation CheckFork enforces plus the two cases the requirements pin
// as explicitly ALLOWED for a fork (Write/Edit and repo git), which is the opposite of
// burlerengine's read-only cluster-reviewer policy.
func TestCheckFork(t *testing.T) {
	layout := fakeLayout()
	fabricRef := fabricengine.NewRefScanner(layout)
	fabricWorktree := fabricengine.WeftWorktree(layout)

	tests := []struct {
		name        string
		fork        shuttleengine.ForkReport
		wantClasses []AuditViolationClass
	}{
		{
			name: "nested Agent call is a hard error even when denied",
			fork: shuttleengine.ForkReport{TranscriptPath: "a", ReportReturned: true, AgentCalls: 1},
			wantClasses: []AuditViolationClass{
				ClassNestedAgent,
			},
		},
		{
			name:        "Write/Edit calls are allowed for an implementer fork",
			fork:        shuttleengine.ForkReport{TranscriptPath: "b", ReportReturned: true, WriteCalls: 5},
			wantClasses: nil,
		},
		{
			name: "warp git commit is allowed (per-card commits are the contract)",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "c", ReportReturned: true,
				BashCommands: []string{"git add internal/foo.go", "git commit -m 'card 1'"},
			},
			wantClasses: nil,
		},
		{
			name: "lyx fabric sync is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "d-fabric", ReportReturned: true,
				BashCommands: []string{"lyx fabric sync"},
			},
			wantClasses: []AuditViolationClass{ClassFabricReference},
		},
		{
			name: "lyx weft sync is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "d", ReportReturned: true,
				BashCommands: []string{"lyx weft sync"},
			},
			wantClasses: []AuditViolationClass{ClassFabricReference},
		},
		{
			name: "git -C <fabric-worktree> add is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "e", ReportReturned: true,
				BashCommands: []string{"git -C " + fabricWorktree + " add -A"},
			},
			wantClasses: []AuditViolationClass{ClassFabricReference},
		},
		{
			// A fork writing Master's own contract files forges the run's
			// terminal judgment (round fable-r3 live: a misidentifying fork
			// overwrote outcome.yaml with a forged stuck mid-run).
			name: "fork write to outcome.yaml is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "f", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/outcome.yaml"},
			},
			wantClasses: []AuditViolationClass{ClassForkContractWrite},
		},
		{
			name: "relative fork write to summary.md is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "g", ReportReturned: true,
				WritePaths: []string{"_lyx/webster/summary.md"},
			},
			wantClasses: []AuditViolationClass{ClassForkContractWrite},
		},
		{
			name: "fork write to its own batch report stays allowed",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "h", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/reports/01-json-flag.yaml"},
			},
			wantClasses: nil,
		},
	}

	const outcomePath = "/hub/master-builder/_lyx/webster/outcome.yaml"
	const summaryPath = "/hub/master-builder/_lyx/webster/summary.md"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckFork(tt.fork, outcomePath, summaryPath, "/hub/master-builder", fabricRef)
			if len(got) != len(tt.wantClasses) {
				t.Fatalf("CheckFork() = %v; want %d violation(s) of class %v", got, len(tt.wantClasses), tt.wantClasses)
			}
			for i, v := range got {
				if v.Class != tt.wantClasses[i] {
					t.Errorf("CheckFork()[%d].Class = %q; want %q", i, v.Class, tt.wantClasses[i])
				}
				if v.TranscriptPath != tt.fork.TranscriptPath {
					t.Errorf("CheckFork()[%d].TranscriptPath = %q; want %q", i, v.TranscriptPath, tt.fork.TranscriptPath)
				}
				if v.Error() == "" {
					t.Errorf("CheckFork()[%d].Error() = empty string; want non-empty", i)
				}
			}
		})
	}
}

// TestCheckParent covers every violation CheckParent enforces plus the two contract-file writes
// pinned as explicitly ALLOWED for Master.
func TestCheckParent(t *testing.T) {
	layout := fakeLayout()
	fabricRef := fabricengine.NewRefScanner(layout)
	fabricWorktree := fabricengine.WeftWorktree(layout)

	const outcomePath = "/hub/master-builder/_lyx/webster/outcome.yaml"
	const summaryPath = "/hub/master-builder/_lyx/webster/summary.md"

	tests := []struct {
		name        string
		audit       shuttleengine.ForkAudit
		wantClasses []AuditViolationClass
	}{
		{
			name:        "named spawn is a hard error",
			audit:       shuttleengine.ForkAudit{NamedSpawns: 1},
			wantClasses: []AuditViolationClass{ClassNamedSpawn},
		},
		{
			name: "write to outcome.yaml is allowed",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{outcomePath},
			},
			wantClasses: nil,
		},
		{
			name: "write to summary.md is allowed",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{summaryPath},
			},
			wantClasses: nil,
		},
		{
			// The transcript records whatever file_path string Master passed to
			// its Write tool; a RELATIVE spelling of a contract file must resolve
			// against the pane cwd, never false-positive (found live in round
			// fable-r3: a fully-done run failed its exit audit on exactly this).
			name: "relative write to outcome.yaml is allowed",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"_lyx/webster/outcome.yaml"},
			},
			wantClasses: nil,
		},
		{
			name: "dot-prefixed relative write to summary.md is allowed",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"./_lyx/webster/summary.md"},
			},
			wantClasses: nil,
		},
		{
			name: "relative write to a source file is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"internal/websterengine/audit.go"},
			},
			wantClasses: []AuditViolationClass{ClassParentWrite},
		},
		{
			name: "write to a source file is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"/hub/master-builder/internal/websterengine/audit.go"},
			},
			wantClasses: []AuditViolationClass{ClassParentWrite},
		},
		{
			name: "write to a reports-dir path is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"/hub/master-builder/_lyx/webster/reports/03-webster-audit-policy.yaml"},
			},
			wantClasses: []AuditViolationClass{ClassParentWrite},
		},
		{
			name: "parent fabric bash is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentBashCommands: []string{"git -C " + fabricWorktree + " commit -am wip"},
			},
			wantClasses: []AuditViolationClass{ClassFabricReference},
		},
		{
			name: "parent lyx fabric sync is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentBashCommands: []string{"lyx fabric sync"},
			},
			wantClasses: []AuditViolationClass{ClassFabricReference},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckParent(tt.audit, outcomePath, summaryPath, "/hub/master-builder", fabricRef)
			if len(got) != len(tt.wantClasses) {
				t.Fatalf("CheckParent() = %v; want %d violation(s) of class %v", got, len(tt.wantClasses), tt.wantClasses)
			}
			for i, v := range got {
				if v.Class != tt.wantClasses[i] {
					t.Errorf("CheckParent()[%d].Class = %q; want %q", i, v.Class, tt.wantClasses[i])
				}
			}
		})
	}
}

// TestForkWarnings pins the one warning-only (never round-failing) class: a fork that never
// returned a final report.
func TestForkWarnings(t *testing.T) {
	tests := []struct {
		name string
		fork shuttleengine.ForkReport
		want []string
	}{
		{
			name: "report returned yields no warning",
			fork: cleanForkReport("a"),
			want: nil,
		},
		{
			name: "report not returned is a warning",
			fork: shuttleengine.ForkReport{TranscriptPath: "b", ReportReturned: false},
			want: []string{`fork "b" never returned a final report`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForkWarnings(tt.fork)
			if len(got) != len(tt.want) {
				t.Fatalf("ForkWarnings() = %v; want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ForkWarnings()[%d] = %q; want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestNewTranscripts pins the defensive re-filter: only ForkReport entries whose TranscriptPath is
// absent from seen come back, in original order.
func TestNewTranscripts(t *testing.T) {
	audit := shuttleengine.ForkAudit{
		Forks: []shuttleengine.ForkReport{
			cleanForkReport("a"),
			cleanForkReport("b"),
			cleanForkReport("c"),
		},
	}

	got := NewTranscripts(audit, []string{"a", "c"})
	if len(got) != 1 || got[0].TranscriptPath != "b" {
		t.Errorf("NewTranscripts() = %v; want exactly [b]", got)
	}

	// A nil/empty seen set reports every fork as new.
	gotAll := NewTranscripts(audit, nil)
	if len(gotAll) != 3 {
		t.Errorf("NewTranscripts(nil seen) = %v; want all 3 forks", gotAll)
	}
}

// recordingSleeper is a Sleeper that never actually blocks — it only records
// each requested duration, so SettleRetry's retry loop runs a scripted
// sequence of "attempts" at zero real wall-clock cost. onSleep, when set, is
// invoked after Sleep records the call, letting a test mutate a shared fetch
// script exactly between two SettleRetry attempts (mirroring
// shuttleengine's scriptedClock pattern).
type recordingSleeper struct {
	slept   []time.Duration
	onSleep func()
}

func (s *recordingSleeper) Sleep(d time.Duration) {
	s.slept = append(s.slept, d)
	if s.onSleep != nil {
		s.onSleep()
	}
}

// TestSettleRetry_ReturnsEarlyOnLaterTick pins SettleRetry's core contract: a transcript that only
// appears on the fetch AFTER the first Sleep call makes SettleRetry return immediately, without
// waiting out the rest of the settle window and without any real sleeping.
func TestSettleRetry_ReturnsEarlyOnLaterTick(t *testing.T) {
	calls := 0
	fetch := func() (shuttleengine.ForkAudit, error) {
		calls++
		if calls == 1 {
			return shuttleengine.ForkAudit{}, nil
		}
		return shuttleengine.ForkAudit{
			Forks: []shuttleengine.ForkReport{cleanForkReport("fork-2")},
		}, nil
	}

	sleeper := &recordingSleeper{}
	audit, newReports, err := SettleRetry(fetch, nil, DefaultSettleWindow, DefaultSettleTick, sleeper)
	if err != nil {
		t.Fatalf("SettleRetry() error = %v; want nil", err)
	}
	if len(newReports) != 1 || newReports[0].TranscriptPath != "fork-2" {
		t.Errorf("SettleRetry() newReports = %v; want exactly [fork-2]", newReports)
	}
	if len(audit.Forks) != 1 {
		t.Errorf("SettleRetry() audit.Forks = %v; want exactly the returned fork", audit.Forks)
	}
	if calls != 2 {
		t.Errorf("fetch called %d time(s); want exactly 2 (one miss, one hit)", calls)
	}
	if len(sleeper.slept) != 1 || sleeper.slept[0] != DefaultSettleTick {
		t.Errorf("sleeper.slept = %v; want exactly one sleep of %v", sleeper.slept, DefaultSettleTick)
	}
}

// TestSettleRetry_WindowExhausted pins the other half of the contract: zero new transcripts across
// every attempt returns with a nil error once window elapses — SettleRetry never manufactures the
// hard error itself.
func TestSettleRetry_WindowExhausted(t *testing.T) {
	fetch := func() (shuttleengine.ForkAudit, error) {
		return shuttleengine.ForkAudit{}, nil
	}

	sleeper := &recordingSleeper{}
	window := 1 * time.Second
	tick := 250 * time.Millisecond
	_, newReports, err := SettleRetry(fetch, nil, window, tick, sleeper)
	if err != nil {
		t.Fatalf("SettleRetry() error = %v; want nil", err)
	}
	if len(newReports) != 0 {
		t.Errorf("SettleRetry() newReports = %v; want empty", newReports)
	}
	wantSleeps := int(window / tick)
	if len(sleeper.slept) != wantSleeps {
		t.Errorf("sleeper.slept has %d entries; want %d (window/tick)", len(sleeper.slept), wantSleeps)
	}
}

// TestSettleRetry_FetchErrorPropagates pins the fail-loud posture: a fetch error returns
// immediately, with no retry — an audit read that itself failed has nothing safe to retry against.
func TestSettleRetry_FetchErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	fetch := func() (shuttleengine.ForkAudit, error) {
		return shuttleengine.ForkAudit{}, wantErr
	}

	sleeper := &recordingSleeper{}
	_, _, err := SettleRetry(fetch, nil, DefaultSettleWindow, DefaultSettleTick, sleeper)
	if !errors.Is(err, wantErr) {
		t.Errorf("SettleRetry() error = %v; want %v", err, wantErr)
	}
	if len(sleeper.slept) != 0 {
		t.Errorf("sleeper.slept = %v; want no sleeps after a fetch error", sleeper.slept)
	}
}

// TestClassifyAttribution pins the pinned check order from discussion.md's fork-audit-policy
// decision: zero new transcripts is always a hard error (regardless of report presence —
// ClassifyAttribution takes no report argument at all, which is itself the enforcement), one new is
// clean, and more than one is a warning, never hard.
func TestClassifyAttribution(t *testing.T) {
	tests := []struct {
		name        string
		newReports  []shuttleengine.ForkReport
		wantWarning string
		wantErr     error
	}{
		{
			name:       "zero new after settle is a hard error",
			newReports: nil,
			wantErr:    ErrNoForkTranscripts,
		},
		{
			name:       "exactly one new is clean",
			newReports: []shuttleengine.ForkReport{cleanForkReport("a")},
		},
		{
			name:        "two new is a warning, never hard",
			newReports:  []shuttleengine.ForkReport{cleanForkReport("a"), cleanForkReport("b")},
			wantWarning: "2 new fork transcripts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := ClassifyAttribution(tt.newReports)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ClassifyAttribution() error = %v; want %v", err, tt.wantErr)
				}
				// The zero-transcript error is one leg of a three-verb refusal
				// circle with no in-band exit (a cross-machine resume of the
				// report-landed crash window reproduces it with no forgery), so
				// its message must name the operator recourse.
				if !strings.Contains(err.Error(), "machine-local") || !strings.Contains(err.Error(), "moving the batch's report file") {
					t.Errorf("ClassifyAttribution() error %q does not name the machine-local transcript caveat and the operator recourse", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ClassifyAttribution() error = %v; want nil", err)
			}
			if tt.wantWarning == "" {
				if warning != "" {
					t.Errorf("ClassifyAttribution() warning = %q; want empty", warning)
				}
				return
			}
			if !strings.Contains(warning, tt.wantWarning) {
				t.Errorf("ClassifyAttribution() warning = %q; want substring %q", warning, tt.wantWarning)
			}
		})
	}
}
