// reflog.go reads a remote-tracking ref's reflog and judges, from the values it held, what origin did to a branch this machine had pushed or fetched:
// which local commits origin already holds, and whether origin rewound or rewrote the branch.

package fabricengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// pushReflogSubject is the reflog subject git writes for a remote-tracking ref this machine's own push updated.
const pushReflogSubject = "update by push"

// rewriteKind is how origin moved a branch past this machine's tip.
type rewriteKind int

// The ways origin moved a branch past this machine's tip.
const (
	// rewriteNone is any move that is neither a rewind nor a rewrite.
	rewriteNone rewriteKind = iota
	// rewriteRewound is origin resetting the branch back past commits this machine holds.
	rewriteRewound
	// rewriteRewritten is origin replacing the branch's history with commits this machine never held.
	rewriteRewritten
)

// trackingReflog returns the values of the remote-tracking ref trackingRef in repoDir, newest last, from the last entry this machine's own push wrote onward.
// A reflog holding no push entry, as for a branch this machine never pushed, returns every value it holds.
// A ref with no reflog returns none.
func trackingReflog(repoDir, trackingRef string) ([]string, error) {
	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", trackingRef}, repoDir)
	if err != nil {
		return nil, fmt.Errorf("look up %s in %s: %w", trackingRef, repoDir, err)
	}
	if strings.TrimSpace(existing) == "" {
		return nil, nil
	}
	listing, err := gitexec.Run([]string{"reflog", "show", "--format=%H%x09%gs", trackingRef}, repoDir)
	if err != nil {
		return nil, fmt.Errorf("read the reflog of %s in %s: %w", trackingRef, repoDir, err)
	}

	var values []string
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		sha, subject, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found || sha == "" {
			continue
		}
		values = append(values, sha)
		if strings.HasPrefix(subject, pushReflogSubject) {
			break
		}
	}
	slices.Reverse(values)
	return slices.Compact(values), nil
}

// classifyRewrite judges how origin's head relates to the values earlier the tracking ref held, for a branch whose local tip is localTip.
// It is rewriteRewound when some earlier value is not an ancestor of originHead while originHead is an ancestor of it and localTip holds commits of that value originHead lacks, which it returns as dropped.
// It is rewriteRewritten when every local commit originHead lacks is reachable from an earlier value and originHead is an ancestor of none of them.
// The two never overlap, since whether originHead is an ancestor of an earlier value separates them; every other move is rewriteNone.
func classifyRewrite(repo *gitrepo.Repo, localTip, originHead string, earlier []string) (kind rewriteKind, dropped []string, err error) {
	earlier = slices.DeleteFunc(slices.Clone(earlier), func(value string) bool { return value == originHead })
	if len(earlier) == 0 {
		return rewriteNone, nil, nil
	}

	originBelowEarlier := false
	for _, value := range slices.Backward(earlier) {
		inOrigin, err := repo.IsAncestor(value, originHead)
		if err != nil {
			return rewriteNone, nil, err
		}
		originAbove, err := repo.IsAncestor(originHead, value)
		if err != nil {
			return rewriteNone, nil, err
		}
		originBelowEarlier = originBelowEarlier || originAbove
		if inOrigin || !originAbove {
			continue
		}
		onlyInValue, err := repo.CommitsNotIn(value, originHead)
		if err != nil {
			return rewriteNone, nil, err
		}
		for _, commit := range onlyInValue {
			held, err := repo.IsAncestor(commit, localTip)
			if err != nil {
				return rewriteNone, nil, err
			}
			if held {
				dropped = append(dropped, commit)
			}
		}
		if len(dropped) > 0 {
			return rewriteRewound, dropped, nil
		}
	}
	if originBelowEarlier {
		return rewriteNone, nil, nil
	}

	localOnly, err := repo.CommitsNotIn(localTip, originHead)
	if err != nil {
		return rewriteNone, nil, err
	}
	if len(localOnly) == 0 {
		return rewriteNone, nil, nil
	}
	for _, commit := range localOnly {
		reached, err := reachedByAny(repo, commit, earlier)
		if err != nil {
			return rewriteNone, nil, err
		}
		if !reached {
			return rewriteNone, nil, nil
		}
	}
	return rewriteRewritten, nil, nil
}

// reachedByAny reports whether commit is an ancestor of, or equal to, any of values.
func reachedByAny(repo *gitrepo.Repo, commit string, values []string) (bool, error) {
	for _, value := range values {
		reached, err := repo.IsAncestor(commit, value)
		if err != nil {
			return false, err
		}
		if reached {
			return true, nil
		}
	}
	return false, nil
}
