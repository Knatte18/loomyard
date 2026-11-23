// onerepository_lifecycle_integration_test.go pins the pair lifecycle in a one-repository hub.
// Each test builds its hub with hubforge.NewOneRepoHub.
// It drives the verbs the two-repository suites drive.
// It reads the one origin with git for-each-ref to see where the records branches, tags and board land.
// It shares the fixture helpers of the clone tests and the TestMain in testmain_test.go.

package fabricengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// The origin ref prefixes of a one-repository hub's tags, below the records namespace root.
const (
	// namespaceTagsPrefix holds the records store's tags.
	namespaceTagsPrefix = fabricengine.RecordsNamespaceRoot + fabricengine.RecordsNamespaceTags
	// namespaceCodeTagsPrefix holds the code store's archive tags.
	namespaceCodeTagsPrefix = fabricengine.RecordsNamespaceRoot + fabricengine.RecordsNamespaceCodeTags
)

// TestOneRepositoryMerge_RefusesARecordsStoreThatLostItsFetchRefspecs asserts a merge reads the records layout first.
// With the records store's origin fetch refspecs unset, a fetch moves no tracking ref of the records side.
// The merge refuses naming the refspecs to restore, and the prime's code branch stays where it was.
func TestOneRepositoryMerge_RefusesARecordsStoreThatLostItsFetchRefspecs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ns-merge-layout"
	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	added := hubforge.AddPair(t, h, slug)
	gitkit.CommitFile(t, h.PairCodeWorktree(slug), "pair-work.txt", "pair\n", "the pair's work")
	before := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD")
	gitkit.Git(t, h.PrimeRecords(), "config", "--unset-all", "remote.origin.fetch")
	fabricengine.ForgetOriginNamespaceForTest(h.PrimeRecords())

	_, err := hubforge.OpenFabric(t, h).MergeIn(added.Branch)

	const wantErrorHas = "config --replace-all remote.origin.fetch '+refs/lym/heads/*:refs/remotes/origin/*'"
	if err == nil || !strings.Contains(err.Error(), wantErrorHas) {
		t.Fatalf("MergeIn(%s) error = %v; want one naming %q", added.Branch, err, wantErrorHas)
	}
	if got := gitkit.RevParse(t, h.PrimeWorktree(), "HEAD"); got != before {
		t.Errorf("the prime's HEAD = %s after the refused merge; want it still at %s", got, before)
	}
}

// TestOneRepositoryOrigin_PlainArchiveTagsAreListedWithTheirMove pins the listing of origin's plain archive tags.
// An earlier binary pushed archive and owner archive tags under origin's plain refs/tags/;
// each is listed with the commands that move it.
// Those commands leave origin holding the tag under the code store's prefix and no plain ref.
// A tag already under that prefix is not listed.
func TestOneRepositoryOrigin_PlainArchiveTagsAreListedWithTheirMove(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	bare, prime := h.CodeBare, h.PrimeWorktree()
	tip := gitkit.RevParse(t, bare, "main")
	archiveName := fabricengine.ArchiveTagName("t5", tip)
	ownerName := fabricengine.OwnerArchiveTagName("t6", tip)
	for _, ref := range []string{"refs/tags/" + archiveName, "refs/tags/" + ownerName, namespaceCodeTagsPrefix + fabricengine.ArchiveTagName("t7", tip)} {
		gitkit.Git(t, bare, "update-ref", ref, tip)
	}
	move := func(name string) string {
		return "move it under " + namespaceCodeTagsPrefix + " with `git -C " + prime + " fetch --no-tags origin refs/tags/" + name +
			"`, then `git -C " + prime + " push --atomic origin " + tip + ":" + namespaceCodeTagsPrefix + name + " :refs/tags/" + name + "`"
	}

	got, err := fabricengine.PlainArchiveTagsOnOrigin(h.Location)

	if err != nil {
		t.Fatalf("PlainArchiveTagsOnOrigin() error = %v", err)
	}
	want := []fabricengine.PlainArchiveTag{
		{Ref: "refs/tags/" + ownerName, SHA: tip, WayForward: move(ownerName)},
		{Ref: "refs/tags/" + archiveName, SHA: tip, WayForward: move(archiveName)},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("PlainArchiveTagsOnOrigin() = %+v; want %+v", got, want)
	}

	gitkit.Git(t, prime, "fetch", "--no-tags", "origin", "refs/tags/"+archiveName)
	gitkit.Git(t, prime, "push", "--atomic", "origin", tip+":"+namespaceCodeTagsPrefix+archiveName, ":refs/tags/"+archiveName)
	if moved := refLines(t, bare, "%(objectname)", namespaceCodeTagsPrefix+archiveName); !slices.Equal(moved, []string{tip}) {
		t.Errorf("origin's %s%s = %v; want one at %s", namespaceCodeTagsPrefix, archiveName, moved, tip)
	}
	if plain := refLines(t, bare, "%(refname)", "refs/tags/"+archiveName); len(plain) != 0 {
		t.Errorf("origin still holds %v after the move; want no plain ref", plain)
	}
}

