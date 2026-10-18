// creelseam_test.go covers the folds of creel's journal entries into the watcher's outcome and sent values.

package orchcli

import (
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// TestCreelSeam_FoldsTheJournal drives InjectOutcome and Sent over scripted journal entries.
func TestCreelSeam_FoldsTheJournal(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	at := func(seconds int) time.Time { return t0.Add(time.Duration(seconds) * time.Second) }
	posted := creelengine.LogEntry{Event: "posted", ID: 7, At: at(0), Kind: "inject", Command: creelengine.CommandClear}
	sending := func(seconds int) creelengine.LogEntry {
		return creelengine.LogEntry{Event: "sending", ID: 7, At: at(seconds)}
	}

	outcomes := []struct {
		name    string
		entries []creelengine.LogEntry
		want    orchengine.CreelOutcome
	}{
		{"a posted inject is queued", []creelengine.LogEntry{posted}, orchengine.CreelOutcome{State: orchengine.CreelQueued}},
		{"a sending inject is sending since its sending record", []creelengine.LogEntry{posted, sending(1)}, orchengine.CreelOutcome{State: orchengine.CreelSending, SendingAt: at(1)}},
		{
			"an injected inject keeps the time of its newest sending record, the typed attempt after a busy one",
			[]creelengine.LogEntry{posted, sending(1), sending(5), {Event: "injected", ID: 7, At: at(6)}},
			orchengine.CreelOutcome{State: orchengine.CreelInjected, SendingAt: at(5)},
		},
		{
			"an unconfirmed inject",
			[]creelengine.LogEntry{posted, sending(1), {Event: "unconfirmed", ID: 7, At: at(2)}},
			orchengine.CreelOutcome{State: orchengine.CreelUnconfirmed, SendingAt: at(1)},
		},
		{
			"an expired inject names the stall of its expired record",
			[]creelengine.LogEntry{posted, {Event: "expired", ID: 7, At: at(2), Reason: string(creelengine.StallNoInputBox)}},
			orchengine.CreelOutcome{State: orchengine.CreelExpired, Stall: string(creelengine.StallNoInputBox)},
		},
		{"an inject expired at its TTL names no stall", []creelengine.LogEntry{posted, {Event: "expired", ID: 7, At: at(2)}}, orchengine.CreelOutcome{State: orchengine.CreelExpired}},
		{
			"an inject expired by a withdraw reads as withdrawn",
			[]creelengine.LogEntry{posted, {Event: "expired", ID: 7, At: at(2), Reason: creelengine.WithdrawnReason}},
			orchengine.CreelOutcome{State: orchengine.CreelWithdrawnState},
		},
		{
			"another inject's entries are ignored",
			[]creelengine.LogEntry{posted, {Event: "injected", ID: 8, At: at(2)}},
			orchengine.CreelOutcome{State: orchengine.CreelQueued},
		},
	}
	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeCreelClient{entries: tt.entries}
			got, err := creelSeam{client: client}.InjectOutcome(7)
			if err != nil {
				t.Fatalf("InjectOutcome(7) error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("InjectOutcome(7) = %+v; want %+v", got, tt.want)
			}
			if want := []creelengine.LogFilter{{ID: 7}}; !reflect.DeepEqual(client.filters, want) {
				t.Errorf("Log filters = %+v; want %+v", client.filters, want)
			}
		})
	}

	t.Run("Sent lists each post and inject the watcher sent with its step and subject", func(t *testing.T) {
		t.Parallel()
		client := &fakeCreelClient{shortname: "tst", entries: []creelengine.LogEntry{
			{Event: "posted", ID: 1, At: at(1), Kind: "mail", Subject: "write the note"},
			{Event: "posted", ID: 2, At: at(2), Kind: "inject", Command: creelengine.CommandCompact, Argument: "keep the plan", Subject: ""},
			{Event: "posted", ID: 3, At: at(3), Kind: "inject", Subject: "carry on"},
			{Event: "read", ID: 1, At: at(4)},
		}}
		got, err := creelSeam{client: client}.Sent("tst:orch", at(0))
		if err != nil {
			t.Fatalf("Sent error = %v; want nil", err)
		}
		want := []orchengine.CreelSent{
			{ID: 1, At: at(1), Mail: true, Subject: "write the note"},
			{ID: 2, At: at(2), Step: orchengine.CreelStep{Command: "compact", Argument: "keep the plan"}},
			{ID: 3, At: at(3), Subject: "carry on"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Sent = %+v; want %+v", got, want)
		}
		wantFilter := []creelengine.LogFilter{{From: "tst:" + orchWatchProgram, To: "tst:orch", Since: at(0)}}
		if !reflect.DeepEqual(client.filters, wantFilter) {
			t.Errorf("Log filters = %+v; want %+v", client.filters, wantFilter)
		}
	})
}
