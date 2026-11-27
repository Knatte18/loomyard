// recordspaths.go reads which paths of a commit are records, and whether a commit is records-only or a fabric merge.
// A one-repository hub keeps its records on the code branch under `<anchor>/_lym` beside the `.lym-anchor` marker.
// Every read here goes through git objects and writes no ref and no worktree file.

package fabricengine

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// MergeTrailerKey is the trailer every fabric merge commit carries.
const MergeTrailerKey = "Fabric-Merge"

// AnchorChangeTrailerKey is the trailer the anchor-change commit of a one-repository hub carries.
const AnchorChangeTrailerKey = "Anchor-Change"

// stripIdentity names the throwaway commits IsFabricMergeCommit builds from stripped trees; none is ever referenced.
const stripIdentity = "fabric"

// RecordsPathsAt returns the records paths of the commit at rev in the repository at repoDir.
// The result holds the `.lym-anchor` marker file itself.
// It also holds the `<anchor>/_lym` directory of every anchor that marker names, read at rev and at its first parent.
// An absent or blank marker contributes no anchor, and a root commit has no parent.
// The list is sorted, deduplicated and forward-slashed.
// The records paths of a merge side are RecordsPathsAt of the side's tip.
func RecordsPathsAt(repoDir, rev string) ([]string, error) {
	parents, err := parentsOf(repoDir, rev)
	if err != nil {
		return nil, err
	}
	revs := []string{rev}
	if len(parents) > 0 {
		revs = append(revs, parents[0])
	}

	set := map[string]bool{lymcwd.AnchorFileName: true}
	for _, r := range revs {
		marker, found, err := committedMarker(repoDir, r)
		if err != nil {
			return nil, err
		}
		if !found || marker == "" {
			continue
		}
		anchor, err := lymcwd.ValidateAnchorRel(marker)
		if err != nil {
			return nil, fmt.Errorf("the committed %s at %s in %s is unusable: %w", lymcwd.AnchorFileName, r, repoDir, err)
		}
		set[filepath.ToSlash(ScopedPathspec(anchor, []string{lymdirs.LymDirName})[0])] = true
	}
	return slices.Sorted(maps.Keys(set)), nil
}

// IsRecordsOnlyCommit reports whether the commit sha in the repository at repoDir changes records paths alone.
// The commit has one parent or none.
// Each path it changes against that parent, or the empty tree, is a records path of the commit or lies under one.
// The message is never read, so a commit changing a code path is never records-only whatever trailer it carries.
func IsRecordsOnlyCommit(repoDir, sha string) (bool, error) {
	parents, err := parentsOf(repoDir, sha)
	if err != nil {
		return false, err
	}
	if len(parents) > 1 {
		return false, nil
	}
	recordsPaths, err := RecordsPathsAt(repoDir, sha)
	if err != nil {
		return false, err
	}

	changed, err := gitexec.Run([]string{"diff-tree", "-r", "-z", "--name-only", "--no-renames", "--no-commit-id", "--root", sha}, repoDir)
	if err != nil {
		return false, fmt.Errorf("fabricengine: list the paths commit %s changes in %s: %w", sha, repoDir, err)
	}
	for _, path := range strings.Split(changed, "\x00") {
		if path != "" && !underRecordsPath(path, recordsPaths) {
			return false, nil
		}
	}
	return true, nil
}

// IsFabricMergeCommit reports whether the commit sha in the repository at repoDir is a merge fabric made.
// It has two parents and a MergeTrailerKey trailer.
// Its tree is the first parent's records paths laid over the clean merge of the two parents' trees.
// That merge is taken with every records path removed from both sides.
// A merge carrying the second parent's records, or a hand-resolved code conflict, therefore reads false.
// A merge whose stripped sides conflict reads false too.
func IsFabricMergeCommit(repoDir, sha string) (bool, error) {
	parents, err := parentsOf(repoDir, sha)
	if err != nil {
		return false, err
	}
	if len(parents) != 2 {
		return false, nil
	}
	message, err := gitexec.Run([]string{"show", "-s", "--format=%B", sha}, repoDir)
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the message of %s in %s: %w", sha, repoDir, err)
	}
	if trailerValue(message, MergeTrailerKey) == "" {
		return false, nil
	}

	firstPaths, err := RecordsPathsAt(repoDir, parents[0])
	if err != nil {
		return false, err
	}
	secondPaths, err := RecordsPathsAt(repoDir, parents[1])
	if err != nil {
		return false, err
	}
	union := slices.Compact(slices.Sorted(slices.Values(slices.Concat(firstPaths, secondPaths))))

	base, err := gitexec.Run([]string{"merge-base", parents[0], parents[1]}, repoDir)
	if err != nil {
		if exitedWithStatusOne(err) {
			return false, nil
		}
		return false, fmt.Errorf("fabricengine: find the merge base of %s in %s: %w", sha, repoDir, err)
	}

	scratch, err := lymtemp.MkdirTemp("fabric-merge-check-*")
	if err != nil {
		return false, fmt.Errorf("fabricengine: create the temporary index for %s: %w", sha, err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	stripped := make([]string, 0, 3)
	for i, rev := range []string{strings.TrimSpace(base), parents[0], parents[1]} {
		commit, err := strippedCommit(repoDir, rev, filepath.Join(scratch, fmt.Sprintf("index-%d", i)), union)
		if err != nil {
			return false, err
		}
		stripped = append(stripped, commit)
	}

	merged, err := gitexec.Run([]string{"merge-tree", "--write-tree", "--merge-base", stripped[0], stripped[1], stripped[2]}, repoDir)
	if err != nil {
		if exitedWithStatusOne(err) {
			return false, nil
		}
		return false, fmt.Errorf("fabricengine: merge the stripped parents of %s in %s: %w", sha, repoDir, err)
	}
	mergedTree, _, _ := strings.Cut(strings.TrimSpace(merged), "\n")

	wantTree, err := layRecordsOver(repoDir, mergedTree, parents[0], filepath.Join(scratch, "index-final"), union)
	if err != nil {
		return false, err
	}
	gotTree, err := gitexec.Run([]string{"rev-parse", sha + "^{tree}"}, repoDir)
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the tree of %s in %s: %w", sha, repoDir, err)
	}
	return wantTree == strings.TrimSpace(gotTree), nil
}

