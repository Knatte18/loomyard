// clone_singlerepo_integration_test.go proves the one-repository clone over hubforge's builder:
// the seed of origin's records namespace, the one-URL join of a seeded origin and the seed push's failure paths.
// Each test reads the origin's refs with git for-each-ref, since the layout is what lands there.
// It shares the fixture helpers of clone_adopt_test.go and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The origin refs of a one-repository hub's records namespace.
const (
	namespaceHeadsPrefix = fabricengine.RecordsNamespaceRoot + fabricengine.RecordsNamespaceHeads
	namespaceBoardRef    = namespaceHeadsPrefix + fabricengine.OneRepositoryBoardBranch
)

// refLines returns the lines of `git for-each-ref --format=<format> <patterns>` in the repository at dir.
func refLines(t *testing.T, dir, format string, patterns ...string) []string {
	t.Helper()

	out := gitkit.Git(t, dir, append([]string{"for-each-ref", "--format=" + format}, patterns...)...)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// requireEmptyDirectory fails t when dir holds an entry.
// For a clone container that means a hub or probe was left behind.
func requireEmptyDirectory(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Errorf("%s holds %d entries; want no hub directory and no probe directory", dir, len(entries))
	}
}

// cloneSingleRepo drives `lym fabric clone --single-repo` over bare into container, as the CLI does.
// A clone that succeeds registers its junction teardown on t.
func cloneSingleRepo(t *testing.T, container, bare, subpath string) (fabricengine.CloneResult, error) {
	t.Helper()

	res, err := fabriccli.CloneAndWire(container, fabricengine.CloneOptions{
		WeftURL:    filepath.ToSlash(bare),
		Subpath:    subpath,
		SingleRepo: true,
		Shortname:  hubforge.TestShortname,
	})
	if err == nil {
		hubforge.RegisterTeardown(t, res.HubPath)
	}
	return res, err
}

// requireOneRepositoryLayout fails t unless the hub at the given paths has the one-repository layout.
// It checks the records sibling and its anchor marker.
// It checks the wired junction and the board on its fixed branch.
// It checks the board's binding to origin and the prime's records branch key.
// It checks the records store's remote-tracking refs and the board's upstream.
func requireOneRepositoryLayout(t *testing.T, boardDir, recordsDir, primeCwd, origin, recordsBranch string) {
	t.Helper()

	if _, err := os.Stat(filepath.Join(recordsDir, lymcwd.AnchorFileName)); err != nil {
		t.Errorf("the records root %s holds no %s: %v", recordsDir, lymcwd.AnchorFileName, err)
	}
	if linked, err := fslink.IsLink(filepath.Join(primeCwd, lymdirs.LymDirName)); err != nil || !linked {
		t.Errorf("%s under %s is not a wired junction (link = %v, err = %v)", lymdirs.LymDirName, primeCwd, linked, err)
	}
	if got := gitkit.CurrentBranch(t, boardDir); got != fabricengine.OneRepositoryBoardBranch {
		t.Errorf("the board is on %q; want %q", got, fabricengine.OneRepositoryBoardBranch)
	}
	binding, err := os.ReadFile(filepath.Join(boardDir, fabricengine.WarpBindingFileName))
	if err != nil {
		t.Fatalf("read the board's binding: %v", err)
	}
	if got := strings.TrimSpace(string(binding)); got != filepath.ToSlash(origin) {
		t.Errorf("the board's %s = %q; want the origin %q", fabricengine.WarpBindingFileName, got, filepath.ToSlash(origin))
	}
	if got := gitkit.Git(t, recordsDir, "config", "--get", fabricengine.PrimeRecordsBranchKey); got != recordsBranch {
		t.Errorf("%s = %q; want %q", fabricengine.PrimeRecordsBranchKey, got, recordsBranch)
	}
	wantTracking := []string{"refs/remotes/origin/" + fabricengine.OneRepositoryBoardBranch, "refs/remotes/origin/" + recordsBranch}
	if got := refLines(t, recordsDir, "%(refname)", "refs/remotes/origin/"); !slices.Equal(got, wantTracking) {
		t.Errorf("the records store's remote-tracking refs = %v; want %v, with no code branch among them", got, wantTracking)
	}
	if got := gitkit.Git(t, boardDir, "config", "--get", "branch."+fabricengine.OneRepositoryBoardBranch+".merge"); got != namespaceBoardRef {
		t.Errorf("the board's upstream merge = %q; want %q", got, namespaceBoardRef)
	}
}

