// trailercommit.go holds the weft commit seams that carry git trailers:
// committing with them, finding a commit by trailer key, and reading an anchored file at a weft commit.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// trailerKeyPattern is the charset of a trailer key a caller may write: a letter, then letters, digits and hyphens.
var trailerKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// validateTrailer returns an error naming t when its key is outside the trailer key charset.
// It does the same when its value is blank or spans lines.
func validateTrailer(t fabriclayout.Trailer) error {
	if !trailerKeyPattern.MatchString(t.Key) {
		return fmt.Errorf("fabricengine: invalid trailer key %q: must match %s", t.Key, trailerKeyPattern.String())
	}
	if strings.TrimSpace(t.Value) == "" || strings.ContainsAny(t.Value, "\r\n") {
		return fmt.Errorf("fabricengine: invalid value %q for trailer %q: must be non-empty and hold no line break", t.Value, t.Key)
	}
	return nil
}

// appendTrailerLine returns message with a "<key>: <value>" trailer line appended.
// It joins an existing trailer block directly or starts one after a blank line.
func appendTrailerLine(message, key, value string) string {
	trimmed := strings.TrimRight(message, "\n")
	line := fmt.Sprintf("%s: %s", key, value)
	if endsInTrailerBlock(trimmed) {
		return trimmed + "\n" + line
	}
	return trimmed + "\n\n" + line
}

// parseTrailerValue extracts the last value of the trailer key from a full commit message.
// It reports ok=false when no line carries the key with a non-empty value.
func parseTrailerValue(message, key string) (value string, ok bool) {
	prefix := key + ":"
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		found := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if found == "" {
			continue
		}
		value, ok = found, true
	}
	return value, ok
}

// CommitAnchoredPathsWithTrailers commits the anchor-relative relPaths on the weft.
// The message is msg followed by a trailer block.
// The block holds `Warp-SHA: <the warp checkout's HEAD>` and then one `Key: Value` line per trailer, in order.
// A warp checkout with no commit yet writes no Warp-SHA line, since there is no commit to name.
//
// A trailer with a key outside [A-Za-z][A-Za-z0-9-]* is refused, naming it.
// So is one whose value is empty or holds a line break.
// Both refusals come before anything is staged.
// With nothing staged, committed is false.
func CommitAnchoredPathsWithTrailers(rec *Mutations, l *lymcwd.Location, relPaths []string, msg string, trailers []fabriclayout.Trailer, opts SyncOptions) (sha string, committed bool, err error) {
	for _, trailer := range trailers {
		if err := validateTrailer(trailer); err != nil {
			return "", false, err
		}
	}
	if opts.SkipGit || len(relPaths) == 0 {
		return "", false, nil
	}

	message, err := messageWithTrailers(l, msg, trailers)
	if err != nil {
		return "", false, err
	}
	return CommitAnchoredPaths(rec, l, relPaths, message, opts)
}

// messageWithTrailers returns msg followed by the trailer block of a trailer commit.
// The block holds the warp checkout's HEAD as a Warp-SHA trailer, left out while the warp has no commit.
// One line per trailer follows, in order.
// The trailers must already be validated.
func messageWithTrailers(l *lymcwd.Location, msg string, trailers []fabriclayout.Trailer) (string, error) {
	f, err := Open(l)
	if err != nil {
		return "", err
	}
	message := msg
	warpSHA, err := f.HeadSHA()
	switch {
	case err == nil:
		message = appendWarpSHATrailer(message, warpSHA)
	case !errors.Is(err, gitrepo.ErrNoCommits):
		return "", fmt.Errorf("fabricengine: read the warp HEAD for the trailer block: %w", err)
	}
	for _, trailer := range trailers {
		message = appendTrailerLine(message, trailer.Key, trailer.Value)
	}
	return message, nil
}

// recordsFirstParentCommits returns every first-parent commit of the records branch from HEAD, newest first.
// A commit's first-parent position is len(commits)-1-index.
// A records branch with no commit yields nil and no error.
func recordsFirstParentCommits(l *lymcwd.Location) ([]gitrepo.LogCommit, error) {
	repo := gitrepo.New(RecordsWorktree(l))
	head, err := repo.CurrentSHA()
	if err != nil {
		if errors.Is(err, gitrepo.ErrNoCommits) {
			return nil, nil
		}
		return nil, fmt.Errorf("fabricengine: resolve the weft HEAD: %w", err)
	}
	commits, err := repo.Log(head, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return nil, fmt.Errorf("fabricengine: walk the weft history: %w", err)
	}
	return commits, nil
}

// TrailerCommits returns every first-parent commit of the records branch whose message carries key.
// The key's value must be non-empty.
// The commits come newest first.
// Each holds its SHA, the key's last value, its Warp-SHA value and its first-parent position.
// A records branch with no commit yet returns nil and no error.
// The walk follows first parents, so a merge's side branches are not read.
func TrailerCommits(l *lymcwd.Location, key string) ([]fabriclayout.TrailerCommit, error) {
	commits, err := recordsFirstParentCommits(l)
	if err != nil {
		return nil, fmt.Errorf("fabricengine: list commits for trailer %q: %w", key, err)
	}

	var found []fabriclayout.TrailerCommit
	for i, commit := range commits {
		value, ok := parseTrailerValue(commit.Message, key)
		if !ok {
			continue
		}
		warpSHA, _ := parseTrailerValue(commit.Message, fabriclayout.WarpSHATrailerKey)
		found = append(found, fabriclayout.TrailerCommit{SHA: commit.SHA, Value: value, CodeHead: warpSHA, Position: len(commits) - 1 - i})
	}
	return found, nil
}

// FindTrailerCommit walks the weft branch's history from HEAD.
// It returns the newest commit whose message carries key when newest is true, and the oldest otherwise.
// found is false when no commit carries the key, and for a weft with no commit yet.
// The walk follows first parents, so a merge's side branches are not read.
func FindTrailerCommit(l *lymcwd.Location, key string, newest bool) (fabriclayout.TrailerCommit, bool, error) {
	found, err := TrailerCommits(l, key)
	if err != nil {
		return fabriclayout.TrailerCommit{}, false, err
	}
	if len(found) == 0 {
		return fabriclayout.TrailerCommit{}, false, nil
	}
	if newest {
		return found[0], true, nil
	}
	return found[len(found)-1], true, nil
}

// RecordsCommitIsAfter reports whether the records commit sha was made after the records commit rev.
// That holds when rev is a proper ancestor of sha.
// It is read-only, so the Fabric Git Invariant exempts it from the commit seam.
func RecordsCommitIsAfter(l *lymcwd.Location, rev, sha string) (bool, error) {
	if rev == sha {
		return false, nil
	}
	after, err := gitrepo.New(RecordsWorktree(l)).IsAncestor(rev, sha)
	if err != nil {
		return false, fmt.Errorf("fabricengine: compare the records commits %s and %s: %w", rev, sha, err)
	}
	return after, nil
}

// AnchoredFileAt returns the anchor-relative relPath as stored in the weft commit rev, unaffected by any later change.
// found is false with a nil error when the path is absent at rev; an invalid rev is an error.
func AnchoredFileAt(l *lymcwd.Location, rev, relPath string) ([]byte, bool, error) {
	scoped := filepath.ToSlash(fabriclayout.ScopedPathspec(l.AnchorRel, []string{relPath})[0])
	data, err := gitrepo.New(RecordsWorktree(l)).FileAtRevision(rev, scoped)
	if err != nil {
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, rev, err)
	}
	return data, true, nil
}
