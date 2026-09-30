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
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lock"
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

// TestExcludeAnchoredDir_AppendsAnchoredLineOnce pins the exact bytes written and the idempotent second call.
func TestExcludeAnchoredDir_AppendsAnchoredLineOnce(t *testing.T) {
	repoDir := newGitRepoForExcludeTest(t)
	excludePath := seedExclude(t, repoDir, operatorExcludePattern+"\n")

	gotPath, changed, err := ExcludeAnchoredDir(repoDir, ".", ".vscode")
	if err != nil {
		t.Fatalf("ExcludeAnchoredDir = %v; want nil", err)
	}
	if !changed {
		t.Error("first call reported changed=false; want true")
	}
	if gotPath != excludePath {
		t.Errorf("returned exclude path = %q; want %q", gotPath, excludePath)
	}
	first := readFileT(t, excludePath)
	if want := operatorExcludePattern + "\n/.vscode/\n"; first != want {
		t.Errorf("exclude content = %q; want %q", first, want)
	}

	_, changed, err = ExcludeAnchoredDir(repoDir, ".", ".vscode")
	if err != nil {
		t.Fatalf("second ExcludeAnchoredDir = %v; want nil", err)
	}
	if changed {
		t.Error("second call reported changed=true; want false")
	}
	if second := readFileT(t, excludePath); second != first {
		t.Errorf("second call changed content to %q; want %q", second, first)
	}
}

// TestExcludeAnchoredDir_AnchorsToSubpath pins that the entry covers only the anchor's own directory.
func TestExcludeAnchoredDir_AnchorsToSubpath(t *testing.T) {
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

// TestExcludeAnchoredDir_HoldsExcludeLock pins that the write waits on the shared exclude lock.
func TestExcludeAnchoredDir_HoldsExcludeLock(t *testing.T) {
	repoDir := newGitRepoForExcludeTest(t)
	excludePath := seedExclude(t, repoDir, operatorExcludePattern+"\n")

	held, err := lock.AcquireWriteLock(filepath.Join(filepath.Dir(excludePath), gitExcludeLockFileName))
	if err != nil {
		t.Fatalf("acquire exclude lock: %v", err)
	}
	released := false
	defer func() {
		if !released {
			_ = held.Release()
		}
	}()

	type result struct {
		changed bool
		err     error
	}
	done := make(chan result, 1)
	go func() {
		_, changed, err := ExcludeAnchoredDir(repoDir, ".", ".vscode")
		done <- result{changed, err}
	}()

	select {
	case r := <-done:
		t.Fatalf("ExcludeAnchoredDir returned (%v, %v) while the lock was held; want blocked", r.changed, r.err)
	case <-time.After(300 * time.Millisecond):
	}
	if got, want := readFileT(t, excludePath), operatorExcludePattern+"\n"; got != want {
		t.Errorf("exclude content while locked = %q; want %q", got, want)
	}

	if err := held.Release(); err != nil {
		t.Fatalf("release exclude lock: %v", err)
	}
	released = true

	select {
	case r := <-done:
		if r.err != nil || !r.changed {
			t.Fatalf("ExcludeAnchoredDir after release = (%v, %v); want (true, nil)", r.changed, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ExcludeAnchoredDir did not return after the lock was released")
	}
	if got, want := readFileT(t, excludePath), operatorExcludePattern+"\n/.vscode/\n"; got != want {
		t.Errorf("exclude content after release = %q; want %q", got, want)
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
