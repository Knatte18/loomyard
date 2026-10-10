package loomcli

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// TestMustSpawnDriver's two mixed rows are the finding the predicate was widened for: a table
// covering only the two pure rows (both false, both true) passes against the narrow
// single-argument version that read the run lock alone. LockHeld_NoStrand_NoSpawn is the hand-started
// "lyx loom run" case: an operator ran the go driver by hand against an llm-seeded run, so the lock
// is held but no driver strand exists -- the bootstrap must still not spawn a second driver.
// LockFree_LiveStrand_NoSpawn is the driver-between-steps case: the driver session takes the run
// lock only inside each "lyx shed step" and releases it between steps, so the lock reads free while
// the driver strand is live -- the bootstrap must not mistake that gap for "no driver running".
//
//testtiming:keep pins the spawn predicate over both the run lock and the live driver strand, including the two mixed rows a lock-only predicate fails; its covering tests run the predicate without asserting its table
func TestMustSpawnDriver(t *testing.T) {
	tests := []struct {
		name             string
		runLockHeld      bool
		driverStrandLive bool
		wantSpawn        bool
	}{
		{"LockFree_NoStrand_Spawn", false, false, true},
		{"LockHeld_LiveStrand_NoSpawn", true, true, false},
		{"LockHeld_NoStrand_NoSpawn", true, false, false},
		{"LockFree_LiveStrand_NoSpawn", false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustSpawnDriver(tt.runLockHeld, tt.driverStrandLive)
			if got != tt.wantSpawn {
				t.Errorf("mustSpawnDriver(%v, %v) = %v; want %v", tt.runLockHeld, tt.driverStrandLive, got, tt.wantSpawn)
			}
		})
	}
}

// TestResolveDriverStrandAction covers resolveDriverStrandAction's three arms.
//
//testtiming:keep pins the driver strand action over every strand shape: none, status-only, live, dead, retiring, full-name and legacy-named; its covering tests resolve it without asserting each arm
func TestResolveDriverStrandAction(t *testing.T) {
	tests := []struct {
		name     string
		strands  []reedengine.StrandStatus
		want     driverStrandAction
		wantGUID string
	}{
		{
			name:    "NoStrandsAtAll",
			strands: nil,
			want:    driverStrandNone,
		},
		{
			name:    "OnlyOtherStrands",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: statusStrandDisplayName, PaneID: "%1", Live: true}},
			want:    driverStrandNone,
		},
		{
			name:     "LiveDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, PaneID: "%0", Live: true}},
			want:     driverStrandLive,
			wantGUID: "g0",
		},
		{
			name:     "DeadDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, PaneID: "", Live: false}},
			want:     driverStrandDead,
			wantGUID: "g0",
		},
		{
			name:     "LiveRetiringDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, PaneID: "%0", Live: true, Retiring: true}},
			want:     driverStrandRetiring,
			wantGUID: "g0",
		},
		{
			name:     "DeadRetiringDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, PaneID: "", Live: false, Retiring: true}},
			want:     driverStrandDead,
			wantGUID: "g0",
		},
		{
			name:     "LiveFullNameDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: "ly:task:driver", PaneID: "%0", Live: true}},
			want:     driverStrandLive,
			wantGUID: "g0",
		},
		{
			name:     "LiveLegacyDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: loomengine.LegacyLoomDriverStrandName, PaneID: "%0", Live: true}},
			want:     driverStrandLive,
			wantGUID: "g0",
		},
		{
			name:     "DeadLegacyDriverStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: loomengine.LegacyLoomDriverStrandName, PaneID: "", Live: false}},
			want:     driverStrandDead,
			wantGUID: "g0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotGUID := resolveDriverStrandAction(tt.strands)
			if got != tt.want {
				t.Errorf("resolveDriverStrandAction(%+v) action = %v; want %v", tt.strands, got, tt.want)
			}
			if gotGUID != tt.wantGUID {
				t.Errorf("resolveDriverStrandAction(%+v) guid = %q; want %q", tt.strands, gotGUID, tt.wantGUID)
			}
		})
	}
}

