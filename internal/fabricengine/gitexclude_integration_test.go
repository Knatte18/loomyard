// gitexclude_integration_test.go pins the serialisation and atomicity of `.git/info/exclude` mutation.
// Every worktree of a hub shares that file in the common gitdir, so concurrent `lym fabric` verbs mutate the same bytes.
// A truncating write lets a concurrent reader see an empty file and write that back, erasing the operator's patterns.
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
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// operatorExcludePattern stands in for an exclude entry that a repository carries before fabric touches the file.
// The entry may be one of git's own comment lines or a pattern the operator wrote by hand; fabric must never remove it.
const operatorExcludePattern = "/operator-build-output"

// TestMutateGitExclude_ConcurrentMutationsPreserveExistingContent runs many simultaneous mutations
// against one repository's exclude file and asserts that pre-existing content survives all of them.
//
//testtiming:keep the operator's pre-existing exclude pattern surviving concurrent mutations exactly once; coverage of its blocks by other tests does not show an assertion of this
func TestMutateGitExclude_ConcurrentMutationsPreserveExistingContent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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

// TestExcludeAnchoredDir_AnchorsToSubpath pins that the entry covers only the anchor's own directory.
func TestExcludeAnchoredDir_AnchorsToSubpath(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	repoDir := newGitRepoForExcludeTest(t)
	seedExclude(t, repoDir, "")

	excludePath, _, err := ExcludeAnchoredDir(repoDir, "wts/some-task", ".vscode")
	if err != nil {
		t.Fatalf("ExcludeAnchoredDir = %v; want nil", err)
	}
	if got, want := readFileT(t, excludePath), "/wts/some-task/.vscode/\n"; got != want {
		t.Errorf("exclude content = %q; want %q", got, want)
	}

	if !checkIgnored(t, repoDir, "wts/some-task/.vscode/tasks.json") {
		t.Error("wts/some-task/.vscode/tasks.json is not ignored; want ignored")
	}
	if checkIgnored(t, repoDir, ".vscode/tasks.json") {
		t.Error("root .vscode/tasks.json is ignored; want not ignored")
	}
}

// TestExcludeAnchoredDir_AlreadyIgnoredWritesNothing pins the check-ignore short-circuit.
func TestExcludeAnchoredDir_AlreadyIgnoredWritesNothing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	repoDir := newGitRepoForExcludeTest(t)
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".vscode/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	for _, args := range [][]string{
		{"add", "--", ".gitignore"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-m", "ignore .vscode"},
	} {
		if _, stderr, exitCode, err := gitexec.RunGit(args, repoDir); err != nil || exitCode != 0 {
			t.Fatalf("git %v: err=%v exit=%d stderr=%s", args, err, exitCode, stderr)
		}
	}
	excludePath, err := resolveGitExcludePath(repoDir)
	if err != nil {
		t.Fatalf("resolveGitExcludePath = %v", err)
	}
	before, _ := os.ReadFile(excludePath)

	_, changed, err := ExcludeAnchoredDir(repoDir, ".", ".vscode")
	if err != nil {
		t.Fatalf("ExcludeAnchoredDir = %v; want nil", err)
	}
	if changed {
		t.Error("reported changed=true for an already-ignored dir; want false")
	}
	after, _ := os.ReadFile(excludePath)
	if string(after) != string(before) {
		t.Errorf("exclude content changed from %q to %q; want untouched", before, after)
	}
}

// seedExclude writes content into repoDir's exclude file and returns that file's path.
func seedExclude(t *testing.T, repoDir, content string) string {
	t.Helper()
	excludePath, err := resolveGitExcludePath(repoDir)
	if err != nil {
		t.Fatalf("resolveGitExcludePath(%q) = %v", repoDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatalf("mkdir exclude dir: %v", err)
	}
	if err := os.WriteFile(excludePath, []byte(content), 0o644); err != nil {
		t.Fatalf("seed exclude file: %v", err)
	}
	return excludePath
}

// readFileT returns path's content as a string, failing the test on error.
func readFileT(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// checkIgnored reports whether git considers relPath ignored in repoDir.
func checkIgnored(t *testing.T, repoDir, relPath string) bool {
	t.Helper()
	_, _, exitCode, err := gitexec.RunGit([]string{"check-ignore", "-q", "--", relPath}, repoDir)
	if err != nil {
		t.Fatalf("git check-ignore %s: %v", relPath, err)
	}
	return exitCode == 0
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
