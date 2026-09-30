// shedtransient.go maps a failure's own package-level classification onto shedengine's transient mark.

package shedtransient

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// Class returns the transient class of err, or the empty class when err is nil or not transient.
// A mark already on err wins, so a class set closer to the failure is never overridden;
// otherwise a never-ready agent start, a git transport failure and a transient GitHub API failure are tried in that order.
func Class(err error) shedengine.TransientClass {
	if err == nil {
		return ""
	}
	if class := shedengine.TransientOf(err); class != "" {
		return class
	}
	switch {
	case errors.Is(err, shuttleengine.ErrNotStarted):
		return shedengine.TransientAgentStart
	case gitexec.IsTransportFailure(err):
		return shedengine.TransientGitTransport
	case githubclient.IsTransient(err):
		return shedengine.TransientGitHubAPI
	}
	return ""
}

// Mark returns err marked with Class(err): err unchanged when nothing matches,
// and never double-wrapped when err already carries a mark.
func Mark(err error) error {
	if shedengine.TransientOf(err) != "" {
		return err
	}
	return shedengine.MarkTransient(Class(err), err)
}