// oneRepoPairFabric opens the fabric handle on the pair slug of h.
func oneRepoPairFabric(t *testing.T, h *hubforge.Hub, slug string) *fabricengine.Fabric {
	t.Helper()

	location, err := lymcwd.ResolveWorktree(h.PairCodeWorktree(slug))
	if err != nil {
		t.Fatalf("ResolveWorktree(%s): %v", slug, err)
	}
	f, err := fabricengine.Open(location)
	if err != nil {
		t.Fatalf("Open(%s): %v", slug, err)
	}
	return f
}

// oneRepoPushRecords pushes the records branch of the pair slug of h to origin's namespace.
func oneRepoPushRecords(t *testing.T, h *hubforge.Hub, slug string) {
	t.Helper()

	if _, err := oneRepoPairFabric(t, h, slug).PushWeft(fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("PushWeft(%s): %v", slug, err)
	}
}

// oneRepoRecordsTip returns the tip of the records branch under origin's namespace.
func oneRepoRecordsTip(t *testing.T, bare, recordsBranch string) string {
	t.Helper()

	return gitkit.RevParse(t, bare, namespaceHeadsPrefix+recordsBranch)
}

// oneRepoPushNamespaced pushes branch of the records repository at dir to its namespace ref.
// That is the ref the records store's push refspec maps the branch to.
func oneRepoPushNamespaced(t *testing.T, dir, branch string) {
	t.Helper()

	gitkit.Git(t, dir, "push", "origin", "refs/heads/"+branch+":"+namespaceHeadsPrefix+branch)
}

// refsAddedSince returns the refs of after that before lacks.
func refsAddedSince(before, after []string) []string {
	var added []string
	for _, ref := range after {
		if !slices.Contains(before, ref) {
			added = append(added, ref)
		}
	}
	return added
}

// oneRepoBoardSlugs returns the slugs of the board entries the board file at ref of bare holds.
func oneRepoBoardSlugs(t *testing.T, bare, ref string) []string {
	t.Helper()

	var file struct {
		Entries []boardengine.Task `json:"entries"`
	}
	if err := json.Unmarshal([]byte(gitkit.Git(t, bare, "show", ref+":"+boardengine.BoardFileRel())), &file); err != nil {
		t.Fatalf("decode the board file at %s: %v", ref, err)
	}
	var slugs []string
	for _, entry := range file.Entries {
		slugs = append(slugs, entry.Slug)
	}
	slices.Sort(slugs)
	return slugs
}