// TestCloneSingleRepo_SeedsTheNamespace asserts that a --single-repo clone leaves the code branches untouched.
// It also asserts that the board and the prime's records branch land under origin's records namespace.
func TestCloneSingleRepo_SeedsTheNamespace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name          string
		defaultBranch string
		recordsBranch string
	}{
		{"MainDefault", "", "main-weft"},
		{"MasterDefault", "master", "master-weft"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bare := hubforge.CopyCodeBare(t, tt.defaultBranch)
			codeRefsBefore := refLines(t, bare, "%(refname) %(objectname)", "refs/heads/")
			container := t.TempDir()

			res, err := cloneSingleRepo(t, container, bare, "")
			if err != nil {
				t.Fatalf("clone --single-repo: %v", err)
			}
			board := fabricengine.NewBolt(res.BoardDir)
			// The clone's walk hands the board's last push to a detached sync, which a test binary never spawns.
			if err := board.Push(fabricengine.SyncOptions{}); err != nil {
				t.Fatalf("push the clone's board commit: %v", err)
			}

			if got := refLines(t, bare, "%(refname) %(objectname)", "refs/heads/"); !slices.Equal(got, codeRefsBefore) {
				t.Errorf("origin's code branches = %v; want %v, as before the clone", got, codeRefsBefore)
			}
			wantNamespace := []string{namespaceBoardRef, namespaceHeadsPrefix + tt.recordsBranch}
			if got := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); !slices.Equal(got, wantNamespace) {
				t.Errorf("origin's records namespace = %v; want %v", got, wantNamespace)
			}

			l, err := lymcwd.Resolve(res.PrimeCwd)
			if err != nil {
				t.Fatalf("resolve the prime: %v", err)
			}
			recordsDir := fabricengine.RecordsWorktree(l)
			requireOneRepositoryLayout(t, res.BoardDir, recordsDir, res.PrimeCwd, bare, tt.recordsBranch)
			wantUpstream := "refs/remotes/origin/" + tt.recordsBranch
			if got := gitkit.Git(t, recordsDir, "rev-parse", "--symbolic-full-name", tt.recordsBranch+"@{u}"); got != wantUpstream {
				t.Errorf("the seeded records branch's upstream = %q; want %q", got, wantUpstream)
			}
			if unpushed, err := board.HasUnpushed(); err != nil || unpushed {
				t.Errorf("HasUnpushed() = %v, %v; want false after the board's push", unpushed, err)
			}

			plain := filepath.Join(t.TempDir(), "plain")
			gitkit.Git(t, filepath.Dir(plain), "clone", filepath.ToSlash(bare), filepath.Base(plain))
			for _, ref := range refLines(t, plain, "%(refname)") {
				if strings.Contains(ref, "lym") {
					t.Errorf("a default clone of the origin fetched %s; want nothing under %s", ref, fabricengine.RecordsNamespaceRoot)
				}
			}
		})
	}
}

