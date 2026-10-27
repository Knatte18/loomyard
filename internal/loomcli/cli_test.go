//go:build tier0

// cli_test.go covers the loomcli cobra seam: the bare-group invocation's git-free guard, and the run/pause/status verbs' own
// refusal paths driven directly against a hand-populated receiver -- bypassing wire entirely, since
// none of the three refusals needs a wired hub.

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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/spf13/cobra"
)

// TestResolvePersistentPreRun_SkipsSlugGroups asserts the loom parent's pre-run never arms the circling, decision or peers group or their verbs, and that loom has no review group.
// cmd/lym sets cobra.EnableTraverseRunHooks, so cobra calls that pre-run for a group verb too;
// over a non-git cwd an arm would write an error envelope and abort, so silence proves the skip.
func TestResolvePersistentPreRun_SkipsSlugGroups(t *testing.T) {
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

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lym loom" invocation succeeds without
// needing a git repository, proving the PersistentPreRunE guard for cmd.Name() == "loom" fires before
// any cwd resolution.
func TestRunCLI_GroupGuard_NoGitRepoNeeded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var out bytes.Buffer
	exitCode := RunCLIIn(dir, &out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLIIn(%q, nil) = %d; want 0", dir, exitCode)
	}
}

// TestNewLoomCLI_SetsInjectedSeams pins newLoomCLI's own fields, since Command() does not expose the receiver it constructs and may not grow an accessor purely for a test.
//
//testtiming:keep pins newLoomCLI setting spawnWatchdog, midMerge, recordedParentBranch and suppressWatchdogSpawn, fields the constructor's callers do not expose; its covering tests build the CLI without asserting them
func TestNewLoomCLI_SetsInjectedSeams(t *testing.T) {
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

// TestProductionFiles_LoomCLILiteralOnlyInFactory proves no production file in this package builds a
// *loomCLI composite literal outside newLoomCLI, by scanning this package's own non-_test.go *.go
// files for the "&loomCLI{" token and allowing it only in cli.go, newLoomCLI's own file -- the same
// source-scan shape internal/burlercli/wiring_test.go already uses
// (TestProductionFiles_NeverReferenceHubWatchdogMechanism) for a boundary property that has no other
// static form.
//
// Together with TestNewLoomCLI_SetsInjectedSeams above, this is what would have caught the alias gotcha newLoomCLI now designs out: that test proves the fields are set, this one proves both constructors go through the place that sets them.
//
//testtiming:keep scans the package's production files for a loomCLI composite literal outside newLoomCLI's file, a source-shape guard no behavior test reaches
func TestProductionFiles_LoomCLILiteralOnlyInFactory(t *testing.T) {
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

// loomVerbCommand builds the single shedverbs command named verb ("run", "step", "status", or
// "pause"), armed against c.specFor(verb) assigned onto c.spec -- the tier-1 seam arm.go's
// specFor/arm split exists for, letting this untagged suite fill a Spec with no resolution and no
// git spawn.
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

// TestLoomVerbHelp_NamesBothSeedingRoutes asserts the step and run help name "lym loom start" and the driverless seed, the seed before the first "lym loom step",
// and that step's help does not claim it bootstraps exactly as start does.
func TestLoomVerbHelp_NamesBothSeedingRoutes(t *testing.T) {
	t.Parallel()
	const driverlessSeed = "lym shed seed self --recipe loom --driver go --param parent=<parent-branch>"
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

// TestVerbRefusals covers the run verb's seed-missing pre-flight, the pause verb's absent-file
// refusal, and the status verb's !found refusal. All three are driven directly against the leaf
// command shedverbs.Verbs returns for a hand-populated *loomCLI -- never through the full
// PersistentPreRunE/wire path, which needs a real git repository this untagged suite must not spawn.
// c.shedPaths.StatusPath/StatusLockPath point at a plain temporary directory that never receives a
// status.json, so each verb's own refusal fires on exactly the precondition it owns.
func TestVerbRefusals(t *testing.T) {
	tests := []struct {
		name       string
		verb       string
		wantRemedy string
		// wantAlso, when set, is a second remedy the output must hold.
		wantAlso string
		// treeArgs, when set, runs these arguments through the whole loom command tree, mounted under a parent as in lym, instead of the one verb.
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

// TestSpecFor_ScratchAndFrictionDir asserts specFor tells the generic verbs the run's scratch
// directory and the friction directory, and tolerates a receiver with no location.
//
//testtiming:keep pins specFor's scratch directory from the location and run-id, the friction directory passed through, and a nil location giving no scratch directory; its covering tests run specFor without asserting those fields
func TestSpecFor_ScratchAndFrictionDir(t *testing.T) {
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

// TestPauseCmd_BareLoomPauseRefusesNamingBothWaysForward runs loom's pause verb on a hand-populated receiver over a running status at Plan-Write:
// a flagless pause refuses naming `lym tender pause <slug>` and `lym loom pause --after Plan-Write`, and leaves the status file byte-identical.
func TestPauseCmd_BareLoomPauseRefusesNamingBothWaysForward(t *testing.T) {
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
