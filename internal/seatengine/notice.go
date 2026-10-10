// notice.go declares the chair-side notice sender, which types one-line advisor notices into the chair's session, and the wording of an advisor's ending notice.

package seatengine

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// advisorNotice returns the one-line notice that tells the chair advisor name ended without finishing.
// An empty outcome stands for a wait that errored.
func advisorNotice(name string, outputs []string, outcome shuttleengine.Outcome) string {
	ended := "ended"
	if outcome != "" {
		ended = fmt.Sprintf("ended (%s)", outcome)
	}
	return fmt.Sprintf("Advisor %s %s and no longer answers; its outputs stand where they exist: %s", name, ended, strings.Join(outputs, ", "))
}

// noticeSender types queued notice lines into one chair's session, one line at a time.
// Its single goroutine is the only writer to the chair's pane, so two lines are never interleaved.
type noticeSender struct {
	chairOutputs []string
	retry        time.Duration

	mu      sync.Mutex
	pending []string
	sent    []string
	stopped bool
	started bool

	wake     chan struct{}
	stopping chan struct{}
	finished chan struct{}
}

// newNoticeSender returns a sender that holds queued lines until start is called.
// It stops delivering once every path in chairOutputs exists, and waits retry between attempts at a line the chair's session could not take.
func newNoticeSender(chairOutputs []string, retry time.Duration) *noticeSender {
	return &noticeSender{
		chairOutputs: chairOutputs,
		retry:        retry,
		wake:         make(chan struct{}, 1),
		stopping:     make(chan struct{}),
		finished:     make(chan struct{}),
	}
}

// queue adds line to the lines still to be typed; a line queued after stop is dropped.
func (s *noticeSender) queue(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.pending = append(s.pending, line)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// start begins typing the queued lines into chair's session.
func (s *noticeSender) start(chair Handle) {
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	go s.drain(chair)
}

// stop ends delivery, drops every line not yet sent and returns once the goroutine has exited.
func (s *noticeSender) stop() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.stopping)
	}
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.finished
	}
}

// wasSent reports whether line landed in the chair's session.
func (s *noticeSender) wasSent(line string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.sent, line)
}

// drain types the queued lines until the sender is stopped or the chair's outputs all exist.
func (s *noticeSender) drain(chair Handle) {
	defer close(s.finished)
	for {
		line, ok := s.next()
		if !ok {
			return
		}
		if !s.deliver(chair, line) {
			return
		}
	}
}

// next blocks until a line is queued and returns it, or reports false once the sender is stopped.
func (s *noticeSender) next() (string, bool) {
	for {
		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return "", false
		}
		if len(s.pending) > 0 {
			line := s.pending[0]
			s.pending = s.pending[1:]
			s.mu.Unlock()
			return line, true
		}
		s.mu.Unlock()
		select {
		case <-s.wake:
		case <-s.stopping:
			return "", false
		}
	}
}

// deliver types line into the chair's session, retrying a busy or unlanded send after the retry interval.
// It reports false when delivery is over: the chair's outputs all exist or the sender was stopped.
// A line that never lands, or that fails for another reason, is logged and dropped.
func (s *noticeSender) deliver(chair Handle, line string) bool {
	for {
		if allExist(s.chairOutputs) {
			return false
		}
		err := chair.Send(line)
		if err == nil {
			s.mu.Lock()
			s.sent = append(s.sent, line)
			s.mu.Unlock()
			return true
		}
		if !errors.Is(err, shuttleengine.ErrSessionBusy) && !errors.Is(err, shuttleengine.ErrSubmissionNotLanded) {
			logger.Warn("seatengine: advisor notice dropped", "notice", line, "error", err)
			return true
		}
		timer := time.NewTimer(s.retry)
		select {
		case <-timer.C:
		case <-s.stopping:
			timer.Stop()
			logger.Warn("seatengine: advisor notice never landed before the sender stopped", "notice", line)
			return false
		}
	}
}

// allExist reports whether every path exists; an empty list is not all-existing, so a sender with no outputs to watch never stops itself.
func allExist(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}
