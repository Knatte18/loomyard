// follow.go follows the adopt record of a running worker until the adopt ends.
// It wakes no more than it must while it waits.
// It prints each step as it ends and returns the record that reads done or stopped.

package adoptcli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/adoptrecord"
)

// The follow's wake starts at followWakeFloor and doubles to followWakeCeiling while the record is unchanged.
const (
	followWakeFloor   = time.Second
	followWakeCeiling = 8 * time.Second
)

// followDeps holds the seams of the follow.
// Exited closes when the worker the caller spawned ends, and is nil when the caller spawned none.
// Cap bounds the follow; zero follows until the adopt ends.
// Steps receives one line for each step that ends.
type followDeps struct {
	ReadRecord func() (adoptrecord.Record, bool, error)
	Alive      func(pid int, startTicks string) bool
	Exited     <-chan struct{}
	Now        func() time.Time
	Sleep      func(time.Duration)
	Cap        time.Duration
	Steps      io.Writer
}

// follow reads the adopt record on each wake until the adopt of the worker with pid workerPID ends.
// It returns the record once it reads done or stopped.
// It returns an error for an interrupted worker, a vanished record and the cap.
// call is the caller's request, and since is when the caller started.
// When the spawned worker lost adopt.lock to a worker that started later, it follows that worker's record instead.
// It does so only when the later adopt names the same subjects.
func follow(deps followDeps, call adoptrecord.Request, workerPID int, since time.Time) (adoptrecord.Record, error) {
	pid, exited := workerPID, deps.Exited
	started := deps.Now()
	rerun := call.Command()
	wake := followWakeFloor
	printed := map[string]bool{}
	var previous string
	missed := false

	for {
		record, found, err := deps.ReadRecord()
		if err != nil {
			return adoptrecord.Record{}, err
		}
		exitedNow := channelClosed(exited)
		followed := found && record.PID == pid

		if found && !followed {
			switch {
			case exited == nil:
				return adoptrecord.Record{}, fmt.Errorf("lym adopt: the adopt followed ended and another started as process %d: `lym beam status` shows it, and `%s` follows or resumes it", record.PID, rerun)
			case exitedNow && record.Start.After(since):
				if !record.Request.SameSubjects(call) {
					return adoptrecord.Record{}, fmt.Errorf("lym adopt: this adopt lost the adopt lock to a running adopt of %s: `lym beam status` shows it, and `%s` runs once that adopt ends", strings.Join(record.Subjects, ", "), rerun)
				}
				pid, exited, printed = record.PID, nil, map[string]bool{}
				continue
			}
		}

		if followed {
			missed = false
			for _, step := range record.Steps {
				if !printed[step.Name] {
					printed[step.Name] = true
					fmt.Fprintf(deps.Steps, "adopt: %s done\n", step.Name)
				}
			}
			switch record.ReadState(deps.Alive) {
			case adoptrecord.StateDone, adoptrecord.StateStopped:
				return record, nil
			case adoptrecord.StateInterrupted:
				return adoptrecord.Record{}, fmt.Errorf("lym adopt: the worker of the adopt (process %d) is gone at the %s step: `%s` resumes it, or `lym adopt --discard` removes its record",
					record.PID, record.InFlight, record.Request.Command())
			}
		} else if exited == nil || exitedNow {
			if exited == nil || missed {
				return adoptrecord.Record{}, fmt.Errorf("lym adopt: the adopt left no record to follow: `lym beam status` shows any adopt, `%s` runs it again, and %s beside the record holds what a worker that refused or lost the lock printed", rerun, workerOutName)
			}
			missed = true
		}

		if deps.Cap > 0 && deps.Now().Sub(started) >= deps.Cap {
			return adoptrecord.Record{}, fmt.Errorf("lym adopt: the adopt is still running after %s: `lym beam status` shows it, and `%s` follows it again", deps.Cap, rerun)
		}

		fingerprint := fmt.Sprintf("%v/%d/%s/%s/%d/%v", found, record.PID, record.State, record.InFlight, len(record.Steps), exitedNow)
		if fingerprint != previous {
			wake = followWakeFloor
		} else {
			wake = min(wake*2, followWakeCeiling)
		}
		previous = fingerprint
		deps.Sleep(wake)
	}
}

// channelClosed reports whether ch is closed; a nil channel never is.
func channelClosed(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// followCap returns how long a follow may run for strand, the caller's full agent name.
// A plain shell, whose strand name is empty, follows until the worker ends and asks keepWarm for nothing.
// An agent session takes the cap keepWarm answers for its role.
func followCap(keepWarm func(strand string) time.Duration, strand string) time.Duration {
	if strand == "" {
		return 0
	}
	return keepWarm(strand)
}
