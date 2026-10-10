// newshed_test.go covers NewShed's field-complete assembly of a *shedengine.Shed from a ShedPaths
// value, and the single-prefix rule card 1's own doc comment states: NewShed adds no prefix of its
// own on top of whatever Parse or Build already returned.

package shedbuild

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
)

// TestNewShed_FieldCompleteAssembly asserts NewShed returns a *shedengine.Shed whose Producers length matches the parsed recipe's own row count, whose StatusPath, LockPath, StatusLockPath, MaxBounces, CommitStatus, RunID and MissingStatusWayForward fields are copied verbatim from the passed ShedPaths, and whose Transient classifier is wired.
func TestNewShed_FieldCompleteAssembly(t *testing.T) {
	t.Parallel()

	const recipeYAML = `
version: 1
entry: row1
terminals: [row2]
producers:
  - name: row1
    engine: Stub
    on_done: row2
  - name: row2
    engine: Stub
`
	recipe, err := Parse([]byte(recipeYAML))
	if err != nil {
		t.Fatalf("Parse(recipeYAML) = _, %v; want nil", err)
	}

	env := envkit.FullEnv(t)
	dir := t.TempDir()

	var committed []string
	commitStatus := func(producer, state string) error {
		committed = append(committed, producer+":"+state)
		return nil
	}

	paths := ShedPaths{
		StatusPath:     filepath.Join(dir, "status.json"),
		LockPath:       filepath.Join(dir, "run.lock"),
		StatusLockPath: filepath.Join(dir, "status.json.lock"),
		MaxBounces:     7,
		CommitStatus:   commitStatus,
		RunID:          "some-slug",

		MissingStatusWayForward: "way forward: told clause",
	}

	shed, err := NewShed([]byte(recipeYAML), env, paths)
	if err != nil {
		t.Fatalf("NewShed() = _, %v; want nil", err)
	}

	if len(shed.Producers) != len(recipe.Producers) {
		t.Errorf("len(shed.Producers) = %d; want %d (the parsed recipe's own row count)", len(shed.Producers), len(recipe.Producers))
	}
	if shed.StatusPath != paths.StatusPath {
		t.Errorf("shed.StatusPath = %q; want %q", shed.StatusPath, paths.StatusPath)
	}
	if shed.LockPath != paths.LockPath {
		t.Errorf("shed.LockPath = %q; want %q", shed.LockPath, paths.LockPath)
	}
	if shed.StatusLockPath != paths.StatusLockPath {
		t.Errorf("shed.StatusLockPath = %q; want %q", shed.StatusLockPath, paths.StatusLockPath)
	}
	if shed.MaxBounces != paths.MaxBounces {
		t.Errorf("shed.MaxBounces = %d; want %d", shed.MaxBounces, paths.MaxBounces)
	}
	if shed.RunID != paths.RunID || shed.MissingStatusWayForward != paths.MissingStatusWayForward {
		t.Errorf("shed.RunID, MissingStatusWayForward = %q, %q; want the told values", shed.RunID, shed.MissingStatusWayForward)
	}

	// The assembled Shed classifies a never-ready agent start as agent-start and a plain error as not transient.
	if shed.Transient == nil {
		t.Fatal("shed.Transient = nil; want the shedtransient classifier")
	}
	if got := shed.Transient(fmt.Errorf("start: %w", shuttleengine.ErrNotStarted)); got != shedengine.TransientAgentStart {
		t.Errorf("Transient(ErrNotStarted-wrapping) = %q; want %q", got, shedengine.TransientAgentStart)
	}
	if got := shed.Transient(errors.New("plain")); got != "" {
		t.Errorf("Transient(plain error) = %q; want empty", got)
	}

	// A func value is comparable only against nil, never against another func value with ==, so
	// CommitStatus identity is asserted by calling the returned Shed's own CommitStatus and
	// observing the passed closure's side effect.
	if shed.CommitStatus == nil {
		t.Fatal("shed.CommitStatus = nil; want the passed closure copied verbatim")
	}
	if err := shed.CommitStatus("row1", "done"); err != nil {
		t.Fatalf("shed.CommitStatus(\"row1\", \"done\") = %v; want nil", err)
	}
	if len(committed) != 1 || committed[0] != "row1:done" {
		t.Errorf("committed = %v after calling shed.CommitStatus; want exactly [%q], proving shed.CommitStatus is paths.CommitStatus itself", committed, "row1:done")
	}

	// NewShedFrom over the parsed recipe assembles the same shed as NewShed over its bytes.
	from, err := NewShedFrom(recipe, env, paths)
	if err != nil {
		t.Fatalf("NewShedFrom() = _, %v; want nil", err)
	}
	if len(from.Producers) != len(shed.Producers) {
		t.Fatalf("NewShedFrom() built %d producers; want %d, as NewShed", len(from.Producers), len(shed.Producers))
	}
	for i := range shed.Producers {
		if from.Producers[i].Name != shed.Producers[i].Name || from.Producers[i].OnDone != shed.Producers[i].OnDone {
			t.Errorf("NewShedFrom() producer %d = %q -> %q; want %q -> %q, as NewShed", i, from.Producers[i].Name, from.Producers[i].OnDone, shed.Producers[i].Name, shed.Producers[i].OnDone)
		}
	}
	if from.StatusPath != shed.StatusPath || from.LockPath != shed.LockPath || from.StatusLockPath != shed.StatusLockPath || from.MaxBounces != shed.MaxBounces || from.RunID != shed.RunID || from.MissingStatusWayForward != shed.MissingStatusWayForward {
		t.Errorf("NewShedFrom() paths fields differ from NewShed's for the same paths")
	}
}

// TestNewShed_EmptyProducersErrorsWithoutDoublePrefix asserts an empty-producer recipe still
// errors, and that NewShed's returned error is identical to Parse's own error for the same bytes
// -- the single-prefix rule card 1 states: NewShed must add no prefix of its own, so a later
// prefix addition here would double-wrap the error and silently reword both recipe packages'
// shipped envelopes (loomrecipe.New and battenrecipe.New each add exactly one prefix of their
// own on top of NewShed's return value).
func TestNewShed_EmptyProducersErrorsWithoutDoublePrefix(t *testing.T) {
	t.Parallel()

	const recipeYAML = `
version: 1
entry: start
terminals: [done]
producers: []
`
	_, wantErr := Parse([]byte(recipeYAML))
	if wantErr == nil {
		t.Fatal("Parse(recipeYAML) = _, nil; want a non-nil error for an empty-producer recipe, so this fixture actually exercises the case under test")
	}

	_, err := NewShed([]byte(recipeYAML), envkit.FullEnv(t), ShedPaths{})
	if err == nil {
		t.Fatal("NewShed(recipeYAML) = _, nil; want a non-nil error for an empty-producer recipe")
	}
	if err.Error() != wantErr.Error() {
		t.Errorf("NewShed(recipeYAML) error = %q; want it identical to Parse's own error %q -- NewShed must add no prefix of its own", err.Error(), wantErr.Error())
	}
	if strings.Contains(err.Error(), "shedbuild: shedbuild:") {
		t.Errorf("NewShed(recipeYAML) error = %q; carries a double shedbuild: prefix, violating the single-prefix rule", err.Error())
	}
}
