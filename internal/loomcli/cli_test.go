// cli_test.go covers the cobra seam: the bare-group git-free guard and the run, pause and status refusals.
// Those refusals are driven directly against a hand-populated receiver.
// They bypass wire, since none of them needs a wired hub.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/spf13/cobra"
)

// TestResolvePersistentPreRun_SkipsSlugGroups asserts that the loom pre-run arms no slug group:
// the circling, decision and peers groups and their verbs, and loom has no review group.
// cmd/lym sets cobra.EnableTraverseRunHooks, so cobra calls that pre-run for a group verb too;
// over a non-git cwd an arm would write an error envelope and abort, so silence proves the skip.
func TestResolvePersistentPreRun_SkipsSlugGroups(t *testing.T) {
	tiergate.Require(t, 0)
	parent := Command()
	for _, child := range parent.Commands() {
		if child.Name() == "review" {
			t.Fatal(`loom has a "review" child; want "lym loom review" to be no command`)
		}
	}
	for _, name := range []string{"circling", decisionGroup, peersGroup} {
		group, _, err := parent.Find([]string{name})
		if err != nil || group.Name() != name {
			t.Fatalf("Find(%q) = %v, %v; want the %s group", name, group, err, name)
		}
		for _, cmd := range append([]*cobra.Command{group}, group.Commands()...) {
			ctx, _ := clihelp.NewExitContext(lymcwd.WithCwd(t.Context(), t.TempDir()))
			cmd.SetContext(ctx)
			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := parent.PersistentPreRunE(cmd, []string{"some-slug"}); err != nil {
				t.Errorf("loom pre-run for %s %q = %v; want nil", name, cmd.Name(), err)
			}
			if out.Len() != 0 || clihelp.ShouldAbort(ctx) {
				t.Errorf("loom pre-run for %s %q wrote %q (abort %v); want it skipped", name, cmd.Name(), out.String(), clihelp.ShouldAbort(ctx))
			}
		}
	}
}

// TestCommand_StartVerbCarriesParentAndHiddenNoAttach pins the start verb under the loom parent command:
// it exposes --parent, and it accepts --no-attach, hidden, for existing callers.
func TestCommand_StartVerbCarriesParentAndHiddenNoAttach(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	var registered *cobra.Command
	for _, sub := range Command().Commands() {
		if sub.Name() == "start" {
			registered = sub
		}
	}
	if registered == nil {
		t.Fatal(`"start" verb is not registered under the loom parent command`)
	}
	if registered.Flags().Lookup("parent") == nil {
		t.Error(`"loom start" is missing the --parent flag`)
	}
	if noAttach := registered.Flags().Lookup("no-attach"); noAttach == nil {
		t.Error(`"loom start" is missing the --no-attach flag`)
	} else if !noAttach.Hidden {
		t.Error(`"loom start"'s --no-attach flag is not hidden`)
	}
}

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lym loom" invocation succeeds outside a git repository.
// It proves the PersistentPreRunE guard for cmd.Name() == "loom" fires before any cwd resolution.
func TestRunCLI_GroupGuard_NoGitRepoNeeded(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	var out bytes.Buffer
	exitCode := RunCLIIn(dir, &out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLIIn(%q, nil) = %d; want 0", dir, exitCode)
	}
}

// TestNewLoomCLI_SetsInjectedSeams pins the fields newLoomCLI sets:
// Command does not expose the receiver it builds, and its other tests do not assert these fields.
//
//testtiming:keep pins newLoomCLI setting spawnWatchdog, midMerge, recordedParentBranch and suppressWatchdogSpawn, fields the constructor's callers do not expose; its covering tests build the CLI without asserting them
func TestNewLoomCLI_SetsInjectedSeams(t *testing.T) {
	tiergate.Require(t, 0)
	c := newLoomCLI()

	if c.spawnWatchdog == nil {
		t.Error("newLoomCLI().spawnWatchdog = nil; want reedengine.SpawnWatchdog")
	}
	if c.midMerge == nil {
		t.Error("newLoomCLI().midMerge = nil; want fabricengine.MidMerge")
	}
	if c.recordedParentBranch == nil {
		t.Error("newLoomCLI().recordedParentBranch = nil; want the origin-record reader")
	}
	if c.suppressWatchdogSpawn != testing.Testing() {
		t.Errorf("newLoomCLI().suppressWatchdogSpawn = %v; want %v (testing.Testing())", c.suppressWatchdogSpawn, testing.Testing())
	}
}

