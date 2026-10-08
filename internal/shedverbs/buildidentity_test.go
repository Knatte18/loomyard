// buildidentity_test.go covers binaryChanged's table: it is true only for two known builds that differ in revision or modified flag.

package shedverbs

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/buildvcs"
)

//testtiming:keep pins the flipped modified flag and the empty-revision rows that never report a change, which its covering test does not
func TestBinaryChanged(t *testing.T) {
	tests := []struct {
		name              string
		running, recorded buildvcs.Identity
		want              bool
	}{
		{"equal pairs", buildvcs.Identity{Revision: "abc"}, buildvcs.Identity{Revision: "abc"}, false},
		{"differing revisions", buildvcs.Identity{Revision: "abc"}, buildvcs.Identity{Revision: "def"}, true},
		{"flipped modified flag", buildvcs.Identity{Revision: "abc", Modified: true}, buildvcs.Identity{Revision: "abc"}, true},
		{"empty recorded revision", buildvcs.Identity{Revision: "abc"}, buildvcs.Identity{Modified: true}, false},
		{"empty running revision", buildvcs.Identity{}, buildvcs.Identity{Revision: "abc", Modified: true}, false},
		{"both empty", buildvcs.Identity{Modified: true}, buildvcs.Identity{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := binaryChanged(tt.running, tt.recorded); got != tt.want {
				t.Errorf("binaryChanged(%+v, %+v) = %v; want %v", tt.running, tt.recorded, got, tt.want)
			}
		})
	}
}
