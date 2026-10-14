// switched_test.go covers SwitchedProducer's verdicts: a flag off passes on unacted, a flag on acts once and passes on even over a failed action, and an unreadable flag is an error.

package shedadapters

import (
	"context"
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

func TestSwitchedProducer_ActsOnlyWhenOnAndNeverHaltsOverTheAction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		enabled   bool
		flagErr   error
		actErr    error
		wantActs  int
		wantError bool
	}{
		{name: "off passes on unacted", enabled: false},
		{name: "on acts once", enabled: true, wantActs: 1},
		{name: "on with a failed action still passes on", enabled: true, actErr: errors.New("no launcher"), wantActs: 1},
		{name: "unreadable flag is an error", flagErr: errors.New("bad yaml"), wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			acts := 0
			producer := NewSwitchedProducer("Open-Window", "OpenWindow", SwitchedAction{
				Enabled: func() (bool, error) { return tc.enabled, tc.flagErr },
				Act: func(context.Context) error {
					acts++
					return tc.actErr
				},
			})
			outcome, _, err := producer.Call(context.Background())
			if tc.wantError {
				if err == nil {
					t.Fatalf("Call() error = nil; want the flag read's error")
				}
			} else if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if acts != tc.wantActs {
				t.Errorf("action ran %d times; want %d", acts, tc.wantActs)
			}
		})
	}
}