// TestProductionFiles_LoomCLILiteralOnlyInFactory proves that only newLoomCLI builds a *loomCLI composite literal.
// It scans the package's non-test production files for the "&loomCLI{" token, which may appear only in cli.go.
// Every CLI must come from newLoomCLI, which sets the injected seams; no behavior test reaches that shape.
//
//testtiming:keep scans the package's production files for a loomCLI composite literal outside newLoomCLI's file, a source-shape guard no behavior test reaches
func TestProductionFiles_LoomCLILiteralOnlyInFactory(t *testing.T) {
	tiergate.Require(t, 0)
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob *.go: %v", err)
	}

	const token = "&loomCLI{"
	const factoryFile = "cli.go"

	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), token) && filepath.Base(path) != factoryFile {
			t.Errorf("%s references %q outside %s; every *loomCLI must be built through newLoomCLI", path, token, factoryFile)
		}
	}
}

// loomVerbCommand returns the shedverbs command named verb, armed against c.specFor(verb) assigned onto c.spec.
// It fills the Spec with no resolution and no git spawn, so this untagged suite can run it.
func loomVerbCommand(c *loomCLI, verb string) *cobra.Command {
	spec := c.specFor(verb)
	c.spec = &spec
	for _, cmd := range shedverbs.Verbs(loomVerbTexts, c.spec) {
		if cmd.Name() == verb {
			return cmd
		}
	}
	panic("loomVerbCommand: no shedverbs command named " + verb)
}

// TestLoomVerbHelp_NamesBothSeedingRoutes asserts that the step and run help name both seed routes:
// "lym loom start" and the driverless seed, which is the seed before the first "lym loom step".
// It also asserts that step's help does not claim it bootstraps exactly as start does.
func TestLoomVerbHelp_NamesBothSeedingRoutes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const driverlessSeed = "lym shed seed self --recipe tapestry --driver go --param parent=<parent-branch>"
	tests := []struct {
		verb       string
		wantAbsent string
		wantExtra  string
	}{
		{verb: "step", wantAbsent: `exactly as "lym loom start" does`, wantExtra: "refuses without one"},
		{verb: "run"},
	}
	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			t.Parallel()
			long := strings.Join(strings.Fields(loomVerbCommand(&loomCLI{}, tt.verb).Long), " ")
			wants := []string{"lym loom start", driverlessSeed, "parent_branch", "lym loom step", tt.wantExtra}
			for _, want := range wants {
				if !strings.Contains(long, want) {
					t.Errorf("%s help lacks %q:\n%s", tt.verb, want, long)
				}
			}
			if seed, step := strings.Index(long, driverlessSeed), strings.Index(long, "lym loom step"); seed < 0 || step < seed {
				t.Errorf("%s help names \"lym loom step\" at %d before the driverless seed at %d", tt.verb, step, seed)
			}
			if tt.wantAbsent != "" && strings.Contains(long, tt.wantAbsent) {
				t.Errorf("%s help still holds %q", tt.verb, tt.wantAbsent)
			}
		})
	}
}

// TestVerbRefusals covers the run verb's seed-missing pre-flight, the pause verb's absent-file refusal,
// and the status verb's !found refusal.
// Each is driven directly against the leaf command shedverbs.Verbs returns for a hand-populated *loomCLI.
// They never go through the full PersistentPreRunE/wire path, which would spawn git in this untagged suite.
// c.shedPaths.StatusPath and StatusLockPath point at a temporary directory that never gets a status.json,
// so each verb's refusal fires on exactly the precondition it owns.
func TestVerbRefusals(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name       string
		verb       string
		wantRemedy string
		// wantAlso, when set, is a second remedy the output must hold.
		wantAlso string
		// treeArgs, when set, runs these arguments through the loom tree as lym mounts it, not the one verb.
		treeArgs []string
	}{
		{
			name:       "Approve_UnknownCommand",
			treeArgs:   []string{"approve"},
			wantRemedy: "unknown subcommand",
			wantAlso:   "approve",
		},
		{
			name:       "Reject_UnknownCommand",
			treeArgs:   []string{"reject"},
			wantRemedy: "unknown subcommand",
			wantAlso:   "reject",
		},
		{
			name:       "Run_SeedMissing",
			verb:       "run",
			wantRemedy: `lym loom start`,
			wantAlso:   `lym loom step`,
		},
		{
			name:       "Pause_AbsentFile",
			verb:       "pause",
			wantRemedy: `lym loom start`,
		},
		{
			name:       "Status_NotFound",
			verb:       "status",
			wantRemedy: `lym loom start`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			c := &loomCLI{
				shedPaths: shedbuild.ShedPaths{
					StatusPath:     filepath.Join(dir, "status.json"),
					StatusLockPath: filepath.Join(dir, "status.json.lock"),
				},
			}

			var out bytes.Buffer
			var exitCode int
			if tt.treeArgs != nil {
				root := &cobra.Command{Use: "lym"}
				root.AddCommand(Command())
				exitCode = clihelp.Execute(root, &out, append([]string{"loom"}, tt.treeArgs...))
			} else {
				exitCode = clihelp.Execute(loomVerbCommand(c, tt.verb), &out, nil)
			}

			if exitCode != 1 {
				t.Errorf("%s: exit code = %d; want 1", tt.name, exitCode)
			}
			if !strings.Contains(out.String(), `"ok":false`) {
				t.Errorf("%s: output missing ok:false envelope; got: %q", tt.name, out.String())
			}
			if !strings.Contains(out.String(), tt.wantRemedy) {
				t.Errorf("%s: output missing remedy %q; got: %q", tt.name, tt.wantRemedy, out.String())
			}
			if tt.wantAlso != "" && !strings.Contains(out.String(), tt.wantAlso) {
				t.Errorf("%s: output missing remedy %q; got: %q", tt.name, tt.wantAlso, out.String())
			}
		})
	}
}

