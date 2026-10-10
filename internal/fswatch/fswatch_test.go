// fswatch_test.go tests Watch against real files in a temp directory.

package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const eventTimeout = 10 * time.Second

func receive(t *testing.T, w *Watcher) Event {
	t.Helper()
	select {
	case event, ok := <-w.Events():
		if !ok {
			t.Fatal("Events closed before an event arrived")
		}
		return event
	case <-time.After(eventTimeout):
		t.Fatal("no event arrived")
	}
	return Event{}
}

func TestWatch_DeliversNamedEntryChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := Watch(dir, "wanted")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(filepath.Join(dir, "ignored"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wanted")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := receive(t, w); got != (Event{Name: "wanted", Op: "create"}) {
		t.Fatalf("create event = %+v", got)
	}
	if _, err := file.WriteString("data"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, w); got != (Event{Name: "wanted", Op: "write"}) {
		t.Fatalf("write event = %+v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, w); got != (Event{Name: "wanted", Op: "remove"}) {
		t.Fatalf("remove event = %+v", got)
	}
}

func TestWatch_NoNamesDeliversEveryEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, err := Watch(dir)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(filepath.Join(dir, "any"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, w); got != (Event{Name: "any", Op: "create"}) {
		t.Fatalf("event = %+v", got)
	}
}

func TestWatcher_CloseEndsChannel(t *testing.T) {
	t.Parallel()
	w, err := Watch(t.TempDir())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-w.Events():
		if ok {
			t.Fatal("Events delivered after Close")
		}
	case <-time.After(eventTimeout):
		t.Fatal("Events not closed by Close")
	}
}

func TestWatch_MissingDirectoryErrors(t *testing.T) {
	t.Parallel()
	if _, err := Watch(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("Watch on a missing directory returned no error")
	}
}
