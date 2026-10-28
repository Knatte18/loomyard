//go:build tier1 && !windows

// gitwrap_unix_test.go holds the git-wrapper tests whose recording git is a shell script.

package websterengine

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
)

// TestCommitsNamedBy_OneBatchCheck pins that commitsNamedBy classifies every object a prefix names in a single `cat-file --batch-check` call, never one `cat-file -t` per object:
// a recording git first on PATH logs each invocation, over a prefix naming a commit, a blob and a tree.
// It sets PATH, process-global state, so it runs without t.Parallel.
func TestCommitsNamedBy_OneBatchCheck(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("git not on PATH: %v", err)
	}

	dir := gitwrapNewScratchRepo(t)
	commit := gitkit.CommitFile(t, dir, "a.txt", "one", "first")
	prefix := commit[:4]

	blobBody := gitwrapObjectNamedLike(t, "blob", prefix, func(attempt int) []byte { return []byte(fmt.Sprintf("blob %d\n", attempt)) })
	blob, err := gitexec.RunStdin([]string{"hash-object", "-w", "--stdin"}, dir, string(blobBody))
	if err != nil {
		t.Fatalf("write blob: %v", err)
	}
	blob = strings.TrimSpace(blob)
	rawBlob, err := hex.DecodeString(blob)
	if err != nil {
		t.Fatalf("decode blob id %q: %v", blob, err)
	}
	treeBody := gitwrapObjectNamedLike(t, "tree", prefix, func(attempt int) []byte {
		return append([]byte(fmt.Sprintf("100644 f%d\x00", attempt)), rawBlob...)
	})
	entryName := strings.TrimPrefix(strings.SplitN(string(treeBody), "\x00", 2)[0], "100644 ")
	tree, err := gitexec.RunStdin([]string{"mktree"}, dir, fmt.Sprintf("100644 blob %s\t%s\n", blob, entryName))
	if err != nil {
		t.Fatalf("write tree: %v", err)
	}
	if strings.TrimSpace(tree)[:4] != prefix || blob[:4] != prefix {
		t.Fatalf("planted blob %s and tree %s do not share prefix %s", blob, strings.TrimSpace(tree), prefix)
	}

	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "git-calls.log")
	script := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %q\nexec %q \"$@\"\n", logPath, realGit)
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write recording git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := commitsNamedBy(dir, prefix)
	if err != nil {
		t.Fatalf("commitsNamedBy(%q) error = %v; want nil", prefix, err)
	}
	if !slices.Equal(got, []string{commit}) {
		t.Errorf("commitsNamedBy(%q) = %v; want [%s]", prefix, got, commit)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	var batchChecks int
	for _, call := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		if strings.HasPrefix(call, "cat-file -t") {
			t.Errorf("per-object call %q; want one cat-file --batch-check", call)
		}
		if call == "cat-file --batch-check" {
			batchChecks++
		}
	}
	if batchChecks != 1 {
		t.Errorf("cat-file --batch-check calls = %d; want 1 in:\n%s", batchChecks, logged)
	}
}
