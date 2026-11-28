// clone_singlerepo_integration_test.go proves the one-repository clone over hubforge's builder:
// the seed of origin's records namespace, the one-URL join of a seeded origin and the seed push's failure paths.
// Each test reads the origin's refs with git for-each-ref, since the layout is what lands there.
// It shares the fixture helpers of clone_adopt_test.go and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// The origin refs of a one-repository hub's records namespace.
const (
	namespaceHeadsPrefix = fabriclayout.RecordsNamespaceRoot + fabriclayout.RecordsNamespaceHeads
	namespaceBoardRef    = namespaceHeadsPrefix + fabriclayout.OneRepositoryBoardBranch
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

// pushRecordsStore pushes every branch of h's records store, the board's included, through its push refspecs.
// The hub's records then hold no commit origin lacks.
func pushRecordsStore(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	gitkit.Git(t, h.PrimeRecords(), "push", "origin")
}

// ignoreInStore appends patterns to the info/exclude of the store at dir, which is not a linked worktree.
func ignoreInStore(t *testing.T, dir string, patterns ...string) {
	t.Helper()

	exclude := filepath.Join(dir, ".git", "info", "exclude")
	content, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatalf("read %s: %v", exclude, err)
	}
	content = append(content, []byte("\n"+strings.Join(patterns, "\n")+"\n")...)
	if err := os.WriteFile(exclude, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", exclude, err)
	}
}

