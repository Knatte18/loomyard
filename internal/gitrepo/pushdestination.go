// pushdestination.go resolves where a push lands.
// It yields the URL git sends to and the remote-tracking ref that records the branch locally.
// It also records a finished push in that ref and in the branch's upstream setting, under the remote-refs lock.

package gitrepo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// defaultPushRemote is the remote a default push goes to.
// It applies when neither the branch nor the repository names a push remote.
const defaultPushRemote = "origin"

// PushReflogSubject is the reflog subject of a remote-tracking ref a recorded push moves.
// A push's own recording writes it, and a reader of the reflog finds the last push boundary by it.
const PushReflogSubject = "update by push"

// pushDestination is where a push of one branch lands and how this repository records it.
// url is what git pushes to.
// A URL writes no remote-tracking ref, so the transfer runs outside the remote-refs lock.
// trackingRef is the ref the remote's fetch refspecs map the branch to.
// It is empty when the remote is not configured or no refspec maps the branch.
// setUpstream is set for a default push of a branch with no upstream yet.
// Such a push takes the remote's branch as its upstream, as git's push.autoSetupRemote does.
type pushDestination struct {
	remote      string
	branch      string
	url         string
	trackingRef string
	setUpstream bool
}

// resolvePushDestination returns where a push of branch to remote lands.
// An empty remote means the default push of the checked-out branch.
// Its remote is, in order: the branch's push remote, the repository's push default, the upstream remote, or origin.
// A remote that is not a configured remote is pushed to as a URL or path, and records nothing.
func (r *Repo) resolvePushDestination(remote, branch string) (pushDestination, error) {
	dest, err := readGoGit(r, func(repo *git.Repository) (pushDestination, error) {
		cfg, err := repo.Config()
		if err != nil {
			return pushDestination{}, err
		}
		dest := pushDestination{remote: remote, branch: branch}
		if remote == "" {
			upstream := cfg.Branches[branch]
			candidates := []string{cfg.Raw.Section("branch").Subsection(branch).Option("pushRemote"), cfg.Raw.Section("remote").Option("pushDefault")}
			if upstream != nil {
				candidates = append(candidates, upstream.Remote)
			}
			dest.remote = defaultPushRemote
			for _, candidate := range candidates {
				if candidate != "" {
					dest.remote = candidate
					break
				}
			}
			dest.setUpstream = upstream == nil || upstream.Remote == "" || upstream.Merge == ""
		}
		remoteConfig, configured := cfg.Remotes[dest.remote]
		if !configured {
			dest.url = dest.remote
			dest.setUpstream = false
			return dest, nil
		}
		branchRef := plumbing.NewBranchReferenceName(branch)
		for _, spec := range remoteConfig.Fetch {
			if spec.Match(branchRef) {
				dest.trackingRef = spec.Dst(branchRef).String()
				break
			}
		}
		return dest, nil
	})
	if err != nil {
		return pushDestination{}, fmt.Errorf("gitrepo: resolve the push destination of %s in %s: %w", branch, r.path, err)
	}
	if dest.url != "" {
		return dest, nil
	}
	url, err := r.runChecked("remote", "get-url", "--push", dest.remote)
	if err != nil {
		return pushDestination{}, fmt.Errorf("gitrepo: read the push URL of remote %s in %s: %w", dest.remote, r.path, err)
	}
	dest.url = strings.TrimSpace(url)
	return dest, nil
}

// recordPush records a finished push to dest in one hold of the remote-refs lock.
// The tracking ref moves to sha, or is deleted when sha is empty,
// and the branch takes its upstream when dest asks for one.
// With keepNewer set, a tracking ref already at a descendant of sha stays there.
// A fetch that ran after the push reads origin past sha, which is why the newer ref is kept.
// A destination with nothing to record takes no lock.
// Its callers log a failure and never return it, since the transfer it records has already landed.
func (r *Repo) recordPush(dest pushDestination, sha string, keepNewer bool) error {
	if dest.trackingRef == "" && !dest.setUpstream {
		return nil
	}
	return r.withRemoteRefs(func() error {
		if dest.trackingRef != "" {
			if err := r.moveTrackingRef(dest.trackingRef, sha, keepNewer); err != nil {
				return fmt.Errorf("gitrepo: record the push of %s in %s: %w", dest.branch, dest.trackingRef, err)
			}
		}
		if !dest.setUpstream {
			return nil
		}
		for _, setting := range [][2]string{{"branch." + dest.branch + ".remote", dest.remote}, {"branch." + dest.branch + ".merge", "refs/heads/" + dest.branch}} {
			if _, err := r.runChecked("config", setting[0], setting[1]); err != nil {
				return fmt.Errorf("gitrepo: set the upstream of %s: %w", dest.branch, err)
			}
		}
		return nil
	})
}

// moveTrackingRef moves trackingRef to sha, or deletes it when sha is empty; the caller holds the remote-refs lock.
// With keepNewer set, a ref already at a descendant of sha stays where it is.
func (r *Repo) moveTrackingRef(trackingRef, sha string, keepNewer bool) error {
	current, err := r.RefSHA(trackingRef)
	absent := errors.Is(err, ErrRefNotFound)
	if err != nil && !absent {
		return err
	}
	switch {
	case sha == "" && absent, current == sha:
		return nil
	case sha == "":
		_, err := r.runChecked("update-ref", "-m", PushReflogSubject, "-d", trackingRef)
		return err
	}
	if keepNewer && !absent {
		newer, err := r.IsAncestor(sha, current)
		if err != nil || newer {
			return err
		}
	}
	_, err = r.runChecked("update-ref", "-m", PushReflogSubject, trackingRef, sha)
	return err
}
