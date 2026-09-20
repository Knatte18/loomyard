// newshed_test.go covers NewShed's field-complete assembly of a *shedengine.Shed from a ShedPaths
// value, and the single-prefix rule card 1's own doc comment states: NewShed adds no prefix of its
// own on top of whatever Parse or Build already returned.

package shedbuild

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNewShed_FieldCompleteAssembly asserts NewShed returns a *shedengine.Shed whose Producers
// length matches the parsed recipe's own row count, and whose StatusPath, LockPath,
// StatusLockPath, MaxBounces, and CommitStatus fields are copied verbatim from the passed
// ShedPaths -- the same five fields the two removed New bodies used to set by hand, proving the
// hoist is field-complete rather than assumed.
func TestNewShed_FieldCompleteAssembly(t *testing.T) {
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

	env := newTestEnv(t)
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
}

// TestNewShed_EmptyProducersErrorsWithoutDoublePrefix asserts an empty-producer recipe still
// errors, and that NewShed's returned error is identical to Parse's own error for the same bytes
// -- the single-prefix rule card 1 states: NewShed must add no prefix of its own, so a later
// prefix addition here would double-wrap the error and silently reword both recipe packages'
// shipped envelopes (loomrecipe.New and battenrecipe.New each add exactly one prefix of their
// own on top of NewShed's return value).
func TestNewShed_EmptyProducersErrorsWithoutDoublePrefix(t *testing.T) {
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

	_, err := NewShed([]byte(recipeYAML), newTestEnv(t), ShedPaths{})
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
