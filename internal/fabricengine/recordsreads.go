// recordsreads.go holds the read-only reads that restore a pair's records after the pair is gone:
// the slug's archive and set-aside tags with their files, one file of a records branch, and a branch's upstream.
// None of them writes a branch, a tag or a remote-tracking ref; a commit only origin holds is fetched by its SHA alone.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// configKeyNotSetExit is the exit code `git config --get` answers for a key that is not set.
const configKeyNotSetExit = 1

// RecordsTag is one tag of a slug's records history:
// an archive tag a teardown pushed, or a set-aside tag a local removal kept.
type RecordsTag struct {
	// Name is the tag's short name.
	Name string
	// Commit is the full hex object name of the commit the tag names, an annotated tag peeled.
	Commit string
	// OnOrigin is true when origin holds the tag.
	OnOrigin bool
	// OriginOnly is true when only origin holds the tag.
	OriginOnly bool
	// CommitterTime is the committer time of Commit.
	CommitterTime time.Time
	// Files holds the content of each requested path found at Commit, keyed by the requested path;
	// a path absent there is left out.
	Files map[string][]byte
}

// SlugRecordsTags lists the tags that keep slug's records:
// its archive tags on origin, and the records repository's local archive and set-aside tags.
// Each tag carries the content of every path in anchoredRels that its commit holds, under that commit's own anchor.
// A commit only origin holds is fetched by SHA into the records repository before it is read;
// it writes no branch, tag or remote-tracking ref.
// A records repository with no origin lists local tags only; a failed ls-remote or fetch is returned.
// The tags are ordered by committer time, then by name.
func SlugRecordsTags(l *lymcwd.Location, slug string, anchoredRels []string) ([]RecordsTag, error) {
	recordsRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return nil, fmt.Errorf("resolve the records repository: %w", err)
	}
	repo := gitrepo.New(recordsRoot)

	var tags []RecordsTag
	local := map[string]bool{}
	for _, prefix := range []string{ArchiveTagPrefix + slug + "/", setAsideTagPrefix + slug + "/"} {
		refs, err := repo.TagRefs(prefix)
		if err != nil {
			return nil, fmt.Errorf("list the local tags %q: %w", prefix+"*", err)
		}
		for _, ref := range refs {
			local[ref.Name] = true
			tags = append(tags, RecordsTag{Name: ref.Name, Commit: ref.Commit, CommitterTime: ref.Committed})
		}
	}

	var origin map[string]string
	if _, urlErr := repo.RemoteURL(originRemoteName); urlErr == nil {
		if origin, err = remoteArchiveTags(recordsRoot, slug); err != nil {
			return nil, fmt.Errorf("read the archive tags of %q from origin with git ls-remote: %w", slug, err)
		}
	}
	for i := range tags {
		_, tags[i].OnOrigin = origin[tags[i].Name]
	}
	originNames := make([]string, 0, len(origin))
	for name := range origin {
		if !local[name] {
			originNames = append(originNames, name)
		}
	}
	sort.Strings(originNames)
	for _, name := range originNames {
		commit := origin[name]
		if !repo.SHAExists(commit) {
			if _, err := repo.FetchWithArgs(originRemoteName, commit); err != nil {
				return nil, fmt.Errorf("fetch the commit %s of the archive tag %q from origin: %w", commit, name, err)
			}
		}
		commits, err := repo.Log(commit, gitrepo.LogQuery{Limit: 1})
		if err != nil || len(commits) == 0 {
			return nil, fmt.Errorf("read the commit %s of the archive tag %q: %v", commit, name, err)
		}
		tags = append(tags, RecordsTag{Name: name, Commit: commit, OnOrigin: true, OriginOnly: true, CommitterTime: commits[0].Committed})
	}
	sort.SliceStable(tags, func(i, j int) bool {
		if !tags[i].CommitterTime.Equal(tags[j].CommitterTime) {
			return tags[i].CommitterTime.Before(tags[j].CommitterTime)
		}
		return tags[i].Name < tags[j].Name
	})

	if len(anchoredRels) == 0 {
		return tags, nil
	}
	names, err := RepoWiredNames(l)
	if err != nil {
		return nil, fmt.Errorf("load the fabric config: %w", err)
	}
	for i := range tags {
		anchor, err := treeAnchor(recordsRoot, tags[i].Commit, names, l.AnchorRel)
		if err != nil {
			return nil, fmt.Errorf("read the anchor of the tag %q: %w", tags[i].Name, err)
		}
		if tags[i].Files, err = recordsFilesAt(recordsRoot, tags[i].Commit, anchor, anchoredRels); err != nil {
			return nil, fmt.Errorf("read the files of the tag %q: %w", tags[i].Name, err)
		}
	}
	return tags, nil
}

