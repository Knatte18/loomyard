// notify.go sends a notice to an orch as creel mail.
// Each Notify function differs in which orch it reaches from a location,
// and the lookups here resolve the orch strand that mail addresses.

package orchcli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchroute"
)

// notifyPoster is the one creel call a notice makes: leaving a body as mail for a receiver, carrying tags.
type notifyPoster interface {
	PostTagged(receiver, body string, tags creelengine.PostTags) (int64, error)
}

// notifyClients holds the functions that build the creel clients a notice posts through.
// One signs as a strand, and the other as the program.
type notifyClients struct {
	strand  func(strandClaim string) (notifyPoster, error)
	program func() (notifyPoster, error)
}

// NotifyPrime sends line as creel mail to the orch strand of the prime of the hub that location's worktree belongs to.
// It is signed as program, and it reports whether the notice was posted.
// A prime with no orch strand recorded is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyPrime(location *lymcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(location, "", program, line, creelengine.PostTags{}, func() (string, bool, error) { return PrimeOrchStrand(location) })
}

// NotifyParent sends line as creel mail to the nearest live orch up the beam chain of the pair's parent.
// It is signed as program, and it reports whether the notice was posted.
// The mail is tagged with the pair's slug as its run, so the pair's teardown drops it while it is unread.
// The destination is resolved at each call; the prime's orch is the answer when no orch above the pair is live.
// A destination with no orch strand recorded is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyParent(pair *lymcwd.Location, program, line string) (posted bool, err error) {
	tags := creelengine.PostTags{Run: pair.WorktreeName}
	return postNotice(pair, "", program, line, tags, func() (string, bool, error) { return parentOrchStrand(pair, OrchLive) })
}

// NotifyFrom sends line as creel mail to the nearest live orch on the beam chain at or above from.
// It is signed as program, and it reports whether the notice was posted.
// It serves a notice of a run whose pair is absent, which the run's beam worktree stands in for.
func NotifyFrom(from *lymcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(from, "", program, line, creelengine.PostTags{}, func() (string, bool, error) { return nearestOrchStrand(from, OrchLive) })
}

// NotifyFromScoped sends line as creel mail to the nearest live orch on the beam chain at or above from.
// It is signed as program scoped to slug, as "<shortname>:<slug>:<NAME>", and it reports whether the notice was posted.
func NotifyFromScoped(from *lymcwd.Location, slug, program, line string) (posted bool, err error) {
	return postNotice(from, slug, program, line, creelengine.PostTags{}, func() (string, bool, error) { return nearestOrchStrand(from, OrchLive) })
}

// NotifyRunFrom sends line as creel mail carrying tags to the nearest live orch on the beam chain at or above from.
// It is signed as program scoped to slug, the unscoped program for an empty slug,
// and it reports whether the notice was posted.
// It serves a notice about one run: its run tag lets the run's teardown drop it unread,
// and its topic lets a newer notice of the same topic replace it unread.
func NotifyRunFrom(from *lymcwd.Location, slug, program string, tags creelengine.PostTags, line string) (posted bool, err error) {
	return postNotice(from, slug, program, line, tags, func() (string, bool, error) { return nearestOrchStrand(from, OrchLive) })
}

// NotifyAsker sends line as creel mail to the orch the agents of location reach as their parent.
// It is signed as program, and it reports whether the notice was posted.
// It serves a hold notice.
// The walk to the nearest live orch starts at the pair's recorded parent when location is a pair.
// Otherwise, for a beam or the prime, it starts at location itself.
// A destination with no orch strand recorded is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyAsker(location *lymcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(location, "", program, line, creelengine.PostTags{}, func() (string, bool, error) { return askerOrchStrand(location, OrchLive) })
}

// NotifyOrchAt sends line as creel mail to the orch strand of worktree itself.
// It is signed as program, and it reports whether the notice was posted.
// It takes no walk up the beam chain.
// A worktree whose orch state records no strand is not an error: the notice is logged and nothing is posted.
// A refusal or failure comes back as an error carrying the full line, and the notice is not retried.
func NotifyOrchAt(worktree *lymcwd.Location, program, line string) (posted bool, err error) {
	return postNotice(worktree, "", program, line, creelengine.PostTags{}, func() (string, bool, error) { return OrchStrandOf(worktree) })
}

// ParentOrchRecorded reports whether the orch a notice of pair goes to records an orch strand.
// The answer decides whether a run's driver is watched or silent.
func ParentOrchRecorded(pair *lymcwd.Location) (bool, error) {
	_, recorded, err := parentOrchStrand(pair, OrchLive)
	return recorded, err
}

