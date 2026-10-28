//go:build tier1

// hubpull_integration_test.go drives `lym fabric pull --hub` through RunCLIIn from a task pair of a hubforge hub:
// the prime resolved from the pair, the real parent lock taken and released, and the envelope on a clean and a stopped pull.

package fabriccli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestRunCLI_PullHubFromATaskPair(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		dirtyPrime   bool
		wantExit     int
		wantOutcomes []string
	}{
		{"EveryMovedSidePulled", false, 0, []string{"board moved", "prime moved"}},
		{"DirtyPrimeStopsAndFailsTheVerb", true, 1, []string{"board moved", "prime stopped"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
			second := hubforge.CloneSecond(t, h)
			for _, dir := range []string{second.BoardDir(), second.PrimeWorktree()} {
				gitkit.CommitFile(t, dir, "moved.txt", "moved on origin", "move origin")
				gitkit.MustRun(t, dir, "git", "push")
			}
			if tt.dirtyPrime {
				tracked := strings.Split(strings.TrimSpace(gitkit.Git(t, h.PrimeWorktree(), "ls-files")), "\n")[0]
				if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), tracked), []byte("uncommitted"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			code, out := runFabric(t, h.PairCodeWorktree(hubforge.TemplatePairSlug), "pull", "--hub")
			if code != tt.wantExit {
				t.Errorf("RunCLI(pull --hub) = %d; want %d\noutput: %s", code, tt.wantExit, out)
			}
			env := envelope.Decode(t, out)
			sides, _ := env.Raw["sides"].([]any)
			var got []string
			for _, side := range sides {
				fields, _ := side.(map[string]any)
				got = append(got, fields["kind"].(string)+" "+fields["outcome"].(string))
			}
			if !slices.Equal(got, tt.wantOutcomes) {
				t.Errorf("sides = %v; want %v\noutput: %s", got, tt.wantOutcomes, out)
			}
			if tt.dirtyPrime && !strings.Contains(env.Error, "prime "+h.Location.WorktreeName) {
				t.Errorf("error = %q; want it to name the stopped prime", env.Error)
			}
			entries, err := (&parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(h.Path))}).List()
			if err != nil || len(entries) != 0 {
				t.Errorf("parent locks after the pull = (%v, %v); want none held", entries, err)
			}
		})
	}
}

// holdParentLock takes the parent lock of branch as an owner that is not the pull, and returns the store and a release.
func holdParentLock(t *testing.T, h *hubforge.Hub, branch string) (*parentlock.Store, func()) {
	t.Helper()
	store := &parentlock.Store{Dir: parentlock.Dir(fabricengine.BoardDir(h.Path))}
	startTime, _ := proc.StartTime(os.Getpid())
	holder := parentlock.Owner{Slug: "holder", Pair: filepath.Join(h.Path, "holder"), RunID: "holder-run", PID: os.Getpid(), StartTime: startTime}
	if attempt, err := store.TryAcquire(branch, holder); err != nil || !attempt.Granted {
		t.Fatalf("take the parent lock of %s = (%+v, %v); want it granted", branch, attempt, err)
	}
	return store, func() {
		if _, err := store.Release(branch, holder); err != nil {
			t.Errorf("release the parent lock of %s: %v", branch, err)
		}
	}
}

// waitForTicket polls the store until a waiter queues on branch, and returns its ticket.
func waitForTicket(t *testing.T, store *parentlock.Store, branch string) parentlock.Ticket {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := store.List()
		if err != nil {
			t.Fatalf("list the parent locks: %v", err)
		}
		for _, entry := range entries {
			if entry.Record.Branch == branch && len(entry.Waiters) > 0 {
				return entry.Waiters[0]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no waiter queued on the parent lock of %s within the deadline", branch)
	return parentlock.Ticket{}
}

func TestRunHubPull_WaitsForAHeldParentLock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		cancelWait   bool
		wantExit     int
		wantOutcomes []string
		wantMoved    bool
	}{
		{"RunsOnceTheHolderReleases", false, 0, []string{"board moved", "prime moved", "beam up_to_date"}, true},
		{"CancelledWaitStopsOnlyThePrimeSide", true, 1, []string{"board moved", "prime stopped", "beam up_to_date"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
			hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
			second := hubforge.CloneSecond(t, h)
			for _, dir := range []string{second.BoardDir(), second.PrimeWorktree()} {
				gitkit.CommitFile(t, dir, "moved.txt", "moved on origin", "move origin")
				gitkit.MustRun(t, dir, "git", "push")
			}
			branch := strings.TrimSpace(gitkit.Git(t, h.PrimeWorktree(), "rev-parse", "--abbrev-ref", "HEAD"))
			store, release := holdParentLock(t, h, branch)
			defer release()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out bytes.Buffer
			done := make(chan int, 1)
			go func() { done <- fabriccli.RunHubPullForTest(ctx, &out, h.Location) }()

			ticket := waitForTicket(t, store, branch)
			if ticket.Owner.RunID != "fabric-pull" {
				t.Errorf("waiter run id = %q; want fabric-pull", ticket.Owner.RunID)
			}
			select {
			case code := <-done:
				t.Fatalf("hub pull returned %d while the parent lock was held; want it to wait\noutput: %s", code, out.String())
			default:
			}
			if tt.cancelWait {
				cancel()
			} else {
				release()
			}
			code := <-done
			if code != tt.wantExit {
				t.Errorf("hub pull = %d; want %d\noutput: %s", code, tt.wantExit, out.String())
			}
			env := envelope.Decode(t, out.String())
			sides, _ := env.Raw["sides"].([]any)
			var got []string
			for _, side := range sides {
				fields, _ := side.(map[string]any)
				got = append(got, fields["kind"].(string)+" "+fields["outcome"].(string))
			}
			if !slices.Equal(got, tt.wantOutcomes) {
				t.Errorf("sides = %v; want %v\noutput: %s", got, tt.wantOutcomes, out.String())
			}
			_, statErr := os.Stat(filepath.Join(h.PrimeWorktree(), "moved.txt"))
			if moved := statErr == nil; moved != tt.wantMoved {
				t.Errorf("prime pulled = %v; want %v", moved, tt.wantMoved)
			}
		})
	}
}
