// notify.go declares the ways a module sends a notice to an orch as creel mail, NotifyPrime to the hub's prime orch, NotifyParent and NotifyFrom to the nearest live orch up a beam chain, NotifyOrchAt to one named worktree's orch, and the lookups of the strand that mail addresses.

package orchcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// notifyPoster is the one creel call a notice makes: leaving a body as mail for a receiver.
type notifyPoster interface {
	Post(receiver, body string) (int64, error)
}

// notifyClients builds the creel clients a notice posts through: one signing as a strand, one signing as the calling program.
type notifyClients struct {
	strand  func(strandClaim string) (notifyPoster, error)
	program func() (notifyPoster, error)
}

// NotifyPrime sends line as creel mail to the orch strand of the prime of the hub that location's worktree belongs to, signed as the module's program, and reports whether it was posted.
// A prime with no orch strand recorded is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyPrime(location *lyxcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(location, program, line, func() (string, bool, error) { return PrimeOrchStrand(location) })
}

// NotifyParent sends line as creel mail, signed as the module's program, to the nearest live orch up the beam chain of the pair's parent, and reports whether it was posted.
// The destination is resolved at each call; the prime's orch is the answer when no orch above the pair is live.
// A destination with no orch strand recorded is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyParent(pair *lyxcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(pair, program, line, func() (string, bool, error) { return parentOrchStrand(pair, OrchLive) })
}

// NotifyFrom sends line like NotifyParent, with the walk to the nearest live orch starting at from itself.
// It serves a notice of a run whose pair is absent, which the run's beam worktree stands in for.
func NotifyFrom(from *lyxcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(from, program, line, func() (string, bool, error) { return nearestOrchStrand(from, OrchLive) })
}

// NotifyOrchAt sends line as creel mail to the orch strand of worktree itself, signed as the module's program, and reports whether it was posted.
// It takes no walk up the beam chain.
// A worktree whose orch state records no strand is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyOrchAt(worktree *lyxcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(worktree, program, line, func() (string, bool, error) { return OrchStrandOf(worktree) })
}

// ParentOrchRecorded reports whether the destination NotifyParent resolves for pair records an orch strand, which is what decides a run's driver to be watched or silent.
func ParentOrchRecorded(pair *lyxcwd.Location) (bool, error) {
	_, recorded, err := parentOrchStrand(pair, OrchLive)
	return recorded, err
}

// postNotice posts line through creel to the orch strand orch answers, signing the call as program from location.
func postNotice(location *lyxcwd.Location, program, line string, orch func() (string, bool, error)) (bool, error) {
	clients := notifyClients{
		strand: func(strandClaim string) (notifyPoster, error) {
			return creelcli.NewStrandClient(location, strandClaim)
		},
		program: func() (notifyPoster, error) {
			return creelcli.NewProgramClient(location, program)
		},
	}
	return notifyPrime(orch, line, os.Getenv(agentname.StrandNameEnv), clients)
}

// NearestLiveParent answers the orch a notice of pair goes to, as the orch's agent name and worktree: the nearest worktree up from the pair's parent whose orch is live, else the prime's.
// The answer's Worktree is empty for the prime.
func NearestLiveParent(pair *lyxcwd.Location) (hubgeom.Parent, error) {
	return nearestLiveParent(pair, OrchLive)
}

// nearestLiveParent is NearestLiveParent over a told liveness predicate.
func nearestLiveParent(pair *lyxcwd.Location, live func(worktree *lyxcwd.Location) (bool, error)) (hubgeom.Parent, error) {
	parent, err := hubgeom.ResolveParent(pair)
	if err != nil {
		return hubgeom.Parent{}, err
	}
	start, err := worktreeOrPrime(pair, parent.Worktree)
	if err != nil {
		return hubgeom.Parent{}, err
	}
	return hubgeom.NearestLiveOrch(start, live)
}