// TestSpecFor_ScratchAndFrictionDir asserts the scratch and friction directories specFor gives the generic verbs.
// It must tolerate a receiver with no location.
//
//testtiming:keep pins specFor's scratch directory from the location and run-id, the friction directory passed through, and a nil location giving no scratch directory; its covering tests run specFor without asserting those fields
func TestSpecFor_ScratchAndFrictionDir(t *testing.T) {
	tiergate.Require(t, 0)
	loc := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}

	t.Run("ScratchDirFromLocationAndRunID", func(t *testing.T) {
		c := &loomCLI{location: loc, runID: "self", caller: "orch"}
		spec := c.specFor("step")
		if got, want := spec.ScratchDir, shedrun.ScratchDir(loc, "self"); got != want {
			t.Errorf("specFor(step).ScratchDir = %q; want %q", got, want)
		}
		if spec.Caller != "orch" {
			t.Errorf("specFor(step).Caller = %q; want orch", spec.Caller)
		}
		if spec.GotoGuard == nil {
			t.Error("specFor(step).GotoGuard = nil; want the settled-review guard for a known location")
		}
	})

	tests := []struct {
		name string
		dir  string
	}{
		{"FrictionDirSet", filepath.Join(t.TempDir(), "friction")},
		{"FrictionDirEmpty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &loomCLI{location: loc, runID: "self", frictionDir: tt.dir}
			if got := c.specFor("step").FrictionDir; got != tt.dir {
				t.Errorf("specFor(step).FrictionDir = %q; want %q", got, tt.dir)
			}
		})
	}

	t.Run("NilLocation", func(t *testing.T) {
		c := &loomCLI{runID: "self"}
		if got := c.specFor("step").ScratchDir; got != "" {
			t.Errorf("specFor(step).ScratchDir = %q; want empty for a nil location", got)
		}
	})

	t.Run("LoopArmedForALocatedReceiver", func(t *testing.T) {
		c := &loomCLI{location: loc, runID: "self", stepIdleTimeout: 7 * time.Minute}
		spec := c.specFor("step")
		loop := spec.Loop

		if loop.LockPath != shedrun.LoopLock(loc, "self") || loop.PIDPath != shedrun.LoopPIDFile(loc, "self") || loop.LogPath != shedrun.LoopLog(loc, "self") ||
			loop.EnvelopePath != shedrun.LoopEnvelope(loc, "self") || loop.JobName != shedrun.LoopJobName(loc, "self") {
			t.Errorf("loop paths = %+v; want shedrun's constructors over the run", loop)
		}
		if got, want := loop.Delivered("abc"), shedrun.LoopDelivered(loc, "self", "abc"); got != want {
			t.Errorf("loop.Delivered(abc) = %q; want %q", got, want)
		}
		traceCopy, stderr := loop.StopFiles("0123456789abcdef")
		if traceCopy != shedrun.StepTraceCopy(loc, "self", "0123456789abcdef") || stderr != shedrun.StepStderr(loc, "self", "0123456789abcdef") {
			t.Errorf("loop.StopFiles = %q, %q; want shedrun's step trace copy and stderr", traceCopy, stderr)
		}
		if files, err := loop.TraceFiles("0123456789abcdef"); err != nil || len(files) != 0 {
			t.Errorf("loop.TraceFiles of a trace group with no file = %v, %v; want none and no error", files, err)
		}
		if loop.Executable == "" || loop.Activity == nil || loop.IdleTimeout != 7*time.Minute || spec.Hooks.AfterInterrupt == nil {
			t.Errorf("loop executable %q, activity set %v, idle %s, interrupt hook set %v; want the binary, a reading, the receiver's window and the hook", loop.Executable, loop.Activity != nil, loop.IdleTimeout, spec.Hooks.AfterInterrupt != nil)
		}
		if loop.OnExit == nil || loop.OnWake == nil {
			t.Errorf("loop exit hook set %v, wake hook set %v; want both for the parent lock", loop.OnExit != nil, loop.OnWake != nil)
		}
	})

	t.Run("LoopLeftZeroForANilLocation", func(t *testing.T) {
		spec := (&loomCLI{runID: "self"}).specFor("step")
		if spec.Loop.EnvelopePath != "" || spec.Loop.Activity != nil || spec.Loop.OnExit != nil || spec.Loop.OnWake != nil || spec.Hooks.AfterInterrupt != nil {
			t.Errorf("loop envelope %q, activity set %v, lock hooks set %v, interrupt hook set %v; want all zero for a nil location", spec.Loop.EnvelopePath, spec.Loop.Activity != nil, spec.Loop.OnExit != nil || spec.Loop.OnWake != nil, spec.Hooks.AfterInterrupt != nil)
		}
	})

	t.Run("InterruptHookSkippedWithTier2Off", func(t *testing.T) {
		f := newHaltFixture(t)
		f.c.runID = "self"
		f.c.frictionDir = ""

		if got := f.c.specFor("step").Hooks.AfterInterrupt(t.Context(), "Plan-Write", "watchdog", "/logs/child.log"); got != frictionengine.StatusSkipped {
			t.Errorf("hook with no friction directory = %q; want %q", got, frictionengine.StatusSkipped)
		}
	})
}

