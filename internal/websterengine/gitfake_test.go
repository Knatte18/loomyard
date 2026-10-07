// gitfake_test.go declares fakeGit, an in-memory websterengine.Git over a commit graph a test declares,
// and writeWorktreeFile, which puts a file in a fixture worktree without a commit.
// A fixture that sets Geometry.Git to a fakeGit runs the bracket verbs with no git process;
// a test whose behavior is git itself builds a real scratch repository in a //go:build integration file instead.

package websterengine_test

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/Knatte18/quarry/quarry"
)

// fakeGit answers every websterengine.Git question from fields a test sets.
// Its history is the commits registered through commit and merge; a SHA never registered does not exist.
// The zero values answer as a clean, single-worktree repository with no ignored path.
type fakeGit struct {
	// head is the SHA HeadSHA returns.
	head string
	// isDirty is what Dirty returns.
	isDirty bool
	// dirtyPaths is what DirtyPaths returns.
	dirtyPaths []string
	// merging is what MergeInProgress returns.
	merging bool
	// parents maps each registered commit to its parent SHAs.
	parents map[string][]string
	// rejections maps a merge commit to the reason MergeRejection gives.
	// A merge not listed qualifies.
	rejections map[string]string
	// others are the sibling worktrees OtherWorktrees returns.
	others []string
	// ignored lists the paths IgnoredPath reports as ignored.
	ignored map[string]bool
	// differing lists the paths PathDiffers reports as differing.
	differing map[string]bool
	// commitBlobs maps "<commit>:<path>" to the blob CommitBlob returns.
	commitBlobs map[string]string
	// treeBlobPaths maps "<commit>:<blob>" to the paths TreePathsWithBlob returns.
	treeBlobPaths map[string][]string
	// registered counts the SHAs minted so far.
	registered int
}

// fakeIndex is the real planindex.Index except Delta, which answers from fields a test sets and records the commit range of every call.
// The zero values answer an empty delta.
type fakeIndex struct {
	planindex.Index
	// delta and deltaErr are what Delta returns.
	delta    quarry.GitDeltaAnswer
	deltaErr error
	// deltaRanges records the commit range of every Delta call, in order.
	deltaRanges []deltaRange
}

// deltaRange is the commit range of one Delta call.
type deltaRange struct {
	from, to string
}

var _ planindex.Index = (*fakeIndex)(nil)

// newFakeIndex returns a fakeIndex over the real index.
func newFakeIndex() *fakeIndex {
	return &fakeIndex{Index: planglyph.NewIndex()}
}

func (i *fakeIndex) Delta(_, fromSHA, toSHA string) (planindex.Delta, error) {
	i.deltaRanges = append(i.deltaRanges, deltaRange{from: fromSHA, to: toSHA})
	return fakeDelta{answer: i.delta}, i.deltaErr
}

// indexOver returns the index a fixture over git runs on: a fakeIndex, returned twice so the caller can steer it, when git is a fake, and the real index, with a nil fakeIndex, when git is nil and the repository on disk answers.
func indexOver(git websterengine.Git) (*fakeIndex, planindex.Index) {
	if git == nil {
		return nil, planglyph.NewIndex()
	}
	fake := newFakeIndex()
	return fake, fake
}

// fakeDelta is a fixed quarry answer behind planindex.Delta, consumed by the real checks.
type fakeDelta struct {
	answer quarry.GitDeltaAnswer
}

func (d fakeDelta) BindHandles(plan *planparser.Plan, planDir string, cards []planparser.Card) ([]planindex.Finding, error) {
	return planglyph.BindHandles(plan, planDir, d.answer, cards)
}

func (d fakeDelta) ScopeGuard(cards []planparser.Card) []planindex.Finding {
	return planglyph.ScopeGuard(cards, d.answer)
}

func (d fakeDelta) DetectDrift(fullPlan *planparser.Plan, completed []planparser.Card, planDir, worktreeRoot, sha, now string) ([]planindex.Finding, error) {
	return planglyph.DetectDrift(fullPlan, planglyph.PendingPlan(fullPlan, completed), planDir, worktreeRoot, d.answer, sha, now)
}

