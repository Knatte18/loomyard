package loomcli

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// TestMustSpawnDriver's two mixed rows are the finding the predicate was widened for: a table
// covering only the two pure rows (both false, both true) passes against the narrow
// single-argument version that read the run lock alone. LockHeld_NoStrand_NoSpawn is the hand-started
// "lyx loom run" case: an operator ran the go driver by hand against an llm-seeded run, so the lock
// is held but no driver strand exists -- the bootstrap must still not spawn a second driver.
// LockFree_LiveStrand_NoSpawn is the ly-drive-between-steps case: the driver session takes the run
// lock only inside each "lyx shed step" and releases it between steps, so the lock reads free while
// the driver strand is live -- the bootstrap must not mistake that gap for "no driver running".
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

// TestDriverStrandDisplayName_AddAndLookupAgree pins that the name used to add the driver strand and
// the name looked up are the same constant, in the shape statusStrandDisplayName and
// operatorStrandDisplayName are already pinned: reed's add has no upsert semantics, so a mismatch
// between the two would stack a second pane rather than match the first.
func TestDriverStrandDisplayName_AddAndLookupAgree(t *testing.T) {
	strands := []reedengine.StrandStatus{{GUID: "g0", Name: driverStrandDisplayName, PaneID: "%0", Live: true}}
	action, guid := resolveDriverStrandAction(strands)
	if action != driverStrandLive {
		t.Fatalf("resolveDriverStrandAction found no match for driverStrandDisplayName %q; the add and lookup names have diverged", driverStrandDisplayName)
	}
	if guid != "g0" {
		t.Errorf("resolveDriverStrandAction(%+v) guid = %q; want %q", strands, guid, "g0")
	}
}

// TestDriverStrandDisplayName_DiffersFromOtherStrandNames guards against a name collision with
// either of the other two pinned strand names, which would append a second pane rather than replace
// the first (reed's add has no upsert semantics).
func TestDriverStrandDisplayName_DiffersFromOtherStrandNames(t *testing.T) {
	if driverStrandDisplayName == statusStrandDisplayName {
		t.Errorf("driverStrandDisplayName and statusStrandDisplayName are both %q; want distinct names", driverStrandDisplayName)
	}
	if driverStrandDisplayName == operatorStrandDisplayName {
		t.Errorf("driverStrandDisplayName and operatorStrandDisplayName are both %q; want distinct names", driverStrandDisplayName)
	}
}

func TestMustAttach(t *testing.T) {
	tests := []struct {
		name       string
		noAttach   bool
		wantAttach bool
	}{
		{"NoAttachSet_NoAttach", true, false},
		{"NoAttachUnset_Attach", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustAttach(tt.noAttach)
			if got != tt.wantAttach {
				t.Errorf("mustAttach(%v) = %v; want %v", tt.noAttach, got, tt.wantAttach)
			}
		})
	}
}

// TestStartVerb_NoAttachFlag_DefaultsFalse pins the regression this flag most plausibly causes: a
// silently flipped default. It reads the built command tree's own flag lookup -- rather than the
// package variable a stray reassignment elsewhere in the package could leave stale -- so the
// assertion covers registration and default together, then ties that default to the branch it
// controls by feeding it straight into mustAttach.
//
// An invocation that never passes --no-attach must take today's attach path unchanged, and nothing
// else in this package would catch a default silently flipped to true.
func TestStartVerb_NoAttachFlag_DefaultsFalse(t *testing.T) {
	c := &loomCLI{}
	flag := c.startCmd().Flags().Lookup("no-attach")
	if flag == nil {
		t.Fatal(`"start" command is missing the --no-attach flag`)
	}
	if flag.DefValue != "false" {
		t.Errorf(`"--no-attach" default = %q; want "false"`, flag.DefValue)
	}

	defaultNoAttach := flag.Value.String() == "true"
	if got := mustAttach(defaultNoAttach); !got {
		t.Errorf("mustAttach(%v) over the registered default = %v; want true -- an invocation with no flag must still attach", defaultNoAttach, got)
	}
}

// countingWait returns a wait seam that counts its own invocations, for use in place of a real
// sleep in awaitRunLock tests.
func countingWait(count *int) func() {
	return func() {
		*count++
	}
}

// stillRunning is the halted seam for every awaitRunLock test whose scenario is not about the
// halted arm: it reports the machine still running, which is what keeps those scenarios reaching
// the arm they are actually about.
func stillRunning() bool { return false }

func TestAwaitRunLock_ReadyOnLaterIteration(t *testing.T) {
	calls := 0
	lockHeld := func() (bool, error) {
		calls++
		return calls >= 3, nil
	}
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockReady {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockReady", got)
	}
	if waits != 2 {
		t.Errorf("awaitRunLock() waited %d times; want 2", waits)
	}
}

func TestAwaitRunLock_ChildDied(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	aliveCalls := 0
	alive := func() bool {
		aliveCalls++
		return aliveCalls < 2
	}
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockChildDied {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockChildDied", got)
	}
}

