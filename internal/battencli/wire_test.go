// wire_test.go proves no seam wire (wire.go) builds is evaluated at wiring time: building the
// wiring for a slug whose worktree does not exist must succeed, and none of the four seams that
// resolve the managed task worktree's own Location -- the status-path resolver, the spawn
// directory, and both teardown halves -- may be invoked here, since each of the three that actually
// resolves that Location reaches the resolver (lyxcwd.ResolveWorktree), which spawns git; this
// suite stays untagged and Tier 1, so it proves laziness structurally rather than by invoking a
// seam and observing it run.

package battencli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWire_BuildsLazilyForANonexistentTaskWorktree asserts wire returns no error even though the managed task worktree named by slug does not exist anywhere on disk -- the mechanical proof that wire itself resolves nothing about that worktree -- and that all lazy seams are each present as an injected closure after wire returns, not already-evaluated values: the status-path resolver (Env.InnerRun.ResolveStatus), the spawn directory (Env.InnerRun.Spawn), both teardown halves (Env.Teardown.Shutdown, Env.Teardown.Remove) and the rest listed below, plus the durable status file's commit seam (shedPaths.CommitStatus).
// Covering every seam separately is deliberate: eager evaluation is exactly the failure laziness exists to avoid, and a check on only one seam would let the others regress silently.
func TestWire_BuildsLazilyForANonexistentTaskWorktree(t *testing.T) {
	t.Parallel()

	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	tests := []struct {
		name    string
		present bool
	}{
		{"StatusPathResolver", c.env.InnerRun.ResolveStatus != nil},
		{"SpawnDirectory", c.env.InnerRun.Spawn != nil},
		{"TeardownShutdown", c.env.Teardown.Shutdown != nil},
		{"TeardownRemove", c.env.Teardown.Remove != nil},
		{"ReadDecision", c.env.InnerRun.ReadDecision != nil},
		{"DriverAlive", c.env.InnerRun.DriverAlive != nil},
		{"DriverStrand", c.env.InnerRun.DriverStrand != nil},
		{"ChildRunLockHeld", c.env.InnerRun.ChildRunLockHeld != nil},
		{"ReviveStrands", c.env.InnerRun.ReviveStrands != nil},
		{"RunShedMarkWatched", c.env.InnerRun.MarkWatched != nil},
		{"SeedChildMarkWatched", c.env.SeedChild.MarkWatched != nil},
		{"OpenIDE", c.env.InnerRun.OpenIDE != nil},
		{"CommitStatus", c.shedPaths.CommitStatus != nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !tt.present {
				t.Errorf("wire() left this seam nil; want an injected closure present but uncalled")
			}
		})
	}
}