// TestOneRepositoryPair_RecordsLiveInTheNamespace asserts where a pair's refs land on origin.
// The records branch and archive tags live under origin's records namespace.
// The only ref a pair adds outside it is its code branch.
func TestOneRepositoryPair_RecordsLiveInTheNamespace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("PushPlacesTheBranchAndRemoveArchivesIt", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-pair"
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		bare := h.CodeBare
		refsBefore := refLines(t, bare, "%(refname)")

		added := hubforge.AddPair(t, h, slug)
		recordsBranch := fabricengine.RecordsBranchName(added.Branch)
		gitkit.CommitFile(t, h.PairCodeWorktree(slug), "work.txt", "work\n", "code work")
		if _, err := oneRepoPairFabric(t, h, slug).PushBranch(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("PushBranch: %v", err)
		}

		sibling := h.PairRecordsSibling(slug)
		tip := gitkit.RevParse(t, sibling, "HEAD")
		if got := oneRepoRecordsTip(t, bare, recordsBranch); got != tip {
			t.Errorf("%s%s = %s; want the pair's records tip %s", namespaceHeadsPrefix, recordsBranch, got, tip)
		}
		if got := gitkit.Git(t, sibling, "config", "--get", "branch."+recordsBranch+".merge"); got != namespaceHeadsPrefix+recordsBranch {
			t.Errorf("branch.%s.merge = %q; want %q", recordsBranch, got, namespaceHeadsPrefix+recordsBranch)
		}
		if unpushed, err := gitrepo.New(sibling).HasUnpushed(); err != nil || unpushed {
			t.Errorf("HasUnpushed() = %v, %v; want false right after the push", unpushed, err)
		}
		var outside []string
		for _, ref := range refsAddedSince(refsBefore, refLines(t, bare, "%(refname)")) {
			if !strings.HasPrefix(ref, fabricengine.RecordsNamespaceRoot) {
				outside = append(outside, ref)
			}
		}
		if want := []string{"refs/heads/" + added.Branch}; !slices.Equal(outside, want) {
			t.Errorf("refs the pair added outside %s = %v; want %v", fabricengine.RecordsNamespaceRoot, outside, want)
		}

		res, err := h.Topology.Remove(h.Location, slug, false, true)
		if err != nil {
			t.Fatalf("Remove(%s): %v", slug, err)
		}
		wantTag := fabricengine.ArchiveTagName(slug, tip)
		if res.ArchiveTag != wantTag {
			t.Fatalf("ArchiveTag = %q; want %q", res.ArchiveTag, wantTag)
		}
		if got := refLines(t, bare, "%(objectname)", namespaceTagsPrefix+wantTag); !slices.Equal(got, []string{tip}) {
			t.Errorf("origin's archive tag under %s = %v; want one at %s", namespaceTagsPrefix, got, tip)
		}
		if got := refLines(t, bare, "%(refname)", namespaceHeadsPrefix+recordsBranch); len(got) != 0 {
			t.Errorf("origin still holds %v after Remove with its remote step; want the records branch deleted", got)
		}
		if got := refLines(t, bare, "%(refname)", "refs/tags/"); len(got) != 0 {
			t.Errorf("origin holds tags %v under refs/tags/; want none", got)
		}
	})

	t.Run("AddReplacesAnArchivedLeftoverInTheNamespace", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-leftover"
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		bare := h.CodeBare
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, h, slug).Branch)
		gitkit.CommitFile(t, h.PairRecordsSibling(slug), "extra.txt", "extra\n", "extra records work")
		oneRepoPushRecords(t, h, slug)
		oldTip := oneRepoRecordsTip(t, bare, recordsBranch)
		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("setup Remove(%s): %v", slug, err)
		}

		res, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
		if err != nil {
			t.Fatalf("re-Add(%s): %v", slug, err)
		}

		if got := refLines(t, bare, "%(refname)", namespaceHeadsPrefix+recordsBranch); len(got) != 0 {
			t.Errorf("origin still holds %v after Add; want the archived leftover deleted", got)
		}
		if got := refLines(t, bare, "%(objectname)", namespaceTagsPrefix+fabricengine.ArchiveTagName(slug, oldTip)); !slices.Equal(got, []string{oldTip}) {
			t.Errorf("origin's archive tag of the leftover = %v; want one at %s", got, oldTip)
		}
		deleted := false
		for _, m := range res.Mutations.Entries() {
			deleted = deleted || (m.Kind == fabricengine.KindRemoteBranchDeleted && m.Target == recordsBranch)
		}
		if !deleted {
			t.Errorf("record %+v carries no %s entry for %q", res.Mutations.Entries(), fabricengine.KindRemoteBranchDeleted, recordsBranch)
		}
	})

	t.Run("ArchiveOfAnOriginOnlyBranchReadsOriginsNamespacedTip", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-origin-only"
		const branch = slug + "-weft"
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsRoot := mustRecordsRepoRoot(t, h.Location)
		mustCreateOrphanWeftBranch(t, recordsRoot, branch)
		oneRepoPushNamespaced(t, recordsRoot, branch)
		tip := gitkit.RevParse(t, recordsRoot, branch)
		gitkit.Git(t, recordsRoot, "branch", "-D", branch)

		tag, reason, err := fabricengine.ArchiveWeftTipForTest(fabricengine.NewMutations(h.Location.HubPath), h.Location, slug, branch)
		if err != nil {
			t.Fatalf("archive of an origin-only branch: %v", err)
		}
		wantTag := fabricengine.ArchiveTagName(slug, tip)
		if tag != wantTag || reason != "" {
			t.Fatalf("archive = (%q, %q); want (%q, empty reason)", tag, reason, wantTag)
		}
		if got := refLines(t, h.CodeBare, "%(objectname)", namespaceTagsPrefix+tag); !slices.Equal(got, []string{tip}) {
			t.Errorf("origin's archive tag = %v; want one at the branch's tip %s", got, tip)
		}
	})
}