// postNotice posts line carrying tags through creel to the orch strand orch answers.
// It signs the call as program scoped to slug from location, and an empty slug signs as the unscoped program.
func postNotice(location *lymcwd.Location, slug, program, line string, tags creelengine.PostTags, orch func() (string, bool, error)) (bool, error) {
	clients := notifyClients{
		strand: func(strandClaim string) (notifyPoster, error) {
			return creelcli.NewStrandClient(location, strandClaim)
		},
		program: func() (notifyPoster, error) {
			return creelcli.NewScopedProgramClient(location, slug, program)
		},
	}
	return notifyPrime(orch, line, tags, os.Getenv(agentname.StrandNameEnv), clients)
}

// NearestLiveParent answers the orch a notice of pair goes to, as the orch's agent name and worktree.
// It is the nearest worktree up from the pair's parent whose orch is live, else the prime's.
// The answer's Worktree is empty for the prime.
func NearestLiveParent(pair *lymcwd.Location) (hubgeom.Parent, error) {
	return nearestLiveParent(pair, OrchLive)
}

// nearestLiveParent answers the orch a notice of pair goes to, deciding each candidate orch's liveness with live.
func nearestLiveParent(pair *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (hubgeom.Parent, error) {
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

// askerOrchStrand answers the orch strand a hold notice of location goes to, and whether that orch's state records one.
func askerOrchStrand(location *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	return orchroute.Asker(location, live)
}

// parentOrchStrand answers the orch strand a notice of pair goes to.
// It is the nearest worktree up from the pair's parent whose orch live reports live, else the prime's.
func parentOrchStrand(pair *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	return orchroute.FromPair(pair, live)
}

// nearestOrchStrand answers the orch strand of the nearest worktree on the beam chain at or above start.
// It is the nearest one whose orch live reports live.
// It falls back to the prime's orch strand when none is live.
func nearestOrchStrand(start *lymcwd.Location, live func(worktree *lymcwd.Location) (bool, error)) (name string, recorded bool, err error) {
	return orchroute.From(start, live)
}

// worktreeOrPrime resolves the hub worktree named worktree.
// It falls back to the prime of the hub of location when worktree is empty or no longer exists.
func worktreeOrPrime(location *lymcwd.Location, worktree string) (*lymcwd.Location, error) {
	return orchroute.WorktreeOrPrime(location, worktree)
}

// PrimeOrchStrand answers the name of the orch strand of the prime of the hub that location's worktree belongs to.
// It reports whether the prime's orch state records one.
// A GUID the prime's reed state no longer holds is answered under the fixed `<shortname>:orch` name.
// Creel decides whether that strand is listed.
func PrimeOrchStrand(location *lymcwd.Location) (name string, recorded bool, err error) {
	prime, err := resolvePrime(location)
	if err != nil {
		return "", false, err
	}
	return OrchStrandOf(prime)
}

// OrchStrandOf answers the name of the orch strand of worktree and whether its orch state records one.
// The name is the one reed's state holds for the recorded GUID.
// A GUID reed no longer holds is answered under a fixed name:
// `<shortname>:orch` for the prime and `<shortname>:<slug>:orch` for a beam.
// Creel decides whether that strand is listed.
func OrchStrandOf(worktree *lymcwd.Location) (name string, recorded bool, err error) {
	return orchroute.StrandOf(worktree)
}

// resolvePrime resolves the prime worktree of the hub that location's worktree belongs to.
func resolvePrime(location *lymcwd.Location) (*lymcwd.Location, error) {
	return orchroute.Prime(location)
}

// notifyPrime posts line carrying tags to the orch strand orch answers.
// A strand claim naming a strand that is not an orch is tried first.
// A claim creel refuses as coming from outside that strand's pane falls back to posting once as the program.
func notifyPrime(orch func() (string, bool, error), line string, tags creelengine.PostTags, strandClaim string, clients notifyClients) (bool, error) {
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
		_, err = poster.PostTagged(receiver, line, tags)
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
	if _, err := poster.PostTagged(receiver, line, tags); err != nil {
		return false, noticeError(line, err)
	}
	return true, nil
}

// claimsNonOrchStrand reports whether strandClaim names a strand whose role is not an orch's.
func claimsNonOrchStrand(strandClaim string) bool {
	name, err := agentname.Parse(strandClaim)
	return err == nil && !agentname.MatchesRole(name.Role, agentname.RoleOrch)
}

// noticeError wraps a failed post with the notice's full line.
// The caller's warning then holds the line, so the operator can recover it.
func noticeError(line string, err error) error {
	return fmt.Errorf("orch: the notice was not delivered: %w; notice: %s", err, line)
}
