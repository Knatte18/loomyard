// shuttle.go declares Handle, the type of a live half of a round as a probe reports it.

package burlerengine

import "github.com/Knatte18/loomyard/internal/shuttleengine"

// Handle is one live half of a round, as a probe reports it.
// A caller that will not resume the round stops it through Stop, which records the stop.
// seatengine.Handle and *shuttleengine.Run satisfy it.
type Handle interface {
	StrandGUID() string
	RunDir() string
	Wait() (shuttleengine.Result, error)
	Stop() error
}

var _ Handle = (*shuttleengine.Run)(nil)
