// audit_test.go table-drives webster's own fork-audit policy over the full violation taxonomy
// CheckFork/CheckParent enforce, the fabricengine.RefScanner matcher (built from a fake
// lyxcwd.Location, never a hardcoded geometry token), and SettleRetry with a recording fake Sleeper.
// Every case here is a pure fact-in/verdict-out table, per the discussion's TDD-centre framing: no
// git spawn, no real sleeping, no filesystem I/O.

package websterengine

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fakeLayout returns a lyxcwd.Location that resolves fabricengine.RecordsWorktree() without spawning git.
func fakeLayout() *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: "/hub", WorktreeName: filepath.Base("/hub/master-builder")}
}

// TestRefScannerMatches matrixes fabricengine.NewRefScanner against every Bash command shape
// CheckFork/CheckParent must classify: `lyx fabric` invocations (the live spelling the Fabric Git
// Invariant bans), the retired pre-cutover per-side spellings, a command referencing the fabric
// worktree path directly (e.g. `git -C <fabric-worktree> add`), and a set of fabric-free commands that
// must never match.
// The `lyx fabric` rows are the regression guard: the fabric cutover deleted the per-side verbs
// and renamed every fabric-touching verb under `lyx fabric`, so a matcher that knows only the old
// spellings bans nothing an agent can actually run today.
// The `lyx.exe` rows are the same guard for the Windows spelling — lyx's primary platform, where an
// agent writing the extension out would otherwise slip the whole audit — paired with a `lyx.exe
// board` row proving the extension did not widen the match to every lyx invocation.
func TestRefScannerMatches(t *testing.T) {
	layout := fakeLayout()
	fabricRef := fabricengine.NewRefScanner(layout)
	fabricWorktree := fabricengine.RecordsWorktree(layout)

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"lyx fabric sync", "lyx fabric sync", true},
		{"lyx fabric commit", "lyx fabric commit", true},
		{"lyx fabric push", "lyx fabric push", true},
		{"lyx fabric checkout", "lyx fabric checkout feature", true},
		{"lyx fabric with leading prose", "cd /hub/pair && lyx fabric sync", true},
		{"lyx weft sync", "lyx weft sync", true},
		{"lyx warp checkout", "lyx warp checkout feature", true},
		{"lyx.exe fabric sync", "lyx.exe fabric sync", true},
		{"lyx.exe weft push", "lyx.exe weft push", true},
		{"absolute lyx.exe fabric push", `C:\bin\lyx.exe fabric push`, true},
		{"git -C fabric-worktree add", "git -C " + fabricWorktree + " add -A", true},
		{"cd into fabric worktree", "cd " + fabricWorktree + " && git status", true},
		{"code-side git commit is not a fabric reference", "git commit -am wip", false},
		{"plain read", "cat notes.txt", false},
		{"code-side status", "git status", false},
		{"unrelated path", "cat /hub/other-repo/README.md", false},
		{"a fabric-named file is not a lyx fabric invocation", "cat fabric-notes.md", false},
		{"lyx board is not a fabric reference", "lyx board list", false},
		{"lyx.exe board is not a fabric reference either", "lyx.exe board list", false},
		{"a docs grep for the fabric verb is not a fabric reference", `grep -n "fabric remove\|lyx fabric.*remove" docs/overview.md`, false},
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
//
//testtiming:keep pins the violation class and path for every write-path and command shape a fork can produce, including the allowed ones; the covering record-batch table reaches a few shapes through whole calls
func TestCheckFork(t *testing.T) {
	layout := fakeLayout()
	fabricRef := fabricengine.NewRefScanner(layout)
	fabricWorktree := fabricengine.RecordsWorktree(layout)

	tests := []struct {
		name string
		fork shuttleengine.ForkReport
		// workdir is the audit workdir a relative write path resolves against; empty means the anchor root.
		workdir     string
		wantClasses []AuditViolationClass
		wantPath    string
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
			name: "code-side git commit is allowed (per-card commits are the contract)",
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
			name: "the retired per-side sync spelling is a hard error",
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
			// The workdir is a subdirectory of the anchor root here, so the relative path names a file
			// that is no contract file: a resolution against the anchor root instead would flag it.
			name: "a relative write resolves against the audit workdir, not the anchor root",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "g2", ReportReturned: true,
				WritePaths: []string{"_lyx/webster/outcome.yaml"},
			},
			workdir:     "/hub/master-builder/_worktrees/fork-w1",
			wantClasses: nil,
		},
		{
			name: "absolute fork write under the plan directory is a plan write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "p1", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/plan/03-x.md"},
			},
			wantClasses: []AuditViolationClass{ClassForkPlanWrite},
			wantPath:    "/hub/master-builder/_lyx/plan/03-x.md",
		},
		{
			name: "relative fork write under the plan directory is a plan write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "p2", ReportReturned: true,
				WritePaths: []string{"_lyx/plan/03-x.md"},
			},
			wantClasses: []AuditViolationClass{ClassForkPlanWrite},
			wantPath:    "_lyx/plan/03-x.md",
		},
		{
			name: "fork write through the second plan spelling is a plan write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "p3", ReportReturned: true,
				WritePaths: []string{"/fabric/records/plan/03-x.md"},
			},
			wantClasses: []AuditViolationClass{ClassForkPlanWrite},
			wantPath:    "/fabric/records/plan/03-x.md",
		},
		{
			name: "fork write to a worktree source file is no plan write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "p4", ReportReturned: true,
				WritePaths: []string{"internal/foo/foo.go"},
			},
			wantClasses: nil,
		},
		{
			name: "two plan writes in one transcript are two findings",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "p5", ReportReturned: true,
				WritePaths: []string{"_lyx/plan/03-x.md", "_lyx/plan/04-y.md"},
			},
			wantClasses: []AuditViolationClass{ClassForkPlanWrite, ClassForkPlanWrite},
		},
		{
			name: "fork write to its own batch report stays allowed",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "h", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/reports/01-json-flag.yaml"},
			},
			wantClasses: nil,
		},
		{
			name: "fork write to state.json is a state write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "s1", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/state.json"},
			},
			wantClasses: []AuditViolationClass{ClassForkStateWrite},
			wantPath:    "/hub/master-builder/_lyx/webster/state.json",
		},
		{
			name: "fork write to another batch's report is a state write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "s2", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/reports/02-other.yaml"},
			},
			wantClasses: []AuditViolationClass{ClassForkStateWrite},
		},
		{
			name: "fork write through the second webster spelling is a state write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "s3", ReportReturned: true,
				WritePaths: []string{"/fabric/records/webster/state.json"},
			},
			wantClasses: []AuditViolationClass{ClassForkStateWrite},
		},
		{
			name: "fork write to outcome.yaml is only a contract write, not also a state write",
			fork: shuttleengine.ForkReport{
				TranscriptPath: "s4", ReportReturned: true,
				WritePaths: []string{"/hub/master-builder/_lyx/webster/outcome.yaml"},
			},
			wantClasses: []AuditViolationClass{ClassForkContractWrite},
		},
	}

	const outcomePath = "/hub/master-builder/_lyx/webster/outcome.yaml"
	const summaryPath = "/hub/master-builder/_lyx/webster/summary.md"
	const ownReport = "/hub/master-builder/_lyx/webster/reports/01-json-flag.yaml"
	planDirs := []string{"/hub/master-builder/_lyx/plan", "/fabric/records/plan"}
	websterDirs := []string{"/hub/master-builder/_lyx/webster", "/fabric/records/webster"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workdir := tt.workdir
			if workdir == "" {
				workdir = "/hub/master-builder"
			}
			got := CheckFork(tt.fork, outcomePath, summaryPath, workdir, planDirs, websterDirs, ownReport, fabricRef)
			if len(got) != len(tt.wantClasses) {
				t.Fatalf("CheckFork() = %v; want %d violation(s) of class %v", got, len(tt.wantClasses), tt.wantClasses)
			}
			if tt.wantPath != "" && got[0].Path != tt.wantPath {
				t.Errorf("CheckFork()[0].Path = %q; want %q", got[0].Path, tt.wantPath)
			}
			if tt.name == "two plan writes in one transcript are two findings" && got[0].Key == got[1].Key {
				t.Errorf("CheckFork() keys = %q and %q; want distinct", got[0].Key, got[1].Key)
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
	fabricWorktree := fabricengine.RecordsWorktree(layout)

	const outcomePath = "/hub/master-builder/_lyx/webster/outcome.yaml"
	const summaryPath = "/hub/master-builder/_lyx/webster/summary.md"

	tests := []struct {
		name  string
		audit shuttleengine.ForkAudit
		// workdir is the audit workdir a relative write path resolves against; empty means the anchor root.
		workdir     string
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
			// The workdir is a subdirectory of the anchor root here, so the relative contract-file
			// spelling names another file: a resolution against the anchor root instead would allow it.
			name: "a relative write resolves against the audit workdir, not the anchor root",
			audit: shuttleengine.ForkAudit{
				ParentWrites: []string{"_lyx/webster/outcome.yaml"},
			},
			workdir:     "/hub/master-builder/_worktrees/fork-w1",
			wantClasses: []AuditViolationClass{ClassParentWrite},
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
			workdir := tt.workdir
			if workdir == "" {
				workdir = "/hub/master-builder"
			}
			got := CheckParent(tt.audit, outcomePath, summaryPath, workdir, fabricRef)
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

// recordingSleeper is a Sleeper that never blocks: it only records each requested duration, so
// SettleRetry's retry loop runs its scripted attempts at no wall-clock cost.
type recordingSleeper struct {
	slept []time.Duration
}

func (s *recordingSleeper) Sleep(d time.Duration) {
	s.slept = append(s.slept, d)
}

// TestSettleRetry pins SettleRetry's contract: a transcript appearing after the first tick returns
// at once without waiting out the window, a window with no new transcript ends after window/tick
// sleeps with no error of its own, and a fetch error returns at once with no retry.
//
//testtiming:keep pins the exact fetch and sleep counts for an early hit and an exhausted window, and no retry after a fetch error; the record-batch tests only observe that a tick happened
func TestSettleRetry(t *testing.T) {
	t.Parallel()
	fetchErr := errors.New("boom")

	tests := []struct {
		name string
		// script is the fetch result per call, the last one repeating once exhausted.
		script       []shuttleengine.ForkAudit
		fetchErr     error
		window, tick time.Duration
		wantNew      []string
		wantCalls    int
		wantSleeps   []time.Duration
	}{
		{
			name:       "a transcript on a later tick returns at once",
			script:     []shuttleengine.ForkAudit{{}, {Forks: []shuttleengine.ForkReport{cleanForkReport("fork-2")}}},
			window:     DefaultSettleWindow,
			tick:       DefaultSettleTick,
			wantNew:    []string{"fork-2"},
			wantCalls:  2,
			wantSleeps: []time.Duration{DefaultSettleTick},
		},
		{
			name:       "an exhausted window returns no new transcript and no error",
			script:     []shuttleengine.ForkAudit{{}},
			window:     time.Second,
			tick:       250 * time.Millisecond,
			wantCalls:  5,
			wantSleeps: []time.Duration{250 * time.Millisecond, 250 * time.Millisecond, 250 * time.Millisecond, 250 * time.Millisecond},
		},
		{
			name:      "a fetch error returns at once with no retry",
			fetchErr:  fetchErr,
			window:    DefaultSettleWindow,
			tick:      DefaultSettleTick,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			fetch := func() (shuttleengine.ForkAudit, error) {
				calls++
				if tt.fetchErr != nil {
					return shuttleengine.ForkAudit{}, tt.fetchErr
				}
				return tt.script[min(calls, len(tt.script))-1], nil
			}

			sleeper := &recordingSleeper{}
			audit, newReports, err := SettleRetry(fetch, nil, tt.window, tt.tick, sleeper)
			if !errors.Is(err, tt.fetchErr) {
				t.Fatalf("SettleRetry() error = %v; want %v", err, tt.fetchErr)
			}
			var gotNew []string
			for _, r := range newReports {
				gotNew = append(gotNew, r.TranscriptPath)
			}
			if !slices.Equal(gotNew, tt.wantNew) {
				t.Errorf("SettleRetry() newReports = %v; want %v", gotNew, tt.wantNew)
			}
			if len(tt.wantNew) > 0 && len(audit.Forks) != len(tt.wantNew) {
				t.Errorf("SettleRetry() audit.Forks = %v; want exactly the returned forks", audit.Forks)
			}
			if calls != tt.wantCalls {
				t.Errorf("fetch called %d time(s); want %d", calls, tt.wantCalls)
			}
			if !slices.Equal(sleeper.slept, tt.wantSleeps) {
				t.Errorf("sleeper.slept = %v; want %v", sleeper.slept, tt.wantSleeps)
			}
		})
	}
}
