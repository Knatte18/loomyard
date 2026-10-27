//go:build tier0

// live_test.go covers the orch liveness predicate over hand-built orch state and a fake reed strand list.

package orchcli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

func TestOrchLiveFrom(t *testing.T) {
	t.Parallel()

	boom := errors.New("reed is down")
	tests := []struct {
		name    string
		st      orchengine.State
		strands func() ([]reedengine.StrandStatus, error)
		want    bool
		wantErr bool
	}{
		{
			name: "No strand recorded",
			st:   orchengine.State{},
			strands: func() ([]reedengine.StrandStatus, error) {
				return nil, boom
			},
		},
		{
			name: "A tracked live strand",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g1", Live: true}}, nil
			},
			want: true,
		},
		{
			name: "A tracked dead strand",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g1", Live: false}}, nil
			},
		},
		{
			name: "An untracked strand",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g2", Live: true}}, nil
			},
		},
		{
			name: "A reed session that is gone",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return nil, fmt.Errorf("no reed session; run \"lym reed up\": %w", reedengine.ErrNoSession)
			},
		},
		{
			name: "A strands error",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return nil, boom
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := orchLiveFrom(tt.st, tt.strands)
			if (err != nil) != tt.wantErr {
				t.Fatalf("orchLiveFrom() error = %v; want error %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("orchLiveFrom() = %t; want %t", got, tt.want)
			}
		})
	}
}
