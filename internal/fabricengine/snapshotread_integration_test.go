// snapshotread_integration_test.go covers the snapshot reader over real hubs and repositories.

package fabricengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabriccore"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSnapshotReader_ReadsAHub reads every field from a hub with two pairs, and then one field alone.
// It reads refs from stores of which two fail.
// It reads the records store of a one-repository hub before and after its layout cannot be read, and an absent one.
// It reads the anchors with a records worktree's directory gone.
func TestSnapshotReader_ReadsAHub(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const kept, unmarked = "reader-kept", "reader-unmarked"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, kept)
	hubforge.AddPair(t, h, unmarked)
	gitkit.Git(t, h.PairRecordsSibling(unmarked), "rm", "-q", lymcwd.AnchorFileName)
	gitkit.Git(t, h.PairRecordsSibling(unmarked), "commit", "-q", "-m", "drop the anchor marker")
	gitkit.Git(t, h.PrimeRecords(), "tag", "-a", "-m", "annotated", "annotated")
	pending := fabriclayout.PendingTeardown{
		Slug: "reader-gone", Kind: "remove", HandedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Worktree: h.PrimeWorktree(),
	}
	if err := fabricengine.WritePendingTeardownForTest(h.Path, pending); err != nil {
		t.Fatalf("WritePendingTeardownForTest() error = %v", err)
	}

	t.Run("EveryField", func(t *testing.T) {
		stores := map[fabriccore.Store]string{
			fabriccore.StoreCode:    h.PrimeWorktree(),
			fabriccore.StoreRecords: h.PrimeRecords(),
			fabriccore.StoreBolt:    h.BoardDir(),
		}
		got, err := fabricengine.ReadSnapshotForTest(h.Path, stores, fabriccore.PhasePreLock, []fabriccore.Field{
			fabriccore.FieldLocalRefs, fabriccore.FieldOriginRefs, fabriccore.FieldWorktrees,
			fabriccore.FieldAnchors, fabriccore.FieldRecordsStore, fabriccore.FieldPendingTeardowns,
		})
		if err != nil {
			t.Fatalf("ReadSnapshotForTest() error = %v", err)
		}

		primeBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		primeRecordsBranch := gitkit.CurrentBranch(t, h.PrimeRecords())
		code := got.Stores[fabriccore.StoreCode]
		records := got.Stores[fabriccore.StoreRecords]
		requireRef := func(label string, refs fabriccore.Refs, ref, want string) {
			t.Helper()
			if refs[ref] != want {
				t.Errorf("%s[%s] = %q; want %q", label, ref, refs[ref], want)
			}
		}
		requireRef("code local", code.Local, "refs/heads/"+primeBranch, gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"))
		requireRef("records local", records.Local, "refs/heads/"+primeRecordsBranch, gitkit.RevParse(t, h.PrimeRecords(), "HEAD"))
		requireRef("records local", records.Local, "refs/tags/annotated", gitkit.RevParse(t, h.PrimeRecords(), "HEAD"))
		requireRef("code origin", code.Origin, "refs/heads/"+primeBranch, gitkit.RevParse(t, h.CodeBare, primeBranch))
		for _, slug := range []string{kept, unmarked} {
			recordsBranch := fabriclayout.RecordsBranchName(slug)
			requireRef("code local", code.Local, "refs/heads/"+slug, gitkit.RevParse(t, h.PairCodeWorktree(slug), "HEAD"))
			requireRef("records local", records.Local, "refs/heads/"+recordsBranch, gitkit.RevParse(t, h.PairRecordsSibling(slug), "HEAD"))
			requireRef("code origin", code.Origin, "refs/heads/"+slug, gitkit.RevParse(t, h.CodeBare, slug))
			requireRef("records origin", records.Origin, "refs/heads/"+recordsBranch, gitkit.RevParse(t, h.RecordsBare, recordsBranch))
		}
		for store, refs := range got.Stores {
			if !refs.OriginRead {
				t.Errorf("Stores[%s].OriginRead = false; want true", store)
			}
		}

		branches := map[string]string{}
		for _, worktree := range got.Worktrees {
			branches[filepath.FromSlash(worktree.Path)] = worktree.Branch
		}
		wantBranches := map[string]string{
			h.PrimeWorktree():            primeBranch,
			h.PairCodeWorktree(kept):     kept,
			h.PairCodeWorktree(unmarked): unmarked,
		}
		if diff := cmp.Diff(wantBranches, branches); diff != "" {
			t.Errorf("Worktrees mismatch (-want +got):\n%s", diff)
		}
		wantAnchors := map[string]string{h.PrimeRecords(): ".", h.PairRecordsSibling(kept): "."}
		if diff := cmp.Diff(wantAnchors, got.Anchors); diff != "" {
			t.Errorf("Anchors mismatch (-want +got):\n%s", diff)
		}
		if want := (fabriccore.RecordsStore{Present: true}); got.RecordsStore != want {
			t.Errorf("RecordsStore = %+v; want %+v", got.RecordsStore, want)
		}
		if len(got.PendingTeardowns) != 1 {
			t.Fatalf("PendingTeardowns = %+v; want the planted record", got.PendingTeardowns)
		}
		listed := got.PendingTeardowns[0]
		listed.WayForward = ""
		if diff := cmp.Diff(pending, listed); diff != "" {
			t.Errorf("PendingTeardowns[0] mismatch (-want +got):\n%s", diff)
		}
	})

	// The hub builder installs the seams once for the test binary.
	t.Run("RequiredSeamsReadSupplied", func(t *testing.T) {
		got, err := fabricengine.ReadSnapshotForTest(h.Path, nil, fabriccore.PhasePreLock, []fabriccore.Field{fabriccore.FieldSeams})
		if err != nil {
			t.Fatalf("ReadSnapshotForTest() error = %v", err)
		}
		want := map[fabriccore.RequiredSeam]bool{
			fabriccore.SeamRunRecordPaths: true,
			fabriccore.SeamRunLocks:       true,
			fabriccore.SeamStatusCodec:    true,
		}
		if diff := cmp.Diff(want, got.Seams); diff != "" {
			t.Errorf("Seams mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("LocalRefsOfTheCodeStoreAlone", func(t *testing.T) {
		stores := map[fabriccore.Store]string{fabriccore.StoreCode: h.PrimeWorktree()}
		got, err := fabricengine.ReadSnapshotForTest(h.Path, stores, fabriccore.PhaseUnderLock, []fabriccore.Field{fabriccore.FieldLocalRefs})
		if err != nil {
			t.Fatalf("ReadSnapshotForTest() error = %v", err)
		}
		code := got.Stores[fabriccore.StoreCode]
		if len(got.Stores) != 1 || len(code.Local) == 0 || code.Origin != nil || code.OriginRead {
			t.Errorf("Stores = %+v; want the code store's local refs alone", got.Stores)
		}
		got.Stores = nil
		if !reflect.DeepEqual(got, fabriccore.Snapshot{}) {
			t.Errorf("snapshot past its stores = %+v; want every other field empty", got)
		}
	})

	t.Run("AFailingStoreStopsNoOther", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent")
		stores := map[fabriccore.Store]string{
			fabriccore.StoreCode:    absent,
			fabriccore.StoreRecords: h.PrimeRecords(),
			fabriccore.StoreBolt:    absent,
		}

		got, err := fabricengine.ReadSnapshotForTest(h.Path, stores, fabriccore.PhasePreLock, []fabriccore.Field{fabriccore.FieldLocalRefs})

		joined, ok := err.(interface{ Unwrap() []error })
		if !ok || len(joined.Unwrap()) != 2 {
			t.Fatalf("ReadSnapshotForTest() error = %v; want two joined failures", err)
		}
		for i, want := range []string{"read local_refs of the code store: ", "read local_refs of the bolt store: "} {
			if part := joined.Unwrap()[i]; !strings.HasPrefix(part.Error(), want) {
				t.Errorf("joined error %d = %q; want the prefix %q", i, part, want)
			}
		}
		ref := "refs/heads/" + gitkit.CurrentBranch(t, h.PrimeRecords())
		if len(got.Stores) != 1 || got.Stores[fabriccore.StoreRecords].Local[ref] != gitkit.RevParse(t, h.PrimeRecords(), "HEAD") {
			t.Errorf("Stores = %+v; want the records store's refs alone", got.Stores)
		}
	})

	t.Run("RecordsStoreWhoseLayoutCannotBeRead", func(t *testing.T) {
		oneRepo := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		stores := map[fabriccore.Store]string{fabriccore.StoreRecords: oneRepo.PrimeRecords()}
		field := []fabriccore.Field{fabriccore.FieldRecordsStore}

		got, err := fabricengine.ReadSnapshotForTest(oneRepo.Path, stores, fabriccore.PhasePreLock, field)
		want := fabriccore.RecordsStore{Present: true, Heads: "refs/lym/heads/", Tags: "refs/lym/tags/"}
		if err != nil || got.RecordsStore != want {
			t.Errorf("RecordsStore = %+v, %v; want %+v", got.RecordsStore, err, want)
		}

		absent := map[fabriccore.Store]string{fabriccore.StoreRecords: filepath.Join(t.TempDir(), "absent")}
		got, err = fabricengine.ReadSnapshotForTest(oneRepo.Path, absent, fabriccore.PhasePreLock, field)
		if err != nil || got.RecordsStore != (fabriccore.RecordsStore{}) {
			t.Errorf("RecordsStore of an absent directory = %+v, %v; want the zero value", got.RecordsStore, err)
		}

		gitkit.Git(t, oneRepo.PrimeRecords(), "config", "--unset-all", "remote.origin.fetch")
		fabricengine.ForgetOriginNamespaceForTest(oneRepo.PrimeRecords())

		_, err = fabricengine.ReadSnapshotForTest(oneRepo.Path, stores, fabriccore.PhasePreLock, field)

		if !errors.Is(err, fabricengine.ErrRecordsLayout) {
			t.Fatalf("ReadSnapshotForTest() error = %v; want one matching ErrRecordsLayout", err)
		}
		if want := "read records_store of the records store: "; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("ReadSnapshotForTest() error = %q; want the prefix %q", err, want)
		}
	})

	// This runs last, since it removes a directory the cases above read.
	t.Run("AnchorsSkipAPrunableRecordsWorktree", func(t *testing.T) {
		if err := os.RemoveAll(h.PairRecordsSibling(kept)); err != nil {
			t.Fatalf("remove the records worktree: %v", err)
		}
		stores := map[fabriccore.Store]string{fabriccore.StoreRecords: h.PrimeRecords()}

		got, err := fabricengine.ReadSnapshotForTest(h.Path, stores, fabriccore.PhasePreLock, []fabriccore.Field{fabriccore.FieldAnchors})

		if err != nil {
			t.Fatalf("ReadSnapshotForTest() error = %v", err)
		}
		if diff := cmp.Diff(map[string]string{h.PrimeRecords(): "."}, got.Anchors); diff != "" {
			t.Errorf("Anchors mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestSnapshotReader_RefStorage reads the worktree list and the local refs under each ref backend.
// A files repository is read with no git on PATH, so the reader starts no git process;
// a reftable repository is listed through git.
// It runs serially: the files rows empty PATH, which is process-global.
func TestSnapshotReader_RefStorage(t *testing.T) {
	tiergate.Require(t, 1)

	backends := []struct {
		name     string
		initArgs []string
		noGit    bool
		// minimumGit is the oldest git that can create the repository, empty for any.
		minimumGit string
	}{
		{name: "Files", initArgs: []string{"init"}, noGit: true},
		{name: "Reftable", initArgs: []string{"init", "--ref-format=reftable"}, minimumGit: "2.45"},
	}
	reads := []struct {
		name  string
		field fabriccore.Field
		// from picks the directory read from, out of the main worktree, the linked one and its subdirectory.
		from func(main, linked, subdirectory string) string
	}{
		{"WorktreesFromTheMainWorktree", fabriccore.FieldWorktrees, func(main, _, _ string) string { return main }},
		{"WorktreesFromALinkedSubdirectory", fabriccore.FieldWorktrees, func(_, _, subdirectory string) string { return subdirectory }},
		{"RefsFromTheMainWorktree", fabriccore.FieldLocalRefs, func(main, _, _ string) string { return main }},
		{"RefsFromALinkedRoot", fabriccore.FieldLocalRefs, func(_, linked, _ string) string { return linked }},
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			if backend.minimumGit != "" {
				gitkit.SkipBelowGitVersion(t, backend.minimumGit, "git init --ref-format=reftable")
			}
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatalf("resolve temp dir: %v", err)
			}
			repo := filepath.Join(root, "repo")
			gitkit.Git(t, root, append(backend.initArgs, repo)...)
			head := gitkit.CommitFile(t, repo, "a.txt", "one", "first")
			branch := gitkit.CurrentBranch(t, repo)
			gitkit.Git(t, repo, "tag", "light")
			gitkit.Git(t, repo, "tag", "-a", "-m", "annotated", "annotated")
			// A lightweight and an annotated tag naming no commit, which the wanted refs leave out.
			gitkit.Git(t, repo, "tag", "blob", head+":a.txt")
			gitkit.Git(t, repo, "tag", "-a", "-m", "annotated blob", "annotated-blob", head+":a.txt")
			linked := filepath.Join(root, "linked")
			gitkit.Git(t, repo, "worktree", "add", "-b", "side", linked)
			subdirectory := filepath.Join(linked, "sub")
			if err := os.MkdirAll(subdirectory, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", subdirectory, err)
			}
			if backend.noGit {
				t.Setenv("PATH", "")
			}

			wantWorktrees := []fabriclayout.WorktreeEntry{
				{Path: repo, Head: head, Branch: branch, Main: true},
				{Path: linked, Head: head, Branch: "side"},
			}
			wantRefs := fabriccore.Refs{
				"refs/heads/" + branch: head,
				"refs/heads/side":      head,
				"refs/tags/light":      head,
				"refs/tags/annotated":  head,
			}
			for _, read := range reads {
				dir := read.from(repo, linked, subdirectory)
				stores := map[fabriccore.Store]string{fabriccore.StoreCode: dir, fabriccore.StoreRecords: dir}
				got, err := fabricengine.ReadSnapshotForTest("", stores, fabriccore.PhasePreLock, []fabriccore.Field{read.field})
				if err != nil {
					t.Fatalf("%s: ReadSnapshotForTest() error = %v", read.name, err)
				}
				if read.field == fabriccore.FieldWorktrees {
					if diff := cmp.Diff(wantWorktrees, got.Worktrees); diff != "" {
						t.Errorf("%s: Worktrees mismatch (-want +got):\n%s", read.name, diff)
					}
					continue
				}
				for store := range stores {
					if diff := cmp.Diff(wantRefs, got.Stores[store].Local); diff != "" {
						t.Errorf("%s: Stores[%s].Local mismatch (-want +got):\n%s", read.name, store, diff)
					}
				}
			}
		})
	}
}
