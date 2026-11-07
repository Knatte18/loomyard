// batchfail_integration_test.go exercises failBatch's suspect-blob recording over a real scratch git repository.

package websterengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// suspectRepo returns a scratch git repo with the tracked file internal/x.go.
// The file is committed as "orig", then modified in the worktree to "forged".
func suspectRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		stdout, stderr, code, err := gitexec.RunGit(args, dir)
		if err != nil || code != 0 {
			t.Fatalf("git %v: %v (exit %d): %s", args, err, code, stderr)
		}
		return strings.TrimSpace(stdout)
	}
	git("init")
	git("config", "user.name", "Test User")
	git("config", "user.email", "test@example.com")
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "internal", "x.go"), []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "internal", "x.go"), []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFailBatch_RecordsSuspectBlobs(t *testing.T) {
	tiergate.Require(t, 1)
	repo := suspectRepo(t)
	want, err := gitexec.Run([]string{"hash-object", "--", "internal/x.go"}, repo)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := failBatchFixture(t, true)
	in.WorktreeRoot = repo
	in.PaneCwd = repo
	in.SuspectPaths = []string{"internal/x.go", "internal/gone.go"}
	if _, err := failBatch(in); err != nil {
		t.Fatal(err)
	}
	got := in.Batch.SuspectPaths
	if len(got) != 2 {
		t.Fatalf("SuspectPaths = %+v", got)
	}
	if got[0].Path != "internal/x.go" || got[0].Blob != strings.TrimSpace(want) {
		t.Errorf("modified tracked file = %+v, want blob %s", got[0], strings.TrimSpace(want))
	}
	if got[1].Path != "internal/gone.go" || got[1].Blob != "" {
		t.Errorf("absent file = %+v, want empty blob", got[1])
	}

	// A pane cwd below the worktree root resolves a relative suspect path there,
	// and the blob probe still runs on the worktree root.
	split, _ := failBatchFixture(t, true)
	split.WorktreeRoot = repo
	split.PaneCwd = filepath.Join(repo, "internal")
	split.SuspectPaths = []string{"x.go"}
	if _, err := failBatch(split); err != nil {
		t.Fatal(err)
	}
	if got := split.Batch.SuspectPaths; len(got) != 1 || got[0].Path != "x.go" || got[0].Blob != strings.TrimSpace(want) {
		t.Errorf("suspect path relative to a subdirectory pane cwd = %+v, want x.go with blob %s", got, strings.TrimSpace(want))
	}
}
