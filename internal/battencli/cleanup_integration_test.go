//go:build integration

// cleanup_integration_test.go drives "lyx batten cleanup" through RunCLIIn over a hubforge hub: the prime-only refusal, the dry run and the applied removal with its records commit and push.
// It stays a white-box "package battencli" test, like the other integration files here.

package battencli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// stringsOf converts a decoded JSON array of strings.
func stringsOf(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value %v is not a JSON array", value)
	}
	var out []string
	for _, item := range items {
		out = append(out, item.(string))
	}
	return out
}

func runCleanup(t *testing.T, cwd string, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, append([]string{"cleanup"}, args...))
	return out.String(), code
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}

func TestCleanupIntegration_RefusesOutsideThePrimeThenDryRunsThenRemovesAndPushes(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	const slug = "finished-task"

	writeCleanupFile(t, shedrun.StatusFile(h.Location, slug), statusJSON(t, "done"))
	writeCleanupFile(t, shedrun.SeedFile(h.Location, slug), []byte("{}"))
	rels := []string{shedrun.SeedRel(h.Location, slug), shedrun.StatusRel(h.Location, slug)}
	if _, committed, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), h.Location, rels, "test: finished run", fabricengine.EnvSyncOptions()); err != nil || !committed {
		t.Fatalf("commit the finished run: committed=%v err=%v", committed, err)
	}
	if _, err := fabricengine.PushAnchored(h.Location, fabricengine.EnvSyncOptions(), fabricengine.StatusPushLockWait); err != nil {
		t.Fatalf("push the finished run: %v", err)
	}

	scratch := shedrun.ScratchDir(h.Location, slug)
	for _, name := range []string{"run.lock", "status.json.lock", "last-commit"} {
		writeCleanupFile(t, filepath.Join(scratch, name), nil)
	}
	runDir := shedrun.RunDir(h.Location, slug)

	assertUntouched := func(t *testing.T, step string) {
		t.Helper()
		if !pathExists(runDir) || !pathExists(filepath.Join(scratch, "last-commit")) {
			t.Errorf("%s: the run directory or its scratch was removed", step)
		}
		if _, found, err := fabricengine.CommittedAnchoredFile(h.Location, shedrun.StatusRel(h.Location, slug)); err != nil || !found {
			t.Errorf("%s: committed status found=%v err=%v, want it still at the records HEAD", step, found, err)
		}
	}

	taskCwd := h.PairCodeWorktree(hubforge.TemplatePairSlug)
	out, code := runCleanup(t, taskCwd, "--apply")
	primeName, err := fabricengine.PrimeName(h.Location)
	if err != nil {
		t.Fatalf("prime name: %v", err)
	}
	envelope.RequireErr(t, out, primeName)
	if code == 0 {
		t.Errorf("cleanup from the task worktree exited 0")
	}
	assertUntouched(t, "refusal")

	out, code = runCleanup(t, h.PrimeWorktree())
	dry := envelope.RequireOK(t, out)
	if code != 0 || dry.Raw["applied"] != false {
		t.Errorf("dry run: code=%d applied=%v", code, dry.Raw["applied"])
	}
	if got := stringsOf(t, dry.Raw["removed"]); !slices.Equal(got, []string{slug}) {
		t.Errorf("dry run removed = %v, want [%s]", got, slug)
	}
	if got := stringsOf(t, dry.Raw["emptied"]); !slices.Equal(got, []string{slug}) {
		t.Errorf("dry run emptied = %v, want [%s]", got, slug)
	}
	assertUntouched(t, "dry run")

	out, code = runCleanup(t, h.PrimeWorktree(), "--apply")
	applied := envelope.RequireOK(t, out)
	if code != 0 || applied.Raw["applied"] != true || applied.Raw["push_error"] != "" {
		t.Errorf("apply: code=%d applied=%v push_error=%v", code, applied.Raw["applied"], applied.Raw["push_error"])
	}
	if got := stringsOf(t, applied.Raw["removed"]); !slices.Equal(got, []string{slug}) {
		t.Errorf("apply removed = %v, want [%s]", got, slug)
	}

	if pathExists(runDir) {
		t.Errorf("the run directory survived --apply")
	}
	if _, found, err := fabricengine.CommittedAnchoredFile(h.Location, shedrun.StatusRel(h.Location, slug)); err != nil || found {
		t.Errorf("committed status found=%v err=%v, want it gone from the records HEAD", found, err)
	}
	records := h.PrimeRecords()
	branch := gitOutput(t, records, "rev-parse", "--abbrev-ref", "HEAD")
	if local, remote := gitOutput(t, records, "rev-parse", "HEAD"), gitOutput(t, h.RecordsBare, "rev-parse", "refs/heads/"+branch); local != remote {
		t.Errorf("records tip %s was not pushed: remote %s is at %s", local, branch, remote)
	}
	for _, name := range []string{"run.lock", "status.json.lock"} {
		if !pathExists(filepath.Join(scratch, name)) {
			t.Errorf("lock file %s did not survive --apply", name)
		}
	}
	if pathExists(filepath.Join(scratch, "last-commit")) {
		t.Errorf("scratch file last-commit survived --apply")
	}
}