var _ websterengine.Git = (*fakeGit)(nil)

// newFakeGit returns a fakeGit whose history is one root commit, which is its head.
func newFakeGit() *fakeGit {
	g := &fakeGit{
		parents:       map[string][]string{},
		rejections:    map[string]string{},
		ignored:       map[string]bool{},
		differing:     map[string]bool{},
		commitBlobs:   map[string]string{},
		treeBlobPaths: map[string][]string{},
	}
	g.commit()
	return g
}

// mint returns a fresh 40-hex SHA no commit holds yet.
func (g *fakeGit) mint() string {
	g.registered++
	return fmt.Sprintf("%040x", g.registered)
}

// commit registers a new commit on top of the current head, makes it the head, and returns its SHA.
func (g *fakeGit) commit() string {
	sha := g.mint()
	var parents []string
	if g.head != "" {
		parents = []string{g.head}
	}
	g.parents[sha] = parents
	g.head = sha
	return sha
}

// merge registers a merge of the current head and a new commit standing for the parent branch's tip, makes the merge the head, and returns its SHA and that tip.
// rejection is the reason MergeRejection gives for the merge, "" for a clean parent merge.
func (g *fakeGit) merge(rejection string) (mergeSHA, parentTip string) {
	parentTip = g.mint()
	g.parents[parentTip] = nil
	mergeSHA = g.mint()
	g.parents[mergeSHA] = []string{g.head, parentTip}
	g.rejections[mergeSHA] = rejection
	g.head = mergeSHA
	return mergeSHA, parentTip
}

func (g *fakeGit) HeadSHA(string) (string, error) { return g.head, nil }

func (g *fakeGit) Dirty(string) (bool, error) { return g.isDirty, nil }

func (g *fakeGit) DirtyPaths(string) ([]string, error) { return g.dirtyPaths, nil }

func (g *fakeGit) MergeInProgress(string) (bool, error) { return g.merging, nil }

func (g *fakeGit) CommitParents(_, commit string) ([]string, error) {
	parents, ok := g.parents[commit]
	if !ok {
		return nil, fmt.Errorf("fakeGit: unknown commit %s", commit)
	}
	return parents, nil
}

func (g *fakeGit) MergeRejection(_, commit string, _ []string, parentBranch websterengine.ParentBranchFunc) string {
	if parentBranch == nil {
		return "the run has no known parent branch"
	}
	return g.rejections[commit]
}

func (g *fakeGit) SHAExists(_, sha string) bool {
	_, ok := g.parents[sha]
	return ok
}

func (g *fakeGit) IsAncestor(_, sha, ref string) (bool, error) {
	if !g.SHAExists("", sha) || !g.SHAExists("", ref) {
		return false, errors.New("fakeGit: unknown commit")
	}
	seen := map[string]bool{}
	queue := []string{ref}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == sha {
			return true, nil
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		queue = append(queue, g.parents[cur]...)
	}
	return false, nil
}

func (g *fakeGit) IgnoredPath(_, path string) (bool, error) { return g.ignored[path], nil }

func (g *fakeGit) OtherWorktrees(string) ([]string, error) { return g.others, nil }

func (g *fakeGit) PathDiffers(_, _, path string) (bool, error) { return g.differing[path], nil }

// WorktreeBlob returns the SHA-1 of the file's bytes, which is all a test compares it with.
func (g *fakeGit) WorktreeBlob(_, path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data)
	return hex.EncodeToString(sum[:]), nil
}

func (g *fakeGit) CommitBlob(_, commit, path string) (string, error) {
	return g.commitBlobs[commit+":"+path], nil
}

func (g *fakeGit) TreePathsWithBlob(_, commit, blob string) ([]string, error) {
	paths := slices.Clone(g.treeBlobPaths[commit+":"+blob])
	sort.Strings(paths)
	return paths, nil
}

// writeWorktreeFile writes content at the slash-separated path rel under root, creating its directories.
func writeWorktreeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
