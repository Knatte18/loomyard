// watchloop.go owns the file-event seam the daemon's wait loops block on: FileWatchOpener and its production opener.

package reedengine

import (
	"github.com/Knatte18/loomyard/internal/fswatch"
)

// FileWatchOpener opens a file-event source on dir for the entries called names.
// It returns the event channel and the func that closes the source.
// It is the seam the daemon's wait loops block on, so a test passes an opener returning a channel it feeds.
type FileWatchOpener func(dir string, names ...string) (<-chan fswatch.Event, func() error, error)

// OpenFileWatch opens an fswatch watcher on dir for the entries called names.
// It returns the watcher's event channel and its Close method.
// It is the production implementation of the opener seam, which tests replace.
func OpenFileWatch(dir string, names ...string) (<-chan fswatch.Event, func() error, error) {
	watcher, err := fswatch.Watch(dir, names...)
	if err != nil {
		return nil, nil, err
	}
	return watcher.Events(), watcher.Close, nil
}
