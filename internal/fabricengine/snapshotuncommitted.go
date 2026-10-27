// snapshotuncommitted.go implements SnapshotUncommitted and SnapshotTagged: the local set-aside of a pair's uncommitted paths that D6's force case takes before a local removal.
// The snapshot is one parentless commit in the weft repository holding the task worktree's uncommitted paths under warp/ and the weft worktree's uncommitted paths outside the record pathspec under weft/.
// It is built from loose blobs and `git mktree`, so no branch moves and neither worktree's index is touched.

package fabricengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

const (
	// snapshotTaskDir is the top-level directory of a snapshot commit that holds the task worktree's uncommitted paths.
	snapshotTaskDir = "warp"
	// snapshotRecordsDir is the top-level directory of a snapshot commit that holds the weft worktree's uncommitted paths.
	snapshotRecordsDir = "weft"
	// snapshotTagMessage is the message that marks a set-aside tag as a snapshot, which SnapshotTagged looks for through the tag's annotated type.
	snapshotTagMessage = "snapshot of the uncommitted paths of a pair set aside before its local removal"
)

// snapshotEntry is one uncommitted path headed for the snapshot commit.
type snapshotEntry struct {
	// treePath is the slash-separated path inside the snapshot tree, under snapshotTaskDir or snapshotRecordsDir.
	treePath string
	// filePath is the absolute path of the file whose content is snapshotted; empty for a path deleted from the worktree, which the snapshot lists and does not hold.
	filePath string
}

// SnapshotUncommitted keeps the uncommitted paths of slug's task worktree and of its weft worktree, outside the record pathspec, under a local annotated `set-aside/<slug>/<sha>` tag, and returns the tag and the paths it holds.
// Untracked paths are included.
// The paths are one parentless commit built from loose blobs, so it moves no branch and touches neither worktree's index.
// It takes the weft write lock by one attempt and returns an error wrapping ErrPairBusy, with nothing written, when another writer holds it.
// With nothing uncommitted it writes nothing and returns an empty tag.
// The commit and the tag are recorded in rec, the tag only after it exists.
func (t *Topology) SnapshotUncommitted(rec *Mutations, l *lymcwd.Location, slug string) (tag string, paths []string, err error) {
	weftWorktree := RecordsWorktreePath(l, slug)
	hasWeftWorktree := pathPresent(weftWorktree)
	if hasWeftWorktree {
		lockDir, err := ensureWeftLockDirAt(weftWorktree)
		if err != nil {
			return "", nil, err
		}
		held, acquired, err := lock.TryAcquireWriteLock(filepath.Join(lockDir, weftWriteLockFile))
		if err != nil {
			return "", nil, fmt.Errorf("fabricengine: acquire weft write lock: %w", err)
		}
		if !acquired {
			return "", nil, fmt.Errorf("the weft write lock of the pair %q is held by another writer: %w", slug, ErrPairBusy)
		}
		defer func() { _ = held.Release() }()
	}

	var entries []snapshotEntry
	taskWorktree := WorktreePath(l, slug)
	if pathPresent(taskWorktree) && isRegisteredLinkedWorktree(l, taskWorktree) {
		listed, err := uncommittedEntries(taskWorktree, snapshotTaskDir, nil)
		if err != nil {
			return "", nil, err
		}
		entries = append(entries, listed...)
	}
	if hasWeftWorktree {
		names, err := PathspecNames(ConfigDirs(l))
		if err != nil {
			return "", nil, fmt.Errorf("load the record pathspec: %w", err)
		}
		listed, err := uncommittedEntries(weftWorktree, snapshotRecordsDir, ScopedPathspec(l.AnchorRel, names))
		if err != nil {
			return "", nil, err
		}
		entries = append(entries, listed...)
	}
	if len(entries) == 0 {
		return "", nil, nil
	}

	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", nil, fmt.Errorf("resolve weft repo root: %w", err)
	}
	sha, err := commitSnapshot(rec, weftRoot, slug, entries)
	if err != nil {
		return "", nil, err
	}
	tag, err = snapshotTag(rec, gitrepo.New(weftRoot), slug, sha)
	if err != nil {
		return "", nil, err
	}
	paths = make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.treePath)
	}
	sort.Strings(paths)
	return tag, paths, nil
}

// SnapshotTagged reports whether the weft repository holds a snapshot tag of slug, an annotated `set-aside/<slug>/` tag SnapshotUncommitted wrote.
// A set-aside tag of a replay is lightweight and does not count.
func SnapshotTagged(l *lymcwd.Location, slug string) (bool, error) {
	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false, fmt.Errorf("resolve weft repo root: %w", err)
	}
	out, err := gitexec.Run([]string{"for-each-ref", "--format=%(objecttype)", "refs/tags/" + setAsideTagPrefix + slug + "/"}, weftRoot)
	if err != nil {
		return false, fmt.Errorf("look up the snapshot tags of %q: %w", slug, err)
	}
	for _, objectType := range strings.Fields(out) {
		if objectType == "tag" {
			return true, nil
		}
	}
	return false, nil
}

