//go:build tier0

// beamfork_test.go covers beamForkPrecondition over told reads: each refusal that stops a beam member's create, the pass, and a probe error;
// and the member-hold read after a pair is made, with the refusal it words and the anchor Worktree-Create reads from the seed.

package battencli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestBeamForkPrecondition asserts a beam member's create is stopped with a precondition naming the beam's create or adopt when the beam's pair is absent here, and the landing and the cancel of the member when the beam's entry is run-held, and passes otherwise.
// A failed origin probe is returned as is.
func TestBeamForkPrecondition(t *testing.T) {
	t.Parallel()

	held := boardengine.RunStatus("running", "Land")
	awaiting := boardengine.RunStatus("running", "Beam-Await")
	aligning := boardengine.RunStatus("running", "Parent-Align")
	probeErr := errors.New("origin unreachable")
	tests := []struct {
		name        string
		entry       boardengine.Task
		entryFound  bool
		pairPresent bool
		onOrigin    bool
		probeErr    error
		// wantUnmet and wantWayForward are substrings of the precondition's two fields; both empty want a pass.
		wantUnmet      []string
		wantWayForward []string
		wantProbeErr   bool
	}{
		{
			name:           "AbsentBeamPairOffOriginNamesTheBeamCreate",
			entryFound:     true,
			wantUnmet:      []string{"m belongs to beam b", "pair is not on this machine"},
			wantWayForward: []string{`"lym beam create b"`, `"lym batten run m"`},
		},
		{
			name:           "AbsentBeamPairOnOriginNamesTheAdopt",
			entryFound:     true,
			onOrigin:       true,
			wantUnmet:      []string{"m belongs to beam b"},
			wantWayForward: []string{`"lym adopt b"`, `"lym batten run m"`},
		},
		{
			name:         "OriginProbeErrorIsReturned",
			entryFound:   true,
			probeErr:     probeErr,
			wantProbeErr: true,
		},
		{
			name:           "BeamHeldByItsLandingNamesTheLandingAndTheMembersCancel",
			entry:          boardengine.Task{Slug: "b", Status: &held},
			entryFound:     true,
			pairPresent:    true,
			wantUnmet:      []string{"m belongs to beam b", "its own landing holds", held},
			wantWayForward: []string{`"lym batten status b"`, `"lym batten cancel m"`},
		},
		{
			name:        "BeamAtBeamAwaitPasses",
			entry:       boardengine.Task{Slug: "b", Kind: boardengine.KindProject, Beam: true, Status: &awaiting},
			entryFound:  true,
			pairPresent: true,
		},
		{
			name:           "BeamAtParentAlignNamesTheLandingAndTheMembersCancel",
			entry:          boardengine.Task{Slug: "b", Kind: boardengine.KindProject, Beam: true, Status: &aligning},
			entryFound:     true,
			pairPresent:    true,
			wantUnmet:      []string{"m belongs to beam b", "its own landing holds", aligning},
			wantWayForward: []string{`"lym batten status b"`, `"lym batten cancel m"`},
		},
		{
			name:        "BeamWithItsPairAndNoRunPasses",
			entry:       boardengine.Task{Slug: "b"},
			entryFound:  true,
			pairPresent: true,
		},
		{
			name:        "BeamEntryNotFoundPasses",
			pairPresent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := beamForkPrecondition("m", "b", tt.entry, tt.entryFound, tt.pairPresent, func() (bool, error) { return tt.onOrigin, tt.probeErr })

			if tt.wantProbeErr {
				var precondition *battenshed.PreconditionError
				if !errors.Is(err, probeErr) || errors.As(err, &precondition) {
					t.Fatalf("beamForkPrecondition() error = %v; want the probe error, not a precondition", err)
				}
				return
			}
			if len(tt.wantUnmet) == 0 {
				if err != nil {
					t.Fatalf("beamForkPrecondition() = %v; want nil", err)
				}
				return
			}
			var precondition *battenshed.PreconditionError
			if !errors.As(err, &precondition) {
				t.Fatalf("beamForkPrecondition() = %v; want a *battenshed.PreconditionError", err)
			}
			for _, want := range tt.wantUnmet {
				if !strings.Contains(precondition.Unmet, want) {
					t.Errorf("Unmet = %q; want it to carry %q", precondition.Unmet, want)
				}
			}
			for _, want := range tt.wantWayForward {
				if !strings.Contains(precondition.WayForward, want) {
					t.Errorf("WayForward = %q; want it to carry %q", precondition.WayForward, want)
				}
			}
		})
	}
}

