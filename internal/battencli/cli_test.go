//go:build tier0

// cli_test.go covers the battencli cobra seam: each verb's argument-count rejection and the bare-group invocation's git-free guard -- mirroring internal/loomcli/cli_test.go's shape.

package battencli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/spf13/cobra"
)

// battenCommandNamed returns the subcommand named verb from the built batten tree, or nil.
func battenCommandNamed(parent *cobra.Command, verb string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == verb {
			return sub
		}
	}
	return nil
}

// TestCommand_EveryVerbArity asserts each of the four verbs' own Args validator, checked independently per verb rather than by reading one shared value, accepts both zero and one positional and rejects two -- cobra.MaximumNArgs(1), the one arity contract all three surfaces ("lyx shed", "lyx batten", "lyx loom") share -- and that two positionals fail the whole command through cobra's own arity error, regardless of cwd, before PersistentPreRunE ever runs.
// Accepting zero is what lets "lyx batten run" with no argument reach arm's own refuseSelfAddress (arm_seed_test.go) with a named message, rather than stopping at cobra's generic count error.
func TestCommand_EveryVerbArity(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"run", "step", "status", "pause", "cancel"} {
		t.Run(verb, func(t *testing.T) {
			t.Parallel()

			cmd := battenCommandNamed(Command(), verb)
			if cmd == nil {
				t.Fatalf("verb %q not found under the batten parent command", verb)
			}
			if cmd.Args == nil {
				t.Fatalf("verb %q has a nil Args validator", verb)
			}

			if err := cmd.Args(cmd, nil); err != nil {
				t.Errorf("%s: Args(zero) = %v; want nil", verb, err)
			}
			if err := cmd.Args(cmd, []string{"one-run-id"}); err != nil {
				t.Errorf("%s: Args(one) = %v; want nil", verb, err)
			}
			if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
				t.Errorf("%s: Args(two) = nil; want a rejection -- MaximumNArgs(1)", verb)
			}

			args := []string{verb, "a", "b"}
			var out bytes.Buffer
			if exitCode := clihelp.Execute(Command(), &out, args); exitCode != 1 {
				t.Errorf("Execute(%v) = %d; want 1", args, exitCode)
			}
		})
	}
}

// TestArmSeed_DriverFlagMatrix pins the four-way matrix between --driver and --child-driver: batten's
// own driver flag still accepts "go" and still refuses both "llm" and an unknown value, while
// --child-driver accepts both "go" and "llm" -- the value it accepts reaches the auto-seeded seed's
// own "child_driver" param, which wire.go's own ChildDriver reader consults -- and still refuses an
// unknown value. The likeliest regression this batch names is lifting both refusals for symmetry;
// only a case asserting the batten driver flag still refuses catches it.
func TestArmSeed_DriverFlagMatrix(t *testing.T) {
	tests := []struct {
		name            string
		driverFlag      string
		childDriverFlag string
		childUntyped    bool
		wantErr         bool
		wantSubstr      string
	}{
		{"BothGo", shedrun.DriverGo, shedrun.DriverGo, false, false, ""},
		{"ChildDriverLLMAccepted", shedrun.DriverGo, shedrun.DriverLLM, false, false, ""},
		{"ChildDriverUntypedWritesNoParam", shedrun.DriverGo, "", true, false, ""},
		// Batten has no bootstrap verb, so the driver it seeds itself with IS the process the operator typed and there is no spawn seam to branch on a seed's driver: the refusal names the missing bootstrap verb.
		{"OwnDriverLLMRefused", shedrun.DriverLLM, shedrun.DriverGo, false, true, "no bootstrap verb"},
		{"BothLLMRefusedByOwnDriver", shedrun.DriverLLM, shedrun.DriverLLM, false, true, "no bootstrap verb"},
		{"OwnDriverUnknownRefused", "rust", shedrun.DriverGo, false, true, ""},
		{"ChildDriverUnknownRefused", shedrun.DriverGo, "rust", false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
			c := &battenCLI{driverFlag: tt.driverFlag, childDriverFlag: tt.childDriverFlag, childDriverFlagSet: !tt.childUntyped}

			err := c.armSeed(loc, "some-slug", "run")
			if (err != nil) != tt.wantErr {
				t.Fatalf("armSeed(driver=%q, childDriver=%q) error = %v; want error = %v", tt.driverFlag, tt.childDriverFlag, err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("armSeed(driver=%q, childDriver=%q) error = %q; want it to contain %q", tt.driverFlag, tt.childDriverFlag, err.Error(), tt.wantSubstr)
				}
				if strings.Contains(err.Error(), "roadmap") {
					t.Errorf("armSeed(driver=%q, childDriver=%q) error = %q; want it to no longer name a roadmap item", tt.driverFlag, tt.childDriverFlag, err.Error())
				}
				return
			}

			seed, found, readErr := shedrun.ReadSeed(loc, "some-slug")
			if readErr != nil || !found {
				t.Fatalf("ReadSeed after armSeed = (found=%v, err=%v); want (true, nil)", found, readErr)
			}
			got, present := seed.Params["child_driver"]
			if tt.childUntyped {
				if present {
					t.Errorf("seed.Params[\"child_driver\"] = %q; want no param for an untyped flag", got)
				}
				return
			}
			if got != tt.childDriverFlag {
				t.Errorf("seed.Params[\"child_driver\"] = %q; want %q", got, tt.childDriverFlag)
			}
		})
	}
}

