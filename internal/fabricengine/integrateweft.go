// integrateweft.go rebuilds a weft branch on its upstream tip and carries the branch's local commits across a change of anchor.
// It builds every commit with git plumbing over a temporary index and moves the branch only once the result is whole.

package fabricengine

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
)

// ErrIntegrateConflict is wrapped by the stop of an integration whose local commit changed a path the upstream changed differently.
// The weft is left as it was.
var ErrIntegrateConflict = errors.New("fabricengine: a local weft commit conflicts with the upstream")

// IntegrateOutcome reports an integration.
// Tip is the commit the weft branch now points at.
// Replayed lists the local commits carried onto the upstream, oldest first.
// Skipped lists the local commits not carried: those that changed the anchor marker, and those the upstream already holds the change of.
type IntegrateOutcome struct {
	Tip      string
	Replayed []string
	Skipped  []string
}

// integrateWeft rebuilds the weft checked out at weftPath on its upstream tip upstream, anchored at target.
// It replays each local commit the upstream lacks, oldest first, skipping each commit that changed the anchor marker, and carries each changed path from under the anchor its parent commit names to under the upstream's anchor.
// A parent without a marker names fallback, or the anchor its tree implies.
// Paths outside every wired name in names, and the marker, are replayed unmoved.
// When the upstream's anchor is not target, it then re-roots every wired name's tracked content to target and sets the marker, in one commit.
// Nothing of the weft changes until the result is whole, when the branch moves to it keeping uncommitted changes.
// A local commit that changed a path the upstream changed differently stops the integration with ErrIntegrateConflict, naming the commit, its paths and the steps that settle it.
// A tracked change on a path the result rewrites returns ErrReplayDirty.
func integrateWeft(rec *Mutations, weftPath, upstream, target string, names []string, fallback string) (IntegrateOutcome, error) {
	repo := gitrepo.New(weftPath)
	upstreamSHA, err := repo.ResolveSHA(upstream)
	if err != nil {
		return IntegrateOutcome{}, err
	}
	head, err := repo.CurrentSHA()
	if err != nil {
		return IntegrateOutcome{}, err
	}
	local, err := repo.CommitsNotIn(head, upstreamSHA)
	if err != nil {
		return IntegrateOutcome{}, err
	}
	slices.Reverse(local)
	upstreamAnchor, err := treeAnchor(weftPath, upstreamSHA, names, fallback)
	if err != nil {
		return IntegrateOutcome{}, err
	}
	upstreamAnchor, target = filepath.ToSlash(filepath.Clean(upstreamAnchor)), filepath.ToSlash(filepath.Clean(target))

	build, err := newIntegrateBuild(weftPath, upstreamSHA, head, target, names)
	if err != nil {
		return IntegrateOutcome{}, err
	}
	var outcome IntegrateOutcome
	for _, sha := range local {
		replayed, err := build.replay(sha, upstreamAnchor, fallback)
		if err != nil {
			return IntegrateOutcome{}, err
		}
		if replayed {
			outcome.Replayed = append(outcome.Replayed, sha)
		} else {
			outcome.Skipped = append(outcome.Skipped, sha)
		}
	}
	if upstreamAnchor != target {
		if err := build.reroot(upstreamAnchor, target); err != nil {
			return IntegrateOutcome{}, err
		}
	}
	outcome.Tip = build.tip
	if outcome.Tip != head {
		if err := resetWeftToIntegrated(rec, weftPath, outcome.Tip); err != nil {
			return IntegrateOutcome{}, err
		}
	}
	return outcome, nil
}

// integrateBuild builds commits in a temporary index file, so the weft's own index, working tree and branch are never touched.
type integrateBuild struct {
	weftPath string
	names    []string
	// head is the weft's local tip and target the anchor the integration ends at, both named by the way forward of a conflict.
	head, target string
	// env points git at the temporary index and keeps pathspecs literal.
	env []string
	// tip is the commit built so far, the upstream tip to begin with.
	tip string
}

// newIntegrateBuild begins a build at the commit upstreamSHA of the weft at weftPath, whose local tip is head and which ends at the anchor target.
func newIntegrateBuild(weftPath, upstreamSHA, head, target string, names []string) (*integrateBuild, error) {
	dir, err := lyxtemp.MkdirTemp("integrate-index-*")
	if err != nil {
		return nil, fmt.Errorf("create the temporary index for the integration: %w", err)
	}
	build := &integrateBuild{
		weftPath: weftPath,
		names:    names,
		head:     head,
		target:   target,
		env:      []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index"), "GIT_LITERAL_PATHSPECS=1"},
		tip:      upstreamSHA,
	}
	if _, err := build.git(nil, "read-tree", upstreamSHA); err != nil {
		return nil, fmt.Errorf("read %s into the temporary index: %w", upstreamSHA, err)
	}
	return build, nil
}

