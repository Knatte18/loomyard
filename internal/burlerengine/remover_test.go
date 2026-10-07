package burlerengine

import (
	"errors"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

func TestReedStrandRemover_RemoveStrandIfLive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reed        *shuttlefake.Reed
		wantErr     bool
		wantRemoved []string
	}{
		{"a live strand is removed", &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "half", Live: true}}}, false, []string{"half"}},
		{"a not-live strand is left alone", &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "half", Live: false}}}, false, nil},
		{"an absent guid is left alone", &shuttlefake.Reed{}, false, nil},
		{"a failing liveness probe is an error and removes nothing", &shuttlefake.Reed{StatusErr: errors.New("reed unreachable")}, true, nil},
		{"a failing removal of a live strand is an error", &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "half", Live: true}}, RemoveErr: errors.New("remove failed")}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := NewReedStrandRemover(tt.reed).RemoveStrandIfLive("half")
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveStrandIfLive() error = %v; wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(tt.reed.RemovedGUIDs, tt.wantRemoved) {
				t.Errorf("RemovedGUIDs = %v; want %v", tt.reed.RemovedGUIDs, tt.wantRemoved)
			}
		})
	}
}