// TestOneRepositoryOrigin_ReconcileAndCleanupSeeTheNamespace asserts the origin-side reads of a one-repository hub.
// They follow its records namespace and leave every ref of it in place.
func TestOneRepositoryOrigin_ReconcileAndCleanupSeeTheNamespace(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	t.Run("ReconcileFetchesAMovedRecordsBranchIntoItsTrackingRef", func(t *testing.T) {
		t.Parallel()

		boardkit.InstallReplay(t)
		const slug = "ns-reconcile"
		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, first, slug).Branch)
		second := hubforge.CloneSecond(t, first)
		if _, err := second.Topology.Add(second.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("adopt on the second machine: %v", err)
		}
		tip := gitkit.CommitFile(t, second.PairRecordsSibling(slug), "second.txt", "second\n", "second machine records")
		oneRepoPushRecords(t, second, slug)

		report, err := fabricengine.ReconcileOrigin(first.Location, fabricengine.ReconcileScope{}, fabricengine.OriginSeams{}, fabricengine.ReconcileOptions{})
		if err != nil {
			t.Fatalf("ReconcileOrigin: %v", err)
		}

		if got := gitkit.RevParse(t, first.PrimeRecords(), "refs/remotes/origin/"+recordsBranch); got != tip {
			t.Errorf("refs/remotes/origin/%s = %s; want the second machine's tip %s", recordsBranch, got, tip)
		}
		found := false
		for _, outcome := range report.Branches {
			if outcome.Branch == recordsBranch && outcome.Repo == "records" {
				found = true
				if outcome.OriginHead != tip {
					t.Errorf("the records outcome's OriginHead = %s; want %s", outcome.OriginHead, tip)
				}
			}
		}
		if !found {
			t.Errorf("the report has no records outcome for %s: %+v", recordsBranch, report.Branches)
		}
	})

	t.Run("CleanupRemoteWarpClassifiesByTheNamespacedEvidence", func(t *testing.T) {
		t.Parallel()

		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		bare := h.CodeBare
		recordsRoot := mustRecordsRepoRoot(t, h.Location)
		defaultBranch := gitkit.CurrentBranch(t, h.PrimeWorktree())
		landed := func(branch string) {
			gitkit.Git(t, h.PrimeWorktree(), "push", "origin", "HEAD:refs/heads/"+branch)
		}
		// A code branch is fabric-managed by a records branch of its own in the namespace.
		landed("by-branch")
		gitkit.Git(t, recordsRoot, "push", "origin", "HEAD:"+namespaceHeadsPrefix+fabricengine.RecordsBranchName("by-branch"))
		// It is also managed by an archive tag in the namespace.
		landed("by-tag")
		archiveTag := fabricengine.ArchiveTagName("by-tag", gitkit.RevParse(t, recordsRoot, "HEAD"))
		gitkit.Git(t, recordsRoot, "tag", archiveTag)
		gitkit.Git(t, recordsRoot, "push", "origin", "refs/tags/"+archiveTag+":"+namespaceTagsPrefix+archiveTag)
		// A code branch named like another's records branch is no evidence of that other's management.
		landed("decoy")
		landed("decoy-weft")
		landed("unmanaged")

		nsBefore := refLines(t, bare, "%(refname) %(objectname)", fabricengine.RecordsNamespaceRoot)
		dry, err := h.Topology.CleanupRemoteWarp(h.Location, false, map[string]bool{})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp dry run: %v", err)
		}
		entries := remoteWarpEntries(dry)
		for _, branch := range []string{"by-branch", "by-tag"} {
			if e := entries[branch]; !e.Candidate || e.Reason != "" {
				t.Errorf("%s = %+v; want a candidate, managed by its namespaced evidence", branch, e)
			}
		}
		for _, branch := range []string{"decoy", "unmanaged"} {
			if e := entries[branch]; e.Candidate || !strings.Contains(e.Reason, "not fabric-managed") {
				t.Errorf("%s = %+v; want a non-candidate that is not fabric-managed", branch, e)
			}
		}
		for branch := range entries {
			if branch == fabricengine.RecordsBranchName("by-branch") || strings.HasPrefix(branch, fabricengine.RecordsNamespaceRoot) {
				t.Errorf("the sweep reports %q as a code branch; want records branches left out", branch)
			}
		}

		applied, err := h.Topology.CleanupRemoteWarp(h.Location, true, map[string]bool{})
		if err != nil {
			t.Fatalf("CleanupRemoteWarp apply: %v", err)
		}
		for _, branch := range []string{"by-branch", "by-tag"} {
			if e := remoteWarpEntries(applied)[branch]; !e.Deleted {
				t.Errorf("%s = %+v; want the managed landed branch deleted", branch, e)
			}
		}
		if got := refLines(t, bare, "%(refname)", "refs/heads/"+defaultBranch); len(got) != 1 {
			t.Errorf("origin's code branch %s = %v after the sweep; want it kept", defaultBranch, got)
		}
		if got := refLines(t, bare, "%(refname) %(objectname)", fabricengine.RecordsNamespaceRoot); !slices.Equal(got, nsBefore) {
			t.Errorf("origin's records namespace = %v after the sweep; want it as before, %v", got, nsBefore)
		}
	})
}

