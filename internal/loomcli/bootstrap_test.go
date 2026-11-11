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

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/statuspane"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The mixed rows catch a predicate that reads only the run lock; the two pure rows alone would pass it.
// LockHeld_NoStrand_NoSpawn covers a driver run by hand: the lock is held but no driver strand exists,
// and the bootstrap must still not spawn a second driver.
// LockFree_LiveStrand_NoSpawn covers a driver between steps.
// It takes the run lock only inside each step, so the lock reads free while the strand is live,
// and the bootstrap must not read that gap as no driver running.
//
//testtiming:keep pins the spawn predicate over the run lock, the driver lock and the live driver strand, including the mixed rows a lock-only predicate fails; its covering tests run the predicate without asserting its table
func TestMustSpawnDriver(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name             string
		runLockHeld      bool
		driverLockHeld   bool
		driverStrandLive bool
		wantSpawn        bool
	}{
		{"AllFree_Spawn", false, false, false, true},
		{"LockHeld_LiveStrand_NoSpawn", true, false, true, false},
		{"LockHeld_NoStrand_NoSpawn", true, false, false, false},
		{"LockFree_LiveStrand_NoSpawn", false, false, true, false},
		{"DriverLockHeld_NoStrand_NoSpawn", false, true, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustSpawnDriver(tt.runLockHeld, tt.driverLockHeld, tt.driverStrandLive)
			if got != tt.wantSpawn {
				t.Errorf("mustSpawnDriver(%v, %v, %v) = %v; want %v", tt.runLockHeld, tt.driverLockHeld, tt.driverStrandLive, got, tt.wantSpawn)
			}
		})
	}
}

// TestResolveDriverStrandAction covers every arm of `resolveDriverStrandAction`.
//
//testtiming:keep pins the driver strand action over every strand shape: none, status-only, live, dead, retiring, full-name and legacy-named; its covering tests resolve it without asserting each arm
func TestResolveDriverStrandAction(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestStartEnvelopeFields_PinsSuccessEnvelope pins the exact key set `lym loom start` prints on success:
// the driver, slug, run id and status file, with no key describing an attach.
func TestStartEnvelopeFields_PinsSuccessEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
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

// countingWait returns a wait seam that counts its own invocations.
// It stands in for a real sleep in the awaitRunLock tests.
func countingWait(count *int) func() {
	return func() {
		*count++
	}
}

// TestAwaitRunLock asserts the handshake's poll order and outcomes.
// A held lock wins over every other signal, then a gone child, then a halted machine;
// only an alive child that never takes the lock while the machine keeps running reaches the deadline.
func TestAwaitRunLock(t *testing.T) {
	tiergate.Require(t, 0)
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
		// The order is load-bearing: a child that took the lock and is about to exit must still be reported ready,
		// so `lockHeld` must win even when the alive check would report false.
		{name: "ready before the alive check, child about to exit", lockHeldOnCall: 1, deadline: 10, want: awaitRunLockReady},
		// The regression guard for a finished driver that is still alive with the lock free.
		// shedengine.Run releases the run lock on return, and the driver then runs the friction reflection agent.
		// That agent can take up to `friction_timeout_min`, against a handshake budget far shorter than that.
		// A finished machine with a live child and a free lock must report halted;
		// otherwise it falls through to the deadline, which dispositionForHandshake refuses.
		// That refusal reports a healthy run as a driver that did not take the run lock.
		// A driver whose machine has finished but whose process still does post-run bookkeeping is not a wedged spawn,
		// and the halt is observable on the first poll.
		{name: "halted while the child is still alive", aliveTrueCalls: -1, halted: true, deadline: 10, want: awaitRunLockHalted},
		// A child that is already gone reports child-died even when the machine is also halted:
		// the more specific signal wins.
		{name: "halted is checked after the alive check", halted: true, deadline: 10, want: awaitRunLockChildDied},
		// An alive child that never takes the lock while the machine stays running is still a wedged spawn.
		// It must still reach the deadline: halted must not make that refusal unreachable.
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
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		sh   shell.Shell
		exe  string
		want string
	}{
		{"Posix", shell.Posix(), "/usr/local/bin/lym", `/usr/local/bin/lym loom checklist`},
		{"Pwsh", shell.Pwsh(), `C:\lym.exe`, ""},
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
			for _, want := range []string{"loom", "checklist"} {
				if !containsSubstring(got, want) {
					t.Errorf("statusStrandCmd(%q) = %q; want it to contain %q", tt.exe, got, want)
				}
			}
		})
	}
}

