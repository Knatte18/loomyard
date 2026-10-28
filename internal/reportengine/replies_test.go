//go:build tier0

// replies_test.go tests ComputeReplies over creel log entries.

package reportengine

import (
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

func TestComputeReplies(t *testing.T) {
	t.Parallel()

	const (
		orch   = "ly:orch"
		driver = "ly:task:driver"
		batten = "ly:BATTEN"
	)
	tests := []struct {
		name    string
		entries []creelengine.LogEntry
		want    []RoleReplies
	}{
		{"an empty journal answers no roles", nil, []RoleReplies{}},
		{"marked posts, a reply to a marked message, chases and an unanswered outcome count per receiver role", []creelengine.LogEntry{
			{Event: "posted", ID: 1, From: orch, To: driver, ExpectReply: true},
			{Event: "posted", ID: 2, From: driver, To: orch, ReplyTo: 1},
			{Event: "posted", ID: 3, From: driver, To: orch, ExpectReply: true},
			{Event: "chased", ID: 3, To: orch, Chase: 1},
			{Event: "chased", ID: 3, To: orch, Chase: 2},
			{Event: "unanswered", ID: 3, To: orch, Reason: creelengine.UnansweredNoReply},
			{Event: "posted", ID: 4, From: driver, To: batten, ExpectReply: true},
		}, []RoleReplies{
			{Role: "BATTEN", Expected: 1},
			{Role: "driver", Expected: 1, Replied: 1},
			{Role: "orch", Expected: 1, Chased: 2, Unanswered: 1},
		}},
		{"a reply to an unmarked message counts as no reply", []creelengine.LogEntry{
			{Event: "posted", ID: 1, From: orch, To: driver},
			{Event: "posted", ID: 2, From: driver, To: orch, ReplyTo: 1},
		}, []RoleReplies{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ComputeReplies(tt.entries); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ComputeReplies() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
