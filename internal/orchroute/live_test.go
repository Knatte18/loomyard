// live_test.go covers the orch liveness reading over hand-built orch state and a fake reed strand list.

package orchroute

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadingFrom(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	boom := errors.New("reed is down")
	tests := []struct {
		name    string
		st      orchengine.State
		strands func() ([]reedengine.StrandStatus, error)
		want    Reading
		wantErr bool
	}{
		{
			name: "No strand recorded",
			st:   orchengine.State{},
			strands: func() ([]reedengine.StrandStatus, error) {
				return nil, boom
			},
			want: Reading{Down: "no-strand-recorded"},
		},
		{
			name: "A tracked live strand",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g1", Live: true}}, nil
			},
			want: Reading{Live: true},
		},
		{
			name: "A tracked strand down names reed's cause",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g1", Down: reedengine.DownPaneElsewhere}}, nil
			},
			want: Reading{Down: "pane-elsewhere"},
		},
		{
			name: "A strand missing from reed's state",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return []reedengine.StrandStatus{{GUID: "g2", Live: true}}, nil
			},
			want: Reading{Down: "strand-not-in-reed-state"},
		},
		{
			name: "A reed session that is gone",
			st:   orchengine.State{Strand: "g1"},
			strands: func() ([]reedengine.StrandStatus, error) {
				return nil, fmt.Errorf("no reed session; run \"lym reed up\": %w", reedengine.ErrNoSession)
			},
			want: Reading{Down: "no-reed-session"},
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
			got, err := readingFrom(tt.st, tt.strands)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readingFrom() error = %v; want error %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("readingFrom() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
