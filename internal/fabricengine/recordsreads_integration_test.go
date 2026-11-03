// recordsreads_integration_test.go covers the read-only reads of the records repository: a slug's archive and set-aside tags with their files, one file of a records branch, and a records branch's upstream.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordsRootOf returns the records repository root of h.
func recordsRootOf(t *testing.T, h *hubforge.Hub) string {
	t.Helper()

	root, err := fabricengine.RecordsRepoRoot(h.Location)
	if err != nil {
		t.Fatalf("RecordsRepoRoot: %v", err)
	}
	return root
}

// TestSlugRecordsTags covers the records tags of a removed pair.
// An archive tag on origin lists with the origin record it holds.
// The same tag deleted locally still lists from origin alone and writes no tag or branch.
// A local removal's set-aside tag lists as not on origin.
// A requested path the commit lacks stays out of the files.
// An origin that cannot be listed is an error naming the ls-remote.
// RecordsBranchFile reads a local branch, an origin-only branch and an absent branch.
// RecordsBranchHasUpstream follows the branch's upstream setting.
func TestSlugRecordsTags(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	originRecord := fabricengine.OriginRecordRel()

	t.Run("an archive tag on origin lists with its origin record", func(t *testing.T) {
		t.Parallel()

		const slug = "records-tag"
		h := removedPair(t, slug)
		root := recordsRootOf(t, h)

		tags, err := fabricengine.SlugRecordsTags(h.Location, slug, []string{originRecord})
		if err != nil {
			t.Fatalf("SlugRecordsTags: %v", err)
		}

		if len(tags) != 1 || !strings.HasPrefix(tags[0].Name, fabricengine.ArchiveTagPrefix+slug+"/") {
			t.Fatalf("SlugRecordsTags = %+v; want the one archive tag of %q", tags, slug)
		}
		if !tags[0].OnOrigin || tags[0].OriginOnly {
			t.Errorf("tag = %+v; want it on origin and held locally too", tags[0])
		}
		want := gitkit.Git(t, root, "show", tags[0].Name+":"+originRecord)
		if got := strings.TrimSpace(string(tags[0].Files[originRecord])); got != strings.TrimSpace(want) || got == "" {
			t.Errorf("origin record at the tag = %q; want %q", got, want)
		}
	})

	t.Run("a tag deleted locally still lists from origin and writes nothing", func(t *testing.T) {
		t.Parallel()

		const slug = "records-origin-only"
		h := removedPair(t, slug)
		root := recordsRootOf(t, h)
		tags, err := fabricengine.SlugRecordsTags(h.Location, slug, nil)
		if err != nil || len(tags) != 1 {
			t.Fatalf("SlugRecordsTags = %+v, %v; want the one archive tag", tags, err)
		}
		mustGit(root, "tag", "-d", tags[0].Name)
		tagsBefore := gitkit.Git(t, root, "tag", "--list")
		branchesBefore := gitkit.Git(t, root, "branch", "--list", "--all")

		got, err := fabricengine.SlugRecordsTags(h.Location, slug, []string{originRecord})
		if err != nil {
			t.Fatalf("SlugRecordsTags: %v", err)
		}

		if len(got) != 1 || got[0].Name != tags[0].Name || !got[0].OnOrigin || !got[0].OriginOnly || got[0].Commit != tags[0].Commit {
			t.Fatalf("SlugRecordsTags = %+v; want the tag %q on origin only at %s", got, tags[0].Name, tags[0].Commit)
		}
		if len(got[0].Files[originRecord]) == 0 {
			t.Errorf("Files[%q] is empty; want the origin record read from the fetched commit", originRecord)
		}
		if after := gitkit.Git(t, root, "tag", "--list"); after != tagsBefore {
			t.Errorf("local tags = %q after the read; want %q", after, tagsBefore)
		}
		if after := gitkit.Git(t, root, "branch", "--list", "--all"); after != branchesBefore {
			t.Errorf("branches = %q after the read; want %q", after, branchesBefore)
		}
	})

	t.Run("a local removal's set-aside tag lists as not on origin", func(t *testing.T) {
		t.Parallel()

		const slug = "records-set-aside"
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		if _, err := h.Topology.RemoveLocal(h.Location, slug, false); err != nil {
			t.Fatalf("RemoveLocal: %v", err)
		}

		tags, err := fabricengine.SlugRecordsTags(h.Location, slug, []string{originRecord, "no/such/file"})
		if err != nil {
			t.Fatalf("SlugRecordsTags: %v", err)
		}

		if len(tags) != 1 || !strings.HasPrefix(tags[0].Name, "set-aside/"+slug+"/") {
			t.Fatalf("SlugRecordsTags = %+v; want the one set-aside tag of %q", tags, slug)
		}
		if tags[0].OnOrigin || tags[0].OriginOnly {
			t.Errorf("tag = %+v; want it off origin", tags[0])
		}
		if len(tags[0].Files[originRecord]) == 0 {
			t.Errorf("Files[%q] is empty; want the origin record", originRecord)
		}
		if _, present := tags[0].Files["no/such/file"]; present {
			t.Errorf("Files holds a path the commit lacks: %v", tags[0].Files)
		}
	})

	t.Run("an origin that cannot be listed is an error naming the ls-remote", func(t *testing.T) {
		t.Parallel()

		const slug = "records-no-origin"
		h := removedPair(t, slug)
		mustGit(recordsRootOf(t, h), "remote", "set-url", "origin", t.TempDir()+"/missing")

		_, err := fabricengine.SlugRecordsTags(h.Location, slug, nil)

		if err == nil || !strings.Contains(err.Error(), "ls-remote") {
			t.Errorf("SlugRecordsTags error = %v; want one naming the ls-remote", err)
		}
	})

	t.Run("a records branch file reads a local branch, an origin-only branch and an absent one", func(t *testing.T) {
		t.Parallel()

		const slug = "records-branch"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		root := recordsRootOf(t, h)
		want := strings.TrimSpace(gitkit.Git(t, root, "show", "refs/heads/"+weftBranch+":"+originRecord))

		content, found, err := fabricengine.RecordsBranchFile(h.Location, weftBranch, originRecord)
		if err != nil || !found || strings.TrimSpace(string(content)) != want || want == "" {
			t.Errorf("RecordsBranchFile(local) = %q, %v, %v; want %q", content, found, err, want)
		}

		if _, err := h.Topology.Remove(h.Location, slug, false, false); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if gitkit.BranchExists(t, root, weftBranch) {
			mustGit(root, "branch", "-D", weftBranch)
		}
		content, found, err = fabricengine.RecordsBranchFile(h.Location, weftBranch, originRecord)
		if err != nil || !found || strings.TrimSpace(string(content)) == "" {
			t.Errorf("RecordsBranchFile(origin only) = %q, %v, %v; want the origin record", content, found, err)
		}

		if content, found, err = fabricengine.RecordsBranchFile(h.Location, "no-such-records-branch", originRecord); err != nil || found || content != nil {
			t.Errorf("RecordsBranchFile(absent) = %q, %v, %v; want nothing found", content, found, err)
		}
	})

	t.Run("a records branch has an upstream only while its setting stands", func(t *testing.T) {
		t.Parallel()

		const slug = "records-upstream"
		weftBranch := fabricengine.RecordsBranchName(slug)
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		hubforge.AddPair(t, h, slug)
		root := recordsRootOf(t, h)
		mustGit(root, "branch", "--set-upstream-to=origin/"+weftBranch, weftBranch)

		if got, err := fabricengine.RecordsBranchHasUpstream(h.Location, weftBranch); err != nil || !got {
			t.Errorf("RecordsBranchHasUpstream(set) = %v, %v; want true", got, err)
		}
		if _, err := gitexec.Run([]string{"branch", "--unset-upstream", weftBranch}, root); err != nil {
			t.Fatalf("unset the upstream: %v", err)
		}
		if got, err := fabricengine.RecordsBranchHasUpstream(h.Location, weftBranch); err != nil || got {
			t.Errorf("RecordsBranchHasUpstream(unset) = %v, %v; want false", got, err)
		}
	})
}