// TestOneRepositorySecondMachine_AdoptPullAndReplay drives a second machine's side of the one-repository lifecycle.
// Each row runs over a first hub and its CloneSecond.
// The rows reach adoption, pull, the board and records replays, the hand-over, and the live local branch probe.
// They also reach the checkout's fetch, the records push and the owner archives.
func TestOneRepositorySecondMachine_AdoptPullAndReplay(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	// pairOnBoth adds the pair slug on a first hub, pushes it, and adopts it on a second machine.
	pairOnBoth := func(t *testing.T, slug string) (first, second *hubforge.Hub, recordsBranch string) {
		t.Helper()
		boardkit.InstallReplay(t)
		first = hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsBranch = fabricengine.RecordsBranchName(hubforge.AddPair(t, first, slug).Branch)
		second = hubforge.CloneSecond(t, first)
		if _, err := second.Topology.Add(second.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("adopt %s on the second machine: %v", slug, err)
		}
		return first, second, recordsBranch
	}

	t.Run("AddAdoptsTheRecordsBranchFromTheNamespace", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-adopt"
		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, first, slug).Branch)
		gitkit.CommitFile(t, first.PairRecordsSibling(slug), "first.txt", "first\n", "first machine records")
		oneRepoPushRecords(t, first, slug)
		second := hubforge.CloneSecond(t, first)

		if _, err := second.Topology.Add(second.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("adopt on the second machine: %v", err)
		}

		sibling := second.PairRecordsSibling(slug)
		if got, want := gitkit.RevParse(t, sibling, "HEAD"), oneRepoRecordsTip(t, first.CodeBare, recordsBranch); got != want {
			t.Errorf("adopted records branch = %s; want origin's tip %s", got, want)
		}
		if _, err := os.Stat(filepath.Join(sibling, "first.txt")); err != nil {
			t.Errorf("the adopted records worktree lacks the first machine's commit: %v", err)
		}
	})

	t.Run("PullBringsInTheFirstMachinesRecords", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-pull"
		first, second, recordsBranch := pairOnBoth(t, slug)
		tip := gitkit.CommitFile(t, first.PairRecordsSibling(slug), "more.txt", "more\n", "more records")
		oneRepoPushRecords(t, first, slug)

		if _, err := oneRepoPairFabric(t, second, slug).Pull(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("Pull on the second machine: %v", err)
		}

		sibling := second.PairRecordsSibling(slug)
		if got := gitkit.RevParse(t, sibling, "HEAD"); got != tip {
			t.Errorf("second records branch = %s after the pull; want %s", got, tip)
		}
		repo := gitrepo.New(sibling)
		if unpulled, err := repo.HasUnpulled(); err != nil || unpulled {
			t.Errorf("HasUnpulled() = %v, %v; want false after the pull", unpulled, err)
		}
		if unpushed, err := repo.HasUnpushed(); err != nil || unpushed {
			t.Errorf("HasUnpushed() = %v, %v; want false after the pull", unpushed, err)
		}
		if got := gitkit.Git(t, sibling, "config", "--get", "branch."+recordsBranch+".merge"); got != namespaceHeadsPrefix+recordsBranch {
			t.Errorf("branch.%s.merge = %q; want %q", recordsBranch, got, namespaceHeadsPrefix+recordsBranch)
		}
	})

	t.Run("RejectedBoardPushReplaysOntoOrigin", func(t *testing.T) {
		t.Parallel()

		boardkit.InstallReplay(t)
		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		second := hubforge.CloneSecond(t, first)
		for slug, hub := range map[string]*hubforge.Hub{"first-entry": first, "second-entry": second} {
			board := filepath.Join(hub.BoardDir(), boardengine.BoardFileRel())
			if err := os.WriteFile(board, []byte(boardFileOf(t, boardEntry(slug))), 0o644); err != nil {
				t.Fatalf("write the board of %s: %v", slug, err)
			}
			if _, _, err := fabricengine.NewBolt(hub.BoardDir()).Commit("board entry "+slug, fabricengine.SyncOptions{}); err != nil {
				t.Fatalf("commit the board of %s: %v", slug, err)
			}
		}

		if err := fabricengine.NewBolt(first.BoardDir()).Push(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("first board push: %v", err)
		}
		if err := fabricengine.NewBolt(second.BoardDir()).Push(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("second board push, which origin rejects and the replay lands: %v", err)
		}

		if got, want := oneRepoBoardSlugs(t, first.CodeBare, namespaceBoardRef), []string{"first-entry", "second-entry"}; !slices.Equal(got, want) {
			t.Errorf("origin's board entries = %v; want both machines' %v", got, want)
		}
	})

	t.Run("RejectedRecordsPushReplaysOntoOrigin", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-records-replay"
		first, second, recordsBranch := pairOnBoth(t, slug)
		gitkit.CommitFile(t, first.PairRecordsSibling(slug), "from-first.txt", "first\n", "first records")
		gitkit.CommitFile(t, second.PairRecordsSibling(slug), "from-second.txt", "second\n", "second records")

		oneRepoPushRecords(t, first, slug)
		secondPair := second.PairCodeWorktree(slug)
		if _, err := fabricengine.CoalescePushBothAt(secondPair, second.PairRecordsSibling(slug), fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("second records push, which origin rejects and the replay lands: %v", err)
		}

		files := strings.Split(gitkit.Git(t, first.CodeBare, "ls-tree", "-r", "--name-only", namespaceHeadsPrefix+recordsBranch), "\n")
		for _, want := range []string{"from-first.txt", "from-second.txt"} {
			if !slices.Contains(files, want) {
				t.Errorf("origin's %s holds %v; want it to hold %s", recordsBranch, files, want)
			}
		}
	})

	t.Run("TakeoverLandsTheEntryInTheNamespace", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-takeover"
		boardkit.InstallReplay(t)
		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, first, slug).Branch)
		_, statusRel, err := shedrun.TreeRunFileRels(slug)
		if err != nil {
			t.Fatalf("TreeRunFileRels(%s): %v", slug, err)
		}
		gitkit.CommitFile(t, first.PairRecordsSibling(slug), statusRel, statusJSON("Implementer"), "pair status")
		oneRepoPushRecords(t, first, slug)
		gitkit.CommitFile(t, first.PrimeRecords(), statusRel, statusJSON("Implementer"), "prime status")
		if _, err := hubforge.OpenFabric(t, first).PushWeft(fabricengine.SyncOptions{}); err != nil {
			t.Fatalf("push the prime's records: %v", err)
		}
		second := hubforge.CloneSecond(t, first)
		if _, err := second.Topology.Add(second.Location, slug, fabricengine.AddOptions{}); err != nil {
			t.Fatalf("adopt on the second machine: %v", err)
		}
		pair, err := lymcwd.ResolveWorktree(second.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}
		refsBefore := refLines(t, first.CodeBare, "%(refname) %(objectname)", "refs/heads/")

		_, err = fabricengine.TakeOverRun(second.Location, fabricengine.TakeOverRequest{
			Slug: slug, Subject: "beam", Machine: "oslo", Pair: pair, At: time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("TakeOverRun: %v", err)
		}

		wantSubject := "takeover of " + slug + " by oslo"
		if got := gitkit.Git(t, first.CodeBare, "log", "-1", "--format=%s", namespaceHeadsPrefix+recordsBranch); got != wantSubject {
			t.Errorf("origin's %s tip subject = %q; want %q", recordsBranch, got, wantSubject)
		}
		if got := refLines(t, first.CodeBare, "%(refname) %(objectname)", "refs/heads/"); !slices.Equal(got, refsBefore) {
			t.Errorf("origin's code branches = %v after the takeover; want them unchanged, %v", got, refsBefore)
		}
	})

	t.Run("AddReachesALiveLocalRecordsBranchBehindOrigin", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-live-local"
		first, second, recordsBranch := pairOnBoth(t, slug)
		removedTip := gitkit.RevParse(t, second.PairRecordsSibling(slug), "HEAD")
		if _, err := second.Topology.Remove(second.Location, slug, false, false); err != nil {
			t.Fatalf("Remove on the second machine: %v", err)
		}
		gitkit.Git(t, second.PrimeRecords(), "branch", recordsBranch, removedTip)
		gitkit.CommitFile(t, first.PairRecordsSibling(slug), "later.txt", "later\n", "later records")
		oneRepoPushRecords(t, first, slug)
		originTip := oneRepoRecordsTip(t, first.CodeBare, recordsBranch)

		res, err := second.Topology.Add(second.Location, slug, fabricengine.AddOptions{})
		if err != nil {
			t.Fatalf("Add on the second machine: %v", err)
		}

		advanced := 0
		for _, m := range res.Mutations.Entries() {
			if m.Kind == fabricengine.KindRepoAdvanced && strings.Contains(m.Detail, originTip) {
				advanced++
			}
		}
		if advanced != 1 {
			t.Errorf("repo_advanced entries naming origin's tip = %d; want 1", advanced)
		}
		if _, err := os.Stat(filepath.Join(second.PairRecordsSibling(slug), "later.txt")); err != nil {
			t.Errorf("the adopted records worktree lacks the first machine's later commit: %v", err)
		}
	})

	t.Run("CheckoutFetchesTheRecordsBranchFromTheNamespace", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-checkout"
		first := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		second := hubforge.CloneSecond(t, first)
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, second, slug).Branch)
		tip := gitkit.CommitFile(t, second.PairRecordsSibling(slug), "second.txt", "second\n", "second machine records")
		oneRepoPushRecords(t, second, slug)
		gitkit.Git(t, first.PrimeWorktree(), "fetch", "origin")
		if gitkit.BranchExists(t, first.PrimeRecords(), recordsBranch) {
			t.Fatalf("the first hub holds %s locally; want it nowhere", recordsBranch)
		}

		if _, err := first.Topology.Checkout(first.Location, slug); err != nil {
			t.Fatalf("Checkout(%s): %v", slug, err)
		}

		if got := gitkit.CurrentBranch(t, first.PrimeRecords()); got != recordsBranch {
			t.Errorf("the records worktree is on %q; want %q", got, recordsBranch)
		}
		if got := gitkit.RevParse(t, first.PrimeRecords(), "refs/remotes/origin/"+recordsBranch); got != tip {
			t.Errorf("refs/remotes/origin/%s = %s; want the second machine's tip %s", recordsBranch, got, tip)
		}
		if got := gitkit.RevParse(t, first.PrimeRecords(), "HEAD"); got != tip {
			t.Errorf("the records worktree's HEAD = %s; want the second machine's tip %s", got, tip)
		}
	})

	t.Run("PushWeftLandsInTheNamespaceAndAddsNoCodeRef", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-push-records"
		h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
		recordsBranch := fabricengine.RecordsBranchName(hubforge.AddPair(t, h, slug).Branch)
		tip := gitkit.CommitFile(t, h.PairRecordsSibling(slug), "unpushed.txt", "unpushed\n", "unpushed records")
		codeRefsBefore := refLines(t, h.CodeBare, "%(refname) %(objectname)", "refs/heads/")

		oneRepoPushRecords(t, h, slug)

		if got := oneRepoRecordsTip(t, h.CodeBare, recordsBranch); got != tip {
			t.Errorf("%s%s = %s; want the pushed commit %s", namespaceHeadsPrefix, recordsBranch, got, tip)
		}
		if got := refLines(t, h.CodeBare, "%(refname) %(objectname)", "refs/heads/"); !slices.Equal(got, codeRefsBefore) {
			t.Errorf("origin's code branches = %v after the push; want them unchanged, %v", got, codeRefsBefore)
		}
	})

	t.Run("OwnerArchivesAreTaggedInTheNamespaceAndReadBackBySecondMachine", func(t *testing.T) {
		t.Parallel()

		const slug = "ns-owner-archive"
		first, second, _ := pairOnBoth(t, slug)
		tip := gitkit.CommitFile(t, first.PairRecordsSibling(slug), "owner.txt", "owner\n", "unpushed records")
		pair, err := lymcwd.ResolveWorktree(first.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree: %v", err)
		}

		tags, err := fabricengine.ArchiveOwnerTips(first.Path, []fabricengine.OwnerArchivePair{{Slug: slug, Pair: pair}})
		if err != nil {
			t.Fatalf("ArchiveOwnerTips: %v", err)
		}

		wantTag := fabricengine.OwnerArchiveTagName(slug, tip)
		if !slices.Contains(tags, wantTag) {
			t.Fatalf("ArchiveOwnerTips() = %v; want the records tip tagged %s", tags, wantTag)
		}
		if got := refLines(t, first.CodeBare, "%(objectname)", namespaceTagsPrefix+wantTag); !slices.Equal(got, []string{tip}) {
			t.Errorf("origin's owner archive tag = %v; want one at %s", got, tip)
		}

		archives, err := second.Topology.ReadOwnerArchives(second.Location, "root", func(archived string) bool { return archived == slug })
		if err != nil {
			t.Fatalf("ReadOwnerArchives: %v", err)
		}
		var records []fabricengine.OwnerArchive
		for _, archive := range archives {
			if archive.Side == fabricengine.OwnerArchiveSideRecords {
				records = append(records, archive)
			}
		}
		if len(records) != 1 || records[0].Tag != wantTag || records[0].Commit != tip {
			t.Fatalf("records-side owner archives = %+v; want one, %s at %s", records, wantTag, tip)
		}
		if got := gitkit.Git(t, records[0].Dir, "tag", "--list", wantTag); got != wantTag {
			t.Errorf("local tags of %s matching %s = %q; want the fetched tag", records[0].Dir, wantTag, got)
		}
	})
}

