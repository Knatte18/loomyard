// episode.go provides Episode and EpisodeSet, the guard for a failure that can repeat on every tick of a loop or poll:
// Warn at the first failure, Debug while it repeats, Info at recovery.

package logger

import "sync"

// failedTicksKey is the field Episode stamps on its repeat and recovery lines.
const failedTicksKey = "failed_ticks"

// Episode tracks one failure that can repeat on every tick.
// The zero value is ready; one owner uses it, with no lock.
// It stamps nothing of its own beyond failed_ticks: the caller's message and fields are logged as given.
type Episode struct {
	failedTicks int
}

// Fail counts one failure and logs msg with args.
// The first failure of an episode logs at Warn;
// every later one logs at Debug with failed_ticks carrying the count so far.
func (e *Episode) Fail(msg string, args ...any) {
	e.failedTicks++
	if e.failedTicks == 1 {
		Warn(msg, args...)
		return
	}
	Debug(msg, withFailedTicks(args, e.failedTicks)...)
}

// Recover ends the episode, logging msg with args and failed_ticks at Info.
// Outside an episode it does nothing, so the next Fail is a Warn again.
func (e *Episode) Recover(msg string, args ...any) {
	if e.failedTicks == 0 {
		return
	}
	Info(msg, withFailedTicks(args, e.failedTicks)...)
	e.failedTicks = 0
}

// EpisodeSet tracks one Episode per key, for a caller guarding several failures, or one per strand or entry, from one value.
// The zero value is ready and safe for concurrent use.
type EpisodeSet struct {
	mu       sync.Mutex
	episodes map[string]*Episode
}

// Fail counts one failure in key's episode, as Episode.Fail does.
func (s *EpisodeSet) Fail(key, msg string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.episodes == nil {
		s.episodes = map[string]*Episode{}
	}
	episode, ok := s.episodes[key]
	if !ok {
		episode = &Episode{}
		s.episodes[key] = episode
	}
	episode.Fail(msg, args...)
}

// Recover ends key's episode, as Episode.Recover does; a key with no episode does nothing.
func (s *EpisodeSet) Recover(key, msg string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	episode, ok := s.episodes[key]
	if !ok {
		return
	}
	episode.Recover(msg, args...)
	delete(s.episodes, key)
}

// withFailedTicks returns args followed by the failed_ticks field, without writing into args' backing array.
func withFailedTicks(args []any, failedTicks int) []any {
	stamped := make([]any, 0, len(args)+2)
	stamped = append(stamped, args...)
	return append(stamped, failedTicksKey, failedTicks)
}
