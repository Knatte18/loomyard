// fswatch.go implements Watch, the file-change event source over fsnotify.

package fswatch

import (
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Event is one change to an entry of the watched directory.
type Event struct {
	// Name is the entry's base name.
	Name string
	// Op is "create", "write", "rename" or "remove".
	Op string
}

// Watcher delivers the events of one watched directory.
type Watcher struct {
	watcher   *fsnotify.Watcher
	events    chan Event
	done      chan struct{}
	finished  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// Watch opens one fsnotify watcher on dir.
// Its Events channel delivers one Event per create, write, rename or remove of an entry whose base name is in names, or of any entry when names is empty.
// An fsnotify failure at open or add is returned, so the caller falls back to polling.
func Watch(dir string, names ...string) (*Watcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := inner.Add(dir); err != nil {
		_ = inner.Close()
		return nil, err
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	w := &Watcher{
		watcher:  inner,
		events:   make(chan Event),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	go w.forward(wanted)
	return w, nil
}

// Events returns the channel of matching events, closed by Close.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Close stops the watcher and closes the Events channel.
// It is safe to call more than once and returns the first call's result.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		close(w.done)
		w.closeErr = w.watcher.Close()
		<-w.finished
	})
	return w.closeErr
}

func (w *Watcher) forward(wanted map[string]struct{}) {
	defer close(w.finished)
	defer close(w.events)
	for {
		select {
		case raw, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			op := operationName(raw.Op)
			if op == "" {
				continue
			}
			name := filepath.Base(raw.Name)
			if _, ok := wanted[name]; len(wanted) > 0 && !ok {
				continue
			}
			select {
			case w.events <- Event{Name: name, Op: op}:
			case <-w.done:
				return
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			logger.Debug("fswatch: fsnotify error", "error", err)
		case <-w.done:
			return
		}
	}
}

func operationName(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Remove):
		return "remove"
	}
	return ""
}