// TestStartEnvelopeFields_PinsSuccessEnvelope pins the exact key set `lyx loom start` prints on success:
// the driver, slug, run id and status file, with no key describing an attach.
func TestStartEnvelopeFields_PinsSuccessEnvelope(t *testing.T) {
	t.Parallel()

	want := map[string]any{
		"driver":      "go",
		"slug":        "slug",
		"run_id":      "slug",
		"status_file": "/s/status.json",
	}
	if diff := cmp.Diff(want, startEnvelopeFields("go", "slug", "slug", "/s/status.json")); diff != "" {
		t.Errorf("startEnvelopeFields mismatch (-want +got):\n%s", diff)
	}
}

// countingWait returns a wait seam that counts its own invocations, for use in place of a real
// sleep in awaitRunLock tests.
func countingWait(count *int) func() {
	return func() {
		*count++
	}
}

// TestAwaitRunLock asserts the handshake's poll order and outcomes: lock held wins over everything, then a gone child, then a halted machine, and only an alive child that never takes the lock while the machine keeps running reaches the deadline.
func TestAwaitRunLock(t *testing.T) {
	tests := []struct {
		name string
		// lockHeldOnCall is the poll on which the lock reads held; zero never.
		lockHeldOnCall int
		lockErr        error
		// aliveTrueCalls is how many alive polls answer true before the child reads gone; negative is always alive.
		aliveTrueCalls int
		halted         bool
		deadline       int
		want           awaitRunLockResult
		wantErr        bool
		wantWaits      int
	}{
		{name: "ready on a later iteration", lockHeldOnCall: 3, aliveTrueCalls: -1, deadline: 10, want: awaitRunLockReady, wantWaits: 2},
		{name: "child died", aliveTrueCalls: 1, deadline: 10, want: awaitRunLockChildDied, wantWaits: 1},
		{name: "deadline", aliveTrueCalls: -1, deadline: 5, want: awaitRunLockDeadline, wantWaits: 5},
		{name: "lock seam errors", lockErr: errors.New("boom"), aliveTrueCalls: -1, deadline: 10, wantErr: true},
		// The order is load-bearing: a child that took the lock and is about to exit must still be
		// reported ready, so lockHeld reporting true must win even when alive would report false.
		{name: "ready before the alive check, child about to exit", lockHeldOnCall: 1, deadline: 10, want: awaitRunLockReady},
		// The regression guard for the defect Tier 2 introduced in `lyx loom start`'s handshake.
		// shedengine.Run releases the run lock on return, and `lyx loom run` then spends up to
		// friction_timeout_min -- thirty minutes in the shipped template -- running the friction
		// reflection agent, against a handshake budget of thirty seconds. Before the halted seam existed,
		// that combination (lock free, child alive, machine finished) fell through to awaitRunLockDeadline,
		// which dispositionForHandshake refuses: a healthy run was reported as "driver did not take the run
		// lock" and the bootstrap refused.
		// A driver whose machine finished but whose process is still doing post-run bookkeeping is not a wedged spawn, and the halt is observable on the first poll.
		{name: "halted while the child is still alive", aliveTrueCalls: -1, halted: true, deadline: 10, want: awaitRunLockHalted},
		// The seam order: a child that is already gone reports child-died even when halted would also
		// report true, so the more specific signal wins and the child-died scenario keeps its meaning.
		{name: "halted is checked after the alive check", halted: true, deadline: 10, want: awaitRunLockChildDied},
		// The other half of the guard: the halted seam must not make the genuine refusal unreachable.
		// A child that is alive, never takes the lock, and leaves the machine in running is still a
		// wedged spawn and must still hit the deadline.
		{name: "still running with a live child still reaches the deadline", aliveTrueCalls: -1, deadline: 4, want: awaitRunLockDeadline, wantWaits: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockCalls := 0
			lockHeld := func() (bool, error) {
				lockCalls++
				return tt.lockHeldOnCall > 0 && lockCalls >= tt.lockHeldOnCall, tt.lockErr
			}
			aliveCalls := 0
			alive := func() bool {
				aliveCalls++
				return tt.aliveTrueCalls < 0 || aliveCalls <= tt.aliveTrueCalls
			}
			halted := func() bool { return tt.halted }
			waits := 0

			got, err := awaitRunLock(lockHeld, alive, halted, countingWait(&waits), tt.deadline)

			if tt.wantErr {
				if !errors.Is(err, tt.lockErr) {
					t.Errorf("awaitRunLock() error = %v; want %v", err, tt.lockErr)
				}
			} else if err != nil {
				t.Fatalf("awaitRunLock() unexpected error: %v", err)
			} else if got != tt.want {
				t.Errorf("awaitRunLock() = %v; want %v", got, tt.want)
			}
			if waits != tt.wantWaits {
				t.Errorf("awaitRunLock() waited %d times; want %d", waits, tt.wantWaits)
			}
		})
	}
}