// TestDriverAliveFrom covers driverAliveFrom's answers without tmux: an absent task worktree is false without reading status, an absent reed session is false with no error, any other status error is returned, and only a live driver strand, under its full name, its role or the legacy loom-driver literal, is true.
func TestDriverAliveFrom(t *testing.T) {
	boom := errors.New("boom")
	status := func(res reedengine.StatusResult, err error) func() (reedengine.StatusResult, error) {
		return func() (reedengine.StatusResult, error) { return res, err }
	}
	strands := func(ss ...reedengine.StrandStatus) reedengine.StatusResult {
		return reedengine.StatusResult{Strands: ss}
	}
	tests := []struct {
		name    string
		present bool
		status  func() (reedengine.StatusResult, error)
		want    bool
		wantErr error
	}{
		{"WorktreeAbsentSkipsStatus", false, func() (reedengine.StatusResult, error) {
			t.Error("status read although the task worktree is absent")
			return reedengine.StatusResult{}, nil
		}, false, nil},
		{"NoSessionIsNotLive", true, status(reedengine.StatusResult{}, fmt.Errorf("wrapped: %w", reedengine.ErrNoSession)), false, nil},
		{"OtherErrorReturned", true, status(reedengine.StatusResult{}, boom), false, boom},
		{"LiveDriver", true, status(strands(reedengine.StrandStatus{Name: loomengine.LoomDriverStrandName, Live: true}), nil), true, nil},
		{"LiveFullNameDriver", true, status(strands(reedengine.StrandStatus{Name: "ly:task:driver", Live: true}), nil), true, nil},
		{"LiveLegacyDriver", true, status(strands(reedengine.StrandStatus{Name: loomengine.LegacyLoomDriverStrandName, Live: true}), nil), true, nil},
		{"DeadDriver", true, status(strands(reedengine.StrandStatus{Name: loomengine.LoomDriverStrandName}), nil), false, nil},
		{"OtherStrandLiveOnly", true, status(strands(reedengine.StrandStatus{Name: "other", Live: true}), nil), false, nil},
		{"NoStrands", true, status(reedengine.StatusResult{}, nil), false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := driverAliveFrom(tt.present, tt.status)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("driverAliveFrom() error = %v; want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("driverAliveFrom() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestDriverStrandFrom covers driverStrandFrom's answers without tmux:
// an absent task worktree is none without reading the directory,
// a directory error is returned,
// and a driver row is live, retiring or dead by its flags, under its full name, its role or the legacy loom-driver literal.
func TestDriverStrandFrom(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	rows := func(rs ...reedengine.DirectoryRow) func() ([]reedengine.DirectoryRow, error) {
		return func() ([]reedengine.DirectoryRow, error) { return rs, nil }
	}
	tests := []struct {
		name      string
		present   bool
		directory func() ([]reedengine.DirectoryRow, error)
		want      battenshed.ChildDriverStrand
		wantErr   error
	}{
		{"WorktreeAbsentSkipsTheDirectory", false, func() ([]reedengine.DirectoryRow, error) {
			t.Error("directory read although the task worktree is absent")
			return nil, nil
		}, battenshed.ChildDriverNone, nil},
		{"DirectoryErrorReturned", true, func() ([]reedengine.DirectoryRow, error) { return nil, boom }, battenshed.ChildDriverNone, boom},
		{"NoRows", true, rows(), battenshed.ChildDriverNone, nil},
		{"NoDriverRow", true, rows(reedengine.DirectoryRow{Name: "other", Live: true}), battenshed.ChildDriverNone, nil},
		{"LiveDriver", true, rows(reedengine.DirectoryRow{Name: loomengine.LoomDriverStrandName, Live: true}), battenshed.ChildDriverLive, nil},
		{"LiveFullNameDriver", true, rows(reedengine.DirectoryRow{Name: "ly:task:driver", Live: true}), battenshed.ChildDriverLive, nil},
		{"LiveLegacyDriver", true, rows(reedengine.DirectoryRow{Name: loomengine.LegacyLoomDriverStrandName, Live: true}), battenshed.ChildDriverLive, nil},
		{"RetiringDriver", true, rows(reedengine.DirectoryRow{Name: loomengine.LoomDriverStrandName, Live: true, Retiring: true}), battenshed.ChildDriverRetiring, nil},
		{"DeadDriver", true, rows(reedengine.DirectoryRow{Name: loomengine.LoomDriverStrandName}), battenshed.ChildDriverDead, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := driverStrandFrom(tt.present, tt.directory)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("driverStrandFrom() error = %v; want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("driverStrandFrom() = %v; want %v", got, tt.want)
			}
		})
	}
}

// TestRunLockHeld covers runLockHeld's answers over a real lock file: a path whose directory does not exist yet is free,
// and a held lock reads as held and reads as free again once released.
func TestRunLockHeld(t *testing.T) {
	t.Parallel()

	lockPath := filepath.Join(t.TempDir(), "absent-dir", "run.lock")

	held, err := runLockHeld(lockPath)
	if err != nil || held {
		t.Fatalf("runLockHeld(unheld, directory absent) = (%v, %v); want (false, nil)", held, err)
	}

	owner, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireWriteLock: %v", err)
	}
	held, err = runLockHeld(lockPath)
	if err != nil || !held {
		t.Errorf("runLockHeld(held) = (%v, %v); want (true, nil)", held, err)
	}

	if err := owner.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	held, err = runLockHeld(lockPath)
	if err != nil || held {
		t.Errorf("runLockHeld(released) = (%v, %v); want (false, nil), a probe never keeping the lock", held, err)
	}
}

// TestWire_ChildDriverDelegatesToChildDriverOf asserts the wired SeedChild.ChildDriver closure
// reads exactly the value childDriverOf(seed) (arm.go) would compute over the same seed, for both
// an explicit child_driver param and an absent one -- the property F2 (crucible round
// sonnet-xhigh-r3) exists to guarantee structurally: refuseAdoptedSeed's own comparison and the
// value Seed-Child actually writes now share one defaulting implementation, so they cannot drift.
func TestWire_ChildDriverDelegatesToChildDriverOf(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"ExplicitLLM", map[string]string{"child_driver": shedrun.DriverLLM}, shedrun.DriverLLM},
		{"AbsentParamDefaultsToGo", nil, shedrun.DriverGo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &battenCLI{}
			location := &lyxcwd.Location{
				RepoName:     "example",
				HubPath:      t.TempDir(),
				WorktreeName: "hub-repo",
				AnchorRel:    ".",
			}
			if err := c.wire(location, "some-slug"); err != nil {
				t.Fatalf("wire() error = %v; want nil", err)
			}
			seed := shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, Params: tt.params}
			if err := shedrun.WriteSeed(location, "some-slug", seed); err != nil {
				t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
			}

			got, err := c.env.SeedChild.ChildDriver()
			if err != nil {
				t.Fatalf("ChildDriver() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("ChildDriver() = %q; want %q", got, tt.want)
			}
			if want := childDriverOf(seed); got != want {
				t.Errorf("ChildDriver() = %q; want it to equal childDriverOf(seed) = %q", got, want)
			}
		})
	}
}

// TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree pins the order inside the wired
// WriteSeed seam: a registered recipe the task worktree cannot bootstrap is refused with
// battenshed.ErrUnsupportedChildRecipe before the seam resolves the task worktree at all -- the
// location here has no worktree, so a loom recipe reaches the absent-pair refusal instead, which
// is an os.Stat, never the resolver, so this file stays Tier 1.
func TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}
	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	err := c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeBatten, shedrun.DriverGo)
	if !errors.Is(err, battenshed.ErrUnsupportedChildRecipe) {
		t.Fatalf("WriteSeed(batten) error = %v; want it to wrap ErrUnsupportedChildRecipe", err)
	}
	if !strings.Contains(err.Error(), shedrun.RecipeLoom) {
		t.Errorf("WriteSeed(batten) error = %q; want it to name the one recipe a child may run", err)
	}

	err = c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeLoom, shedrun.DriverGo)
	if errors.Is(err, battenshed.ErrUnsupportedChildRecipe) || errors.Is(err, battenshed.ErrUnknownRecipe) {
		t.Fatalf("WriteSeed(loom) error = %v; want a loom child admitted past the recipe checks", err)
	}
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Errorf("WriteSeed(loom) error = %v; want the absent-pair refusal, proving the recipe check ran first", err)
	}
}

// TestChildSeedFor_CarriesRecipeDriverAndParams asserts the child's seed carries the given recipe, driver and params.
//
//testtiming:keep pins the child seed's driver and params, which the end-to-end four-row run covering it never reads back
func TestChildSeedFor_CarriesRecipeDriverAndParams(t *testing.T) {
	params := map[string]string{"parent": "main"}

	got := childSeedFor(shedrun.RecipeLoom, shedrun.DriverLLM, params)
	if got.Recipe != shedrun.RecipeLoom || got.Driver != shedrun.DriverLLM || got.Params["parent"] != "main" {
		t.Errorf("childSeedFor() = %+v; want the given recipe, driver and params", got)
	}
}

