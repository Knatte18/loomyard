// creelseam_test.go covers the folds of creel's journal entries into the watcher's outcome and sent values.

package orchcli

import (
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// TestCreelSeam_MapsEachCallOntoTheClient drives Post, Inject, ReadMailbox, Hold, Release and Withdraw and checks the request the client received and the values converted back.
func TestCreelSeam_MapsEachCallOntoTheClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, seam creelSeam)
		want []string
	}{
		{"Post leaves the body as mail for the receiver", func(t *testing.T, seam creelSeam) {
			if id, err := seam.Post("tst:orch", "hello"); err != nil || id != 11 {
				t.Errorf("Post = %d, %v; want 11, nil", id, err)
			}
		}, []string{"post:tst:orch:hello"}},
		{"a step with a command injects the command with its ttl", func(t *testing.T, seam creelSeam) {
			step := orchengine.CreelStep{Command: "compact", Argument: "keep the plan"}
			if id, err := seam.Inject("tst:orch", step, 90*time.Second); err != nil || id != 13 {
				t.Errorf("Inject = %d, %v; want 13, nil", id, err)
			}
		}, []string{"inject-command:tst:orch:compact keep the plan:1m30s"}},
		{"a step with only text injects the text with its ttl", func(t *testing.T, seam creelSeam) {
			if id, err := seam.Inject("tst:orch", orchengine.CreelStep{Text: "carry on"}, 10*time.Minute); err != nil || id != 12 {
				t.Errorf("Inject = %d, %v; want 12, nil", id, err)
			}
		}, []string{"inject:tst:orch:carry on:10m0s"}},
		{"Hold holds the receiver for the duration", func(t *testing.T, seam creelSeam) {
			if err := seam.Hold("tst:orch", 2*time.Minute); err != nil {
				t.Errorf("Hold error = %v; want nil", err)
			}
		}, []string{"hold:tst:orch:2m0s"}},
		{"Release ends the hold on the receiver", func(t *testing.T, seam creelSeam) {
			if err := seam.Release("tst:orch"); err != nil {
				t.Errorf("Release error = %v; want nil", err)
			}
		}, []string{"release:tst:orch"}},
		{"Withdraw answers the id and subject of each message it removed", func(t *testing.T, seam creelSeam) {
			got, err := seam.Withdraw("tst:orch")
			if err != nil {
				t.Fatalf("Withdraw error = %v; want nil", err)
			}
			want := []orchengine.CreelWithdrawn{{ID: 5, Subject: "resume"}, {ID: 6, Subject: "note"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Withdraw = %+v; want %+v", got, want)
			}
		}, []string{"withdraw:tst:orch"}},
		{"ReadMailbox hands the handler each message's id, sender, reply target and body", func(t *testing.T, seam creelSeam) {
			var got []orchengine.CreelMessage
			err := seam.ReadMailbox(func(messages []orchengine.CreelMessage) error {
				got = messages
				return nil
			})
			if err != nil {
				t.Fatalf("ReadMailbox error = %v; want nil", err)
			}
			want := []orchengine.CreelMessage{{ID: 3, From: "tst:orch", ReplyTo: 2, Body: "written x.md"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ReadMailbox handed %+v; want %+v", got, want)
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeCreelClient{
				messages:  []creelengine.Message{{ID: 3, From: "tst:orch", ReplyTo: 2, Body: "written x.md"}},
				withdrawn: creelengine.WithdrawResult{Messages: []creelengine.WithdrawnMessage{{ID: 5, Subject: "resume"}, {ID: 6, Subject: "note"}}},
			}
			tt.run(t, creelSeam{client: client})
			if !reflect.DeepEqual(client.calls, tt.want) {
				t.Errorf("client calls = %v; want %v", client.calls, tt.want)
			}
		})
	}
}

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