//testtiming:keep pins the status strand lookup by exact name and by full agent name, and a name that is only a prefix not matching; its covering tests resolve the strand action without asserting the lookup
func TestFindStatusStrand(t *testing.T) {
	strands := []reedengine.StrandStatus{
		{GUID: "g1", Name: "loom-status-extra", PaneID: "%1"},
		{GUID: "g2", Name: statusStrandDisplayName, PaneID: "%2"},
	}

	tests := []struct {
		name      string
		strands   []reedengine.StrandStatus
		want      string
		wantFound bool
	}{
		{"Hit", strands, "loom-status", true},
		{"Missing", strands, "no-such-strand", false},
		{"ExactNameOverPrefix", strands, statusStrandDisplayName, true},
	}
	t.Run("FullName", func(t *testing.T) {
		got, found := findStatusStrand([]reedengine.StrandStatus{{GUID: "g3", Name: "ly:task:loom-status"}}, statusStrandDisplayName)
		if !found || got.GUID != "g3" {
			t.Errorf("findStatusStrand over a full name = %+v, %v; want g3, true", got, found)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := findStatusStrand(tt.strands, tt.want)
			if found != tt.wantFound {
				t.Fatalf("findStatusStrand(..., %q) found = %v; want %v", tt.want, found, tt.wantFound)
			}
			if found && got.Name != tt.want {
				t.Errorf("findStatusStrand(..., %q) = %+v; want Name %q", tt.want, got, tt.want)
			}
		})
	}
}

func TestStatusStrandCmd(t *testing.T) {
	tests := []struct {
		name string
		sh   shell.Shell
		exe  string
		want string
	}{
		{"Posix", shell.Posix(), "/usr/local/bin/lyx", `/usr/local/bin/lyx loom status --watch`},
		{"Pwsh", shell.Pwsh(), `C:\lyx.exe`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusStrandCmd(tt.sh, tt.exe)
			wantPrefix := tt.sh.Invoke(tt.exe)
			if got == "" {
				t.Fatalf("statusStrandCmd(%q) = empty string", tt.exe)
			}
			if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
				t.Errorf("statusStrandCmd(%q) = %q; want prefix %q", tt.exe, got, wantPrefix)
			}
			for _, want := range []string{"loom", "status", "--watch"} {
				if !containsSubstring(got, want) {
					t.Errorf("statusStrandCmd(%q) = %q; want it to contain %q", tt.exe, got, want)
				}
			}
		})
	}
}

// containsSubstring is a tiny local helper so this test file adds no new import for a one-line
// substring check.
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestDispositionForHandshake pins that only a deadline refuses the bootstrap.
// The ChildDied row is the original regression guard: the verb used to test
// `result != awaitRunLockReady`, which collapsed a driver that ran and finished into the same
// refusal as a wedged spawn, reported "driver did not take the run lock" for a run that had taken it
// and released it, and withheld the success envelope.
// The Halted row guards the same failure arriving through Tier 2's door: a driver whose machine has
// finished but whose process is still running the friction reflection agent is alive with the lock
// free, which used to land on the refusal below.
func TestDispositionForHandshake(t *testing.T) {
	tests := []struct {
		name   string
		result awaitRunLockResult
		want   handshakeDisposition
	}{
		{"Ready", awaitRunLockReady, handshakeProceed},
		{"ChildDied", awaitRunLockChildDied, handshakeProceed},
		{"Halted", awaitRunLockHalted, handshakeProceed},
		{"Deadline", awaitRunLockDeadline, handshakeRefuse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dispositionForHandshake(tt.result); got != tt.want {
				t.Errorf("dispositionForHandshake(%v) = %v; want %v", tt.result, got, tt.want)
			}
		})
	}
}