// writeFiles writes a file at each of the slash-separated paths under dir, making its directories.
func writeFiles(t *testing.T, dir string, paths ...string) {
	t.Helper()

	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("make the directory of %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte("content\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// leaveLymsIgnoredFiles gives h ignored files and a link of the kinds a used hub holds and no operator wrote.
// It pushes the records store first, so the hub then holds no work origin lacks.
func leaveLymsIgnoredFiles(t *testing.T, h *hubforge.Hub) {
	t.Helper()

	pushRecordsStore(t, h)
	ignoreInStore(t, h.PrimeWorktree(), "/.vscode/", "/cache-link")
	writeFiles(t, h.PrimeWorktree(), "CLAUDE.local.md", ".claude/settings.local.json", ".vscode/settings.json", ".vscode/tasks.json")
	if err := os.Symlink(t.TempDir(), filepath.Join(h.PrimeWorktree(), "cache-link")); err != nil {
		t.Fatalf("make an ignored link: %v", err)
	}
	ignoreInStore(t, h.PrimeRecords(), "board.lock", "board.push.lock", ".board-rendered.json")
	writeFiles(t, h.BoardDir(), "board.lock", "board.push.lock", ".board-rendered.json")
	writeFiles(t, h.PrimeRecords(), ".weft/weft.write.lock")
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

	res, err := fabriccli.CloneAndWire(container, fabriclayout.CloneOptions{
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
	if got := gitkit.CurrentBranch(t, boardDir); got != fabriclayout.OneRepositoryBoardBranch {
		t.Errorf("the board is on %q; want %q", got, fabriclayout.OneRepositoryBoardBranch)
	}
	binding, err := os.ReadFile(filepath.Join(boardDir, fabriclayout.WarpBindingFileName))
	if err != nil {
		t.Fatalf("read the board's binding: %v", err)
	}
	if got := strings.TrimSpace(string(binding)); got != filepath.ToSlash(origin) {
		t.Errorf("the board's %s = %q; want the origin %q", fabriclayout.WarpBindingFileName, got, filepath.ToSlash(origin))
	}
	if got := gitkit.Git(t, recordsDir, "config", "--get", fabriclayout.PrimeRecordsBranchKey); got != recordsBranch {
		t.Errorf("%s = %q; want %q", fabriclayout.PrimeRecordsBranchKey, got, recordsBranch)
	}
	wantTracking := []string{"refs/remotes/origin/" + fabriclayout.OneRepositoryBoardBranch, "refs/remotes/origin/" + recordsBranch}
	if got := refLines(t, recordsDir, "%(refname)", "refs/remotes/origin/"); !slices.Equal(got, wantTracking) {
		t.Errorf("the records store's remote-tracking refs = %v; want %v, with no code branch among them", got, wantTracking)
	}
	if got := gitkit.Git(t, boardDir, "config", "--get", "branch."+fabriclayout.OneRepositoryBoardBranch+".merge"); got != namespaceBoardRef {
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
			if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); !slices.Equal(got, wantNamespace) {
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
					t.Errorf("a default clone of the origin fetched %s; want nothing under %s", ref, fabriclayout.RecordsNamespaceRoot)
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
		for _, file := range []string{fabriclayout.WarpBindingFileName, fabriclayout.ShortnameFileName} {
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
		firstBinding, err := os.ReadFile(filepath.Join(first.BoardDir(), fabriclayout.WarpBindingFileName))
		if err != nil {
			t.Fatalf("read the first board's binding: %v", err)
		}
		secondBinding, err := os.ReadFile(filepath.Join(second.BoardDir(), fabriclayout.WarpBindingFileName))
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

		_, err := fabricengine.CloneHub(container, fabriclayout.CloneOptions{WeftURL: filepath.ToSlash(first.CodeBare), Shortname: "other"})
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("recorded shortname is %q", hubforge.TestShortname)) {
			t.Errorf("CloneHub with another shortname error = %v; want a refusal naming the recorded shortname %q", err, hubforge.TestShortname)
		}
		requireEmptyDirectory(t, container)
	})

	t.Run("BindingNamingAnotherURLIsRefused", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		binding := filepath.Join(first.BoardDir(), fabriclayout.WarpBindingFileName)
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

		_, err := fabricengine.CloneHub(container, fabriclayout.CloneOptions{WeftURL: filepath.ToSlash(first.CodeBare), Shortname: hubforge.TestShortname})
		if !errors.Is(err, fabricengine.ErrWarpBindingMismatch) {
			t.Errorf("CloneHub over a rewritten binding error = %v; want %v", err, fabricengine.ErrWarpBindingMismatch)
		}
		requireEmptyDirectory(t, container)
	})

	t.Run("ResetWithAnotherSubpathIsRefusedAndKeepsTheHub", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		second := hubforge.CloneSecond(t, first)

		// DiscardWork passes over whatever the stores hold, so the subpath check is what refuses.
		_, err := fabricengine.CloneHub(second.Container, fabriclayout.CloneOptions{
			WeftURL:     filepath.ToSlash(first.CodeBare),
			Subpath:     "backend",
			Reset:       true,
			DiscardWork: true,
			Shortname:   hubforge.TestShortname,
		})
		if err == nil || !strings.Contains(err.Error(), `recorded anchor "."`) {
			t.Errorf("CloneHub --reset with --subpath backend error = %v; want a refusal naming the recorded anchor \".\"", err)
		}
		if _, statErr := os.Stat(filepath.Join(second.PrimeRecords(), lymcwd.AnchorFileName)); statErr != nil {
			t.Errorf("the existing hub lost its anchor marker: %v", statErr)
		}
	})

	t.Run("ResetOfAHubHoldingWorkOriginLacksIsRefusedAndKeepsTheHub", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name string
			// discardable reports whether DiscardWork lets the reset remove the hub, so the refusal names that clone.
			discardable bool
			// leaveWork gives the hub work origin lacks and returns a text the refusal names it by.
			leaveWork func(t *testing.T, h *hubforge.Hub) string
		}{
			{"Pair", false, func(t *testing.T, h *hubforge.Hub) string {
				hubforge.AddPairWith(t, h, "kept", fabricengine.AddOptions{SkipPush: true})
				return "the pair worktree " + fabricengine.WorktreePath(h.Location, "kept")
			}},
			{"UnpushedCodeCommit", true, func(t *testing.T, h *hubforge.Hub) string {
				gitkit.CommitFile(t, h.PrimeWorktree(), "local.txt", "local\n", "a commit origin lacks")
				return "1 commit(s) of " + h.PrimeWorktree() + " that origin lacks"
			}},
			{"UnpushedRecordsCommit", true, func(t *testing.T, h *hubforge.Hub) string {
				pushRecordsStore(t, h)
				gitkit.CommitFile(t, h.PrimeRecords(), "record.md", "record\n", "a records commit origin lacks")
				return "1 commit(s) of " + h.PrimeRecords() + " that origin lacks"
			}},
			{"UnpushedBoardCommit", true, func(t *testing.T, h *hubforge.Hub) string {
				pushRecordsStore(t, h)
				gitkit.CommitFile(t, h.BoardDir(), "note.md", "note\n", "a board commit origin lacks")
				return "1 commit(s) of " + h.PrimeRecords() + " that origin lacks"
			}},
			{"UncommittedBoltChange", true, func(t *testing.T, h *hubforge.Hub) string {
				if err := os.WriteFile(filepath.Join(h.BoardDir(), "draft.md"), []byte("draft\n"), 0o644); err != nil {
					t.Fatalf("write an uncommitted board file: %v", err)
				}
				return "uncommitted or untracked changes in " + h.BoardDir()
			}},
			{"IgnoredFilesBesideTheOnesLymWrites", true, func(t *testing.T, h *hubforge.Hub) string {
				leaveLymsIgnoredFiles(t, h)
				ignoreInStore(t, h.PrimeWorktree(), "secret.env")
				writeFiles(t, h.PrimeWorktree(), "secret.env", ".vscode/launch.json")
				return "ignored files in " + h.PrimeWorktree() + ": .vscode/launch.json, secret.env"
			}},
			{"EarlierLockFileNameInTheCodeStore", true, func(t *testing.T, h *hubforge.Hub) string {
				ignoreInStore(t, h.PrimeWorktree(), "board.lock")
				writeFiles(t, h.PrimeWorktree(), "board.lock")
				return "ignored files in " + h.PrimeWorktree() + ": board.lock"
			}},
			{"IgnoredDirectoryOfAnEarlierLockFileName", true, func(t *testing.T, h *hubforge.Hub) string {
				ignoreInStore(t, h.PrimeRecords(), "board.lock")
				writeFiles(t, h.BoardDir(), "board.lock/kept.txt")
				return "ignored files in " + h.BoardDir() + ": board.lock/"
			}},
			{"MoreIgnoredFilesThanARefusalNames", true, func(t *testing.T, h *hubforge.Hub) string {
				ignoreInStore(t, h.PrimeWorktree(), "*.env")
				writeFiles(t, h.PrimeWorktree(), "a.env", "b.env", "c.env", "d.env", "e.env", "f.env", "g.env")
				return "ignored files in " + h.PrimeWorktree() + ": a.env, b.env, c.env, d.env, e.env and 2 more"
			}},
			{"CommitOnADetachedBoltHead", true, func(t *testing.T, h *hubforge.Hub) string {
				pushRecordsStore(t, h)
				gitkit.Git(t, h.BoardDir(), "checkout", "--detach")
				gitkit.CommitFile(t, h.BoardDir(), "detached.md", "detached\n", "a board commit on a detached HEAD")
				return "1 commit(s) of " + h.BoardDir() + "'s HEAD that origin lacks"
			}},
			{"CommitOnlyALocalTagHolds", true, func(t *testing.T, h *hubforge.Hub) string {
				gitkit.CommitFile(t, h.PrimeWorktree(), "tagged.txt", "tagged\n", "a commit only a local tag holds")
				gitkit.Git(t, h.PrimeWorktree(), "tag", fabriclayout.ArchiveTagPrefix+"old/tip")
				gitkit.Git(t, h.PrimeWorktree(), "reset", "--hard", "HEAD~1")
				return "1 commit(s) of " + h.PrimeWorktree() + " that origin lacks"
			}},
			{"CommitOnADetachedHead", true, func(t *testing.T, h *hubforge.Hub) string {
				gitkit.Git(t, h.PrimeWorktree(), "checkout", "--detach")
				gitkit.CommitFile(t, h.PrimeWorktree(), "detached.txt", "detached\n", "a commit on a detached HEAD")
				return "1 commit(s) of " + h.PrimeWorktree() + " that origin lacks"
			}},
			{"UncommittedFileInThePrime", true, func(t *testing.T, h *hubforge.Hub) string {
				gitkit.Git(t, h.PrimeWorktree(), "config", "status.showUntrackedFiles", "no")
				if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "edit.txt"), []byte("edit\n"), 0o644); err != nil {
					t.Fatalf("write an uncommitted file: %v", err)
				}
				return "uncommitted or untracked changes in " + h.PrimeWorktree()
			}},
			{"Stash", true, func(t *testing.T, h *hubforge.Hub) string {
				if err := os.WriteFile(filepath.Join(h.PrimeWorktree(), "stashed.txt"), []byte("stashed\n"), 0o644); err != nil {
					t.Fatalf("write a file to stash: %v", err)
				}
				gitkit.Git(t, h.PrimeWorktree(), "stash", "push", "--include-untracked")
				return "a stash in " + h.PrimeWorktree()
			}},
			{"PendingTeardown", false, func(t *testing.T, h *hubforge.Hub) string {
				record := fabriclayout.PendingTeardown{Slug: "gone", Kind: "remove", Worktree: h.PrimeWorktree(), Rerun: "lym fabric remove gone --remote"}
				if err := fabricengine.WritePendingTeardownForTest(h.Path, record); err != nil {
					t.Fatalf("write the pending teardown: %v", err)
				}
				return "the unfinished teardown of \"gone\" (run `lym fabric remove gone --remote` again"
			}},
			{"TeardownStillRunning", false, func(t *testing.T, h *hubforge.Hub) string {
				record := fabriclayout.PendingTeardown{Slug: "running", Kind: "remove", Worktree: h.PrimeWorktree()}
				if err := fabricengine.WritePendingTeardownForTest(h.Path, record); err != nil {
					t.Fatalf("write the pending teardown: %v", err)
				}
				holdTeardownLock(t, h, "running")
				return "the unfinished teardown of \"running\""
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
				wantNamed := tt.leaveWork(t, h)

				_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
					WeftURL:   filepath.ToSlash(h.CodeBare),
					Reset:     true,
					Shortname: hubforge.TestShortname,
				})
				if err == nil || !strings.Contains(err.Error(), "it holds work origin lacks: ") || !strings.Contains(err.Error(), wantNamed) {
					t.Fatalf("CloneHub --reset over a hub holding work error = %v; want a refusal naming %q", err, wantNamed)
				}
				if !strings.Contains(err.Error(), "`mv "+h.Path+" "+h.Path+".old && git -C ") {
					t.Errorf("CloneHub --reset error = %q; want the move that keeps the hub as its way forward", err.Error())
				}
				discardClone := " --reset --" + fabricengine.DiscardWorkFlag + "`"
				if got := strings.Contains(err.Error(), discardClone); got != tt.discardable {
					t.Errorf("CloneHub --reset error = %q; naming the clone that discards the work = %v, want %v", err.Error(), got, tt.discardable)
				}
				if !tt.discardable {
					_, err = fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
						WeftURL:     filepath.ToSlash(h.CodeBare),
						Reset:       true,
						DiscardWork: true,
						Shortname:   hubforge.TestShortname,
					})
					if err == nil || !strings.Contains(err.Error(), wantNamed) {
						t.Errorf("CloneHub --reset --discard-work over a hub in use error = %v; want a refusal naming %q", err, wantNamed)
					}
				}
				if _, statErr := os.Stat(filepath.Join(h.PrimeRecords(), lymcwd.AnchorFileName)); statErr != nil {
					t.Errorf("the hub holding work lost its anchor marker: %v", statErr)
				}
			})
		}
	})

	t.Run("ResetProceedsOverTheIgnoredFilesLymWrites", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		leaveLymsIgnoredFiles(t, h)

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
			WeftURL:   filepath.ToSlash(h.CodeBare),
			Reset:     true,
			Shortname: hubforge.TestShortname,
		})
		if err != nil {
			t.Errorf("CloneHub --reset error = %v; want the hub replaced, since no operator wrote its ignored files", err)
		}
	})

	t.Run("ResetProceedsOverTheLockFilesAnEarlierLayoutLeft", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		// The tracked record makes git list the lock file beside it by itself.
		writeFiles(t, h.PrimeRecords(), "_lym/runs/old/settle-record.json")
		gitkit.Git(t, h.PrimeRecords(), "add", "_lym/runs/old/settle-record.json")
		gitkit.Git(t, h.PrimeRecords(), "commit", "-m", "a run's record")
		pushRecordsStore(t, h)
		// An earlier layout kept these lock files beside the files they guarded and hid them with exclude lines.
		ignoreInStore(t, h.PrimeRecords(), ".gitrepo-push.lock", "board.json.swaplock", "tasks.json.swaplock", "settle-record.json.lock")
		writeFiles(t, h.BoardDir(), ".gitrepo-push.lock", "board.json.swaplock", "tasks.json.swaplock")
		// The directory of the run "gone" holds no file but its lock, so git lists the directory alone.
		writeFiles(t, h.PrimeRecords(), "settle-record.json.lock",
			"_lym/runs/old/settle-record.json.lock", "_lym/runs/gone/settle-record.json.lock")

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
			WeftURL:   filepath.ToSlash(h.CodeBare),
			Reset:     true,
			Shortname: hubforge.TestShortname,
		})
		if err != nil {
			t.Errorf("CloneHub --reset error = %v; want the hub replaced, since no current code writes an earlier layout's lock files", err)
		}
	})

	t.Run("ResetOfAHubWithAnUnreadableStoreNamesTheMove", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		pointer := "gitdir: " + filepath.ToSlash(filepath.Join(t.TempDir(), "missing")) + "\n"
		if err := os.WriteFile(filepath.Join(h.BoardDir(), ".git"), []byte(pointer), 0o644); err != nil {
			t.Fatalf("point the board at a missing git dir: %v", err)
		}

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
			WeftURL:     filepath.ToSlash(h.CodeBare),
			Reset:       true,
			DiscardWork: true,
			Shortname:   hubforge.TestShortname,
		})
		if err == nil || !strings.Contains(err.Error(), "way forward: move it aside") {
			t.Errorf("CloneHub --reset over an unreadable store error = %v; want a refusal naming the move aside", err)
		}
		if _, statErr := os.Stat(filepath.Join(h.PrimeRecords(), lymcwd.AnchorFileName)); statErr != nil {
			t.Errorf("the hub with an unreadable store lost its anchor marker: %v", statErr)
		}
	})

	t.Run("ResetWithDiscardWorkReplacesAHubHoldingStoreWork", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		gitkit.CommitFile(t, h.PrimeRecords(), "record.md", "record\n", "a records commit origin lacks")
		untracked := filepath.Join(h.PrimeWorktree(), "edit.txt")
		if err := os.WriteFile(untracked, []byte("edit\n"), 0o644); err != nil {
			t.Fatalf("write an untracked file: %v", err)
		}

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
			WeftURL:     filepath.ToSlash(h.CodeBare),
			Reset:       true,
			DiscardWork: true,
			Shortname:   hubforge.TestShortname,
		})
		if err != nil {
			t.Fatalf("CloneHub --reset --discard-work error = %v; want the hub replaced", err)
		}
		if _, statErr := os.Stat(untracked); !os.IsNotExist(statErr) {
			t.Errorf("stat %s after the replacement = %v; want the discarded file gone", untracked, statErr)
		}
	})

	t.Run("ResetProceedsOverACommitOriginHoldsUnderATagAlone", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		// Origin takes the records store's commits first, so the tagged code commit is the only work in question.
		pushRecordsStore(t, h)
		gitkit.CommitFile(t, h.PrimeWorktree(), "released.txt", "released\n", "a commit origin holds under a tag")
		gitkit.Git(t, h.PrimeWorktree(), "tag", "released")
		gitkit.Git(t, h.PrimeWorktree(), "push", "origin", "refs/tags/released")
		gitkit.Git(t, h.PrimeWorktree(), "reset", "--hard", "HEAD~1")

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{
			WeftURL:   filepath.ToSlash(h.CodeBare),
			Reset:     true,
			Shortname: hubforge.TestShortname,
		})
		if err != nil {
			t.Errorf("CloneHub --reset error = %v; want the hub replaced, since origin holds the tagged commit", err)
		}
	})

	t.Run("TheMoveAsideCommandKeepsTheHubsWorktreesWorking", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		hubforge.AddPairWith(t, h, "kept", fabricengine.AddOptions{SkipPush: true})

		_, err := fabricengine.CloneHub(h.Container, fabriclayout.CloneOptions{WeftURL: filepath.ToSlash(h.CodeBare), Reset: true})
		if err == nil {
			t.Fatalf("CloneHub --reset over a hub holding a pair = nil error; want a refusal")
		}
		_, rest, found := strings.Cut(err.Error(), "move it aside with `")
		command, _, closed := strings.Cut(rest, "`")
		if !found || !closed {
			t.Fatalf("CloneHub --reset error = %q; want a move-aside command", err)
		}
		if out, runErr := exec.Command("sh", "-c", command).CombinedOutput(); runErr != nil {
			t.Fatalf("run %q: %v\n%s", command, runErr, out)
		}

		moved := h.Path + ".old"
		// The hub's own teardown looks for it where it was made.
		t.Cleanup(func() { _ = os.Rename(moved, h.Path) })
		for _, worktree := range []string{h.PairCodeWorktree("kept"), h.PairRecordsSibling("kept"), fabriclayout.BoltDir(h.Path)} {
			gitkit.Git(t, moved+strings.TrimPrefix(worktree, h.Path), "status", "--porcelain")
		}
	})

	t.Run("ResetWithAMissingSubpathIsRefusedBeforeTheHubIsTouched", func(t *testing.T) {
		t.Parallel()

		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		gitkit.Git(t, first.CodeBare, "update-ref", "-d", namespaceHeadsPrefix+"main-weft")

		// DiscardWork passes over whatever the stores hold, so the subpath check is what refuses.
		_, err := fabricengine.CloneHub(first.Container, fabriclayout.CloneOptions{
			WeftURL:     filepath.ToSlash(first.CodeBare),
			Subpath:     "no-such-directory",
			Reset:       true,
			DiscardWork: true,
			Shortname:   hubforge.TestShortname,
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
		if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); len(got) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, the records push restored", got)
		}

		if err := os.Remove(hook); err != nil {
			t.Fatalf("remove the hook: %v", err)
		}
		if _, err := cloneSingleRepo(t, container, bare, ""); err != nil {
			t.Fatalf("rerun of the clone with the hook removed: %v", err)
		}
		wantNamespace := []string{namespaceBoardRef, recordsRef}
		if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); !slices.Equal(got, wantNamespace) {
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
			if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); !slices.Equal(got, wantRefs) {
				t.Errorf("origin's records namespace = %v; want %v", got, wantRefs)
			}
		})
	}

	t.Run("RefusedCredentialRemovesTheHubAndNamesTheCredentials", func(t *testing.T) {
		tests := []struct {
			name string
			// refusedRef is the origin ref whose push meets the refusal.
			refusedRef string
		}{
			{"RecordsPush", recordsRef},
			{"BoardPushAfterTheRecordsPushLanded", namespaceBoardRef},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bare := hubforge.CopyCodeBare(t, "")
				refusal := filepath.Join(t.TempDir(), "refuse.sh")
				container := t.TempDir()
				fabricengine.SetSeedPushHookForTest(t, func(ref string) {
					if ref != tt.refusedRef {
						return
					}
					stores, err := filepath.Glob(filepath.Join(container, "*", "*"+weftname.Suffix))
					if err != nil || len(stores) != 1 {
						t.Fatalf("records stores under %s = (%v, %v); want one", container, stores, err)
					}
					// GitHub's answer to a push the credentials may not make, in the wording the gitrepo push tests record.
					// Origin's HTTP front end refuses before any receive-pack runs, so git prints no rejection line.
					// The refusal is given once: the transport first restores origin's URL for the rollback that follows.
					script := "#!/bin/sh\n" +
						"env -u GIT_DIR git -C '" + filepath.ToSlash(stores[0]) + "' config remote.origin.url '" + filepath.ToSlash(bare) + "'\n" +
						"echo 'remote: Permission to acme/records.git denied to someone.' >&2\n" +
						"echo \"fatal: unable to access 'https://github.com/acme/records.git/': The requested URL returned error: 403\" >&2\n" +
						"exit 128\n"
					if err := os.WriteFile(refusal, []byte(script), 0o755); err != nil {
						t.Fatalf("write the refusing transport: %v", err)
					}
					gitkit.Git(t, stores[0], "config", "protocol.ext.allow", "always")
					gitkit.Git(t, stores[0], "config", "remote.origin.url", "ext::sh "+filepath.ToSlash(refusal))
				})

				_, err := cloneSingleRepo(t, container, bare, "")
				if err == nil || !strings.Contains(err.Error(), "for want of authentication or permission") || !strings.Contains(err.Error(), "give this machine's git credentials push access") {
					t.Fatalf("clone with a refused credential error = %v; want the refusal and the credentials way forward", err)
				}
				if strings.Contains(err.Error(), "may hold") {
					t.Errorf("clone error = %q; a refused credential took nothing, so no ref origin may hold is named", err.Error())
				}
				requireEmptyDirectory(t, container)
				if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); len(got) != 0 {
					t.Errorf("origin's records namespace = %v; want it empty", got)
				}
			})
		}
	})

	t.Run("TransientRefusalIsRetriedByTheSameClone", func(t *testing.T) {
		bare := hubforge.CopyCodeBare(t, "")
		lockDir := filepath.Join(bare, filepath.FromSlash(namespaceHeadsPrefix))
		if err := os.MkdirAll(lockDir, 0o755); err != nil {
			t.Fatalf("create the namespace directory: %v", err)
		}
		lock := filepath.Join(lockDir, fabriclayout.OneRepositoryBoardBranch+".lock")
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
		if got := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); len(got) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, the records push restored", got)
		}
	})

	t.Run("AbandonedSeedRemovesTheHubAndNamesTheRerun", func(t *testing.T) {
		bare := hubforge.CopyCodeBare(t, "")
		container := t.TempDir()
		res, err := fabricengine.CloneHub(container, fabriclayout.CloneOptions{
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
		if refs := refLines(t, bare, "%(refname)", fabriclayout.RecordsNamespaceRoot); len(refs) != 0 {
			t.Errorf("origin's records namespace = %v; want it empty, since nothing was pushed", refs)
		}
	})

	t.Run("AbandonedTwoURLCloneKeepsTheHub", func(t *testing.T) {
		fixtures := t.TempDir()
		recordsRemote := makeEmptyBareRemote(t, fixtures, "records")
		codeRemote := makeBareRemote(t, fixtures, "code")
		container := t.TempDir()
		res, err := fabricengine.CloneHub(container, fabriclayout.CloneOptions{
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
		const resetWayForward = "run the same `lym fabric clone --single-repo` again with `--reset --discard-work`"
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
				wantErrorHas: []string{"origin holds no board under " + fabriclayout.RecordsNamespaceRoot + ", so it was never seeded onto origin"},
				wantReset:    true,
			},
			{
				name:            "OriginHoldsTheSeed",
				originHoldsSeed: true,
				wantErrorHas: []string{
					"so the seed landed and its clone stopped before it set the upstreams",
					"`git -C {board} config branch." + fabriclayout.OneRepositoryBoardBranch + ".remote origin`",
					"`git -C {board} config branch." + fabriclayout.OneRepositoryBoardBranch + ".merge " + namespaceBoardRef + "`",
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
					res, err = fabricengine.CloneHub(t.TempDir(), fabriclayout.CloneOptions{
						WeftURL:    filepath.ToSlash(bare),
						SingleRepo: true,
						Shortname:  hubforge.TestShortname,
					})
				}
				if err != nil {
					t.Fatalf("clone --single-repo: %v", err)
				}
				recordsBranch := gitkit.Git(t, res.BoardDir, "config", "--get", fabriclayout.PrimeRecordsBranchKey)
				if tt.originHoldsSeed {
					for _, branch := range []string{fabriclayout.OneRepositoryBoardBranch, recordsBranch} {
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
				originBefore := refLines(t, bare, "%(refname) %(objectname)", fabriclayout.RecordsNamespaceRoot)

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
				if got := refLines(t, bare, "%(refname) %(objectname)", fabriclayout.RecordsNamespaceRoot); !slices.Equal(got, originBefore) {
					t.Errorf("origin's records namespace = %v; want it unchanged at %v", got, originBefore)
				}
				if records, err := fabricengine.ListFailedPushes(res.HubPath); err != nil || len(records) != 0 {
					t.Errorf("ListFailedPushes() = %v, %v; want no failed-push record", records, err)
				}

				if tt.wantPushLands {
					for _, branch := range []string{fabriclayout.OneRepositoryBoardBranch, recordsBranch} {
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
