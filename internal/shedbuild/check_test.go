// check_test.go holds exactly two cases pinning that Check forwards a Recipe's told Entry and
// Terminals to shedcheck.Check in the right argument positions. internal/shedcheck's own behaviour
// is exhaustively tested in its own package and is deliberately not re-tested here.

package shedbuild

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shedcheck"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
)

func TestCheck_CleanGraphReturnsNoFindings(t *testing.T) {
	r := Recipe{
		Entry:     "Row-1",
		Terminals: []string{"Row-2"},
		Producers: []Row{
			{Name: "Row-1", Engine: "Stub", OnDone: "Row-2"},
			{Name: "Row-2", Engine: "Stub"},
		},
	}

	defs, err := Build(r, shedrecipe.Env{})
	if err != nil {
		t.Fatalf("Build() error = %v; want nil", err)
	}

	findings := Check(r, defs)
	if len(findings) != 0 {
		t.Errorf("Check() = %v (len %d); want length zero", findings, len(findings))
	}
}

func TestCheck_DanglingTargetForwardsEntryAndTerminals(t *testing.T) {
	r := Recipe{
		Entry:     "Row-1",
		Terminals: []string{"Row-2"},
		Producers: []Row{
			{Name: "Row-1", Engine: "Stub", OnDone: "No-Such-Row"},
			{Name: "Row-2", Engine: "Stub"},
		},
	}

	defs, err := Build(r, shedrecipe.Env{})
	if err != nil {
		t.Fatalf("Build() error = %v; want nil", err)
	}

	findings := Check(r, defs)
	found := false
	for _, f := range findings {
		if f.Kind == shedcheck.KindDanglingTarget && f.Producer == "Row-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("Check() = %v; want a %s finding for producer %q", findings, shedcheck.KindDanglingTarget, "Row-1")
	}
}