func TestAwaitRunLock_Deadline(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 5)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockDeadline {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockDeadline", got)
	}
	if waits != 5 {
		t.Errorf("awaitRunLock() waited %d times; want 5", waits)
	}
}

func TestAwaitRunLock_LockSeamErrors(t *testing.T) {
	wantErr := errors.New("boom")
	lockHeld := func() (bool, error) { return false, wantErr }
	alive := func() bool { return true }
	waits := 0

	_, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if !errors.Is(err, wantErr) {
		t.Errorf("awaitRunLock() error = %v; want %v", err, wantErr)
	}
	if waits != 0 {
		t.Errorf("awaitRunLock() waited %d times on immediate lock error; want 0", waits)
	}
}

func TestAwaitRunLock_ReadyBeforeAliveCheck_ChildAboutToExit(t *testing.T) {
	// The order is load-bearing: a child that took the lock and is about to exit must still be
	// reported ready, so lockHeld reporting true must win even when alive would report false.
	lockHeld := func() (bool, error) { return true, nil }
	alive := func() bool { return false }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockReady {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockReady even though alive() would report false", got)
	}
}

// TestAwaitRunLock_HaltedWhileChildStillAlive is the regression guard for the defect Tier 2
// introduced in `lyx loom start`'s handshake. shedengine.Run releases the run lock on return, and
// `lyx loom run` then spends up to friction_timeout_min -- thirty minutes in the shipped template
// -- running the friction reflection agent, against a handshake budget of thirty seconds. Before the
// halted seam existed, that combination (lock free, child alive, machine finished) fell through to
// awaitRunLockDeadline, which dispositionForHandshake refuses: a healthy run was reported as
// "driver did not take the run lock" and the bootstrap skipped its own terminal handover. Reproduced
// live against a real hub before this test was written.
func TestAwaitRunLock_HaltedWhileChildStillAlive(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	halted := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, halted, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockHalted {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockHalted -- a driver whose machine finished but whose process is still doing post-run bookkeeping is not a wedged spawn", got)
	}
	if waits != 0 {
		t.Errorf("awaitRunLock() waited %d times; want 0 -- the halt is observable on the first poll", waits)
	}
}

// TestAwaitRunLock_HaltedCheckedAfterAliveCheck pins the seam order: a child that is already gone
// reports child-died even when halted would also report true, so the more specific signal wins and
// the existing ChildDied scenario keeps its meaning.
func TestAwaitRunLock_HaltedCheckedAfterAliveCheck(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return false }
	halted := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, halted, countingWait(&waits), 10)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockChildDied {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockChildDied even though halted() also reports true", got)
	}
}

// TestAwaitRunLock_StillRunningChildAliveStillReachesDeadline is the other half of the guard: the
// halted seam must not make the genuine refusal unreachable. A child that is alive, never takes the
// lock, and leaves the machine in running is still a wedged spawn and must still hit the deadline.
func TestAwaitRunLock_StillRunningChildAliveStillReachesDeadline(t *testing.T) {
	lockHeld := func() (bool, error) { return false, nil }
	alive := func() bool { return true }
	waits := 0

	got, err := awaitRunLock(lockHeld, alive, stillRunning, countingWait(&waits), 4)
	if err != nil {
		t.Fatalf("awaitRunLock() unexpected error: %v", err)
	}
	if got != awaitRunLockDeadline {
		t.Errorf("awaitRunLock() = %v; want awaitRunLockDeadline -- the halted arm must not swallow the wedged-spawn refusal", got)
	}
	if waits != 4 {
		t.Errorf("awaitRunLock() waited %d times; want 4", waits)
	}
}

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
// and released it, and skipped the tmux handover entirely -- so the status strand showing the actual
// halt was the one place the operator was not put.
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

// TestOperatorStrandAddSpec pins operatorStrandAddSpec's whole output shape, including the two
// fields an implementer would most plausibly "fix" to something else and be wrong: Focus, because
// Display.Focus is persisted and re-evaluated on every subsequent AddStrand, so a true value would
// re-capture focus on every later agent-pane spawn for the rest of the run; and Cmd, because a
// Strand's Cmd is typed into an already-running shell via send-keys, not passed as a trailing
// split-window argument, so a non-empty value here would nest a shell inside the pane's own shell.
func TestOperatorStrandAddSpec(t *testing.T) {
	got := operatorStrandAddSpec()

	if got.NameOverride != operatorStrandDisplayName {
		t.Errorf("operatorStrandAddSpec().NameOverride = %q; want %q", got.NameOverride, operatorStrandDisplayName)
	}
	if !got.IfAbsent {
		t.Error("operatorStrandAddSpec().IfAbsent = false; want true -- lyx loom start is re-entrant")
	}
	if got.Cmd != "" {
		t.Errorf("operatorStrandAddSpec().Cmd = %q; want empty -- the pane runs whatever shell tmux gives a freshly split pane", got.Cmd)
	}
	if got.Display.Anchor != render.AnchorBelowParent {
		t.Errorf("operatorStrandAddSpec().Display.Anchor = %q; want %q", got.Display.Anchor, render.AnchorBelowParent)
	}
	if got.Display.Focus {
		t.Error("operatorStrandAddSpec().Display.Focus = true; want false -- Focus is persisted and re-evaluated on every later AddStrand")
	}
	if got.Display.ShrinkWhenWaitingOnChild {
		t.Error("operatorStrandAddSpec().Display.ShrinkWhenWaitingOnChild = true; want false -- the operator's own pane must never collapse")
	}
}

