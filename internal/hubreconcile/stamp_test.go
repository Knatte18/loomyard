// stamp_test.go covers the build key, the stamp file round trip and the stamp and lock paths.

package hubreconcile

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/buildvcs"
)

// TestBuildKey_DiffersOnEveryInput verifies each input moves the key and equal inputs agree.
func TestBuildKey_DiffersOnEveryInput(t *testing.T) {
	t.Parallel()

	base := BuildKey(buildvcs.Identity{Revision: "abc", Modified: false}, "fp")
	cases := []struct {
		name string
		id   buildvcs.Identity
		fp   string
		same bool
	}{
		{"equal inputs", buildvcs.Identity{Revision: "abc"}, "fp", true},
		{"changed revision", buildvcs.Identity{Revision: "abd"}, "fp", false},
		{"flipped modified flag", buildvcs.Identity{Revision: "abc", Modified: true}, "fp", false},
		{"changed fingerprint", buildvcs.Identity{Revision: "abc"}, "fq", false},
		{"fields shifted across the boundary", buildvcs.Identity{Revision: "ab"}, "cfp", false},
	}
	for _, tc := range cases {
		if got := BuildKey(tc.id, tc.fp) == base; got != tc.same {
			t.Errorf("%s: key equal to base = %v, want %v", tc.name, got, tc.same)
		}
	}
}

// TestStamp_ReadWrite verifies an absent stamp is not found without error and a written stamp round-trips all three fields.
func TestStamp_ReadWrite(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "build-stamp.json")
	if _, found, err := readStamp(path); found || err != nil {
		t.Fatalf("readStamp(absent) = found %v, err %v; want not found, nil", found, err)
	}

	want := stamp{BuildKey: "key", Identity: buildvcs.Identity{Revision: "abc", Modified: true}}
	if err := writeStamp(path, want); err != nil {
		t.Fatalf("writeStamp: %v", err)
	}
	got, found, err := readStamp(path)
	if err != nil || !found {
		t.Fatalf("readStamp = found %v, err %v; want found, nil", found, err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// TestGeometry_Paths verifies the stamp and lock sit side by side under the never-tracked mirror of the config dir.
func TestGeometry_Paths(t *testing.T) {
	t.Parallel()

	geom := Geometry{BoardDir: filepath.Join("hub", "_board")}
	if got, want := geom.StampPath(), filepath.Join("hub", "_board", ".lyx", "config", "build-stamp.json"); got != want {
		t.Errorf("StampPath = %q, want %q", got, want)
	}
	if got, want := geom.LockPath(), filepath.Join("hub", "_board", ".lyx", "config", "reconcile.lock"); got != want {
		t.Errorf("LockPath = %q, want %q", got, want)
	}
}