// TestOneRepositoryMerge_LeavesNoRecordsOnTheCodeBranch asserts what a merge into the prime's code branch leaves behind.
// The branch's tree holds no records path, and the merge commit holds no merge trailer.
func TestOneRepositoryMerge_LeavesNoRecordsOnTheCodeBranch(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	const slug = "ns-merge"
	h := hubforge.NewOneRepoHub(t, hubforge.OneRepoOptions{})
	added := hubforge.AddPair(t, h, slug)
	gitkit.CommitFile(t, h.PairCodeWorktree(slug), "pair-work.txt", "pair\n", "the pair's work")
	gitkit.CommitFile(t, h.PrimeWorktree(), "prime-work.txt", "prime\n", "the prime's own work")

	res, err := hubforge.OpenFabric(t, h).MergeIn(added.Branch)
	if err != nil {
		t.Fatalf("MergeIn(%s): %v", added.Branch, err)
	}
	if len(res.Conflicts) != 0 || res.AlreadyUpToDate {
		t.Fatalf("MergeIn(%s) = %+v; want a clean merge", added.Branch, res)
	}

	prime := h.PrimeWorktree()
	if parents := strings.Fields(gitkit.Git(t, prime, "rev-list", "--parents", "-n", "1", "HEAD")); len(parents) != 3 {
		t.Fatalf("HEAD has parents %v; want a merge commit", parents[1:])
	}
	for _, path := range strings.Split(gitkit.Git(t, prime, "ls-tree", "-r", "--name-only", "HEAD"), "\n") {
		if path == "_lym" || strings.HasPrefix(path, "_lym/") {
			t.Errorf("the merged tree holds %s; want no records path on the code branch", path)
		}
	}
	if message := gitkit.Git(t, prime, "log", "-1", "--format=%B", "HEAD"); strings.Contains(message, "Fabric-Merge") {
		t.Errorf("the merge commit message = %q; want no Fabric-Merge trailer", message)
	}
}
