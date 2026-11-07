// trailercommit.go holds the weft commit seams that carry git trailers:
// committing with them, finding a commit by trailer key, and reading an anchored file at a weft commit.

package fabricengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// trailerKeyPattern is the charset of a trailer key a caller may write: a letter, then letters, digits and hyphens.
var trailerKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

// Trailer is one git trailer a commit message ends with.
type Trailer struct {
	// Key is the trailer's key, matching [A-Za-z][A-Za-z0-9-]*.
	Key string
	// Value is the trailer's value: non-empty and on one line.
	Value string
}

// validateTrailer returns an error naming t when its key is outside the trailer key charset.
// It does the same when its value is blank or spans lines.
func validateTrailer(t Trailer) error {
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
func CommitAnchoredPathsWithTrailers(rec *Mutations, l *lymcwd.Location, relPaths []string, msg string, trailers []Trailer, opts SyncOptions) (sha string, committed bool, err error) {
	for _, trailer := range trailers {
		if err := validateTrailer(trailer); err != nil {
			return "", false, err
		}
	}
	if opts.SkipGit || len(relPaths) == 0 {
		return "", false, nil
	}

	f, err := Open(l)
	if err != nil {
		return "", false, err
	}
	message := msg
	warpSHA, err := f.HeadSHA()
	switch {
	case err == nil:
		message = appendWarpSHATrailer(message, warpSHA)
	case !errors.Is(err, gitrepo.ErrNoCommits):
		return "", false, fmt.Errorf("fabricengine: read the warp HEAD for the trailer block: %w", err)
	}
	for _, trailer := range trailers {
		message = appendTrailerLine(message, trailer.Key, trailer.Value)
	}
	return CommitAnchoredPaths(rec, l, relPaths, message, opts)
}

// TrailerCommit is a weft commit that carries a trailer key.
type TrailerCommit struct {
	// SHA is the weft commit's full object name.
	SHA string
	// Value is the last value of the searched key in the commit's message.
	Value string
	// CodeHead is the commit's Warp-SHA value, the code checkout's HEAD the commit was made at, empty when it carries none.
	CodeHead string
}

// FindTrailerCommit walks the weft branch's history from HEAD.
// It returns the newest commit whose message carries key when newest is true, and the oldest otherwise.
// found is false when no commit carries the key, and for a weft with no commit yet.
// The walk follows first parents, so a merge's side branches are not read.
func FindTrailerCommit(l *lymcwd.Location, key string, newest bool) (TrailerCommit, bool, error) {
	repo := gitrepo.New(RecordsWorktree(l))
	head, err := repo.CurrentSHA()
	if err != nil {
		if errors.Is(err, gitrepo.ErrNoCommits) {
			return TrailerCommit{}, false, nil
		}
		return TrailerCommit{}, false, fmt.Errorf("fabricengine: resolve the weft HEAD for trailer %q: %w", key, err)
	}
	commits, err := repo.Log(head, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return TrailerCommit{}, false, fmt.Errorf("fabricengine: walk the weft history for trailer %q: %w", key, err)
	}

	for i := range commits {
		commit := commits[i]
		if !newest {
			commit = commits[len(commits)-1-i]
		}
		value, ok := parseTrailerValue(commit.Message, key)
		if !ok {
			continue
		}
		warpSHA, _ := parseTrailerValue(commit.Message, WarpSHATrailerKey)
		return TrailerCommit{SHA: commit.SHA, Value: value, CodeHead: warpSHA}, true, nil
	}
	return TrailerCommit{}, false, nil
}

// AnchoredFileAt returns the anchor-relative relPath as stored in the weft commit rev, unaffected by any later change.
// found is false with a nil error when the path is absent at rev; an invalid rev is an error.
func AnchoredFileAt(l *lymcwd.Location, rev, relPath string) ([]byte, bool, error) {
	scoped := filepath.ToSlash(ScopedPathspec(l.AnchorRel, []string{relPath})[0])
	data, err := gitrepo.New(RecordsWorktree(l)).FileAtRevision(rev, scoped)
	if err != nil {
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fabricengine: read %s at %s: %w", relPath, rev, err)
	}
	return data, true, nil
}