// uncommittedEntries lists the uncommitted paths of the worktree at dir, untracked ones included, under treeDir.
// A path inside one of the skip pathspec entries, which are relative to dir, is left out.
func uncommittedEntries(dir, treeDir string, skip []string) ([]snapshotEntry, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain", "-z", "--untracked-files=all"}, dir)
	if err != nil {
		return nil, fmt.Errorf("list the uncommitted paths of %s: %w", dir, err)
	}
	var entries []snapshotEntry
	records := strings.Split(stdout, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		rel := record[3:]
		// A rename or copy record is followed by one holding its source.
		if strings.ContainsAny(record[:2], "RC") {
			i++
		}
		if underAnyPathspec(rel, skip) {
			continue
		}
		entry := snapshotEntry{treePath: path.Join(treeDir, rel)}
		filePath := filepath.Join(dir, filepath.FromSlash(rel))
		if _, statErr := os.Lstat(filePath); statErr == nil {
			entry.filePath = filePath
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// underAnyPathspec reports whether rel equals or lies under one of the slash-separated pathspec entries.
func underAnyPathspec(rel string, pathspec []string) bool {
	for _, entry := range pathspec {
		entry = path.Clean(filepath.ToSlash(entry))
		if rel == entry || strings.HasPrefix(rel, entry+"/") {
			return true
		}
	}
	return false
}

// snapshotTreeNode is one directory of the snapshot tree being assembled.
type snapshotTreeNode struct {
	// blobs maps a file name to its `git mktree` line fields: mode and blob SHA.
	blobs map[string][2]string
	dirs  map[string]*snapshotTreeNode
}

func newSnapshotTreeNode() *snapshotTreeNode {
	return &snapshotTreeNode{blobs: map[string][2]string{}, dirs: map[string]*snapshotTreeNode{}}
}

// commitSnapshot writes the blobs of entries into the repository at repoRoot, assembles them into a tree, commits it with no parent and returns the commit's SHA.
// The commit is recorded in rec.
func commitSnapshot(rec *Mutations, repoRoot, slug string, entries []snapshotEntry) (string, error) {
	root := newSnapshotTreeNode()
	for _, entry := range entries {
		if entry.filePath == "" {
			continue
		}
		mode, blob, err := writeSnapshotBlob(repoRoot, entry.filePath)
		if err != nil {
			return "", err
		}
		node := root
		dirs := strings.Split(path.Dir(entry.treePath), "/")
		for _, dir := range dirs {
			child, ok := node.dirs[dir]
			if !ok {
				child = newSnapshotTreeNode()
				node.dirs[dir] = child
			}
			node = child
		}
		node.blobs[path.Base(entry.treePath)] = [2]string{mode, blob}
	}
	treeSHA, err := writeSnapshotTree(repoRoot, root)
	if err != nil {
		return "", err
	}
	out, err := gitexec.Run([]string{"commit-tree", treeSHA, "-m", "snapshot of the uncommitted paths of " + slug}, repoRoot)
	if err != nil {
		return "", fmt.Errorf("commit the snapshot of %q: %w", slug, err)
	}
	sha := strings.TrimSpace(out)
	rec.Append(KindCommitCreated, repoRoot, sha)
	return sha, nil
}

// writeSnapshotBlob writes the content of the file or symlink at filePath into the repository at repoRoot and returns its tree mode and blob SHA.
func writeSnapshotBlob(repoRoot, filePath string) (mode, blob string, err error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return "", "", fmt.Errorf("read %s for the snapshot: %w", filePath, err)
	}
	var out string
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(filePath)
		if err != nil {
			return "", "", fmt.Errorf("read the link %s for the snapshot: %w", filePath, err)
		}
		mode = "120000"
		out, err = gitexec.RunStdin([]string{"hash-object", "-w", "--stdin"}, repoRoot, target)
		if err != nil {
			return "", "", fmt.Errorf("write the link %s into the snapshot: %w", filePath, err)
		}
	default:
		mode = "100644"
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		out, err = gitexec.Run([]string{"hash-object", "-w", "--no-filters", "--", filePath}, repoRoot)
		if err != nil {
			return "", "", fmt.Errorf("write %s into the snapshot: %w", filePath, err)
		}
	}
	return mode, strings.TrimSpace(out), nil
}

// writeSnapshotTree writes node and every directory under it into the repository at repoRoot, bottom up, and returns node's tree SHA.
func writeSnapshotTree(repoRoot string, node *snapshotTreeNode) (string, error) {
	var lines []string
	for name, child := range node.dirs {
		sha, err := writeSnapshotTree(repoRoot, child)
		if err != nil {
			return "", err
		}
		lines = append(lines, "040000 tree "+sha+"\t"+name)
	}
	for name, fields := range node.blobs {
		lines = append(lines, fields[0]+" blob "+fields[1]+"\t"+name)
	}
	sort.Strings(lines)
	out, err := gitexec.RunStdin([]string{"mktree", "-z"}, repoRoot, strings.Join(lines, "\x00")+"\x00")
	if err != nil {
		return "", fmt.Errorf("write a snapshot tree: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// snapshotTag puts the local annotated tag `set-aside/<slug>/<short sha>` on sha, marked as a snapshot by its message, and returns its name.
// A tag already on sha is kept.
// It appends KindTagCreated, with the repository root as the target and the tag name as the detail, only after the tag exists.
func snapshotTag(rec *Mutations, repo *gitrepo.Repo, slug, sha string) (string, error) {
	tag := setAsideTagPrefix + slug + "/" + sha[:archiveTagSHALen]
	commonDir, err := repo.CommonDir()
	if err != nil {
		return "", fmt.Errorf("tag snapshot commit %s: %w", sha, err)
	}
	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/tags/" + tag}, commonDir)
	if err != nil {
		return "", fmt.Errorf("look up snapshot tag %q: %w", tag, err)
	}
	if strings.TrimSpace(existing) == "" {
		if _, err := gitexec.Run([]string{"tag", "-a", "-m", snapshotTagMessage, tag, sha}, commonDir); err != nil {
			return "", fmt.Errorf("create snapshot tag %q at %s: %w", tag, sha, err)
		}
	}
	rec.Append(KindTagCreated, filepath.Dir(commonDir), tag)
	return tag, nil
}