// TestResolveStatusStrandAction is the regression guard for a status pane that never came back.
// The DeadEntry row is the defect: the bootstrap used to decide by presence alone, and reed keeps
// tracking a strand whose pane is gone, so after any reed server restart -- a reboot, a crash, a
// kill-server, or reed's own zombie-boot force-reap -- every subsequent "lyx loom start" in that
// worktree saw the stale "loom-status" entry, reported "already there", and left the operator with
// no status read-out at all.
func TestResolveStatusStrandAction(t *testing.T) {
	cur := buildIdentity{Path: "/bin/lyx", Size: 10, ModTime: 100}
	matching := &statusSidecar{GUID: "g0", Build: cur}
	tests := []struct {
		name     string
		strands  []reedengine.StrandStatus
		sidecar  *statusSidecar
		want     statusStrandAction
		wantGUID string
	}{
		{
			name:    "NoStrandsAtAll",
			strands: nil,
			want:    statusStrandAdd,
		},
		{
			name:    "OnlyOtherStrands",
			strands: []reedengine.StrandStatus{{GUID: "g1", Name: "discussion::g1", PaneID: "%2", Live: true}},
			want:    statusStrandAdd,
		},
		{
			name:     "LiveStatusStrand",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true}},
			sidecar:  matching,
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithClearedPaneBinding",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "", Live: false}},
			sidecar:  matching,
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithADeadPane",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: false}},
			sidecar:  matching,
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name: "LiveStatusStrandAmongOthers",
			strands: []reedengine.StrandStatus{
				{GUID: "g1", Name: "plan::g1", PaneID: "%3", Live: true},
				{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true},
			},
			sidecar:  matching,
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
		{
			name:     "LiveStrandDifferentBuildReplaces",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true}},
			sidecar:  &statusSidecar{GUID: "g0", Build: buildIdentity{Path: "/bin/lyx", Size: 11, ModTime: 100}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name:     "LiveStrandNoSidecarReplaces",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name:     "LiveStrandSidecarNamesOtherGUIDReplaces",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true}},
			sidecar:  &statusSidecar{GUID: "other", Build: cur},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotGUID := resolveStatusStrandAction(tt.strands, tt.sidecar, cur)
			if got != tt.want {
				t.Errorf("resolveStatusStrandAction(%+v) action = %v; want %v", tt.strands, got, tt.want)
			}
			if gotGUID != tt.wantGUID {
				t.Errorf("resolveStatusStrandAction(%+v) guid = %q; want %q", tt.strands, gotGUID, tt.wantGUID)
			}
		})
	}
}

// driverFieldRead records one `<seedTypedIdent>.Driver` field read the Driver Choice Single-Site
// Invariant's scan found, at the file (repo-root-relative, slash-normalized), enclosing top-level
// function (empty outside any function), and line it occurred at.
type driverFieldRead struct {
	relPath  string
	function string
	line     int
}

// isShedrunSeedTypeExpr reports whether expr is the type expression "shedrun.Seed".
func isShedrunSeedTypeExpr(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "shedrun" && sel.Sel.Name == "Seed"
}

// isShedrunReadSeedCall reports whether expr is a call to shedrun.ReadSeed.
func isShedrunReadSeedCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "shedrun" && sel.Sel.Name == "ReadSeed"
}

// scanFileForDriverFieldReads parses one Go source file and returns every "<ident>.Driver" selector
// where ident is, within that same file, either a function parameter typed shedrun.Seed, a var/const
// declared with that explicit type, or a short-var-decl target of a shedrun.ReadSeed(...) call's
// first return value.
//
// This is a tripwire, not a completeness proof, in the same sense the Completion Signal Invariant's
// own scan is: it recognizes the concrete patterns this codebase's own driver-seeding code uses, not
// every conceivable way a future diff could alias a shedrun.Seed value. A pattern it cannot see is a
// gap to close by hand when found, not a promise this scan already keeps.
func scanFileForDriverFieldReads(path string) ([]driverFieldRead, error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		// A file that fails to parse outright is not this scan's concern -- go build already
		// catches that.
		return nil, nil //nolint:nilerr
	}
	return driverFieldReadsIn(fset, astFile), nil
}