// TestChildSpawnError asserts a child bootstrap's exit status is turned into a diagnosis: a non-zero exit carries the child's own output, a silent child passes the run error through, a nil run error stays nil, and over-long output is truncated with an explicit marker, on a valid UTF-8 boundary even when a multi-byte rune straddles it -- a naive byte slice at maxChildOutputInError can land mid-rune and embed an invalid tail.
// Only the start refusal of kind shedrun.StartNotParkedKind reaches InnerRun as battenshed.ErrChildNotParked, wherever it sits in the output.
func TestChildSpawnError(t *testing.T) {
	t.Parallel()

	runErr := errors.New("exit status 1")
	longOutput := strings.Repeat("x", maxChildOutputInError+50)
	notParkedLine := `{"error":"loom: the driver has not parked yet","kind":"` + shedrun.StartNotParkedKind + `","ok":false}`

	tests := []struct {
		name          string
		runErr        error
		childOutput   string
		wantNil       bool
		wantNotParked bool
		wantSubstr    []string
	}{
		{
			name:        "nil_run_error_stays_nil",
			runErr:      nil,
			childOutput: `{"ok":true}`,
			wantNil:     true,
		},
		{
			name:        "output_is_folded_into_the_error",
			runErr:      runErr,
			childOutput: `{"error":"shedrun: refusing to overwrite with disagreeing seed","ok":false}`,
			wantSubstr:  []string{"exit status 1", "disagreeing seed"},
		},
		{
			name:          "not_parked_refusal_wraps_the_sentinel_and_the_run_error",
			runErr:        runErr,
			childOutput:   notParkedLine,
			wantNotParked: true,
			wantSubstr:    []string{"exit status 1", "not parked yet"},
		},
		{
			name:          "not_parked_refusal_after_log_noise_still_wraps_the_sentinel",
			runErr:        runErr,
			childOutput:   "log noise\n" + notParkedLine + "\n",
			wantNotParked: true,
			wantSubstr:    []string{"exit status 1"},
		},
		{
			name:        "another_refusal_is_not_the_sentinel",
			runErr:      runErr,
			childOutput: `{"error":"loom: some other refusal","ok":false}`,
			wantSubstr:  []string{"exit status 1", "some other refusal"},
		},
		{
			name:        "another_refusal_kind_is_not_the_sentinel",
			runErr:      runErr,
			childOutput: `{"error":"x","kind":"busy","ok":false}`,
			wantSubstr:  []string{"exit status 1"},
		},
		{
			name:        "non_json_output_naming_the_kind_is_not_the_sentinel",
			runErr:      runErr,
			childOutput: `not json ` + shedrun.StartNotParkedKind,
			wantSubstr:  []string{"exit status 1"},
		},
		{
			name:        "silent_child_passes_the_run_error_through",
			runErr:      runErr,
			childOutput: "   \n  ",
			wantSubstr:  []string{"exit status 1"},
		},
		{
			name:        "over_long_output_is_truncated_visibly",
			runErr:      runErr,
			childOutput: longOutput,
			wantSubstr:  []string{"exit status 1", "... (truncated)"},
		},
		{
			name:        "truncation_keeps_a_straddling_multi_byte_rune_whole",
			runErr:      runErr,
			childOutput: strings.Repeat("x", maxChildOutputInError-1) + "€ trailing text after the cut point",
			wantSubstr:  []string{"exit status 1", "... (truncated)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := childSpawnError(tt.runErr, tt.childOutput)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("childSpawnError(nil, %q) = %v; want nil", tt.childOutput, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("childSpawnError(%v, ...) = nil; want an error", tt.runErr)
			}
			if !errors.Is(got, tt.runErr) {
				t.Errorf("childSpawnError(...) does not unwrap to the run error; want errors.Is to hold")
			}
			if errors.Is(got, battenshed.ErrChildNotParked) != tt.wantNotParked {
				t.Errorf("childSpawnError(...) = %v; errors.Is(ErrChildNotParked) = %v, want %v", got, !tt.wantNotParked, tt.wantNotParked)
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("childSpawnError(...) = %q; want it to contain %q", got.Error(), want)
				}
			}
			if !utf8.ValidString(got.Error()) {
				t.Errorf("childSpawnError(...) = %q; want valid UTF-8", got.Error())
			}
			if len(got.Error()) > maxChildOutputInError+200 {
				t.Errorf("childSpawnError(...) produced %d bytes; want the output capped near maxChildOutputInError", len(got.Error()))
			}
		})
	}
}

