package burlerengine

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

func TestRunnerShuttle_StartGated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addErr    error
		wantStart bool
	}{
		{name: "a started run returns a handle naming its strand", wantStart: true},
		{name: "a failed start returns an interface-nil handle and the error", addErr: errors.New("reed: add strand failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			reed := &shuttlefake.Reed{AddErr: tt.addErr}
			cfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30}
			shuttle := RunnerShuttle(shuttleengine.NewRunner(reed, &shuttlefake.Engine{}, root, root, cfg))

			handle, err := shuttle.StartGated(shuttleengine.Spec{Prompt: "review", OutputFiles: []string{"out.md"}}, nil)

			if !tt.wantStart {
				if !errors.Is(err, tt.addErr) {
					t.Errorf("StartGated() error = %v; want it to wrap %v", err, tt.addErr)
				}
				if handle != nil {
					t.Errorf("StartGated() handle = %#v; want an interface-nil Handle", handle)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartGated() = %v; want nil error", err)
			}
			if len(reed.Strands) != 1 || handle.StrandGUID() != reed.Strands[0].GUID {
				t.Errorf("handle strand = %q; want the one strand reed holds, %+v", handle.StrandGUID(), reed.Strands)
			}
		})
	}
}
