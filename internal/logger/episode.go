// episode.go provides Episode and EpisodeSet, which guard a failure that repeats on every tick of a loop or poll.
// The first failure logs at Warn, each repeat at Debug, and the recovery at Info.

package logger

import "sync"

// failedTicksKey is the log field that carries an episode's failed-tick count on repeat and recovery lines.
const failedTicksKey = "failed_ticks"

// Episode tracks one failure that can repeat on every tick.
// The zero value is ready to use; it is not safe for concurrent use, so one owner calls it without a lock.
// Its only added field is failed_ticks: the caller's message and fields are logged as given.
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
// Outside an episode it does nothing; the next failure then logs at Warn.
func (e *Episode) Recover(msg string, args ...any) {
	if e.failedTicks == 0 {
		return
	}
	Info(msg, withFailedTicks(args, e.failedTicks)...)
	e.failedTicks = 0
}

// EpisodeSet tracks one Episode per key.
// It suits a caller that guards several failures from one value, or one failure per strand or entry.
// The zero value is ready and safe for concurrent use.
type EpisodeSet struct {
	mu       sync.Mutex
	episodes map[string]*Episode
}

// Fail counts one failure in key's episode and logs msg with args.
// The first failure of an episode logs at Warn;
// every later one logs at Debug with failed_ticks carrying the count so far.
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

// Recover ends key's episode, logging msg with args and failed_ticks at Info.
// A key with no open episode logs nothing.
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

// Ticks returns a copy of each open episode's failed-tick count, by key.
func (s *EpisodeSet) Ticks() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticks := make(map[string]int, len(s.episodes))
	for key, episode := range s.episodes {
		ticks[key] = episode.failedTicks
	}
	return ticks
}

// Restore replaces the set's episodes with ticks, one open episode per key at the given failed-tick count.
// A restored key's next failure continues its episode at Debug, and its recovery logs at Info.
// A key absent from ticks starts a new episode on its next failure.
func (s *EpisodeSet) Restore(ticks map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.episodes = make(map[string]*Episode, len(ticks))
	for key, failedTicks := range ticks {
		s.episodes[key] = &Episode{failedTicks: failedTicks}
	}
}

// withFailedTicks returns args followed by the failed_ticks field, without writing into args' backing array.
func withFailedTicks(args []any, failedTicks int) []any {
	stamped := make([]any, 0, len(args)+2)
	stamped = append(stamped, args...)
	return append(stamped, failedTicksKey, failedTicks)
}
