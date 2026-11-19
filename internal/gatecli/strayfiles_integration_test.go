//go:build !windows

// strayfiles_integration_test.go covers the failure of a tier run that leaves new untracked files in its git worktree.
// The tier command is a POSIX shell, so the file is left out of the Windows build.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newStrayRepo returns a git repository root that ignores `*.ignored` and holds no other file.
func newStrayRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitkit.Git(t, root, "init", "-q")
	gitkit.CommitFile(t, root, ".gitignore", "*.ignored\n", "ignore")
	return root
}

// strayRequest returns a request rooted at root that writes its stderr lines to stderr.
func strayRequest(root string, stderr io.Writer) testRequest {
	return testRequest{root: root, stderr: stderr, timeoutUnit: 30 * time.Second, pipeWaitDelay: defaultPipeWaitDelay}
}

// strayRun returns a tier1 run of command with one invocation in each of dirs.
func strayRun(command string, dirs ...string) tierRun {
	var invocations []invocation
	for _, dir := range dirs {
		invocations = append(invocations, invocation{dir: dir})
	}
	return tierRun{number: 1, tier: gateslot.Tier{Command: command, Counter: counterGoJSON, TimeoutMin: 1}, invocations: invocations}
}

func TestRunTierStrayFiles(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// noRepo runs the tier in a directory outside every git repository.
		noRepo  bool
		command string
		// wantExit is the tier's exit code.
		wantExit int
		// wantNamed are the files the stray-file line names, in order; nil means no line.
		wantNamed []string
		// wantRest is the count of new files the line leaves unnamed.
		wantRest int
	}{
		{name: "ANewFileFailsAPassingTier", command: "mkdir -p internal/p && touch internal/p/.lock", wantExit: 1, wantNamed: []string{"internal/p/.lock"}},
		{name: "AFailingTierKeepsItsExitCode", command: "touch z.lock a.lock; exit 3", wantExit: 3, wantNamed: []string{"a.lock", "z.lock"}},
		{name: "AFileUntrackedBeforeTheRunIsNotNamed", command: "touch before.txt"},
		{name: "AnIgnoredFileIsNotNamed", command: "touch out.ignored"},
		{name: "AFileTheRunRemovesIsNotNamed", command: "touch gone.lock && rm gone.lock"},
		{
			name:      "MoreFilesThanTheLineNamesAreCounted",
			command:   "touch f00 f01 f02 f03 f04 f05 f06 f07 f08 f09 f10 f11",
			wantExit:  1,
			wantNamed: []string{"f00", "f01", "f02", "f03", "f04", "f05", "f06", "f07", "f08", "f09"},
			wantRest:  2,
		},
		{name: "ANameWithANewlineStaysOneQuotedItem", command: "touch 'a\nb'", wantExit: 1, wantNamed: []string{"a\nb"}},
		{name: "ADirectoryOutsideEveryRepositoryIsNotChecked", noRepo: true, command: "touch new.lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if !tt.noRepo {
				root = newStrayRepo(t)
				if err := os.WriteFile(filepath.Join(root, "before.txt"), []byte("untracked\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stderr bytes.Buffer
			request := strayRequest(root, &stderr)
			run := strayRun(tt.command, root)

			result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

			if result.refusal != "" || result.record.ExitCode != tt.wantExit {
				t.Fatalf("runTier() = refusal %q, exit %d; want exit %d and no refusal", result.refusal, result.record.ExitCode, tt.wantExit)
			}
			wantOutcome := "failed"
			if tt.wantExit == 0 {
				wantOutcome = "passed"
			}
			if result.record.Outcome != wantOutcome {
				t.Errorf("runTier() outcome = %q; want %q", result.record.Outcome, wantOutcome)
			}
			if tt.wantNamed == nil {
				if stderr.Len() != 0 || len(result.causes) != 0 {
					t.Errorf("runTier() stderr = %q, causes = %q; want neither", stderr.String(), result.causes)
				}
				return
			}

			quoted := make([]string, len(tt.wantNamed))
			for i, name := range tt.wantNamed {
				quoted[i] = strconv.Quote(name)
			}
			rest := ""
			if tt.wantRest > 0 {
				rest = fmt.Sprintf(" and %d more", tt.wantRest)
			}
			wantPrefix := "gate test: tier1 left new untracked files in the worktree: " + strings.Join(quoted, ", ") + rest + "; "
			line, oneLine := strings.CutSuffix(stderr.String(), "\n")
			if !oneLine || strings.Contains(line, "\n") || !strings.HasPrefix(line, wantPrefix) || !strings.Contains(line, "; way forward: ") {
				t.Fatalf("runTier() stderr = %q; want one line opening %q with a way forward", stderr.String(), wantPrefix)
			}
			if !slices.Contains(result.causes, line) || !strings.Contains(string(result.log), line+"\n") {
				t.Errorf("runTier() causes = %q, log = %q; want the line in both", result.causes, result.log)
			}
		})
	}
}

func TestRunTierStrayFilesSkipsARefusedRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	root := newStrayRepo(t)
	var stderr bytes.Buffer
	// The first invocation leaves a file; the second starts in a directory that does not exist, which refuses the run.
	run := strayRun("touch r.lock", root, filepath.Join(root, "missing"))

	result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, strayRequest(root, &stderr))

	if result.refusal == "" {
		t.Fatalf("runTier() refusal = %q; want one for a shell that cannot start", result.refusal)
	}
	if strings.Contains(stderr.String(), "untracked files") || len(result.causes) != 0 {
		t.Errorf("runTier() stderr = %q, causes = %q; want no stray-file line for a refused run", stderr.String(), result.causes)
	}
}

func TestRunTierCheckedHoldsEveryAttemptToTheFirstListing(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	root := newStrayRepo(t)
	env := []string{"PATH=" + os.Getenv("PATH")}
	before, listed := untrackedWorktreeFiles(root)
	if !listed {
		t.Fatal("untrackedWorktreeFiles() = not listed; want the repository listed")
	}

	first := runTierChecked(context.Background(), io.Discard, strayRun("touch r.lock; exit 1", root), env, strayRequest(root, io.Discard), before, listed)
	var stderr bytes.Buffer
	second := runTierChecked(context.Background(), io.Discard, strayRun("true", root), env, strayRequest(root, &stderr), before, listed)

	if first.record.ExitCode != 1 {
		t.Fatalf("first attempt exit = %d; want 1", first.record.ExitCode)
	}
	if second.record.ExitCode != 1 || !strings.Contains(stderr.String(), `"r.lock"`) {
		t.Errorf("second attempt = exit %d, stderr %q; want exit 1 naming the file the first attempt left", second.record.ExitCode, stderr.String())
	}
}

func TestRunTierCopiesStrayFile(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	root := newStrayRepo(t)
	var stderr bytes.Buffer
	request := strayRequest(root, &stderr)
	request.copiesRun = 2
	run := strayRun("touch c.lock", root)

	result := runTier(context.Background(), io.Discard, run, []string{"PATH=" + os.Getenv("PATH")}, request)

	line, _ := strings.CutSuffix(stderr.String(), "\n")
	if result.record.ExitCode != 1 || len(result.causes) != 1 || result.causes[0] != line || !strings.Contains(line, `"c.lock"`) {
		t.Fatalf("runTier() = exit %d, causes %q, stderr %q; want exit 1 and the stray-file line once", result.record.ExitCode, result.causes, stderr.String())
	}
	exe := &execution{request: request}

	exe.addFailedRuns(run, result, "run.log")

	if len(exe.failed) != 1 || exe.failed[0].label != runLabel(run) || !slices.Equal(exe.failed[0].causes, []string{line}) {
		t.Errorf("addFailedRuns() failed = %+v; want one run labelled %q holding the stray-file line", exe.failed, runLabel(run))
	}
}
