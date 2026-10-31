//go:build tier0

// checkmark_test.go pins the tmux argument lists SetCheckMark issues on the engine's session.

package reedengine

import (
	"reflect"
	"testing"
	"time"
)

// TestSetCheckMark pins the exact set-option calls of a mark with a start, a mark without one, and a clear.
func TestSetCheckMark(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state string
		text  string
		start time.Time
		want  func(target string) [][]string
	}{
		{
			name:  "a mark with a start sets the text, the state and the start in epoch seconds",
			state: "running",
			text:  "adopt check: tier1",
			start: time.Unix(1787000000, 0),
			want: func(target string) [][]string {
				return [][]string{
					{"set-option", "-t", target, "@lym_check", "adopt check: tier1"},
					{"set-option", "-t", target, "@lym_check_state", "running"},
					{"set-option", "-t", target, "@lym_check_start", "1787000000"},
				}
			},
		},
		{
			name:  "a mark without a start unsets the start",
			state: "green",
			text:  "adopt check green: tier0 40s",
			want: func(target string) [][]string {
				return [][]string{
					{"set-option", "-t", target, "@lym_check", "adopt check green: tier0 40s"},
					{"set-option", "-t", target, "@lym_check_state", "green"},
					{"set-option", "-u", "-t", target, "@lym_check_start"},
				}
			},
		},
		{
			name: "an empty text unsets all three options",
			want: func(target string) [][]string {
				return [][]string{
					{"set-option", "-u", "-t", target, "@lym_check"},
					{"set-option", "-u", "-t", target, "@lym_check_state"},
					{"set-option", "-u", "-t", target, "@lym_check_start"},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newTestEngine(t)
			fake := installFakeTmux(t, e)

			if err := e.SetCheckMark(tt.state, tt.text, tt.start); err != nil {
				t.Fatalf("SetCheckMark: %v", err)
			}
			want := tt.want(exactSessionWindowTarget(e.SessionName()))
			if got := fake.ArgvFor("set-option"); !reflect.DeepEqual(got, want) {
				t.Errorf("set-option calls = %v; want %v", got, want)
			}
		})
	}
}
