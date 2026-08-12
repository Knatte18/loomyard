//go:build integration

// gitexclude_integration_test.go pins the serialisation and atomicity of `.git/info/exclude`
// mutation.
// The file lives in the repo's COMMON gitdir, so every worktree of a hub shares it and two `lyx
// fabric` verbs running side by side mutate the same bytes.
// Before mutateGitExclude existed, each caller did its own read-modify-write with os.WriteFile,
// whose truncate-then-write let a concurrent reader observe an EMPTY file and write that emptiness
// back — destroying the operator's own exclude patterns as well as fabric's junction exclusions.
// It is integration-tagged because resolving the exclude path spawns real git.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// operatorExcludePattern stands in for the exclude entries a repository already carries before
// fabric touches the file — git's own comment block, or a pattern the operator wrote by hand.
// Nothing fabric does may ever remove it.
const operatorExcludePattern = "/operator-build-output"

// TestMutateGitExclude_ConcurrentMutationsPreserveExistingContent runs many simultaneous mutations
// against one repository's exclude file and asserts that pre-existing content survives all of them.
func TestMutateGitExclude_ConcurrentMutationsPreserveExistingContent(t *testing.T) {
	repoDir := newGitRepoForExcludeTest(t)

	excludePath, err := resolveGitExcludePath(repoDir)
	if err != nil {
		t.Fatalf("resolveGitExcludePath(%q) = %v; want nil", repoDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatalf("mkdir exclude dir: %v", err)
	}
	if err := os.WriteFile(excludePath, []byte(operatorExcludePattern+"\n"), 0o644); err != nil {
		t.Fatalf("seed exclude file: %v", err)
	}

	const writers = 8
	const iterations = 40

	var waitGroup sync.WaitGroup
	failures := make(chan error, writers*iterations)

	for writer := range writers {
		waitGroup.Add(1)
		go func(writer int) {
			defer waitGroup.Done()
			pattern := fmt.Sprintf("/junction-%d", writer)
			for range iterations {
				if _, _, err := mutateGitExclude(repoDir, appendPatternOnce(pattern)); err != nil {
					failures <- fmt.Errorf("seed %q: %w", pattern, err)
					return
				}
				if _, _, err := mutateGitExclude(repoDir, removePattern(pattern)); err != nil {
					failures <- fmt.Errorf("unseed %q: %w", pattern, err)
					return
				}
			}
		}(writer)
	}

	waitGroup.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("concurrent mutation returned an error: %v", err)
	}

	final, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read exclude file after the storm: %v", err)
	}
	got := countLine(string(final), operatorExcludePattern)
	if got != 1 {
		t.Errorf("operator pattern %q appears %d times after %d concurrent mutations; want exactly 1\nfile:\n%s",
			operatorExcludePattern, got, writers*iterations*2, final)
	}
}

// TestMutateGitExclude_ReportsWhetherContentChanged pins the no-op contract a caller relies on to
// decide whether it actually unwired anything.
func TestMutateGitExclude_ReportsWhetherContentChanged(t *testing.T) {
	repoDir := newGitRepoForExcludeTest(t)

	_, changed, err := mutateGitExclude(repoDir, appendPatternOnce("/_lyx"))
	if err != nil {
		t.Fatalf("first mutateGitExclude = %v; want nil", err)
	}
	if !changed {
		t.Error("first mutateGitExclude reported changed=false; want true")
	}

	_, changed, err = mutateGitExclude(repoDir, appendPatternOnce("/_lyx"))
	if err != nil {
		t.Fatalf("second mutateGitExclude = %v; want nil", err)
	}
	if changed {
		t.Error("re-applying the same pattern reported changed=true; want false")
	}
}

// appendPatternOnce builds a rewrite that adds pattern unless it is already present.
func appendPatternOnce(pattern string) func(string) (string, error) {
	return func(content string) (string, error) {
		if countLine(content, pattern) > 0 {
			return content, nil
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + pattern + "\n", nil
	}
}

// removePattern builds a rewrite that strips every line equal to pattern.
func removePattern(pattern string) func(string) (string, error) {
	return func(content string) (string, error) {
		kept := make([]string, 0)
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) == pattern {
				continue
			}
			kept = append(kept, line)
		}
		return strings.Join(kept, "\n"), nil
	}
}

// countLine reports how many lines of content trim to exactly want.
func countLine(content, want string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

// newGitRepoForExcludeTest initialises a real git repository and returns its worktree root.
func newGitRepoForExcludeTest(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	if _, stderr, exitCode, err := gitexec.RunGit([]string{"init", "-b", "main", "."}, repoDir); err != nil || exitCode != 0 {
		t.Fatalf("git init in %q: err=%v exit=%d stderr=%s", repoDir, err, exitCode, stderr)
	}
	return repoDir
}