// TestTaskWorktree_AbsentPairIsAnAnswerAndANamedRefusal asserts an unmaterialized task worktree is reported on its own terms.
// taskWorktreePresent, the create row's idempotency probe, answers false with no error, so a genuinely absent worktree still reaches fabric's own create rather than short-circuiting the row.
// taskWorktreeLocation names the run and the expected path and points at a real remedy, never a fabric command that would actually mutate prime itself, rather than the resolver's generic "not a git repository" failure.
//
// The present case needs a real git worktree and so lives at the integration tier (TestBattenIntegration_Rows); this suite stays untagged and never spawns git.
//
//testtiming:keep pins the absent-pair answer and the refusal's wording, which the covering tests never assert
func TestTaskWorktree_AbsentPairIsAnAnswerAndANamedRefusal(t *testing.T) {
	t.Parallel()

	prime := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}

	present, err := taskWorktreePresent(prime, "never-created")
	if err != nil {
		t.Fatalf("taskWorktreePresent(prime, \"never-created\") error = %v; want nil", err)
	}
	if present {
		t.Error("taskWorktreePresent(prime, \"never-created\") = true; want false")
	}

	_, err = taskWorktreeLocation(prime, "never-created")
	if err == nil {
		t.Fatal("taskWorktreeLocation(prime, \"never-created\") = nil error; want a named refusal")
	}
	for _, want := range []string{"never-created", "is not present at", "Resolve it by hand", shedrun.RunDir(prime, "never-created"), "local and remote", "discarding any of the task's work"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("taskWorktreeLocation(...) = %q; want it to contain %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "lyx fabric checkout") {
		t.Errorf("taskWorktreeLocation(...) = %q; want it to never suggest \"lyx fabric checkout\" -- run from prime, that command mutates prime's own branch rather than restoring the missing task worktree", err.Error())
	}
	if strings.Contains(err.Error(), "so a resumed run reaches a fresh create") {
		t.Errorf("taskWorktreeLocation(...) = %q; want it to never promise that deleting branches alone rewinds the run -- the resumed run re-enters its persisted row, not Worktree-Create", err.Error())
	}
	if strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("taskWorktreeLocation(...) = %q; want the absent-pair case reported on its own terms, not as a resolver failure", err.Error())
	}
}

// TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout asserts the create closure rewords
// fabric's leftover-branch refusal for a caller standing in prime -- naming the branch and its
// deletion on both the local and the remote side, and never "lyx fabric checkout", which from prime
// switches prime itself -- while every other create error passes through unchanged.
func TestCreateRefusal_LeftoverBranchRemedyNeverNamesCheckout(t *testing.T) {
	leftover := &fabricengine.ErrBranchExists{Branch: "lyx-some-slug"}
	other := errors.New("source worktree has uncommitted changes")

	tests := []struct {
		name        string
		err         error
		wantContain []string
		wantSame    bool
	}{
		{"LeftoverBranch", leftover, []string{`"lyx-some-slug"`, "git branch -D lyx-some-slug", "git push origin --delete lyx-some-slug", "lyx fabric cleanup --apply --remote", "resume this run"}, false},
		{"WrappedLeftoverBranch", fmt.Errorf("create: %w", leftover), []string{"git push origin --delete lyx-some-slug"}, false},
		{"OtherError", other, []string{other.Error()}, true},
		{"NilError", nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createRefusal(tt.err)
			if tt.wantSame {
				if got != tt.err {
					t.Fatalf("createRefusal(%v) = %v; want the same error passed through", tt.err, got)
				}
				return
			}
			if got == nil {
				t.Fatal("createRefusal(leftover branch) = nil; want a reworded refusal")
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("createRefusal(...) = %q; want it to contain %q", got.Error(), want)
				}
			}
			if strings.Contains(got.Error(), "lyx fabric checkout") && !strings.Contains(got.Error(), "never \"lyx fabric checkout\"") {
				t.Errorf("createRefusal(...) = %q; want it to never suggest \"lyx fabric checkout\" from prime", got.Error())
			}
		})
	}
}

