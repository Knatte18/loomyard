// locations_test.go covers Locations, the extraction and placement of the `path:line` locations a verify log names.

package failuretrace

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

const (
	testModule   = "example.com/m"
	testRootPath = "/work/wt"
)

// testPlacement places locations in a worktree at testRootPath whose only files at head are those named.
func testPlacement(existing ...string) Placement {
	return Placement{
		WorktreeRoot: testRootPath,
		ModulePath:   testModule,
		ExistsAtHead: func(rel string) bool {
			for _, name := range existing {
				if name == rel {
					return true
				}
			}
			return false
		},
	}
}

func TestLocations(t *testing.T) {
	t.Parallel()

	var manyTail []string
	var manyWant []string
	for line := 1; line <= 25; line++ {
		manyTail = append(manyTail, fmt.Sprintf("internal/x/y.go:%d: boom", line))
		if line <= 20 {
			manyWant = append(manyWant, fmt.Sprintf("internal/x/y.go:%d", line))
		}
	}

	tests := []struct {
		name         string
		failure      websterengine.VerifyFailure
		wantPlaced   []string
		wantUntraced []string
	}{
		{
			name:       "a test failure's bare file name is placed through its package",
			failure:    websterengine.VerifyFailure{Kind: websterengine.FailureKindTest, Package: testModule + "/internal/x", Tail: "--- FAIL: TestX (0.00s)\n    x_test.go:12: got 1; want 2"},
			wantPlaced: []string{"internal/x/x_test.go:12"},
		},
		{
			name:       "a build error path that exists at head is placed as written and loses its column",
			failure:    websterengine.VerifyFailure{Kind: websterengine.FailureKindPackage, Package: testModule + "/internal/x", Tail: "internal/x/y.go:3:5: undefined: foo"},
			wantPlaced: []string{"internal/x/y.go:3"},
		},
		{
			name:       "an absolute worktree path is placed with the root stripped",
			failure:    websterengine.VerifyFailure{Kind: websterengine.FailureKindTest, Package: testModule + "/internal/x", Tail: "\t/work/wt/internal/x/z.go:7 +0x1d"},
			wantPlaced: []string{"internal/x/z.go:7"},
		},
		{
			name:       "a trimpath panic frame is placed with the module path stripped",
			failure:    websterengine.VerifyFailure{Kind: websterengine.FailureKindTest, Package: testModule + "/internal/x", Tail: "\t" + testModule + "/internal/x/w.go:9 +0x2f"},
			wantPlaced: []string{"internal/x/w.go:9"},
		},
		{
			name:         "a dot-relative location of an opaque identity is untraced as written",
			failure:      websterengine.VerifyFailure{Kind: websterengine.FailureKindOpaque, Tail: "./y.go:4:2: syntax error"},
			wantUntraced: []string{"./y.go:4"},
		},
		{
			name:    "a standard-library frame is dropped",
			failure: websterengine.VerifyFailure{Kind: websterengine.FailureKindTest, Package: testModule + "/internal/x", Tail: "\ttesting/testing.go:1700 +0x10"},
		},
		{
			name:       "a repeated location is placed once and more than twenty are capped",
			failure:    websterengine.VerifyFailure{Kind: websterengine.FailureKindPackage, Package: testModule + "/internal/x", Tail: strings.Join(append(manyTail, manyTail[0]), "\n")},
			wantPlaced: manyWant,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			placed, untraced := Locations([]websterengine.VerifyFailure{tt.failure}, testPlacement("internal/x/y.go"))
			if !reflect.DeepEqual(placed, tt.wantPlaced) {
				t.Errorf("Locations(%q) placed = %q; want %q", tt.failure.Tail, placed, tt.wantPlaced)
			}
			if !reflect.DeepEqual(untraced, tt.wantUntraced) {
				t.Errorf("Locations(%q) untraced = %q; want %q", tt.failure.Tail, untraced, tt.wantUntraced)
			}
		})
	}
}