// parentsOf returns the parent commits of rev in the repository at repoDir, empty for a root commit.
func parentsOf(repoDir, rev string) ([]string, error) {
	out, err := gitexec.Run([]string{"rev-list", "--parents", "-n", "1", rev}, repoDir)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read the parents of %s in %s: %w", rev, repoDir, err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return nil, nil
	}
	return fields[1:], nil
}

// underRecordsPath reports whether path equals one of recordsPaths or lies below one of them.
func underRecordsPath(path string, recordsPaths []string) bool {
	for _, recordsPath := range recordsPaths {
		if path == recordsPath || strings.HasPrefix(path, recordsPath+"/") {
			return true
		}
	}
	return false
}

// exitedWithStatusOne reports whether err is git exiting with status 1.
// Both merge-base and merge-tree use that status for a clean "no".
func exitedWithStatusOne(err error) bool {
	var gitErr *gitexec.GitError
	return errors.As(err, &gitErr) && gitErr.ExitCode == 1
}

// indexEnv returns the environment that points git at the throwaway index file with literal pathspecs.
func indexEnv(indexFile string) []string {
	return []string{"GIT_INDEX_FILE=" + indexFile, "GIT_LITERAL_PATHSPECS=1"}
}

// strippedCommit writes a parentless commit whose tree is the tree of rev with every path of recordsPaths removed.
// The tree is built in the throwaway index at indexFile, so no ref, real index or worktree file changes.
// merge-tree needs a commit for its merge base, which is why the stripped tree is wrapped in one.
func strippedCommit(repoDir, rev, indexFile string, recordsPaths []string) (string, error) {
	env := indexEnv(indexFile)
	if _, err := gitexec.RunWithEnv([]string{"read-tree", rev}, repoDir, env); err != nil {
		return "", fmt.Errorf("fabricengine: read the tree of %s in %s: %w", rev, repoDir, err)
	}
	removal := slices.Concat([]string{"rm", "--cached", "-r", "-f", "-q", "--ignore-unmatch", "--"}, recordsPaths)
	if _, err := gitexec.RunWithEnv(removal, repoDir, env); err != nil {
		return "", fmt.Errorf("fabricengine: remove the records paths from the tree of %s in %s: %w", rev, repoDir, err)
	}
	tree, err := gitexec.RunWithEnv([]string{"write-tree"}, repoDir, env)
	if err != nil {
		return "", fmt.Errorf("fabricengine: write the stripped tree of %s in %s: %w", rev, repoDir, err)
	}

	identity := []string{
		"GIT_AUTHOR_NAME=" + stripIdentity, "GIT_AUTHOR_EMAIL=" + stripIdentity + "@localhost", "GIT_AUTHOR_DATE=@0 +0000",
		"GIT_COMMITTER_NAME=" + stripIdentity, "GIT_COMMITTER_EMAIL=" + stripIdentity + "@localhost", "GIT_COMMITTER_DATE=@0 +0000",
	}
	commit, err := gitexec.RunWithEnv([]string{"-c", "commit.gpgsign=false", "commit-tree", strings.TrimSpace(tree), "-m", "stripped"}, repoDir, identity)
	if err != nil {
		return "", fmt.Errorf("fabricengine: commit the stripped tree of %s in %s: %w", rev, repoDir, err)
	}
	return strings.TrimSpace(commit), nil
}

// layRecordsOver returns the id of the tree that tree becomes once source's entries of recordsPaths are added.
// A path the source does not hold adds nothing.
// The tree is built in the throwaway index at indexFile.
func layRecordsOver(repoDir, tree, source, indexFile string, recordsPaths []string) (string, error) {
	env := indexEnv(indexFile)
	if _, err := gitexec.RunWithEnv([]string{"read-tree", tree}, repoDir, env); err != nil {
		return "", fmt.Errorf("fabricengine: read the merged tree %s in %s: %w", tree, repoDir, err)
	}
	for _, recordsPath := range recordsPaths {
		listed, err := gitexec.RunWithEnv([]string{"ls-tree", source, "--", recordsPath}, repoDir, env)
		if err != nil {
			return "", fmt.Errorf("fabricengine: list %s of %s in %s: %w", recordsPath, source, repoDir, err)
		}
		entry, _, _ := strings.Cut(strings.TrimSpace(listed), "\t")
		fields := strings.Fields(entry)
		if len(fields) != 3 {
			continue
		}
		mode, objectType, object := fields[0], fields[1], fields[2]
		args := []string{"update-index", "--add", "--cacheinfo", mode + "," + object + "," + recordsPath}
		if objectType == "tree" {
			args = []string{"read-tree", "--prefix=" + recordsPath + "/", object}
		}
		if _, err := gitexec.RunWithEnv(args, repoDir, env); err != nil {
			return "", fmt.Errorf("fabricengine: lay %s of %s over the merged tree in %s: %w", recordsPath, source, repoDir, err)
		}
	}
	out, err := gitexec.RunWithEnv([]string{"write-tree"}, repoDir, env)
	if err != nil {
		return "", fmt.Errorf("fabricengine: write the expected tree of %s in %s: %w", source, repoDir, err)
	}
	return strings.TrimSpace(out), nil
}