// TestCloneOneURL_JoinsTheNamespacedHub asserts that a one-URL clone of a seeded origin joins its board.
// It also asserts that the clone joins the origin's records branch.
// It also asserts that its refusals name the recorded fact they contradict and leave no hub behind.
func TestCloneOneURL_JoinsTheNamespacedHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("SecondMachineBuildsTheSameLayout", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		second := hubforge.CloneSecond(t, first)

		requireOneRepositoryLayout(t, second.BoardDir(), second.PrimeRecords(), second.PrimeWorktree(), second.CodeBare, "main-weft")
		firstBoardTip := gitkit.RevParse(t, first.BoardDir(), "HEAD")
		if !gitkit.IsAncestor(t, second.BoardDir(), firstBoardTip, "HEAD") {
			t.Errorf("the second board does not hold the first hub's board commit %s", firstBoardTip)
		}
		for _, file := range []string{fabricengine.WarpBindingFileName, fabricengine.ShortnameFileName} {
			want, err := os.ReadFile(filepath.Join(first.BoardDir(), file))
			if err != nil {
				t.Fatalf("read the first board's %s: %v", file, err)
			}
			got, err := os.ReadFile(filepath.Join(second.BoardDir(), file))
			if err != nil {
				t.Fatalf("read the second board's %s: %v", file, err)
			}
			if string(got) != string(want) {
				t.Errorf("the second board's %s = %q; want the first board's %q", file, got, want)
			}
		}
	})

	t.Run("CodeDefaultBranchOtherThanMain", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{DefaultBranch: "master"})
		second := hubforge.CloneSecond(t, first)

		requireOneRepositoryLayout(t, second.BoardDir(), second.PrimeRecords(), second.PrimeWorktree(), second.CodeBare, "master-weft")
		firstBinding, err := os.ReadFile(filepath.Join(first.BoardDir(), fabricengine.WarpBindingFileName))
		if err != nil {
			t.Fatalf("read the first board's binding: %v", err)
		}
		secondBinding, err := os.ReadFile(filepath.Join(second.BoardDir(), fabricengine.WarpBindingFileName))
		if err != nil {
			t.Fatalf("read the second board's binding: %v", err)
		}
		if string(secondBinding) != string(firstBinding) {
			t.Errorf("the second board's binding = %q; want the first board's %q", secondBinding, firstBinding)
		}
	})

	t.Run("ShortnameOtherThanTheRecordedOneIsRefused", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		container := t.TempDir()

		_, err := fabricengine.CloneHub(container, fabricengine.CloneOptions{WeftURL: filepath.ToSlash(first.CodeBare), Shortname: "other"})
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("recorded shortname is %q", hubforge.TestShortname)) {
			t.Errorf("CloneHub with another shortname error = %v; want a refusal naming the recorded shortname %q", err, hubforge.TestShortname)
		}
		requireEmptyDirectory(t, container)
	})

	t.Run("BindingNamingAnotherURLIsRefused", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		binding := filepath.Join(first.BoardDir(), fabricengine.WarpBindingFileName)
		if err := os.WriteFile(binding, []byte("https://example.com/another-code\n"), 0o644); err != nil {
			t.Fatalf("rewrite the board's binding: %v", err)
		}
		board := fabricengine.NewBolt(first.BoardDir())
		if _, _, err := board.Commit("point the binding at another URL", fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("commit the rewritten binding: %v", err)
		}
		if err := board.Push(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("push the rewritten binding: %v", err)
		}
		container := t.TempDir()

		_, err := fabricengine.CloneHub(container, fabricengine.CloneOptions{WeftURL: filepath.ToSlash(first.CodeBare), Shortname: hubforge.TestShortname})
		if !errors.Is(err, fabricengine.ErrWarpBindingMismatch) {
			t.Errorf("CloneHub over a rewritten binding error = %v; want %v", err, fabricengine.ErrWarpBindingMismatch)
		}
		requireEmptyDirectory(t, container)
	})

	t.Run("ResetWithAnotherSubpathIsRefusedAndKeepsTheHub", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		second := hubforge.CloneSecond(t, first)

		_, err := fabricengine.CloneHub(second.Container, fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(first.CodeBare),
			Subpath:   "backend",
			Reset:     true,
			Shortname: hubforge.TestShortname,
		})
		if err == nil || !strings.Contains(err.Error(), `recorded anchor "."`) {
			t.Errorf("CloneHub --reset with --subpath backend error = %v; want a refusal naming the recorded anchor \".\"", err)
		}
		if _, statErr := os.Stat(filepath.Join(second.PrimeRecords(), lymcwd.AnchorFileName)); statErr != nil {
			t.Errorf("the existing hub lost its anchor marker: %v", statErr)
		}
	})

	t.Run("ResetWithAMissingSubpathIsRefusedBeforeTheHubIsTouched", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		gitkit.Git(t, first.CodeBare, "update-ref", "-d", namespaceHeadsPrefix+"main-weft")

		_, err := fabricengine.CloneHub(first.Container, fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(first.CodeBare),
			Subpath:   "no-such-directory",
			Reset:     true,
			Shortname: hubforge.TestShortname,
		})
		if err == nil || !strings.Contains(err.Error(), "does not exist as a directory") {
			t.Errorf("CloneHub --reset with a missing --subpath error = %v; want a refusal naming the missing directory", err)
		}
		if _, statErr := os.Stat(first.BoardDir()); statErr != nil {
			t.Errorf("the existing hub's board was touched: %v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(first.PrimeRecords(), lymcwd.AnchorFileName)); statErr != nil {
			t.Errorf("the existing hub lost its anchor marker: %v", statErr)
		}
	})
}

