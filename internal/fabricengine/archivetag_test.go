// archivetag_test.go holds the unit test for the archive tag push's retry schedule.

package fabricengine

import (
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestArchiveTagPushDelays pins the archive push's schedule: 2 s doubling to 32 s.
func TestArchiveTagPushDelays(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second}
	if got := archiveTagPushDelays(); !reflect.DeepEqual(got, want) {
		t.Errorf("archiveTagPushDelays() = %v; want %v", got, want)
	}
}