// TestTeardownRefusal_RecordsRemedyNamesCommitRecordsNeverForce asserts a sibling-dirt refusal is reworded to batten's own recovery -- inspect the sibling's out-of-pathspec content, commit or remove it by hand, then resume -- and never names --force, that a failed archive names the resume, and that every other teardown error passes through unchanged.
func TestTeardownRefusal_RecordsRemedyNamesCommitRecordsNeverForce(t *testing.T) {
	dirty := fmt.Errorf("remove: %w", fabricengine.ErrPairSiblingDirty)

	got := teardownRefusal(dirty, "some-slug")
	if got == nil {
		t.Fatal("teardownRefusal(sibling dirty) = nil; want a reworded refusal")
	}
	if !errors.Is(got, fabricengine.ErrPairSiblingDirty) {
		t.Errorf("teardownRefusal(sibling dirty) = %v; want it to still wrap ErrPairSiblingDirty", got)
	}
	for _, want := range []string{"some-slug", "sibling worktree under the hub", "outside the run record paths", "commit or remove that content by hand", "lyx batten run some-slug"} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("teardownRefusal(sibling dirty) = %q; want it to contain %q", got.Error(), want)
		}
	}
	if strings.Contains(got.Error(), "--force") {
		t.Errorf("teardownRefusal(sibling dirty) = %q; want it to never name --force", got.Error())
	}

	archiveFailed := fmt.Errorf("%w: push refused", fabricengine.ErrArchiveFailed)
	got = teardownRefusal(archiveFailed, "some-slug")
	if !errors.Is(got, fabricengine.ErrArchiveFailed) {
		t.Errorf("teardownRefusal(archive failed) = %v; want it to still wrap ErrArchiveFailed", got)
	}
	for _, want := range []string{"left in place", "push refused", "fix the failure named here", "lyx batten run some-slug"} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("teardownRefusal(archive failed) = %q; want it to contain %q", got.Error(), want)
		}
	}
	if strings.Contains(got.Error(), "--force") {
		t.Errorf("teardownRefusal(archive failed) = %q; want it to never name --force", got.Error())
	}

	other := errors.New("the task worktree has uncommitted changes")
	if got := teardownRefusal(other, "some-slug"); got != other {
		t.Errorf("teardownRefusal(other) = %v; want the same error passed through", got)
	}
}

// TestFinishRemoval asserts a removal that finds nothing of the pair reports done -- the state a process killed right after Remove succeeded leaves, since shedengine persists the transition only after the producer returns -- rather than stranding the run at teardown, that a finished half-removed pair reports done, and that a remote branch left behind halts the row resumable, with no verdict naming the retired sibling-remnant refusal or its fabric prune remedy.
// The real half-removed state is driven at the integration tier.
//
//testtiming:keep pins that no removal verdict names the retired fabric prune remedy, which the integration steps never assert
func TestFinishRemoval(t *testing.T) {
	t.Parallel()

	const slug = "half-torn"

	tests := []struct {
		name       string
		result     fabricengine.RemoveResult
		removeErr  error
		wantErr    bool
		wantSubstr []string
	}{
		{
			name:      "NotFoundIsDone",
			removeErr: fmt.Errorf("remove: %w: nothing of %q remains", fabricengine.ErrPairNotFound, slug),
		},
		{name: "FinishedHalfRemovedPairIsDone", result: fabricengine.RemoveResult{Finished: true}},
		{
			name:       "RemoteBranchErrorHaltsResumable",
			result:     fabricengine.RemoveResult{RemoteBranchError: "push refused"},
			wantErr:    true,
			wantSubstr: []string{slug, "push refused", "resume this run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := finishRemoval(tt.result, tt.removeErr, slug)
			if !tt.wantErr {
				if err != nil {
					t.Errorf("finishRemoval(%s) = %v; want nil", tt.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("finishRemoval(%s) = nil; want the row to halt resumable", tt.name)
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("finishRemoval(%s) = %q; want it to contain %q", tt.name, err.Error(), want)
				}
			}
			if strings.Contains(err.Error(), "lyx fabric prune") {
				t.Errorf("finishRemoval(%s) = %q; want it to never name lyx fabric prune", tt.name, err.Error())
			}
		})
	}
}
