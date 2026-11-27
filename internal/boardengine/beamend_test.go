// beamend_test.go covers the end of a beam's life on the board in beamend.go.

package boardengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestEndBeam pins the end of a beam without a landing.
// The beam's flag, recipe, owner and colour are cleared, its status and member kept,
// and the beam key the board refused on it before is a no-op after.
// An entry that is no beam is left as it was.
func TestEndBeam(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	var boardPath string
	b := newProjectBoard(t, func(cfg *boardengine.Config) { boardPath = cfg.Path })
	seedBoardJSON(t, boardPath,
		`{"id":1,"slug":"beamer","title":"B","kind":"project","labels":["bug"],"beam":true,"recipe":"pawl","owner_machine":"this-machine","beam_color":"#123456","status":"draft"}`,
		`{"id":2,"slug":"member","title":"M","kind":"task","labels":["bug"],"project":"beamer","status":"draft"}`,
		`{"id":3,"slug":"plain","title":"P","kind":"project","labels":["bug"]}`,
		`{"id":4,"slug":"lone","title":"L","kind":"task","labels":["bug"]}`,
	)

	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"Beam", "beamer", true},
		{"BeamAlreadyEnded", "beamer", false},
		{"PlainProject", "plain", false},
		{"Task", "lone", false},
		{"AbsentEntry", "absent", false},
	}
	// The rows run in order on one board, since the second ends the beam the first already ended.
	for _, tt := range tests {
		got, err := b.EndBeam(tt.slug)
		if err != nil || got != tt.want {
			t.Errorf("%s: EndBeam(%s) = %v, %v; want %v, nil", tt.name, tt.slug, got, err, tt.want)
		}
	}

	beam, found, err := b.GetTask("beamer")
	if err != nil || !found {
		t.Fatalf("GetTask(beamer) = found %v, %v; want the entry", found, err)
	}
	if beam.Beam || beam.Recipe != "" || beam.OwnerMachine != "" || beam.BeamColor != "" {
		t.Errorf("beamer beam, recipe, owner, colour = %v, %q, %q, %q; want false and all empty", beam.Beam, beam.Recipe, beam.OwnerMachine, beam.BeamColor)
	}
	if beam.Kind != boardengine.KindProject || beam.Status == nil || *beam.Status != "draft" {
		t.Errorf("beamer kind, status = %q, %v; want a project kept at draft", beam.Kind, beam.Status)
	}
	member, _, err := b.GetTask("member")
	if err != nil || member.Project != "beamer" {
		t.Errorf("member project = %q, %v; want beamer, nil", member.Project, err)
	}
	if _, err := b.UpsertTask(map[string]any{"slug": "beamer", "beam": false}); err != nil {
		t.Errorf("UpsertTask(beamer, beam false) error = %v; want nil once the beam is ended", err)
	}
}