// parentOrchStrand answers the orch strand a notice of pair goes to: the nearest worktree up from the pair's parent whose orch live reports live, else the prime's.
// The parent worktree comes from the pair's origin record, and the prime stands in when it names none or is gone.
func parentOrchStrand(pair *lyxcwd.Location, live func(worktree *lyxcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	parent, err := hubgeom.ResolveParent(pair)
	if err != nil {
		return "", false, err
	}
	start, err := worktreeOrPrime(pair, parent.Worktree)
	if err != nil {
		return "", false, err
	}
	return nearestOrchStrand(start, live)
}

// nearestOrchStrand answers the orch strand of the nearest worktree, from start up its beam chain, whose orch live reports live, else the prime's.
func nearestOrchStrand(start *lyxcwd.Location, live func(worktree *lyxcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	destination, err := hubgeom.NearestLiveOrch(start, live)
	if err != nil {
		return "", false, err
	}
	worktree, err := worktreeOrPrime(start, destination.Worktree)
	if err != nil {
		return "", false, err
	}
	return OrchStrandOf(worktree)
}

// worktreeOrPrime resolves the hub worktree named worktree, and the prime of the hub of location when worktree is empty or no longer exists.
func worktreeOrPrime(location *lyxcwd.Location, worktree string) (*lyxcwd.Location, error) {
	if worktree != "" {
		root := filepath.Join(location.HubPath, worktree)
		if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
			return lyxcwd.ResolveWorktree(root)
		}
	}
	return resolvePrime(location)
}

// PrimeOrchStrand answers the name of the orch strand of the prime of the hub that location's worktree belongs to, and whether the prime's orch state records one.
// A GUID the prime's reed state no longer holds is answered under the fixed `<shortname>:orch` name, so creel decides whether that strand is listed.
func PrimeOrchStrand(location *lyxcwd.Location) (name string, recorded bool, err error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return "", false, err
	}
	return OrchStrandOf(prime)
}

// OrchStrandOf answers the name of the orch strand of worktree and whether its orch state records one.
// The name is the one reed's state holds for the recorded GUID; a GUID reed no longer holds is answered under the fixed name, `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam, so creel decides whether that strand is listed.
func OrchStrandOf(worktree *lyxcwd.Location) (name string, recorded bool, err error) {
	orch, err := orchengine.LoadState(orchPaths(worktree))
	if err != nil {
		return "", false, err
	}
	if orch.Strand == "" {
		return "", false, nil
	}
	reedState, err := reedengine.LoadState(filepath.Join(worktree.AnchorPath(), lyxdirs.DotLyxDirName))
	if err != nil {
		return "", false, err
	}
	if reedState != nil {
		for _, strand := range reedState.Strands {
			if strand.GUID == orch.Strand {
				return strand.Name, true, nil
			}
		}
	}
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(worktree.HubPath))
	if !found {
		return "", false, fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lyx fabric shortname <shortname>`", worktree.WorktreePath())
	}
	primeName, err := fabricengine.PrimeName(worktree)
	if err != nil {
		return "", false, err
	}
	slug := worktree.WorktreeName
	if slug == primeName {
		slug = ""
	}
	fixed, err := agentname.Format(shortname, slug, agentname.RoleOrch)
	if err != nil {
		return "", false, err
	}
	return fixed, true, nil
}

// resolvePrime resolves the prime worktree of the hub that location's worktree belongs to.
func resolvePrime(location *lyxcwd.Location) (*lyxcwd.Location, error) {
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, err
	}
	return lyxcwd.ResolveWorktree(fabricengine.WorktreePath(location, primeName))
}

// notifyPrime posts line to the orch strand orch answers.
// A strand claim naming a strand that is not an orch is tried first, and a claim creel refuses as coming from outside that strand's pane falls back to posting once as the program.
func notifyPrime(orch func() (string, bool, error), line, strandClaim string, clients notifyClients) (bool, error) {
	receiver, recorded, err := orch()
	if err != nil {
		return false, err
	}
	if !recorded {
		logger.Info("orch: notice not posted, no orch strand recorded", "line", line)
		return false, nil
	}
	if claimsNonOrchStrand(strandClaim) {
		poster, err := clients.strand(strandClaim)
		if err != nil {
			return false, noticeError(line, err)
		}
		_, err = poster.Post(receiver, line)
		switch {
		case err == nil:
			return true, nil
		case !errors.Is(err, creelengine.ErrStrandClaimOutsidePane):
			return false, noticeError(line, err)
		}
	}
	poster, err := clients.program()
	if err != nil {
		return false, noticeError(line, err)
	}
	if _, err := poster.Post(receiver, line); err != nil {
		return false, noticeError(line, err)
	}
	return true, nil
}

// claimsNonOrchStrand reports whether strandClaim names a strand whose role is not an orch's.
func claimsNonOrchStrand(strandClaim string) bool {
	name, err := agentname.Parse(strandClaim)
	return err == nil && !agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// noticeError wraps a failed post with the notice's full line, so the caller's warning holds the line for the operator to recover.
func noticeError(line string, err error) error {
	return fmt.Errorf("orch: the notice was not delivered: %w; notice: %s", err, line)
}
