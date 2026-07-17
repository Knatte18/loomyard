// audit_test.go table-drives webster's own fork-audit policy over the full
// violation taxonomy CheckFork/CheckParent enforce, the warning-only
// ForkWarnings case, the weftReferencePattern matcher (built from a fake
// hubgeometry.Layout, never a hardcoded geometry token), and the attribution
// pipeline (NewTranscripts, SettleRetry with a recording fake Sleeper, and
// ClassifyAttribution's pinned check order). Every case here is a pure
// fact-in/verdict-out table, per the discussion's TDD-centre framing: no git
// spawn, no real sleeping, no filesystem I/O.

package websterengine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeLayout returns a hubgeometry.Layout that resolves WeftWorktree() to a
// deterministic, OS-native path without ever spawning git — the same
// direct-construction pattern shuttleengine's own tests use for a fake
// Layout (e.g. wait_test.go's newWaitTestRunner).
func fakeLayout() *hubgeometry.Layout {
	return &hubgeometry.Layout{
		Hub:          "/hub",
		WorktreeRoot: "/hub/master-builder",
	}
}

// TestWeftReferencePattern matrixes weftReferencePattern against every Bash
// command shape CheckFork/CheckParent must classify: lyx weft/warp
// invocations, a command referencing the weft worktree path directly (e.g.
// `git -C <weft-worktree> add`), and a set of weft-free commands that must
// never match.
func TestWeftReferencePattern(t *testing.T) {
	layout := fakeLayout()
	weftRef := weftReferencePattern(layout)
	weftWorktree := layout.WeftWorktree()

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"lyx weft sync", "lyx weft sync", true},
		{"lyx warp checkout", "lyx warp checkout feature", true},
		{"git -C weft-worktree add", "git -C " + weftWorktree + " add -A", true},
		{"cd into weft worktree", "cd " + weftWorktree + " && git status", true},
		{"host git commit is not a weft reference", "git commit -am wip", false},
		{"plain read", "cat notes.txt", false},
		{"host status", "git status", false},
		{"unrelated path", "cat /hub/other-repo/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weftRef.MatchString(tt.cmd); got != tt.want {
				t.Errorf("weftRef.MatchString(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// cleanForkReport returns a ForkReport that violates none of CheckFork's
// rules — tests mutate a copy to trigger exactly one violation at a time.
func cleanForkReport(path string) shuttleengine.ForkReport {
	return shuttleengine.ForkReport{TranscriptPath: path, ReportReturned: true}
}

// TestCheckFork covers every violation CheckFork enforces plus the two cases
// the requirements pin as explicitly ALLOWED for a fork (Write/Edit and
// host-repo git), which is the opposite of burlerengine's read-only
// cluster-reviewer policy.
func TestCheckFork(t *testing.T) {
	layout := fakeLayout()
	weftRef := weftReferencePattern(layout)
	weftWorktree := layout.WeftWorktree()

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
			name: "host git commit is allowed (per-card commits are the contract)",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "c", ReportReturned: true,
				BashCommands: []string{"git add internal/foo.go", "git commit -m 'card 1'"},
			},
			wantClasses: nil,
		},
		{
			name: "lyx weft sync is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "d", ReportReturned: true,
				BashCommands: []string{"lyx weft sync"},
			},
			wantClasses: []AuditViolationClass{ClassWeftReference},
		},
		{
			name: "git -C <weft-worktree> add is a hard error",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "e", ReportReturned: true,
				BashCommands: []string{"git -C " + weftWorktree + " add -A"},
			},
			wantClasses: []AuditViolationClass{ClassWeftReference},
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
			got := CheckFork(tt.fork, outcomePath, summaryPath, "/hub/master-builder", weftRef)
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

// TestCheckParent covers every violation CheckParent enforces plus the two
// contract-file writes pinned as explicitly ALLOWED for Master.
func TestCheckParent(t *testing.T) {
	layout := fakeLayout()
	weftRef := weftReferencePattern(layout)
	weftWorktree := layout.WeftWorktree()

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
			name: "parent weft bash is a hard error",
			audit: shuttleengine.ForkAudit{
				ParentBashCommands: []string{"git -C " + weftWorktree + " commit -am wip"},
			},
			wantClasses: []AuditViolationClass{ClassWeftReference},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckParent(tt.audit, outcomePath, summaryPath, "/hub/master-builder", weftRef)
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

// TestForkWarnings pins the one warning-only (never round-failing) class: a
// fork that never returned a final report.
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

// TestNewTranscripts pins the defensive re-filter: only ForkReport entries
// whose TranscriptPath is absent from seen come back, in original order.
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

// TestSettleRetry_ReturnsEarlyOnLaterTick pins SettleRetry's core contract:
// a transcript that only appears on the fetch AFTER the first Sleep call
// makes SettleRetry return immediately, without waiting out the rest of the
// settle window and without any real sleeping.
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

// TestSettleRetry_WindowExhausted pins the other half of the contract:
// zero new transcripts across every attempt returns with a nil error once
// window elapses — SettleRetry never manufactures the hard error itself.
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

// TestSettleRetry_FetchErrorPropagates pins the fail-loud posture: a fetch
// error returns immediately, with no retry — an audit read that itself
// failed has nothing safe to retry against.
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

// TestClassifyAttribution pins the pinned check order from discussion.md's
// fork-audit-policy decision: zero new transcripts is always a hard error
// (regardless of report presence — ClassifyAttribution takes no report
// argument at all, which is itself the enforcement), one new is clean, and
// more than one is a warning, never hard.
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
