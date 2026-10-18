// readtext_test.go pins the plain text `lyx creel read` prints for each kind of sender; it is an internal test because the CLI cannot post as a program.

package creelcli

import (
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
)

func TestWriteMessagesText(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-5 * time.Minute)
	tests := []struct {
		name     string
		messages []creelengine.Message
		want     string
	}{
		{"NothingUnread", nil, "nothing unread\n"},
		{
			"StrandMailNamesTheIdAndTheReplyCommand",
			[]creelengine.Message{{ID: 3, From: "ly:alpha:driver", Kind: "mail", Body: "two\nlines\n", Posted: fresh}},
			"#3 from ly:alpha:driver — reply with: lyx creel post --reply 3 <text>\ntwo\nlines\n",
		},
		{
			"ExternalMailTakesAReplyAndShowsNoEnvelope",
			[]creelengine.Message{{ID: 4, From: "ly:EXTERNAL", Envelope: "claude, pts/4", Body: "hi", Posted: fresh}},
			"#4 from ly:EXTERNAL — reply with: lyx creel post --reply 4 <text>\nhi\n",
		},
		{
			"ProgramMailNamesOnlyTheSender",
			[]creelengine.Message{{ID: 5, From: "ly:BATTEN", Body: "task-x: landed", Posted: fresh}},
			"from ly:BATTEN\ntask-x: landed\n",
		},
		{
			"OldReplyAndReturnedMailNameWhatTheyAre",
			[]creelengine.Message{
				{ID: 6, From: "ly:alpha:driver", Kind: "reply", ReplyTo: 2, Body: "answer", Posted: now.Add(-5*time.Minute - time.Second)},
				{ID: 7, From: "ly:BATTEN", EndedUnreadBy: "ly:beta:driver", Path: []string{"ly:beta:driver"}, Body: "back", Posted: fresh},
			},
			"#6 from ly:alpha:driver, in reply to #2, posted 2026-10-18T11:54:59Z — reply with: lyx creel post --reply 6 <text>\nanswer\n" +
				"\nfrom ly:BATTEN, returned unread by ly:beta:driver\nback\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if err := writeMessagesText(&out, tt.messages, now); err != nil {
				t.Fatalf("writeMessagesText() = %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Errorf("writeMessagesText() = %q; want %q", got, tt.want)
			}
		})
	}
}