// TestPauseCmd_BareLoomPauseRefusesNamingBothWaysForward runs loom's pause verb on a hand-populated receiver.
// The receiver holds a running status at Plan-Write.
// A flagless pause refuses and names both ways forward:
// `lym tender pause <slug>` and `lym loom pause --after Plan-Write`.
// The status file stays byte-identical.
func TestPauseCmd_BareLoomPauseRefusesNamingBothWaysForward(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	c := &loomCLI{
		location: &lymcwd.Location{HubPath: dir, WorktreeName: "some-slug", AnchorRel: "."},
		shedPaths: shedbuild.ShedPaths{
			StatusPath:     filepath.Join(dir, "status.json"),
			StatusLockPath: filepath.Join(dir, "status.json.lock"),
		},
	}
	writeStatusFixture(t, c.shedPaths.StatusPath, c.shedPaths.StatusLockPath, shedengine.Status{
		CurrentProducer: "Plan-Write",
		State:           shedengine.StateRunning,
		History:         []shedengine.HistoryEntry{},
	})
	before, err := os.ReadFile(c.shedPaths.StatusPath)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if exitCode := clihelp.Execute(loomVerbCommand(c, "pause"), &out, nil); exitCode != 1 {
		t.Fatalf("bare pause exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	for _, want := range []string{`lym tender pause some-slug`, `lym loom pause --after Plan-Write`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("bare pause output %q does not name %q", out.String(), want)
		}
	}
	after, err := os.ReadFile(c.shedPaths.StatusPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Errorf("status file after a refused bare pause = %q, %v; want it unchanged: %q", after, err, before)
	}
}

// TestSpecFor_RestepHold asserts specFor's view of a loom recipe:
// the escalate-halt check is RestepHold and pawl is none, and an empty recipe reads as tapestry.
func TestSpecFor_RestepHold(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		recipe   string
		told     bool
		producer string
		reason   string
		want     bool
	}{
		{shedrun.RecipeTapestry, true, "Finalize", landingshed.EscalateStopPrefix + "the fixer needs a decision", true},
		{shedrun.RecipeTapestry, true, "Finalize", "a plain reason", false},
		{shedrun.RecipeDarn, true, "Parent-Align", landingshed.EscalateStopPrefix + "the fixer needs a decision", true},
		{shedrun.RecipeDarn, true, "Parent-Align", "a plain reason", false},
		{shedrun.RecipePawl, true, "Finalize", landingshed.EscalateStopPrefix + "the fixer needs a decision", true},
		{"", true, "Finalize", landingshed.EscalateStopPrefix + "the fixer needs a decision", true},
	}
	for _, tt := range tests {
		t.Run(tt.recipe+"/"+tt.reason, func(t *testing.T) {
			t.Parallel()
			c := &loomCLI{runID: shedrun.SelfRunID, recipe: tt.recipe}
			spec := c.specFor("status")
			if (spec.RestepHold != nil) != tt.told {
				t.Fatalf("specFor(status).RestepHold told = %v; want %v", spec.RestepHold != nil, tt.told)
			}
			if tt.told {
				if got := spec.RestepHold(tt.producer, tt.reason); got != tt.want {
					t.Errorf("RestepHold(%q, %q) = %v; want %v", tt.producer, tt.reason, got, tt.want)
				}
			}
		})
	}
}
