//go:build tier0

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

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/lymcwd"
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

	hub := t.TempDir()
	location := locationkit.Location(hub, "prime", "")
	beam := locationkit.Location(hub, "b", "")
	segments := segmentsOver(location, func(*lymcwd.Location) ([]*lymcwd.Location, error) {
		return []*lymcwd.Location{location, beam}, nil
	})

	var names []string
	for _, s := range segments {
		names = append(names, s.name)
	}
	if want := []string{"batten", "board", "orch", "scratch", "logs", "temp"}; !slices.Equal(names, want) {
		t.Fatalf("segment names = %v, want %v", names, want)
	}

	// The batten segment prints push_spawned beside push_error: true for a pass that spawned its push, false for one that committed nothing.
	for _, tt := range []struct {
		name   string
		report battencli.CleanupReport
		want   bool
	}{
		{"CommittedAndSpawned", battencli.CleanupReport{Removed: []string{"alpha"}, PushSpawned: true}, true},
		{"CommittedNothing", battencli.CleanupReport{}, false},
	} {
		fields := battenListed(tt.report)
		if spawned, present := fields["push_spawned"]; !present || spawned != tt.want {
			t.Errorf("%s: push_spawned = %v (present %v), want %v", tt.name, spawned, present, tt.want)
		}
		if pushError, present := fields["push_error"]; !present || pushError != "" {
			t.Errorf("%s: push_error = %v (present %v), want empty", tt.name, pushError, present)
		}
	}

	// A cycle in flight keeps every handoff.
	paths := orchcli.PrimePaths(location)
	if err := orchengine.SaveState(paths, orchengine.State{Phase: orchengine.PhaseClearing, LastHandoff: "new.md"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(paths.HandoffsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A beam's idle orch cleans its stale handoff beside the prime's.
	beamPaths := orchcli.PrimePaths(beam)
	if err := orchengine.SaveState(beamPaths, orchengine.State{Phase: orchengine.PhaseIdle, LastHandoff: "new.md"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(beamPaths.HandoffsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, handoffsDir := range []string{paths.HandoffsDir, beamPaths.HandoffsDir} {
		for _, name := range []string{"old.md", "new.md"} {
			if err := os.WriteFile(filepath.Join(handoffsDir, name), []byte("note"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
		}
	}

	listed, err := segments[2].run(true)
	if err != nil {
		t.Fatalf("orch segment: %v", err)
	}
	worktrees := listed.(map[string]any)["worktrees"].([]map[string]any)
	if len(worktrees) != 2 {
		t.Fatalf("orch segment listed %d worktrees, want the prime and the beam", len(worktrees))
	}
	if name := worktrees[0]["worktree"]; name != "prime" {
		t.Errorf("first worktree = %v, want prime", name)
	}
	if removed := worktrees[0]["removed"].([]string); len(removed) != 0 {
		t.Errorf("orch segment removed %v in the clearing phase, want nothing", removed)
	}
	if reason := worktrees[0]["reason"].(string); !strings.Contains(reason, "phase clearing") {
		t.Errorf("orch segment reason = %q, want it to name the phase", reason)
	}
	if name := worktrees[1]["worktree"]; name != "b" {
		t.Errorf("second worktree = %v, want b", name)
	}
	if kept := worktrees[1]["kept"]; kept != "new.md" {
		t.Errorf("beam kept = %v, want new.md", kept)
	}
	if removed := worktrees[1]["removed"].([]string); !slices.Equal(removed, []string{"old.md"}) {
		t.Errorf("beam removed = %v, want [old.md]", removed)
	}
	for _, name := range []string{"old.md", "new.md"} {
		if _, err := os.Stat(filepath.Join(paths.HandoffsDir, name)); err != nil {
			t.Errorf("prime handoff %s did not survive: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(beamPaths.HandoffsDir, "old.md")); !os.IsNotExist(err) {
		t.Errorf("beam's stale handoff survived (stat err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(beamPaths.HandoffsDir, "new.md")); err != nil {
		t.Errorf("beam's kept handoff did not survive: %v", err)
	}
}