// TestMemberHoldRefusal asserts the refusal names the member's cancel from the beam's parent worktree, the board move off the beam, and the member's run once the beam's status shows Beam-Await.
func TestMemberHoldRefusal(t *testing.T) {
	t.Parallel()

	err := memberHoldRefusal("m", "b", "running · Parent-Align")

	var precondition *battenshed.PreconditionError
	if !errors.As(err, &precondition) {
		t.Fatalf("memberHoldRefusal() = %v; want a *battenshed.PreconditionError", err)
	}
	for _, want := range []string{"m belongs to beam b", "running · Parent-Align", "the pair of m was removed"} {
		if !strings.Contains(precondition.Unmet, want) {
			t.Errorf("Unmet = %q; want it to carry %q", precondition.Unmet, want)
		}
	}
	for _, want := range []string{`"lym batten cancel m"`, "`lym board upsert`", `"lym batten status b"`, `"lym batten run m"`} {
		if !strings.Contains(precondition.WayForward, want) {
			t.Errorf("WayForward = %q; want it to carry %q", precondition.WayForward, want)
		}
	}
}

// TestAnchorFromSeed asserts a seed's anchor param is read back, and a run with no seed or no param reads none.
func TestAnchorFromSeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		seed   *shedrun.Seed
		wantAt string
	}{
		{name: "SeedWithAnAnchorParamReadsIt", seed: &shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, Params: map[string]string{anchorParam: "sub/dir"}}, wantAt: "sub/dir"},
		{name: "SeedWithNoParamsReadsNone", seed: &shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo}},
		{name: "NoSeedReadsNone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "code", AnchorRel: "."}
			if tt.seed != nil {
				if err := shedrun.WriteSeed(location, "some-slug", *tt.seed); err != nil {
					t.Fatalf("WriteSeed: %v", err)
				}
			}

			got, err := anchorFromSeed(location, "some-slug")

			if err != nil || got != tt.wantAt {
				t.Errorf("anchorFromSeed() = %q, %v; want %q, nil", got, err, tt.wantAt)
			}
		})
	}
}

// TestRefuseHeldMember asserts a member is judged against its beam's hold read through beamHold: a slug outside a beam and a beam that is not held pass untouched, and a failed read is returned.
// The held case removes a real pair, which the integration tests drive.
func TestRefuseHeldMember(t *testing.T) {
	t.Parallel()

	readFailure := errors.New("board unreadable")
	tests := []struct {
		name    string
		slug    string
		hold    func(beam string) (string, bool, error)
		wantErr error
	}{
		{name: "SlugOutsideABeamIsNeverRead", slug: "loose", hold: func(string) (string, bool, error) { return "", true, errors.New("read for a slug with no beam") }},
		{name: "BeamNotHeldPasses", slug: "m", hold: func(beam string) (string, bool, error) { return "running · Beam-Await", false, nil }},
		{name: "FailedReadIsReturned", slug: "m", hold: func(string) (string, bool, error) { return "", false, readFailure }, wantErr: readFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := t.TempDir()
			location := &lymcwd.Location{HubPath: hub, WorktreeName: "code", AnchorRel: "."}
			if err := os.MkdirAll(fabricengine.BoardDir(hub), 0o755); err != nil {
				t.Fatal(err)
			}
			board := `{"version":1,"entries":[` +
				`{"id":0,"slug":"b","title":"Beam","kind":"project","labels":["bug"],"beam":true},` +
				`{"id":1,"slug":"m","title":"Member","kind":"task","labels":["bug"],"project":"b"},` +
				`{"id":2,"slug":"loose","title":"Loose","kind":"task","labels":["bug"]}]}`
			if err := os.WriteFile(filepath.Join(fabricengine.BoardDir(hub), "board.json"), []byte(board), 0o644); err != nil {
				t.Fatal(err)
			}
			c := &battenCLI{beamHold: tt.hold}

			err := c.refuseHeldMember(context.Background(), location, tt.slug)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("refuseHeldMember(%s) = %v; want %v", tt.slug, err, tt.wantErr)
			}
		})
	}
}
