// dobbywatch.go opens dobby's file watch: one fswatch watcher per directory of interest, merged into the one channel the loop reads, and re-synced as runs appear.

package beamcli

import (
	"path/filepath"
	"sync"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fswatch"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// dobbyWatchDirs returns the lister of the directories dobby watches, each with the file names it cares about, empty meaning every entry.
// They are the prime's batten runs directory, where a new run appears, each run's directory for batten's own status file, each task pair's status directory for the child's, the board's `board.json` and the hub's pause record.
func dobbyWatchDirs(prime *lymcwd.Location, pauseRecordPath, boardDir string) func() map[string][]string {
	return func() map[string][]string {
		dirs := map[string][]string{
			shedrun.RunsRoot(prime):       nil,
			boardDir:                      {boardengine.BoardFileRel()},
			filepath.Dir(pauseRecordPath): {pauserecord.RecordFileName},
		}
		slugs, err := shedrun.List(prime)
		if err != nil {
			logger.Debug("beamcli: listing the runs for the watch failed", "error", err)
			return dirs
		}
		for _, slug := range slugs {
			battenStatus := shedrun.StatusFile(prime, slug)
			dirs[filepath.Dir(battenStatus)] = []string{filepath.Base(battenStatus)}
			pair, err := pairLocation(prime, slug)
			if err != nil {
				logger.Warn("beamcli: locating a pair for the watch failed; the pair is left out", "slug", slug, "error", err)
				continue
			}
			childStatus := shedrun.StatusFile(pair, shedrun.SelfRunID)
			dirs[filepath.Dir(childStatus)] = []string{filepath.Base(childStatus)}
		}
		return dirs
	}
}

// watchSet merges the events of many fswatch watchers into one channel and opens a watcher for each directory list names as it appears.
type watchSet struct {
	list    func() map[string][]string
	out     chan struct{}
	resync  chan struct{}
	done    chan struct{}
	workers sync.WaitGroup

	mu       sync.Mutex
	watchers map[string]*fswatch.Watcher
	closed   bool
}

// openDobbyWatch opens a watcher for every directory list names that exists, and returns the merged event channel with the function that releases them.
// A directory that does not exist yet, or that cannot be watched, is tried again at the next event; when none opens, the error of the last attempt is returned and the loop's timer drives alone.
func openDobbyWatch(list func() map[string][]string) (<-chan struct{}, func(), error) {
	set := &watchSet{
		list:     list,
		out:      make(chan struct{}, 1),
		resync:   make(chan struct{}, 1),
		done:     make(chan struct{}),
		watchers: map[string]*fswatch.Watcher{},
	}
	if err := set.sync(); err != nil && len(set.watchers) == 0 {
		return nil, nil, err
	}
	set.workers.Add(1)
	go set.resyncLoop()
	return set.out, set.close, nil
}

// sync opens a watcher for every listed directory that has none, and returns the last failure to open one.
func (s *watchSet) sync() error {
	var last error
	for dir, names := range s.list() {
		s.mu.Lock()
		_, watched := s.watchers[dir]
		closed := s.closed
		s.mu.Unlock()
		if watched || closed {
			continue
		}
		watcher, err := fswatch.Watch(dir, names...)
		if err != nil {
			last = err
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = watcher.Close()
			continue
		}
		s.watchers[dir] = watcher
		s.mu.Unlock()
		s.workers.Add(1)
		go s.forward(watcher)
	}
	return last
}

// forward turns each event of watcher into one signal on the merged channel and one re-sync request.
func (s *watchSet) forward(watcher *fswatch.Watcher) {
	defer s.workers.Done()
	for range watcher.Events() {
		select {
		case s.out <- struct{}{}:
		default:
		}
		select {
		case s.resync <- struct{}{}:
		default:
		}
	}
}

// resyncLoop opens the watchers a new directory needs, once per request, until the set is closed.
func (s *watchSet) resyncLoop() {
	defer s.workers.Done()
	for {
		select {
		case <-s.resync:
			_ = s.sync()
		case <-s.done:
			return
		}
	}
}

// close releases every watcher and waits for the goroutines that forward their events.
func (s *watchSet) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.done)
	for _, watcher := range s.watchers {
		_ = watcher.Close()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