// git runs git with args in the weft against the temporary index, with extraEnv after the index variables.
func (b *integrateBuild) git(extraEnv []string, args ...string) (string, error) {
	return gitexec.RunWithEnv(args, b.weftPath, append(slices.Clone(b.env), extraEnv...))
}

// indexEntry is one tracked file: its mode, blob and path.
type indexEntry struct {
	mode, blob, path string
}

// treeChange is one path a commit changed against its parent, with the blob it had before (empty when added) and the entry it has after (zero blob when deleted).
type treeChange struct {
	path             string
	oldBlob          string
	newMode, newBlob string
}

// replay carries the local commit sha onto the build, with its paths moved from the anchor its parent names to frame.
// It reports false for a commit that changed the marker or whose change the build already holds.
func (b *integrateBuild) replay(sha, frame, fallback string) (bool, error) {
	repo := gitrepo.New(b.weftPath)
	detail, err := repo.CommitDetail(sha)
	if err != nil {
		return false, err
	}
	if slices.Contains(detail.Paths, lyxcwd.AnchorFileName) {
		return false, nil
	}
	parents, err := repo.CommitParents(sha)
	if err != nil {
		return false, err
	}
	parent, parentAnchor := "", fallback
	if len(parents) > 0 {
		parent = parents[0]
		if parentAnchor, err = treeAnchor(b.weftPath, parent, b.names, fallback); err != nil {
			return false, err
		}
	}
	parentAnchor = filepath.ToSlash(filepath.Clean(parentAnchor))

	changes, err := b.changesOf(parent, sha)
	if err != nil {
		return false, err
	}
	var conflicts []string
	var apply []treeChange
	for _, change := range changes {
		moved := change
		moved.path = moveUnderAnchor(change.path, parentAnchor, frame, b.names)
		current, err := b.indexBlob(moved.path)
		if err != nil {
			return false, err
		}
		switch {
		case current == moved.newBlob:
		case current == change.oldBlob:
			apply = append(apply, moved)
		default:
			conflicts = append(conflicts, change.path)
		}
	}
	if len(conflicts) > 0 {
		return false, integrateConflict(b.weftPath, sha, b.head, b.target, conflicts)
	}
	if len(apply) == 0 {
		return false, nil
	}
	for _, change := range apply {
		if err := b.set(change.path, change.newMode, change.newBlob); err != nil {
			return false, err
		}
	}

	author, err := commitAuthorEnv(b.weftPath, sha)
	if err != nil {
		return false, err
	}
	message := strings.TrimRight(detail.Message, "\r\n")
	if err := b.commit(message, author); err != nil {
		return false, err
	}
	return true, nil
}

// reroot moves every tracked file under a wired name from under the anchor from to under the anchor to, sets the marker to to, and commits the result.
func (b *integrateBuild) reroot(from, to string) error {
	for _, name := range b.names {
		dir := anchorJoin(from, name)
		entries, err := b.indexEntries(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			moved := anchorJoin(to, name) + strings.TrimPrefix(entry.path, dir)
			existing, err := b.indexBlob(moved)
			if err != nil {
				return err
			}
			if existing != "" {
				return fmt.Errorf("fabricengine: re-rooting %s to %s would overwrite the tracked %s", entry.path, to, moved)
			}
			if err := b.set(entry.path, "", ""); err != nil {
				return err
			}
			if err := b.set(moved, entry.mode, entry.blob); err != nil {
				return err
			}
		}
	}
	marker, err := gitexec.RunStdin([]string{"hash-object", "-w", "--stdin"}, b.weftPath, to+"\n")
	if err != nil {
		return fmt.Errorf("write the marker %s: %w", to, err)
	}
	if err := b.set(lyxcwd.AnchorFileName, "100644", strings.TrimSpace(marker)); err != nil {
		return err
	}
	return b.commit(fmt.Sprintf("fabric anchor: re-root the weft from %s to %s", from, to), nil)
}

// commit writes the temporary index as a tree and a commit with message on top of tip, with env as its author, and moves tip to it.
func (b *integrateBuild) commit(message string, authorEnv []string) error {
	tree, err := b.git(nil, "write-tree")
	if err != nil {
		return fmt.Errorf("write the integrated tree: %w", err)
	}
	created, err := b.git(authorEnv, "commit-tree", strings.TrimSpace(tree), "-p", b.tip, "-m", message)
	if err != nil {
		return fmt.Errorf("commit the integrated tree: %w", err)
	}
	b.tip = strings.TrimSpace(created)
	return nil
}

// set puts the file at filePath into the temporary index with mode and blob, and removes it when blob is empty.
func (b *integrateBuild) set(filePath, mode, blob string) error {
	if isZeroBlob(blob) {
		_, err := b.git(nil, "update-index", "--force-remove", "--", filePath)
		return err
	}
	_, err := b.git(nil, "update-index", "--add", "--cacheinfo", mode+","+blob+","+filePath)
	return err
}