// TestOperatorStrandDisplayName_DiffersFromStatusStrandDisplayName guards the exact failure
// resolveStatusStrandAction's own doc comment describes: reed's add has no upsert semantics, so a
// name collision between the two pinned strand names would append a second pane rather than replace
// the first.
func TestOperatorStrandDisplayName_DiffersFromStatusStrandDisplayName(t *testing.T) {
	if operatorStrandDisplayName == statusStrandDisplayName {
		t.Errorf("operatorStrandDisplayName and statusStrandDisplayName are both %q; want distinct names", operatorStrandDisplayName)
	}
}

// TestResolveStatusStrandAction is the regression guard for a status pane that never came back.
// The DeadEntry row is the defect: the bootstrap used to decide by presence alone, and reed keeps
// tracking a strand whose pane is gone, so after any reed server restart -- a reboot, a crash, a
// kill-server, or reed's own zombie-boot force-reap -- every subsequent "lyx loom start" in that
// worktree saw the stale "loom-status" entry, reported "already there", and left the operator with
// no status read-out at all.
func TestResolveStatusStrandAction(t *testing.T) {
	tests := []struct {
		name     string
		strands  []reedengine.StrandStatus
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
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithClearedPaneBinding",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "", Live: false}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name:     "DeadEntryWithADeadPane",
			strands:  []reedengine.StrandStatus{{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: false}},
			want:     statusStrandReplace,
			wantGUID: "g0",
		},
		{
			name: "LiveStatusStrandAmongOthers",
			strands: []reedengine.StrandStatus{
				{GUID: "g1", Name: "plan::g1", PaneID: "%3", Live: true},
				{GUID: "g0", Name: statusStrandDisplayName, PaneID: "%0", Live: true},
			},
			want:     statusStrandKeep,
			wantGUID: "g0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotGUID := resolveStatusStrandAction(tt.strands)
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
// Invariant's scan found, at the file (repo-root-relative, slash-normalized) and line it occurred at.
type driverFieldRead struct {
	relPath string
	line    int
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

	var found []driverFieldRead
	ast.Inspect(astFile, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Driver" {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || !seedTyped[ident.Name] {
			return true
		}
		found = append(found, driverFieldRead{line: fset.Position(sel.Pos()).Line})
		return true
	})
	return found, nil
}

// scanRepoForDriverFieldReads walks every production (non-test) .go file under repoRoot, skipping
// .git, testdata, and internal/shedrun (the sole legitimate parser and writer of the Seed struct, per
// the Shed Run-Directory Invariant), and returns every driver-field read scanFileForDriverFieldReads
// finds, each stamped with its repo-root-relative, slash-normalized path.
func scanRepoForDriverFieldReads(t *testing.T, repoRoot string) []driverFieldRead {
	t.Helper()
	var all []driverFieldRead

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "internal/shedrun" || strings.HasPrefix(rel, "internal/shedrun/") {
			return nil
		}

		found, scanErr := scanFileForDriverFieldReads(path)
		if scanErr != nil {
			return scanErr
		}
		for _, f := range found {
			f.relPath = rel
			all = append(all, f)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repo for driver field reads: %v", err)
	}
	return all
}

// driverFieldReadCarveOuts are the repo-root-relative production files, outside internal/loomcli
// and internal/shedrun, that may read a seed's Driver field.
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
var driverFieldReadCarveOuts = map[string]bool{
	"internal/battencli/arm.go": true,
}

// TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField is the Driver Choice
// Single-Site Invariant's tripwire: the only production readers of the seed's Driver field outside
// internal/shedrun (the field's sole parser/writer) must be internal/loomcli, that recipe's own
// bootstrap package, and the files named in driverFieldReadCarveOuts. Adding a reader anywhere else
// fails this test and forces a human to confirm the new site really belongs to a recipe's bootstrap
// verb rather than a producer, a generic verb, or an engine gating behaviour on the recorded value.
func TestDriverChoiceSingleSiteInvariant_OnlyLoomcliReadsTheSeedDriverField(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// Three levels up from internal/loomcli/bootstrap_test.go -> repo root.
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	found := scanRepoForDriverFieldReads(t, repoRoot)

	var outside []driverFieldRead
	sawLoomcli := false
	for _, f := range found {
		if strings.HasPrefix(f.relPath, "internal/loomcli/") {
			sawLoomcli = true
			continue
		}
		if driverFieldReadCarveOuts[f.relPath] {
			continue
		}
		outside = append(outside, f)
	}

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