// RecordsBranchFile reads the file anchoredRel of the records branch.
// It resolves the path under the anchor the branch itself records.
// It reads the local branch when it exists, else origin's copy;
// that copy is fetched by SHA into the records repository, writing no branch, tag or remote-tracking ref.
// found is false when the branch, or the file on it, is absent.
func RecordsBranchFile(l *lymcwd.Location, branch, anchoredRel string) (content []byte, found bool, err error) {
	recordsRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return nil, false, fmt.Errorf("resolve the records repository: %w", err)
	}
	repo := gitrepo.New(recordsRoot)

	rev := "refs/heads/" + branch
	exists, err := repo.BranchExists(branch)
	if err != nil {
		return nil, false, fmt.Errorf("look up the records branch %q: %w", branch, err)
	}
	if !exists {
		if _, urlErr := repo.RemoteURL(originRemoteName); urlErr != nil {
			return nil, false, nil
		}
		if rev, err = remoteHeadTip(recordsRoot, branch); err != nil || rev == "" {
			return nil, false, err
		}
		if !repo.SHAExists(rev) {
			if _, err := repo.FetchWithArgs(originRemoteName, rev); err != nil {
				return nil, false, fmt.Errorf("fetch the tip %s of the records branch %q from origin: %w", rev, branch, err)
			}
		}
	}

	names, err := RepoWiredNames(l)
	if err != nil {
		return nil, false, fmt.Errorf("load the fabric config: %w", err)
	}
	anchor, err := treeAnchor(recordsRoot, rev, names, l.AnchorRel)
	if err != nil {
		return nil, false, fmt.Errorf("read the anchor of the records branch %q: %w", branch, err)
	}
	files, err := recordsFilesAt(recordsRoot, rev, anchor, []string{anchoredRel})
	if err != nil {
		return nil, false, fmt.Errorf("read the records branch %q: %w", branch, err)
	}
	content, found = files[anchoredRel]
	return content, found, nil
}

// RecordsBranchHasUpstream reports whether the records repository configures branch.<branch>.remote or branch.<branch>.merge.
func RecordsBranchHasUpstream(l *lymcwd.Location, branch string) (bool, error) {
	recordsRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false, fmt.Errorf("resolve the records repository: %w", err)
	}
	for _, key := range []string{"branch." + branch + ".remote", "branch." + branch + ".merge"} {
		_, err := gitexec.Run([]string{"config", "--get", key}, recordsRoot)
		if err == nil {
			return true, nil
		}
		var gitErr *gitexec.GitError
		if !errors.As(err, &gitErr) || gitErr.ExitCode != configKeyNotSetExit {
			return false, fmt.Errorf("read %s in %s: %w", key, recordsRoot, err)
		}
	}
	return false, nil
}

// recordsFilesAt reads each of rels under anchor at rev in the repository at repoDir.
// It keys each file by the rel it was asked for and leaves out a path absent at rev.
func recordsFilesAt(repoDir, rev, anchor string, rels []string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(rels))
	for _, rel := range rels {
		data, err := showFile(repoDir, rev, filepath.ToSlash(filepath.Join(anchor, rel)))
		if err != nil {
			return nil, fmt.Errorf("read %s at %s: %w", rel, rev, err)
		}
		if data != nil {
			files[rel] = data
		}
	}
	return files, nil
}
