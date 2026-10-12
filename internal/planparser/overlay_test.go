// overlay_test.go covers Overlay: which names it admits, that a plan parsed through it reads, rewrites and re-parses in memory while the plan directory stays byte-identical, and that the checks which re-read a card file follow it.

package planparser_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

func TestOverlay(t *testing.T) {
	t.Parallel()

	const (
		cardOne = "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** from disk, one.\n"
		cardTwo = "# Card 2 — card2\n\n**Edit:**\n- `internal/foo/baz.go`\n**Intent:** from disk, two.\n"
	)
	diskFiles := map[string]string{
		"00-overview.md": rewriteOverview("card1", "card2"),
		"01-card1.md":    cardOne,
		"02-card2.md":    cardTwo,
	}

	t.Run("an overlaid card is parsed from memory while the other card comes from disk", func(t *testing.T) {
		t.Parallel()
		dir := writePlanFiles(t, diskFiles)

		overlay, err := planparser.NewOverlay(dir, map[string]string{
			"01-card1.md": "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/other.go`\n**Intent:** from memory.\n",
		})
		if err != nil {
			t.Fatalf("NewOverlay() error = %v; want nil", err)
		}
		plan, err := overlay.Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v; want nil", err)
		}
		if got := plan.Cards[0]; got.Intent != "from memory." || !slices.Equal(got.Targets, []string{"internal/foo/other.go#"}) {
			t.Errorf("card 1 = intent %q, targets %q; want the overlaid card", got.Intent, got.Targets)
		}
		if got := plan.Cards[1].Intent; got != "from disk, two." {
			t.Errorf("card 2 intent = %q; want the card on disk", got)
		}
		if read, err := plan.ReadFile("02-card2.md"); err != nil || string(read) != cardTwo {
			t.Errorf("ReadFile(02-card2.md) = (%q, %v); want the card on disk", read, err)
		}
	})

	t.Run("an overlay admits only the overview and the cards the Card Index lists", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name        string
			overlaid    string
			removeFirst string
			wantErr     error
		}{
			{name: "a path below the plan directory", overlaid: "sub/01-x.md", wantErr: planparser.ErrOverlayName},
			{name: "a path above the plan directory", overlaid: "../00-overview.md", wantErr: planparser.ErrOverlayName},
			{name: "a file that is not markdown", overlaid: "notes.txt", wantErr: planparser.ErrOverlayName},
			{name: "the amendment log", overlaid: "amendments.md", wantErr: planparser.ErrOverlayName},
			{name: "a card file on disk the Card Index does not list", overlaid: "99-orphan.md", wantErr: planparser.ErrOverlayName},
			{name: "a listed card whose file was removed from disk", overlaid: "02-card2.md", removeFirst: "02-card2.md", wantErr: planparser.ErrOverlayFileMissing},
			{name: "the overview", overlaid: "00-overview.md"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				dir := writePlanFiles(t, diskFiles)
				if err := os.WriteFile(filepath.Join(dir, "99-orphan.md"), []byte("# Card 99 — orphan\n"), 0o644); err != nil {
					t.Fatalf("write orphan: %v", err)
				}
				if tc.removeFirst != "" {
					if err := os.Remove(filepath.Join(dir, tc.removeFirst)); err != nil {
						t.Fatalf("remove %s: %v", tc.removeFirst, err)
					}
				}

				_, err := planparser.NewOverlay(dir, map[string]string{tc.overlaid: diskFiles["00-overview.md"]})
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("NewOverlay(%q) error = %v; want errors.Is %v", tc.overlaid, err, tc.wantErr)
				}
			})
		}
	})

	t.Run("RewriteRefs on an overlaid plan leaves the plan directory byte-identical and Reparse returns the substitution", func(t *testing.T) {
		t.Parallel()
		dir := writePlanFiles(t, diskFiles)

		overlay, err := planparser.NewOverlay(dir, map[string]string{
			"01-card1.md": "# Card 1 — card1\n\n**Edit:**\n- `internal/foo#Old`\n**Intent:** from memory.\n",
		})
		if err != nil {
			t.Fatalf("NewOverlay() error = %v; want nil", err)
		}
		plan, err := overlay.Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v; want nil", err)
		}
		if err := plan.RewriteRefs(map[string]string{"internal/foo#Old": "internal/foo#New"}); err != nil {
			t.Fatalf("RewriteRefs() error = %v; want nil", err)
		}

		for name, want := range diskFiles {
			if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != want {
				t.Errorf("%s on disk = (%q, %v); want it byte-identical to %q", name, got, err, want)
			}
		}
		reparsed, err := plan.Reparse()
		if err != nil {
			t.Fatalf("Reparse() error = %v; want nil", err)
		}
		if got := reparsed.Cards[0].Targets; !slices.Equal(got, []string{"internal/foo#New"}) {
			t.Errorf("reparsed card 1 targets = %q; want the substituted ref", got)
		}
	})

	t.Run("card-numbering judges an overlaid card's heading", func(t *testing.T) {
		t.Parallel()
		dir := writePlanFiles(t, diskFiles)

		overlay, err := planparser.NewOverlay(dir, map[string]string{
			"01-card1.md": "# Card 7 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** from memory.\n",
		})
		if err != nil {
			t.Fatalf("NewOverlay() error = %v; want nil", err)
		}
		plan, err := overlay.Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v; want nil", err)
		}
		var numbered []string
		for _, finding := range planparser.ValidateFormat(plan, t.TempDir()) {
			if finding.Check == "card-numbering" {
				numbered = append(numbered, finding.Card)
			}
		}
		if !slices.Equal(numbered, []string{"1-card1"}) {
			t.Errorf("card-numbering findings on cards %q; want [1-card1]", numbered)
		}
	})
}
