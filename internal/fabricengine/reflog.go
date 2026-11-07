// reflog.go reads a remote-tracking ref's reflog and judges what origin did to a branch this machine pushed or fetched.
// It reports which local commits origin already holds, and whether origin rewound or rewrote the branch.

package fabricengine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

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

// trackingReflog returns the values of the remote-tracking ref trackingRef in repoDir, newest last.
// The values start at the entry of the last push lym recorded, and include that entry.
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
		if strings.HasPrefix(subject, gitrepo.PushReflogSubject) {
			break
		}
	}
	slices.Reverse(values)
	return slices.Compact(values), nil
}

// classifyRewrite judges how origin's head relates to the earlier values of the tracking ref.
// localTip is the local tip of the same branch.
// It is rewriteRewound when origin's head is an ancestor of an earlier value that origin's head does not contain.
// The commits of that value that localTip holds and origin's head lacks are returned as dropped.
// It is rewriteRewritten when every local commit that origin's head lacks is reachable from an earlier value.
// That also needs origin's head to be an ancestor of none of the earlier values.
// The two never overlap, since whether origin's head is an ancestor of an earlier value separates them.
// Every other move is rewriteNone.
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