// indexBlob returns the blob the temporary index holds at filePath, empty when it holds none.
func (b *integrateBuild) indexBlob(filePath string) (string, error) {
	entries, err := b.indexEntries(filePath)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.path == filePath {
			return entry.blob, nil
		}
	}
	return "", nil
}

// indexEntries lists the tracked files of the temporary index under pathspec.
func (b *integrateBuild) indexEntries(pathspec string) ([]indexEntry, error) {
	out, err := b.git(nil, "ls-files", "-s", "-z", "--", pathspec)
	if err != nil {
		return nil, fmt.Errorf("list %s in the temporary index: %w", pathspec, err)
	}
	var entries []indexEntry
	for _, record := range strings.Split(out, "\x00") {
		meta, entryPath, found := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) < 2 {
			continue
		}
		entries = append(entries, indexEntry{mode: fields[0], blob: fields[1], path: entryPath})
	}
	return entries, nil
}

// changesOf lists the paths the commit sha changed against parent, or every path of a root commit when parent is empty.
func (b *integrateBuild) changesOf(parent, sha string) ([]treeChange, error) {
	args := []string{"diff-tree", "-r", "--no-renames", "--no-abbrev", "--no-commit-id", "-z", "--raw"}
	if parent == "" {
		args = append(args, "--root", sha)
	} else {
		args = append(args, parent, sha)
	}
	out, err := gitexec.Run(args, b.weftPath)
	if err != nil {
		return nil, fmt.Errorf("list the paths %s changed: %w", sha, err)
	}
	var changes []treeChange
	records := strings.Split(out, "\x00")
	for i := 0; i+1 < len(records); i += 2 {
		fields := strings.Fields(strings.TrimPrefix(records[i], ":"))
		if len(fields) < 5 {
			return nil, fmt.Errorf("fabricengine: unreadable diff-tree entry %q for %s", records[i], sha)
		}
		change := treeChange{path: records[i+1], newMode: fields[1], newBlob: fields[3]}
		if !isZeroBlob(fields[2]) {
			change.oldBlob = fields[2]
		}
		if isZeroBlob(change.newBlob) {
			change.newBlob = ""
		}
		changes = append(changes, change)
	}
	return changes, nil
}

// isZeroBlob reports whether blob names no object: empty, or git's all-zero name.
func isZeroBlob(blob string) bool {
	return strings.Trim(blob, "0") == ""
}

// anchorJoin returns the slash path of name under anchor, "." being the repository root.
func anchorJoin(anchor, name string) string {
	if anchor == "." {
		return name
	}
	return path.Join(anchor, name)
}

// moveUnderAnchor returns filePath moved from under the anchor from to under the anchor to, when it lies under one of the wired names.
// The marker and every path outside the wired names come back unmoved.
func moveUnderAnchor(filePath, from, to string, names []string) string {
	if from == to || filePath == lyxcwd.AnchorFileName {
		return filePath
	}
	for _, name := range names {
		if rest, under := strings.CutPrefix(filePath, anchorJoin(from, name)+"/"); under {
			return anchorJoin(to, name) + "/" + rest
		}
	}
	return filePath
}

// commitAuthorEnv returns the variables that give a commit made on top of sha's change the author, address and date of sha.
func commitAuthorEnv(weftPath, sha string) ([]string, error) {
	out, err := gitexec.Run([]string{"show", "-s", "--format=%an%x00%ae%x00%aI", sha}, weftPath)
	if err != nil {
		return nil, fmt.Errorf("read the author of %s: %w", sha, err)
	}
	fields := strings.Split(strings.TrimSpace(out), "\x00")
	if len(fields) != 3 {
		return nil, fmt.Errorf("fabricengine: unreadable author of %s: %q", sha, out)
	}
	return []string{"GIT_AUTHOR_NAME=" + fields[0], "GIT_AUTHOR_EMAIL=" + fields[1], "GIT_AUTHOR_DATE=" + fields[2]}, nil
}

// integrateConflict returns the ErrIntegrateConflict stop for the local commit sha, which changed conflicts differently than the upstream did.
// Its way forward keeps every local commit reachable under a tag while the changes are saved and written back at the target anchor.
func integrateConflict(weftPath, sha, head, target string, conflicts []string) error {
	slug := filepath.Base(weftPath)
	if warpSlug, ok := WeftWarpSlug(slug); ok {
		slug = warpSlug
	}
	return fmt.Errorf("%w: local commit %s changed %s, which the upstream changed too; "+
		"tag the local tip by hand (`git -C %s tag archive/%s/%s %s`), "+
		"save the changes of %s and of every later local commit outside the weft, "+
		"reset the weft branch by hand to its parent (`git -C %s reset --keep %s^`), "+
		"re-run `lyx fabric anchor %s --set %s`, "+
		"then write the saved changes back at %s and commit them; the tag keeps every dropped commit reachable until they are back",
		ErrIntegrateConflict, sha, strings.Join(conflicts, ", "),
		weftPath, slug, head, head,
		sha,
		weftPath, sha,
		slug, target,
		target)
}