// refuseBoardHook is a pre-receive hook that refuses the board's origin ref and accepts every other ref.
var refuseBoardHook = fmt.Sprintf(`#!/bin/sh
while read old new ref; do
	if [ "$ref" = "%s" ]; then
		echo "the board ref is refused by this host" >&2
		exit 1
	fi
done
exit 0
`, namespaceBoardRef)

// leftoverCommit creates a commit in the bare at bare that no branch holds and returns its SHA.
// It stands for the records tip a failed seed attempt leaves on origin.
func leftoverCommit(t *testing.T, bare string) string {
	t.Helper()

	return gitkit.Git(t, bare, "commit-tree", "-m", "leftover records", gitkit.Git(t, bare, "rev-parse", "refs/heads/main^{tree}"))
}

// TestCloneSingleRepo_FailedSeedPushLeavesNothing asserts that a --single-repo clone whose seed push fails leaves no hub.
// It also asserts that origin's records branch returns to what the clone observed,
// and that the error names the way forward its cause calls for.
// It is not parallel: its lease rows move origin through the seed push's process-global test hook.
func TestCloneSingleRepo_FailedSeedPushLeavesNothing(t *testing.T) {
	tiergate.Require(t, 1)

	recordsRef := namespaceHeadsPrefix + "main-weft"

	t.Run("RefusedBoard", func(t *testing.T) {
		bare := hubforge.CopyCodeBare(t, "")
		hook := filepath.Join(bare, "hooks", "pre-receive")
		if err := os.WriteFile(hook, []byte(refuseBoardHook), 0o755); err != nil {
			t.Fatalf("write the pre-receive hook: %v", err)
		}
		container := t.TempDir()

		_, err := cloneSingleRepo(t, container, bare, "")
		if err == nil || !strings.Contains(err.Error(), "its hooks or rules declined the board ref "+namespaceBoardRef) {
			t.Fatalf("clone over a hook declining the board error = %v; want the declined-board way forward", err)
		}
		if strings.Contains(err.Error(), "use a host that accepts refs") {
			t.Errorf("clone error = %q; a host that took the records branch must not be named as refusing refs outside refs/heads/", err.Error())
		}
		requireEmptyDirectory(t, container)
		if got := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); len(got) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, the records push restored", got)
		}

		if err := os.Remove(hook); err != nil {
			t.Fatalf("remove the hook: %v", err)
		}
		if _, err := cloneSingleRepo(t, container, bare, ""); err != nil {
			t.Fatalf("rerun of the clone with the hook removed: %v", err)
		}
		wantNamespace := []string{namespaceBoardRef, recordsRef}
		if got := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); !slices.Equal(got, wantNamespace) {
			t.Errorf("origin's records namespace after the rerun = %v; want %v", got, wantNamespace)
		}
	})

	leftoverTests := []struct {
		name       string
		subpath    string
		wantAnchor string
	}{
		{"LeftoverReplacedUnderTheSameSubpath", "", "."},
		{"LeftoverReplacedUnderAnotherSubpath", "backend", "backend"},
	}
	for _, tt := range leftoverTests {
		t.Run(tt.name, func(t *testing.T) {
			bare := hubforge.CopyCodeBare(t, "")
			leftover := leftoverCommit(t, bare)
			gitkit.Git(t, bare, "update-ref", recordsRef, leftover)

			if _, err := cloneSingleRepo(t, t.TempDir(), bare, tt.subpath); err != nil {
				t.Fatalf("clone over a leftover records branch: %v", err)
			}

			tip := gitkit.RevParse(t, bare, recordsRef)
			if tip == leftover {
				t.Fatalf("origin's %s is still the leftover %s; want it replaced", recordsRef, leftover)
			}
			if got := gitkit.Git(t, bare, "show", tip+":"+lymcwd.AnchorFileName); got != tt.wantAnchor {
				t.Errorf("the new tip's %s = %q; want %q", lymcwd.AnchorFileName, got, tt.wantAnchor)
			}
			if !slices.Contains(strings.Split(gitkit.Git(t, bare, "log", "--format=%s", tip), "\n"), "fabric clone: record anchor") {
				t.Errorf("the new tip's history holds no fresh anchor-marker commit")
			}
		})
	}

	leaseTests := []struct {
		name string
		// hookRef is the origin ref whose push the hook moves origin before.
		hookRef string
		// preseed creates a leftover records branch before the clone.
		preseed bool
		// move changes origin when the hook fires, given the code branch's tip.
		move func(t *testing.T, bare, codeTip string)
		// wantErrorHas are substrings of the clone's error.
		wantErrorHas []string
		// wantRecordsAfter names where the records branch must end:
		// "" for absent, "code" for the code tip, "leftover" for the pre-seeded tip.
		wantRecordsAfter string
		// wantBoardAfter reports whether the board ref must exist at the end, the hook having created it.
		wantBoardAfter bool
	}{
		{
			name:    "RecordsLeaseLost",
			hookRef: recordsRef,
			move: func(t *testing.T, bare, codeTip string) {
				gitkit.Git(t, bare, "update-ref", recordsRef, codeTip)
			},
			wantErrorHas:     []string{"run the same clone again"},
			wantRecordsAfter: "code",
		},
		{
			name:    "BoardLeaseLostWithNoRecordsBranchObserved",
			hookRef: namespaceBoardRef,
			move: func(t *testing.T, bare, codeTip string) {
				gitkit.Git(t, bare, "update-ref", namespaceBoardRef, codeTip)
			},
			wantErrorHas:   []string{"to join that hub"},
			wantBoardAfter: true,
		},
		{
			name:    "BoardLeaseLostWithALeftoverObserved",
			hookRef: namespaceBoardRef,
			preseed: true,
			move: func(t *testing.T, bare, codeTip string) {
				gitkit.Git(t, bare, "update-ref", namespaceBoardRef, codeTip)
			},
			wantErrorHas:     []string{"to join that hub"},
			wantRecordsAfter: "leftover",
			wantBoardAfter:   true,
		},
		{
			name:    "RestoreFailsAfterTheBoardLeaseIsLost",
			hookRef: namespaceBoardRef,
			move: func(t *testing.T, bare, codeTip string) {
				gitkit.Git(t, bare, "update-ref", namespaceBoardRef, codeTip)
				gitkit.Git(t, bare, "update-ref", recordsRef, codeTip)
			},
			wantErrorHas:     []string{"lost its lease", "restoring the records branch main-weft on origin also failed"},
			wantRecordsAfter: "code",
			wantBoardAfter:   true,
		},
	}
	for _, tt := range leaseTests {
		t.Run(tt.name, func(t *testing.T) {
			bare := hubforge.CopyCodeBare(t, "")
			codeTip := gitkit.RevParse(t, bare, "refs/heads/main")
			leftover := ""
			if tt.preseed {
				leftover = leftoverCommit(t, bare)
				gitkit.Git(t, bare, "update-ref", recordsRef, leftover)
			}
			fabricengine.SetSeedPushHookForTest(t, func(ref string) {
				if ref == tt.hookRef {
					tt.move(t, bare, codeTip)
				}
			})
			container := t.TempDir()

			_, err := cloneSingleRepo(t, container, bare, "")
			if err == nil {
				t.Fatalf("clone with a lost lease = nil error; want a refusal")
			}
			for _, want := range tt.wantErrorHas {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("clone error = %q; want it to contain %q", err.Error(), want)
				}
			}
			requireEmptyDirectory(t, container)

			wantRefs := []string{}
			if tt.wantBoardAfter {
				wantRefs = append(wantRefs, namespaceBoardRef)
			}
			switch tt.wantRecordsAfter {
			case "":
			case "code":
				wantRefs = append(wantRefs, recordsRef)
				if got := gitkit.RevParse(t, bare, recordsRef); got != codeTip {
					t.Errorf("%s = %s; want %s, where the hook put it", recordsRef, got, codeTip)
				}
			case "leftover":
				wantRefs = append(wantRefs, recordsRef)
				if got := gitkit.RevParse(t, bare, recordsRef); got != leftover {
					t.Errorf("%s = %s; want the observed leftover %s", recordsRef, got, leftover)
				}
			}
			if got := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); !slices.Equal(got, wantRefs) {
				t.Errorf("origin's records namespace = %v; want %v", got, wantRefs)
			}
		})
	}

	t.Run("TransientRefusalIsRetriedByTheSameClone", func(t *testing.T) {
		bare := hubforge.CopyCodeBare(t, "")
		lockDir := filepath.Join(bare, filepath.FromSlash(namespaceHeadsPrefix))
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatalf("create the namespace directory: %v", err)
		}
		lock := filepath.Join(lockDir, fabricengine.OneRepositoryBoardBranch+".lock")
		if err := os.WriteFile(lock, nil, 0o644); err != nil {
			t.Fatalf("create the board ref's lock file: %v", err)
		}
		container := t.TempDir()

		_, err := cloneSingleRepo(t, container, bare, "")
		if err == nil || !strings.Contains(err.Error(), "run the same clone again") {
			t.Fatalf("clone over a locked board ref error = %v; want the rerun way forward", err)
		}
		if strings.Contains(err.Error(), "use a host that accepts refs") {
			t.Errorf("clone error = %q; a transient refusal must not name the refused-host way forward", err.Error())
		}
		requireEmptyDirectory(t, container)
		if got := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); len(got) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, the records push restored", got)
		}
	})

	t.Run("AbandonedSeedRemovesTheHubAndNamesTheRerun", func(t *testing.T) {
		bare := hubforge.CopyCodeBare(t, "")
		container := t.TempDir()
		res, err := fabricengine.CloneHub(container, fabricengine.CloneOptions{
			WeftURL:    filepath.ToSlash(bare),
			SingleRepo: true,
			Shortname:  hubforge.TestShortname,
		})
		if err != nil {
			t.Fatalf("CloneHub --single-repo: %v", err)
		}

		cause := errors.New("the wiring failed")
		got := fabricengine.AbandonSeededClone(res, fabricengine.NewMutations(res.HubPath), cause)
		if !errors.Is(got, cause) || !strings.Contains(got.Error(), "run the same `lym fabric clone --single-repo` again") {
			t.Errorf("AbandonSeededClone error = %v; want the cause and the rerun way forward", got)
		}
		requireEmptyDirectory(t, container)
		if refs := refLines(t, bare, "%(refname)", fabricengine.RecordsNamespaceRoot); len(refs) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, since nothing was pushed", refs)
		}
	})

	t.Run("AbandonedTwoURLCloneKeepsTheHub", func(t *testing.T) {
		fixtures := t.TempDir()
		recordsRemote := makeEmptyBareRemote(t, fixtures, "records")
		codeRemote := makeBareRemote(t, fixtures, "code")
		container := t.TempDir()
		res, err := fabricengine.CloneHub(container, fabricengine.CloneOptions{
			WeftURL:   filepath.ToSlash(recordsRemote),
			WarpURL:   filepath.ToSlash(codeRemote),
			Shortname: hubforge.TestShortname,
		})
		if err != nil {
			t.Fatalf("two-URL CloneHub: %v", err)
		}

		cause := errors.New("the wiring failed")
		if got := fabricengine.AbandonSeededClone(res, fabricengine.NewMutations(res.HubPath), cause); got != cause {
			t.Errorf("AbandonSeededClone of a two-URL clone = %v; want the cause unchanged", got)
		}
		if _, err := os.Stat(res.HubPath); err != nil {
			t.Errorf("the two-URL clone's hub was removed: %v", err)
		}
	})

	t.Run("BoardWithNoUpstreamIsNeverPushed", func(t *testing.T) {
		const resetWayForward = "run the same `lym fabric clone --single-repo` again with `--reset`"
		tests := []struct {
			name string
			// originHoldsSeed seeds origin through a whole clone.
			// The row then drops the upstreams, as a clone killed after its seed push leaves them.
			originHoldsSeed bool
			// commitOnBoard commits on the board after the clone, so the board holds a commit origin lacks.
			commitOnBoard bool
			// originAdvances puts a commit made in another clone on top of origin's board, which this store does not hold.
			originAdvances bool
			// anotherBoard puts a board this hub never held onto origin.
			anotherBoard bool
			// originUnreadable points the records store's origin at a path that holds no repository.
			originUnreadable bool
			// wantErrorHas are substrings of the push's error.
			// {board}, {hub} and {records} stand for the board directory, the hub and the prime's records branch.
			wantErrorHas []string
			// wantReset reports whether the error names the --reset clone.
			wantReset bool
			// wantPushLands reports whether a push after the named upstream settings must succeed.
			wantPushLands bool
		}{
			{
				name:         "OriginHoldsNoBoard",
				wantErrorHas: []string{"origin holds no board under " + fabricengine.RecordsNamespaceRoot + ", so it was never seeded onto origin"},
				wantReset:    true,
			},
			{
				name:            "OriginHoldsTheSeed",
				originHoldsSeed: true,
				wantErrorHas: []string{
					"so the seed landed and its clone stopped before it set the upstreams",
					"`git -C {board} config branch." + fabricengine.OneRepositoryBoardBranch + ".remote origin`",
					"`git -C {board} config branch." + fabricengine.OneRepositoryBoardBranch + ".merge " + namespaceBoardRef + "`",
					"`git -C {board} config branch.{records}.remote origin`",
					"`git -C {board} config branch.{records}.merge " + namespaceHeadsPrefix + "{records}`",
				},
				wantPushLands: true,
			},
			{
				name:            "OriginHoldsAnAncestorOfTheCommittedBoard",
				originHoldsSeed: true,
				commitOnBoard:   true,
				wantErrorHas:    []string{"so the seed landed and its clone stopped before it set the upstreams"},
				wantPushLands:   true,
			},
			{
				name:            "OriginHoldsADescendantOfTheSeed",
				originHoldsSeed: true,
				commitOnBoard:   true,
				originAdvances:  true,
				wantErrorHas:    []string{"so the seed landed and its clone stopped before it set the upstreams"},
			},
			{
				name:         "OriginHoldsAnotherBoard",
				anotherBoard: true,
				wantErrorHas: []string{
					"while this board holds no commits, so another clone seeded origin",
					"remove the hub at {hub} and run `lym fabric clone <url>` without `--single-repo` to join that hub",
				},
			},
			{
				name:          "OriginHoldsAnUnrelatedBoardToACommittedBoard",
				commitOnBoard: true,
				anotherBoard:  true,
				wantErrorHas: []string{
					"which shares no history with this board",
					"remove the hub at {hub} and run `lym fabric clone <url>` without `--single-repo` to join that hub",
				},
			},
			{
				name:             "OriginCannotBeRead",
				originUnreadable: true,
				wantErrorHas: []string{
					"origin's board could not be read to tell whether its seed landed",
					"run the same command again once origin answers",
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bare := hubforge.CopyCodeBare(t, "")
				var res fabricengine.CloneResult
				var err error
				if tt.originHoldsSeed {
					res, err = cloneSingleRepo(t, t.TempDir(), bare, "")
				} else {
					res, err = fabricengine.CloneHub(t.TempDir(), fabricengine.CloneOptions{
						WeftURL:    filepath.ToSlash(bare),
						SingleRepo: true,
						Shortname:  hubforge.TestShortname,
					})
				}
				if err != nil {
					t.Fatalf("clone --single-repo: %v", err)
				}
				recordsBranch := gitkit.Git(t, res.BoardDir, "config", "--get", fabricengine.PrimeRecordsBranchKey)
				if tt.originHoldsSeed {
					for _, branch := range []string{fabricengine.OneRepositoryBoardBranch, recordsBranch} {
						gitkit.Git(t, res.BoardDir, "config", "--unset", "branch."+branch+".remote")
						gitkit.Git(t, res.BoardDir, "config", "--unset", "branch."+branch+".merge")
					}
				}
				if tt.commitOnBoard {
					gitkit.Git(t, res.BoardDir, "commit", "--allow-empty", "-m", "board commit after the clone")
				}
				if tt.originAdvances {
					seedTip := gitkit.Git(t, bare, "rev-parse", namespaceBoardRef)
					advanced := gitkit.Git(t, bare, "commit-tree", "-p", seedTip, "-m", "board commit from another clone", seedTip+"^{tree}")
					gitkit.Git(t, bare, "update-ref", namespaceBoardRef, advanced)
				}
				if tt.anotherBoard {
					gitkit.Git(t, bare, "update-ref", namespaceBoardRef, gitkit.Git(t, bare, "rev-parse", "HEAD"))
				}
				if tt.originUnreadable {
					gitkit.Git(t, res.BoardDir, "config", "remote.origin.url", filepath.Join(t.TempDir(), "no-such-repository"))
				}
				originBefore := refLines(t, bare, "%(refname) %(objectname)", fabricengine.RecordsNamespaceRoot)

				err = fabricengine.NewBolt(res.BoardDir).Push(fabricengine.SyncOptions{})
				if err == nil {
					t.Fatal("Push of a board with no upstream = nil error; want a refusal")
				}
				placeholders := strings.NewReplacer("{board}", res.BoardDir, "{hub}", res.HubPath, "{records}", recordsBranch)
				for _, want := range tt.wantErrorHas {
					if want = placeholders.Replace(want); !strings.Contains(err.Error(), want) {
						t.Errorf("Push error = %q; want it to contain %q", err.Error(), want)
					}
				}
				if got := strings.Contains(err.Error(), resetWayForward); got != tt.wantReset {
					t.Errorf("Push error = %q; naming the --reset clone = %v, want %v", err.Error(), got, tt.wantReset)
				}
				if got := refLines(t, bare, "%(refname) %(objectname)", fabricengine.RecordsNamespaceRoot); !slices.Equal(got, originBefore) {
					t.Errorf("origin's records namespace = %v; want it unchanged at %v", got, originBefore)
				}
				if records, err := fabricengine.ListFailedPushes(res.HubPath); err != nil || len(records) != 0 {
					t.Errorf("ListFailedPushes() = %v, %v; want no failed-push record", records, err)
				}

				if tt.wantPushLands {
					for _, branch := range []string{fabricengine.OneRepositoryBoardBranch, recordsBranch} {
						gitkit.Git(t, res.BoardDir, "config", "branch."+branch+".remote", "origin")
						gitkit.Git(t, res.BoardDir, "config", "branch."+branch+".merge", namespaceHeadsPrefix+branch)
					}
					if err := fabricengine.NewBolt(res.BoardDir).Push(fabricengine.SyncOptions{}); err != nil {
						t.Errorf("Push after setting the named upstreams = %v; want nil", err)
					}
				}
			})
		}
	})
}