// driverFieldReadsIn is scanFileForDriverFieldReads' matcher over an already-parsed file.
func driverFieldReadsIn(fset *token.FileSet, astFile *ast.File) []driverFieldRead {
	seedTyped := map[string]bool{}
	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Type.Params != nil {
				for _, field := range node.Type.Params.List {
					if !isShedrunSeedTypeExpr(field.Type) {
						continue
					}
					for _, name := range field.Names {
						seedTyped[name.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			if isShedrunSeedTypeExpr(node.Type) {
				for _, name := range node.Names {
					seedTyped[name.Name] = true
				}
			}
		case *ast.AssignStmt:
			for i, rhs := range node.Rhs {
				if i >= len(node.Lhs) || !isShedrunReadSeedCall(rhs) {
					continue
				}
				if ident, ok := node.Lhs[i].(*ast.Ident); ok {
					seedTyped[ident.Name] = true
				}
			}
		}
		return true
	})

	// Each hit is stamped with its enclosing top-level declaration's function name, so a carve-out
	// can name one reading function rather than a whole file.
	var found []driverFieldRead
	for _, decl := range astFile.Decls {
		function := ""
		if fn, ok := decl.(*ast.FuncDecl); ok {
			function = fn.Name.Name
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Driver" {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || !seedTyped[ident.Name] {
				return true
			}
			found = append(found, driverFieldRead{function: function, line: fset.Position(sel.Pos()).Line})
			return true
		})
	}
	return found
}

// driverScanMinFiles is the plausible floor for how many production .go files the module holds outside internal/shedrun;
// below it the walk has read the wrong tree.
const driverScanMinFiles = 100

// scanRepoForDriverFieldReads walks every production (non-test) .go file in the module, skipping internal/shedrun (the sole legitimate parser and writer of the Seed struct, per the Shed Run-Directory Invariant),
// and returns every driver-field read driverFieldReadsIn finds, each stamped with its repo-root-relative, slash-normalized path, together with the count of files scanned.
func scanRepoForDriverFieldReads(t *testing.T) ([]driverFieldRead, int) {
	t.Helper()
	var all []driverFieldRead

	scanned := scankit.Walk(t, scankit.Options{}, func(f *scankit.File) {
		if strings.HasPrefix(f.Rel, "internal/shedrun/") {
			return
		}
		astFile := f.AST(t, 0)
		for _, r := range driverFieldReadsIn(f.FileSet(), astFile) {
			r.relPath = f.Rel
			all = append(all, r)
		}
	})
	return all, scanned
}

// driverFieldReadCarveOuts are the production functions, outside internal/loomcli and
// internal/shedrun, that may read a seed's Driver field, each keyed "<repo-root-relative file>:<function>".
// The key names a function rather than a file, so a second reader added anywhere else in the same
// file still fails the scan.
//
// Exactly one entry, and it is a deliberate carve-out rather than a widening: internal/battencli's
// refuseAdoptedSeed compares a --driver value the operator just typed against the one the addressed
// run is already seeded with, and refuses on the envelope when they disagree. That is the loud,
// command-line-visible failure the Driver Choice Single-Site Invariant's own rationale prefers, not
// the silent divergence it is written against -- the comparison selects no spawn, gates no
// behaviour, and is unreachable from any producer, engine, or generic verb. Dropping it would
// restore the defect it was added for: silently discarding a typed --driver against a seeded run.
//
// Every other new entry needs the same explicit justification here, next to the one it joins.
var driverFieldReadCarveOuts = []scankit.Entry{
	{
		Key: "internal/battencli/arm.go:refuseAdoptedSeed",
		Why: "compares a typed --driver against the run's recorded one and refuses on the envelope; selects no spawn and gates no behaviour",
	},
}

// TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField is the Driver Choice
// Single-Site Invariant's tripwire: the only production readers of the seed's Driver field outside
// internal/shedrun (the field's sole parser/writer) must be internal/loomcli, that recipe's own
// bootstrap package, and the functions named in driverFieldReadCarveOuts. Adding a reader anywhere else
// fails this test and forces a human to confirm the new site really belongs to a recipe's bootstrap
// verb rather than a producer, a generic verb, or an engine gating behaviour on the recorded value.
//
//lyx:guard
func TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField(t *testing.T) {
	found, scanned := scanRepoForDriverFieldReads(t)
	scankit.RequireFloor(t, scanned, driverScanMinFiles, "driver-field-read scan")

	carveOuts := scankit.NewAllowlist(driverFieldReadCarveOuts)
	var outside []driverFieldRead
	sawLoomcli := false
	for _, f := range found {
		if strings.HasPrefix(f.relPath, "internal/loomcli/") {
			sawLoomcli = true
			continue
		}
		if carveOuts.Allowed(f.relPath + ":" + f.function) {
			continue
		}
		outside = append(outside, f)
	}
	carveOuts.RequireNoStale(t)

	if len(outside) > 0 {
		locs := make([]string, 0, len(outside))
		for _, f := range outside {
			locs = append(locs, fmt.Sprintf("%s:%d", f.relPath, f.line))
		}
		sort.Strings(locs)
		t.Errorf("found a production reader of shedrun.Seed's Driver field outside internal/loomcli and internal/shedrun: %s -- per the Driver Choice Single-Site Invariant, a recorded seed driver value is read in exactly one place per recipe, that recipe's own bootstrap verb; review the new site and, if it is legitimately a second recipe's own bootstrap package, extend this scan's allowlist deliberately rather than widen it silently", strings.Join(locs, ", "))
	}
	if !sawLoomcli {
		t.Error("the scan found no driver-field reader inside internal/loomcli at all -- want at least sharedbootstrap.go's resolveSeedDriver to be found; either that site moved or the scan itself has stopped matching real code")
	}
}

