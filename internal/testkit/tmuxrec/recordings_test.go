// recordings_test.go replays each scenario over its recording, and pins what a replay refuses.

package tmuxrec_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

func TestRecordings_Replay(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			session, replay := tmuxrec.PlaySession(t, name)
			t.Cleanup(func() {
				if unanswered := replay.Unanswered(); len(unanswered) > 0 {
					t.Errorf("recording %s holds no answer to %q", name, unanswered)
				}
			})
			rec, err := tmuxrec.Embedded(name)
			if err != nil {
				t.Fatalf("Embedded(%q) error = %v", name, err)
			}

			got := scenario.run(t, &world{session: session, endProcess: func(int) {}, wait: func() {}})

			if diff := cmp.Diff(rec.Readings, got); diff != "" {
				t.Errorf("readings over the replay mismatch (-recorded +got):\n%s", diff)
			}
			if remaining := replay.Remaining(); len(remaining) > 0 {
				t.Errorf("recorded changes the scenario never made: %q", remaining)
			}
		})
	}
}

func TestReplay_RefusesWhatItsRecordingCannotAnswer(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	rec, err := tmuxrec.Embedded("session-start-stop")
	if err != nil {
		t.Fatalf("Embedded() error = %v", err)
	}
	recorded := func(prefix ...string) tmuxrec.Call {
		for _, call := range rec.Calls {
			if len(call.Args) >= len(prefix) && slices.Equal(call.Args[:len(prefix)], prefix) {
				return call
			}
		}
		t.Fatalf("the recording holds no call starting %q", prefix)
		return tmuxrec.Call{}
	}
	read := recorded("has-session")
	firstChange := recorded("new-session")
	secondChange := recorded("set-option", "-g", "remain-on-exit")
	changedArgs := firstChange
	changedArgs.Args = firstChange.Args[:len(firstChange.Args)-1]
	cut := rec
	cut.Calls = []tmuxrec.Call{read, firstChange, secondChange}

	tests := []struct {
		name  string
		calls []tmuxrec.Call
		// wantRefused says for each call whether the replay refuses it.
		wantRefused []bool
	}{
		{"AChangeOutOfItsRecordedOrder", []tmuxrec.Call{secondChange}, []bool{true}},
		{"AChangeWithOtherArguments", []tmuxrec.Call{changedArgs}, []bool{true}},
		{"AReadAfterAChangeWithNoRecordedReadSince", []tmuxrec.Call{read, firstChange, read}, []bool{false, false, true}},
		{"AReadRepeatedBeforeTheNextChange", []tmuxrec.Call{read, read, firstChange}, []bool{false, false, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			replay := tmuxrec.NewReplay("cut", cut, tmuxrec.Bindings{})
			tmux := replay.Outside(nil).Tmux

			var refused []bool
			for _, call := range tt.calls {
				before := len(replay.Unanswered())
				// A recorded failure is an answer too, so the refusal shows in Unanswered and not in the error.
				_, _ = tmux(call.Capture, nil, call.Args...)
				refused = append(refused, len(replay.Unanswered()) > before)
			}

			if diff := cmp.Diff(tt.wantRefused, refused); diff != "" {
				t.Errorf("refused calls mismatch (-want +got):\n%s\nunanswered: %q", diff, replay.Unanswered())
			}
		})
	}

	t.Run("UnansweredNamesTheRefusedCall", func(t *testing.T) {
		t.Parallel()
		replay := tmuxrec.NewReplay("cut", cut, tmuxrec.Bindings{})

		_, err := replay.Outside(nil).Tmux(false, nil, secondChange.Args...)

		if err == nil {
			t.Errorf("Tmux() error = nil; want the refusal")
		}
		if diff := cmp.Diff([]string{"tmux set-option -g remain-on-exit on"}, replay.Unanswered()); diff != "" {
			t.Errorf("Unanswered() mismatch (-want +got):\n%s", diff)
		}
	})
}
