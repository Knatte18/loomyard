// lock_test.go runs the lock verbs through the command tree against a temporary parent lock store.

package battencli

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// runLockVerb runs args through the batten command tree and returns the exit code and the printed envelope.
func runLockVerb(args ...string) (int, string) {
	var out bytes.Buffer
	code := clihelp.Execute(Command(), &out, append([]string{"lock"}, args...))
	return code, out.String()
}

// TestLockVerbs asserts the lock verbs against a temporary store.
// List reports each record's owner, liveness, age and waiters and an unreadable record with its error.
// Release frees one branch or, with --all, every branch and names them, and the refusals name their way forward.
// It replaces openLockStore, which is process-global, so it runs serially.
func TestLockVerbs(t *testing.T) {
	store := &parentlock.Store{Dir: t.TempDir()}
	original := openLockStore
	openLockStore = func(context.Context) (*parentlock.Store, error) { return store, nil }
	t.Cleanup(func() { openLockStore = original })

	self := proc.SelfRecord()
	holder := parentlock.Owner{Slug: "holder-task", Pair: "/hub/holder-task", RunID: "run-1", PID: self.PID, StartTime: self.StartTime}
	// A queued waiter counts only while its process lives, so the waiter runs as this test's parent process.
	waiterPID := os.Getppid()
	waiterStart, _ := proc.StartTime(waiterPID)
	waiter := parentlock.Owner{Slug: "waiting-task", Pair: "/hub/waiting-task", RunID: "run-2", PID: waiterPID, StartTime: waiterStart}
	acquire := func(branch string, owner parentlock.Owner) {
		t.Helper()
		if _, err := store.TryAcquire(branch, owner); err != nil {
			t.Fatalf("TryAcquire(%s) error = %v", branch, err)
		}
	}

	t.Run("ListReportsEveryRecord", func(t *testing.T) {
		acquire("main", holder)
		acquire("main", waiter)
		acquire("develop", holder)
		unreadable := filepath.Join(store.Dir, hex.EncodeToString([]byte("broken"))+".json")
		if err := os.WriteFile(unreadable, []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}

		code, out := runLockVerb("list")

		env := envelope.RequireOK(t, out)
		locks, _ := env.Raw["locks"].([]any)
		byBranch := map[string]map[string]any{}
		for _, lock := range locks {
			entry, _ := lock.(map[string]any)
			byBranch[entry["branch"].(string)] = entry
		}
		if code != 0 || len(byBranch) != 3 {
			t.Fatalf("list = exit %d, branches %v; want exit 0 and main, develop and broken", code, byBranch)
		}
		main := byBranch["main"]
		if main["slug"] != "holder-task" || main["pair"] != "/hub/holder-task" || main["run_id"] != "run-1" || main["pid"] != float64(self.PID) || main["alive"] != true {
			t.Errorf("main entry = %v; want the holder's slug, pair, run id, pid and liveness", main)
		}
		if age, _ := main["age"].(string); age == "" {
			t.Errorf("main entry = %v; want an age", main)
		}
		waiters, _ := main["waiters"].([]any)
		if len(waiters) != 1 || waiters[0].(map[string]any)["slug"] != "waiting-task" {
			t.Errorf("main waiters = %v; want the one queued waiting-task", waiters)
		}
		if msg, _ := byBranch["broken"]["error"].(string); msg == "" {
			t.Errorf("broken entry = %v; want its read error", byBranch["broken"])
		}
		if err := os.Remove(unreadable); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ReleaseFreesOneBranchAndRefusesAnUnknownOne", func(t *testing.T) {
		code, out := runLockVerb("release", "develop")
		env := envelope.RequireOK(t, out)
		if code != 0 || env.Raw["released"].([]any)[0] != "develop" {
			t.Errorf("release develop = exit %d, %v; want it released", code, env.Raw)
		}
		if _, found, err := store.Read("develop"); err != nil || found {
			t.Errorf("record after the release = found %v, error %v; want none", found, err)
		}

		code, out = runLockVerb("release", "develop")
		envelope.RequireErr(t, out, `no parent lock on "develop"; way forward: run "lyx batten lock list" to see the held locks`)
		if code != 1 {
			t.Errorf("release of an unknown branch = exit %d; want 1", code)
		}
	})

	t.Run("ReleaseAllNamesEveryBranch", func(t *testing.T) {
		acquire("release", holder)

		code, out := runLockVerb("release", "--all")

		env := envelope.RequireOK(t, out)
		released, _ := env.Raw["released"].([]any)
		if code != 0 || len(released) != 2 {
			t.Errorf("release --all = exit %d, %v; want main and release named", code, env.Raw)
		}
		code, out = runLockVerb("release", "--all")
		env = envelope.RequireOK(t, out)
		if released, _ := env.Raw["released"].([]any); code != 0 || released == nil || len(released) != 0 {
			t.Errorf("release --all over an empty store = exit %d, %v; want an empty list", code, env.Raw)
		}
	})

	t.Run("ReleaseRefusesABranchBesideAllAndNeitherForm", func(t *testing.T) {
		for _, args := range [][]string{{"release", "main", "--all"}, {"release"}} {
			code, out := runLockVerb(args...)
			envelope.RequireErr(t, out, `way forward: run "lyx batten lock release <parent-branch>" or "lyx batten lock release --all"`)
			if code != 1 {
				t.Errorf("%v = exit %d; want 1", args, code)
			}
		}
	})

	t.Run("RefusesOutsideAHubPrime", func(t *testing.T) {
		openLockStore = original
		t.Chdir(t.TempDir())

		code, out := runLockVerb("list")

		if env := envelope.Decode(t, out); code != 1 || env.OK || env.Error == "" {
			t.Errorf("list outside a repository = exit %d, %q; want an error envelope", code, out)
		}
	})
}