// TestScanFileForDriverFieldReads_StampsTheEnclosingFunction pins the granularity the carve-out
// relies on: two readers in one file are told apart by their enclosing function, so a carve-out for
// one never admits the other.
//
//testtiming:keep pins the driver-field scan telling two readers in one file apart by enclosing function, which the carve-out key relies on; the repo-wide scan asserts only that no reader sits outside
func TestScanFileForDriverFieldReads_StampsTheEnclosingFunction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two_readers.go")
	src := `package fixture

import "github.com/Knatte18/loomyard/internal/shedrun"

func allowedReader(seed shedrun.Seed) string { return seed.Driver }

func plantedGate(seed shedrun.Seed) bool { return seed.Driver == shedrun.DriverLLM }
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := scanFileForDriverFieldReads(path)
	if err != nil {
		t.Fatalf("scanFileForDriverFieldReads() error = %v; want nil", err)
	}
	var functions []string
	for _, f := range found {
		functions = append(functions, f.function)
	}
	sort.Strings(functions)
	if got, want := strings.Join(functions, ","), "allowedReader,plantedGate"; got != want {
		t.Errorf("scanFileForDriverFieldReads() functions = %q; want %q", got, want)
	}
}

func TestStatusStrandAddSpec(t *testing.T) {
	t.Run("fields", func(t *testing.T) {
		got := statusStrandAddSpec("watch-cmd")
		if got.NameOverride != statusStrandDisplayName {
			t.Errorf("NameOverride = %q; want %q", got.NameOverride, statusStrandDisplayName)
		}
		if got.Cmd != "watch-cmd" {
			t.Errorf("Cmd = %q; want %q", got.Cmd, "watch-cmd")
		}
		if got.IfAbsent {
			t.Error("IfAbsent = true; want false")
		}
		if got.Display.Anchor != render.AnchorBelowParent {
			t.Errorf("Display.Anchor = %v; want AnchorBelowParent", got.Display.Anchor)
		}
		if got.Display.Focus {
			t.Error("Display.Focus = true; want false")
		}
	})

	// The spec ties to render's layout: stacked above a driver strand the status strand is the
	// collapsed placement and is pinned at collapsed_rows, and alone it is the bottom-most strand and
	// is not pinned. Both strands are parentless, as the bootstrap adds them; the layout rule sizes
	// them by insertion position alone.
	t.Run("pinned by render", func(t *testing.T) {
		status := render.Strand{GUID: "s", Display: statusStrandAddSpec("x").Display, PaneID: "%1", Live: true}
		driver := render.Strand{
			GUID:    "d",
			Display: driverSpec("p", "r", loomengine.DriverSettings{}).Display,
			PaneID:  "%2",
			Live:    true,
		}
		box := render.Box{W: 200, H: 50}
		params := render.Params{CollapsedRows: 2, MinFullRows: 3}

		pins := render.FixedHeightPins([]render.Strand{status, driver}, box, params)
		want := []render.Pin{{PaneID: "%1", Height: params.CollapsedRows}}
		if diff := cmp.Diff(want, pins); diff != "" {
			t.Errorf("pins mismatch (-want +got):\n%s", diff)
		}

		pins = render.FixedHeightPins([]render.Strand{status}, box, params)
		if len(pins) != 0 {
			t.Errorf("lone status strand pins = %v; want none", pins)
		}
	})
}

