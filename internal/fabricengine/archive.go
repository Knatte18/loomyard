// archive.go implements archiveWeftTip, the helper every weft-branch teardown of an existing pair calls before it deletes that branch: it tags the weft tip under the archive/<slug>/ namespace and pushes the tag to the weft origin, so the run records on that branch stay reachable after the branch is gone.
// Tag creation and push are not destructive, so this lives outside destroy.go.

package fabricengine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// archiveTagSHALen is how many leading hex digits of the tip SHA the archive tag name carries.
const archiveTagSHALen = 12

// ErrArchiveFailed is the sentinel every archiveWeftTip error wraps, so a teardown caller can tell a failed archive -- which left the pair untouched, so a plain re-run retries it -- from any other refusal with errors.Is and offer its own remedy.
// It is worded without naming either side of the pair, for callers outside the fabric vocabulary owner set.
var ErrArchiveFailed = errors.New("archiving the pair's records before teardown failed")

// archiveWeftTip tags the tip of weftBranch as archive/<slug>/<first 12 hex of the tip SHA> and pushes that tag to the weft repo's origin.
// The tip is the local refs/heads/<weftBranch> when present, else the origin's copy, fetched.
// The tag name is deterministic from slug and tip, so a resumed teardown on the same tip finds its own tag and succeeds, while a new tip archives under a new name.
//
// No origin remote returns no tag, a skippedReason and no error.
// A branch present neither locally nor on origin returns no tag, no reason and no error, since there is nothing to archive.
// An existing tag of that name pointing elsewhere, and any push failure, are returned errors.
// Every returned error wraps ErrArchiveFailed.
// KindTagPushed is recorded on rec only after the push observably succeeded.
func archiveWeftTip(rec *Mutations, l *lyxcwd.Location, slug, weftBranch string) (tag string, skippedReason string, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w: %w", ErrArchiveFailed, err)
		}
	}()

	weftRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return "", "", fmt.Errorf("archive weft tip of %q: resolve weft repo: %w", weftBranch, err)
	}

	if _, urlErr := gitrepo.New(weftRoot).RemoteURL(originRemoteName); urlErr != nil {
		return "", fmt.Sprintf(
			"no archive tag pushed: the weft repo has no %q remote configured: %v",
			originRemoteName, urlErr), nil
	}

	tip, err := resolveArchiveTip(weftRoot, weftBranch)
	if err != nil {
		return "", "", err
	}
	if tip == "" {
		return "", "", nil
	}

	tag = "archive/" + slug + "/" + tip[:archiveTagSHALen]
	tagRef := "refs/tags/" + tag

	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", tagRef}, weftRoot)
	if err != nil {
		return "", "", fmt.Errorf("look up archive tag %q: %w", tag, err)
	}
	switch existing = strings.TrimSpace(existing); {
	case existing == "":
		if _, err := gitexec.Run([]string{"tag", tag, tip}, weftRoot); err != nil {
			return "", "", fmt.Errorf("create archive tag %q at %s: %w", tag, tip, err)
		}
	case existing != tip:
		return "", "", fmt.Errorf("archive tag %q already exists at %s, not the weft tip %s", tag, existing, tip)
	}

	if _, err := gitexec.Run([]string{"push", originRemoteName, tagRef}, weftRoot); err != nil {
		return "", "", fmt.Errorf("push archive tag %q to %q: %w", tag, originRemoteName, err)
	}
	rec.Append(KindTagPushed, weftRoot, tag)
	return tag, "", nil
}

// resolveArchiveTip returns the full SHA of weftBranch in the weft repo at weftRoot: the local branch when present, else the origin's copy fetched into the repo.
// It returns an empty SHA and no error when the branch exists in neither place, and an error when the origin cannot be read to find out.
func resolveArchiveTip(weftRoot, weftBranch string) (string, error) {
	local, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/heads/" + weftBranch}, weftRoot)
	if err != nil {
		return "", fmt.Errorf("resolve weft branch %q: %w", weftBranch, err)
	}
	if tip := strings.TrimSpace(local); tip != "" {
		return tip, nil
	}

	listed, err := gitexec.Run([]string{"ls-remote", "--heads", originRemoteName, "refs/heads/" + weftBranch}, weftRoot)
	if err != nil {
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) {
			return "", fmt.Errorf("look up weft branch %q on %q: %w", weftBranch, originRemoteName, err)
		}
		return "", err
	}
	if strings.TrimSpace(listed) == "" {
		return "", nil
	}
	if _, err := gitexec.Run([]string{"fetch", originRemoteName, "refs/heads/" + weftBranch}, weftRoot); err != nil {
		return "", fmt.Errorf("fetch weft branch %q from %q: %w", weftBranch, originRemoteName, err)
	}
	fetched, err := gitexec.Run([]string{"rev-parse", "--verify", "FETCH_HEAD^{commit}"}, weftRoot)
	if err != nil {
		return "", fmt.Errorf("resolve fetched weft branch %q: %w", weftBranch, err)
	}
	return strings.TrimSpace(fetched), nil
}
