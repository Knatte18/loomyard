// cli_test.go covers the segment runner over fake segments and the prime's segment list over a synthetic location.

package cleanupcli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

func TestRunSegments(t *testing.T) {
	t.Parallel()

	var ran []string
	fake := func(name string, err error) segment {
		return segment{name: name, run: func(apply bool) (any, error) {
			ran = append(ran, name)
			return map[string]any{"apply": apply}, err
		}}
	}

	results, allSucceeded := runSegments([]segment{fake("first", nil), fake("second", errors.New("boom")), fake("third", nil)}, true)

	if want := []string{"first", "second", "third"}; !slices.Equal(ran, want) {
		t.Errorf("segments ran %v, want %v in order, the later ones despite the failure", ran, want)
	}
	if allSucceeded {
		t.Errorf("allSucceeded = true, want false after a failing segment")
	}
	want := []segmentResult{
		{Name: "first", Listed: map[string]any{"apply": true}},
		{Name: "second", Listed: map[string]any{"apply": true}, Error: "boom"},
		{Name: "third", Listed: map[string]any{"apply": true}},
	}
	if !reflect.DeepEqual(results, want) {
		t.Errorf("results = %+v, want %+v", results, want)
	}

	if _, allSucceeded := runSegments([]segment{fake("only", nil)}, false); !allSucceeded {
		t.Errorf("allSucceeded = false for a run of succeeding segments")
	}
}

func TestPrimeSegments(t *testing.T) {
	t.Parallel()

	location := locationkit.Location(t.TempDir(), "prime", "")
	segments := primeSegments(location)

	var names []string
	for _, s := range segments {
		names = append(names, s.name)
	}
	if want := []string{"batten", "board", "orch", "scratch", "logs", "temp"}; !slices.Equal(names, want) {
		t.Fatalf("segment names = %v, want %v", names, want)
	}

	// A cycle in flight keeps every handoff, as `lyx orch cleanup` does.
	paths := orchcli.PrimePaths(location)
	if err := orchengine.SaveState(paths, orchengine.State{Phase: orchengine.PhaseClearing, LastHandoff: "new.md"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(paths.HandoffsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, name := range []string{"old.md", "new.md"} {
		if err := os.WriteFile(filepath.Join(paths.HandoffsDir, name), []byte("note"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	listed, err := segments[2].run(true)
	if err != nil {
		t.Fatalf("orch segment: %v", err)
	}
	fields := listed.(map[string]any)
	if removed := fields["removed"].([]string); len(removed) != 0 {
		t.Errorf("orch segment removed %v in the clearing phase, want nothing", removed)
	}
	if reason := fields["reason"].(string); !strings.Contains(reason, "phase clearing") {
		t.Errorf("orch segment reason = %q, want it to name the phase", reason)
	}
	for _, name := range []string{"old.md", "new.md"} {
		if _, err := os.Stat(filepath.Join(paths.HandoffsDir, name)); err != nil {
			t.Errorf("handoff %s did not survive: %v", name, err)
		}
	}
}