// TestRunCLI_GroupGuard_NoGitRepoNeeded asserts that a bare "lyx batten" invocation succeeds
// without needing a git repository, proving the PersistentPreRunE guard for cmd.Name() ==
// "batten" fires before any cwd resolution.
func TestRunCLI_GroupGuard_NoGitRepoNeeded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var out bytes.Buffer
	// An empty, non-nil slice: a nil one makes cobra read os.Args, which carries the test binary's own flags.
	exitCode := RunCLIIn(dir, &out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLIIn(%q, no args) = %d; want 0", dir, exitCode)
	}
}

// TestCommand_HelpCarriesNoTeardownGotoExample asserts no help text shows the forward move to Worktree-Teardown as an example.
func TestCommand_HelpCarriesNoTeardownGotoExample(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if strings.Contains(cmd.Long, "--to Worktree-Teardown") {
			t.Errorf("%q help carries a --to Worktree-Teardown example", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestStartFirstPass covers the detached first pass over a hand-populated receiver with fake pass, dobby-liveness, dobby-spawn and parent-owner seams:
// a done slug and a slug a pause covers return an envelope whose ok is false naming their way forward and start nothing;
// a startable slug starts one pass with the prime's anchor as its working directory and starts its parent's dobby once when that reads not live and never when it reads live;
// a failed dobby spawn still returns the started pass's envelope with a warning naming `lyx beam start` for the parent;
// a parent another machine owns starts no dobby and warns naming `lyx adopt` for the parent, or `lyx adopt root` for the prime;
// a parent beam whose landing hold stands starts no dobby and warns naming `lyx batten status`.
func TestStartFirstPass(t *testing.T) {
	t.Parallel()

	const (
		beam      = "some-beam"
		otherHost = "elsewhere"
	)
	tests := []struct {
		name          string
		status        shedengine.State
		pauseCovers   bool
		driverSet     bool
		childSet      bool
		beamParent    bool
		dobbyLive     bool
		dobbySpawnErr error
		parentOwner   string
		landingHeld   bool
		// anchor is the first pass's anchor option, which the seed the pass arms records as its anchor param.
		anchor         string
		wantRefusal    []string
		wantPassArgs   []string
		wantDobbySpawn int
		wantWarning    []string
		wantParent     string
	}{
		{
			name:           "StartableSlugStartsItsPassAndTheDobbyOnce",
			wantPassArgs:   []string{"batten", "run", "some-slug"},
			wantDobbySpawn: 1,
			wantParent:     "hub-repo",
		},
		{
			name:         "LiveDobbyIsLeftAlone",
			dobbyLive:    true,
			wantPassArgs: []string{"batten", "run", "some-slug"},
			wantParent:   "hub-repo",
		},
		{
			name:           "TypedDriversAreCarriedThrough",
			driverSet:      true,
			childSet:       true,
			wantPassArgs:   []string{"batten", "run", "some-slug", "--driver", "go", "--child-driver", "go"},
			wantDobbySpawn: 1,
			wantParent:     "hub-repo",
		},
		{
			name:           "AnchorOptionIsRecordedAsTheSeedsAnchorParam",
			anchor:         "sub/dir",
			wantPassArgs:   []string{"batten", "run", "some-slug"},
			wantDobbySpawn: 1,
			wantParent:     "hub-repo",
		},
		{
			name:           "BlockedStatusResumesAsAPass",
			status:         shedengine.StateBlocked,
			wantPassArgs:   []string{"batten", "run", "some-slug"},
			wantDobbySpawn: 1,
			wantParent:     "hub-repo",
		},
		{
			name:           "BeamParentGetsItsOwnDobby",
			beamParent:     true,
			wantPassArgs:   []string{"batten", "run", "some-slug"},
			wantDobbySpawn: 1,
			wantParent:     beam,
		},
		{
			name:           "FailedDobbySpawnWarnsAndKeepsThePass",
			dobbySpawnErr:  errors.New("exec failed"),
			wantPassArgs:   []string{"batten", "run", "some-slug"},
			wantDobbySpawn: 1,
			wantWarning:    []string{"exec failed", "lyx beam start root"},
			wantParent:     "hub-repo",
		},
		{
			name:         "PrimeOwnedElsewhereStartsNoDobby",
			parentOwner:  otherHost,
			wantPassArgs: []string{"batten", "run", "some-slug"},
			wantWarning:  []string{otherHost, "lyx adopt root"},
			wantParent:   "hub-repo",
		},
		{
			name:         "BeamParentWhoseLandingHoldStandsStartsNoDobby",
			beamParent:   true,
			landingHeld:  true,
			wantPassArgs: []string{"batten", "run", "some-slug"},
			wantWarning:  []string{"lyx batten status " + beam},
			wantParent:   beam,
		},
		{
			name:         "BeamOwnedElsewhereStartsNoDobby",
			beamParent:   true,
			parentOwner:  otherHost,
			wantPassArgs: []string{"batten", "run", "some-slug"},
			wantWarning:  []string{otherHost, "lyx adopt " + beam},
			wantParent:   beam,
		},
		{
			name:        "DoneSlugIsRefusedBeforeAnythingStarts",
			status:      shedengine.StateDone,
			wantRefusal: []string{"has already completed", "lyx batten cleanup --apply", "different slug"},
		},
		{
			name:        "CoveredSlugIsRefusedBeforeAnythingStarts",
			pauseCovers: true,
			wantRefusal: []string{"lyx tender resume"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			hub := c.location.HubPath
			if err := os.MkdirAll(filepath.Join(hub, "hub-repo", ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.beamParent {
				if err := os.MkdirAll(filepath.Join(hub, beam), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(fabricengine.BoardDir(hub), 0o755); err != nil {
					t.Fatal(err)
				}
				board := `{"version":1,"entries":[` +
					`{"id":0,"slug":"` + beam + `","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
					`{"id":1,"slug":"some-slug","title":"Member","kind":"task","labels":["bug"],"project":"` + beam + `"}]}`
				if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(hub), "board.json"), []byte(board), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.pauseCovers {
				geometry, err := hubgeom.PauseGeometry(c.location)
				if err != nil {
					t.Fatalf("hubgeom.PauseGeometry: %v", err)
				}
				now := time.Now()
				if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
					record.Live = append(record.Live, pauserecord.Entry{Scope: c.slug, PausedAt: now, Caller: "EXTERNAL"})
					return nil
				}); err != nil {
					t.Fatalf("pauserecord.Update: %v", err)
				}
			}
			if tt.status != "" {
				writeStatus(t, c, shedengine.Status{CurrentProducer: battenrecipe.NameWorktreeTeardown, State: tt.status})
			}
			c.driverFlag, c.childDriverFlag = "go", "go"
			c.driverFlagSet, c.childDriverFlagSet = tt.driverSet, tt.childSet
			if tt.anchor != "" {
				// StartFirstPass arms the run, which writes the seed, before the pass starts.
				c.firstPassAnchor = tt.anchor
				if err := c.armSeed(c.location, c.slug, "run"); err != nil {
					t.Fatalf("armSeed(run) = %v; want nil", err)
				}
			}

			var passDir string
			var passArgs []string
			dobbySpawns := 0
			c.passSpawn = func(dir string, args []string) (int, error) {
				passDir, passArgs = dir, args
				return 4242, nil
			}
			c.parentOwner = func(string) (string, error) { return tt.parentOwner, nil }
			c.landingHeld = func(string) (bool, error) { return tt.landingHeld, nil }
			c.dobbyLive = func(dobbyengine.Geometry) (bool, error) { return tt.dobbyLive, nil }
			c.dobbySpawn = func(dobbyengine.Geometry) error {
				dobbySpawns++
				return tt.dobbySpawnErr
			}

			cmd := &cobra.Command{Use: "run", RunE: func(cmd *cobra.Command, args []string) error { return c.startFirstPass(cmd) }}
			var out bytes.Buffer
			exitCode := clihelp.Execute(cmd, &out, []string{})
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not one JSON object: %v", out.String(), err)
			}

			if tt.wantRefusal != nil {
				if exitCode != 1 || envelope["ok"] != false {
					t.Fatalf("exit %d, envelope %v; want exit 1 and ok=false", exitCode, envelope)
				}
				for _, want := range tt.wantRefusal {
					if msg, _ := envelope["error"].(string); !strings.Contains(msg, want) {
						t.Errorf("error %q does not name its way forward %q", msg, want)
					}
				}
				if passArgs != nil || dobbySpawns != 0 {
					t.Errorf("a refused slug started a pass %v and %d dobbies; want nothing started", passArgs, dobbySpawns)
				}
				return
			}
			if exitCode != 0 || envelope["ok"] != true {
				t.Fatalf("exit %d, envelope %v; want exit 0 and ok=true", exitCode, envelope)
			}
			if !slices.Equal(passArgs, tt.wantPassArgs) || passDir != c.location.AnchorPath() {
				t.Errorf("pass started as %v in %q; want %v in %q", passArgs, passDir, tt.wantPassArgs, c.location.AnchorPath())
			}
			if envelope["pid"] != float64(4242) || envelope["parent_worktree"] != tt.wantParent {
				t.Errorf("envelope %v; want pid 4242 and parent_worktree %q", envelope, tt.wantParent)
			}
			if dobbySpawns != tt.wantDobbySpawn {
				t.Errorf("dobby spawns = %d; want %d", dobbySpawns, tt.wantDobbySpawn)
			}
			seed, seeded, err := shedrun.ReadSeed(c.location, c.slug)
			if err != nil {
				t.Fatalf("ReadSeed = %v; want nil", err)
			}
			if seeded && seed.Params[anchorParam] != tt.anchor {
				t.Errorf("seed anchor param = %q; want %q", seed.Params[anchorParam], tt.anchor)
			}
			warning, hasWarning := envelope["warning"].(string)
			if hasWarning != (tt.wantWarning != nil) {
				t.Errorf("envelope warning = %q (present %v); want present %v", warning, hasWarning, tt.wantWarning != nil)
			}
			for _, want := range tt.wantWarning {
				if !strings.Contains(warning, want) {
					t.Errorf("warning %q does not name %q", warning, want)
				}
			}
		})
	}
}
