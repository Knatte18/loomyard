// cache_test.go pins StatusCache.Poll's file contract: absent, found, unchanged-time skip, and the two-strike unreadable rule.

package statuspane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestStatusCache_Poll(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "status.json")
	cache := NewStatusCache()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	write := func(content []byte, modTime time.Time) {
		t.Helper()
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write status file: %v", err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set status file time: %v", err)
		}
	}
	statusJSON := func(producer string) []byte {
		t.Helper()
		data, err := json.Marshal(shedengine.Status{CurrentProducer: producer, State: shedengine.StateRunning})
		if err != nil {
			t.Fatalf("marshal status: %v", err)
		}
		return data
	}
	steps := []struct {
		name         string
		do           func()
		wantFound    bool
		wantProducer string
		wantUnread   bool
	}{
		{name: "AbsentFileIsNotFoundAndNoFailure", do: func() {}},
		{name: "WrittenStatusIsFound", do: func() { write(statusJSON("one"), base) }, wantFound: true, wantProducer: "one"},
		{name: "UnchangedModificationTimeIsNotReDecoded", do: func() { write(statusJSON("two"), base) }, wantFound: true, wantProducer: "one"},
		{name: "FirstUndecodableWriteKeepsThePreviousStatus", do: func() { write([]byte("{not json"), base.Add(time.Minute)) }, wantFound: true, wantProducer: "one"},
		{name: "SecondPollOfTheSameFailedWriteIsUnreadable", do: func() {}, wantUnread: true},
		{name: "GoodWriteClearsTheFailure", do: func() { write(statusJSON("three"), base.Add(2*time.Minute)) }, wantFound: true, wantProducer: "three"},
		{name: "RemovedFileIsNotFoundAgain", do: func() {
			if err := os.Remove(path); err != nil {
				t.Fatalf("remove status file: %v", err)
			}
		}},
	}
	for _, step := range steps {
		step.do()
		got := cache.Poll(path)
		if got.Found != step.wantFound || got.Unreadable != step.wantUnread || got.Status.CurrentProducer != step.wantProducer {
			t.Errorf("%s: Poll = {found %v, unreadable %v, producer %q}, want {found %v, unreadable %v, producer %q}",
				step.name, got.Found, got.Unreadable, got.Status.CurrentProducer, step.wantFound, step.wantUnread, step.wantProducer)
		}
	}
}