// containsSubstring is a local helper, so this test file needs no new import for a one-line substring check.
func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestDispositionForHandshake pins that only a deadline refuses the bootstrap.
// The ChildDied row guards a driver that ran and finished.
// Refusing it as a wedged spawn would report the run lock as never taken and withhold the success envelope.
// The Halted row guards a driver whose machine has finished but whose process is still alive:
// it holds the lock free, and it must not be refused either.
func TestDispositionForHandshake(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestResolveStatusStrandAction guards against a status pane that never comes back.
// The DeadEntry row covers a strand whose pane is gone while reed still tracks it.
// Reed keeps such entries after a server restart, a reboot, a crash, a kill-server, or its zombie-boot force-reap.
// A bootstrap that decides by the entry's presence alone reports the pane as already there.
// It never brings the status pane back.
func TestResolveStatusStrandAction(t *testing.T) {
	tiergate.Require(t, 0)
	cur := statuspane.BuildIdentity{Path: "/bin/lym", Size: 10, ModTime: 100}
	matching := &statuspane.Sidecar{GUID: "g0", Build: cur}
	tests := []struct {
		name     string
		strands  []reedengine.StrandStatus
		sidecar  *statuspane.Sidecar
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
			sidecar:  &statuspane.Sidecar{GUID: "g0", Build: statuspane.BuildIdentity{Path: "/bin/lym", Size: 11, ModTime: 100}},
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
			sidecar:  &statuspane.Sidecar{GUID: "other", Build: cur},
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

// driverFieldRead records one `<seedTypedIdent>.Driver` read found by the Driver Choice Single-Site Invariant.
// It holds the file (repo-root-relative, slash-normalized) and the line of the read.
// Its enclosing top-level function is empty outside any function.
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

// scanFileForDriverFieldReads parses one Go source file and returns every "<ident>.Driver" selector.
// An ident counts as seed-typed when the file declares it with type shedrun.Seed.
// It also counts when the file assigns it the first result of a shedrun.ReadSeed call with :=.
//
// It is a tripwire, not a completeness proof:
// it recognizes the patterns the driver-seeding code uses, not every way a diff could alias a shedrun.Seed value.
// A pattern it cannot see is a gap to close by hand when found.
func scanFileForDriverFieldReads(path string) ([]driverFieldRead, error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		// A file that fails to parse outright is not this scan's concern, since go build already catches it.
		return nil, nil //nolint:nilerr
	}
	return driverFieldReadsIn(fset, astFile), nil
}

// driverFieldReadsIn returns every ".Driver" selector in an already-parsed file whose operand is a shedrun.Seed.
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

	// Each hit is stamped with its enclosing top-level declaration's function name,
	// so an allowlist entry can name one reading function rather than a whole file.
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

// driverScanMinFiles is the floor for the number of production .go files the scan reads outside internal/shedrun.
// A scan that reads fewer files than this has walked the wrong tree.
const driverScanMinFiles = 100

// scanRepoForDriverFieldReads walks every production (non-test) .go file in the module.
// It skips internal/shedrun, the only package that parses and writes the seed.
// It returns every driver-field read it finds, each with its repo-root-relative, slash-normalized path.
// It also returns the count of files scanned.
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

// driverFieldReadCarveOuts lists the production functions that may read a seed's Driver field.
// Each entry is keyed "<repo-root-relative file>:<function>" and must lie outside internal/loomcli and internal/shedrun.
// The key names a function rather than a file, so a second reader added to the same file still fails the scan.
//
// Each entry is a deliberate exception, not a widening, and states its justification beside the entry it joins.
// The entry below compares a --driver value the operator typed against the run's recorded one.
// It refuses on the envelope when they differ: a loud, command-line-visible failure.
// Dropping it would restore the silent discard it prevents, a typed --driver ignored against a seeded run.
// It selects no spawn, gates no behaviour, and no producer, engine or generic verb reaches it.
var driverFieldReadCarveOuts = []scankit.Entry{
	{
		Key: "internal/battencli/arm.go:refuseAdoptedSeed",
		Why: "compares a typed --driver against the run's recorded one and refuses on the envelope; selects no spawn and gates no behaviour",
	},
}

// TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField guards the seed's Driver field.
// Only internal/loomcli may read the field outside internal/shedrun, its sole parser and writer.
// The functions listed in driverFieldReadCarveOuts are the only other allowed readers.
// Adding a reader anywhere else fails this test.
// A human then confirms the new site belongs to a recipe's bootstrap verb:
// not a producer, a generic verb, or an engine gating behaviour on the recorded value.
//
//lym:guard
func TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField(t *testing.T) {
	tiergate.Require(t, 0)
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

// TestScanFileForDriverFieldReads_StampsTheEnclosingFunction pins that two readers in one file are told apart:
// by their enclosing function, so an allowlist entry for one never admits the other.
//
//testtiming:keep pins the driver-field scan telling two readers in one file apart by enclosing function, which the carve-out key relies on; the repo-wide scan asserts only that no reader sits outside
func TestScanFileForDriverFieldReads_StampsTheEnclosingFunction(t *testing.T) {
	tiergate.Require(t, 0)
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
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		title       string
		wantDisplay render.Display
	}{
		{"ABareRecipeTitlesTheWindow", "pawl", render.Display{Anchor: render.AnchorOverview, Order: statusStrandOrder, Title: "pawl"}},
		{"AHandleTitledRecipeTitlesTheWindow", "T4: tapestry", render.Display{Anchor: render.AnchorOverview, Order: statusStrandOrder, Title: "T4: tapestry"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := statusStrandAddSpec("watch-cmd", tt.title)
			if got.Display.Order <= orchcli.RunsStrandOrder {
				t.Errorf("Display.Order = %d; want it after the runs strand's %d", got.Display.Order, orchcli.RunsStrandOrder)
			}
			if got.NameOverride != statusStrandDisplayName {
				t.Errorf("NameOverride = %q; want %q", got.NameOverride, statusStrandDisplayName)
			}
			if got.Cmd != "watch-cmd" {
				t.Errorf("Cmd = %q; want %q", got.Cmd, "watch-cmd")
			}
			if got.IfAbsent {
				t.Error("IfAbsent = true; want false")
			}
			if got.Display != tt.wantDisplay {
				t.Errorf("Display = %+v; want %+v", got.Display, tt.wantDisplay)
			}
		})
	}
}

// TestStatusStrandSpec_TitlesTheWindowWithTheRunsHandleAndRecipe pins the spec handed to reed for the status strand:
// the recipe title opened from the run's handle, and the checklist command.
func TestStatusStrandSpec_TitlesTheWindowWithTheRunsHandleAndRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	location := locationkit.Location(t.TempDir(), "run", "")
	boardkit.WriteBoard(t, fabricengine.BoardDir(location.HubPath), `{"id":1,"slug":"run","kind":"task","labels":["bug"],"handle":"T4"}`)
	c := &loomCLI{location: location, recipe: "darn"}

	got := c.statusStrandSpec("/fixed/lym")

	if want := "T4 [P0] darn"; got.Display.Title != want {
		t.Errorf("statusStrandSpec().Display.Title = %q; want %q", got.Display.Title, want)
	}
	if want := statusStrandCmd(shell.ForGOOS(), "/fixed/lym"); got.Cmd != want {
		t.Errorf("statusStrandSpec().Cmd = %q; want %q", got.Cmd, want)
	}
}

// TestStatusWindowTitle pins that the status window opens its recipe with the worktree's console title parts.
// It also pins that the bare recipe is kept when the hub and board name none.
func TestStatusWindowTitle(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// shortname is the hub's recorded shortname; empty records none.
		shortname string
		// entries are the board entries the hub's board holds; none means the hub has no board.
		entries []string
		want    string
	}{
		{"TheRunsHandleOpensTheTitle", "", []string{`{"id":1,"slug":"run","kind":"task","labels":["bug"],"handle":"T4"}`}, "T4 [P0] tapestry"},
		{"TheShortnameAndProjectHandleStandInBrackets", "ly", []string{
			`{"id":1,"slug":"proj","kind":"project","labels":["bug"],"handle":"P3"}`,
			`{"id":2,"slug":"run","kind":"task","labels":["bug"],"project":"proj","handle":"T4"}`,
		}, "T4 [ly:P3] tapestry"},
		{"AnEntryOfAnotherSlugKeepsTheRecipe", "", []string{`{"id":1,"slug":"other","kind":"task","labels":["bug"],"handle":"T4"}`}, "tapestry"},
		{"NoBoardKeepsTheRecipe", "", nil, "tapestry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := locationkit.Location(t.TempDir(), "run", "")
			if len(tt.entries) > 0 {
				boardkit.WriteBoard(t, fabricengine.BoardDir(location.HubPath), tt.entries...)
			}
			if tt.shortname != "" {
				if err := fabricengine.WriteShortname(fabricengine.BoardDir(location.HubPath), tt.shortname); err != nil {
					t.Fatalf("record the shortname: %v", err)
				}
			}
			if got := statusWindowTitle(location, "tapestry"); got != tt.want {
				t.Errorf("statusWindowTitle(run, tapestry) = %q; want %q", got, tt.want)
			}
		})
	}
}
