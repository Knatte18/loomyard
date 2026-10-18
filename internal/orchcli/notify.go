// notify.go declares NotifyPrime, the one way a task-worktree module sends a notice to the hub's prime orch as creel mail, and PrimeOrchStrand, the lookup of the strand that mail addresses.

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
	prime, err := resolvePrime(location)
	if err != nil {
		return false, err
	}
	clients := notifyClients{
		strand: func(strandClaim string) (notifyPoster, error) {
			return creelcli.NewStrandClient(location, strandClaim)
		},
		program: func() (notifyPoster, error) {
			return creelcli.NewProgramClient(location, program)
		},
	}
	return notifyPrime(func() (string, bool, error) { return PrimeOrchStrand(prime) }, line, os.Getenv(agentname.StrandNameEnv), clients)
}

// PrimeOrchStrand answers the name of the prime's orch strand and whether the prime's orch state records one.
// A GUID the prime's reed state no longer holds is answered under the fixed `<shortname>:orch` name, so creel decides whether that strand is listed.
func PrimeOrchStrand(prime *lyxcwd.Location) (name string, recorded bool, err error) {
	orch, err := orchengine.LoadState(PrimePaths(prime))
	if err != nil {
		return "", false, err
	}
	if orch.Strand == "" {
		return "", false, nil
	}
	reedState, err := reedengine.LoadState(filepath.Join(prime.AnchorPath(), lyxdirs.DotLyxDirName))
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
	shortname, found := fabricengine.ReadShortname(fabricengine.BoardDir(prime.HubPath))
	if !found {
		return "", false, fmt.Errorf("orch: the hub of %s has no usable shortname record to name the orch strand; way forward: run `lyx fabric shortname <shortname>`", prime.WorktreePath())
	}
	fixed, err := agentname.Format(shortname, "", agentname.RoleOrch)
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
