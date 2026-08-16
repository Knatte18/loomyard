// validate_test.go covers all 14 of Validate's plan-format checks, each with at least one
// triggering and one clean case.
// The three existence-dependent checks (move-source-missing, move-target-collision, path-missing)
// build a hermetic t.TempDir() worktreeRoot and materialize real files on disk — no git, no
// fixtures outside this package — per the go-test-tiers-and-hermetic-git Shared Decision.
// The golden happy-path test reuses the contracts/specs/loom-plan-spec.md worked example
// (testdata/goodplan, already parsed by parse_test.go's TestParsePlan_GoldenFixture) and
// materializes exactly the files its cards' Edits:/Context: fields and Moves: source name,
// deliberately leaving the Moves: destination and any Creates: target absent, so the whole 14-check
// Validate run returns zero findings.

package planparser_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// validCard returns a fully well-formed Card at position number with slug —
// every required field present, no malformed path, no Moves:, a correctly
// prefixed Commit:, and dependsOn (defaulting to none) naming only earlier
// cards. Each subtest below starts from this baseline and mutates exactly the
// one field its own check cares about, so a finding it observes can only come
// from the check under test, never incidental noise from the other 13.
func validCard(number int, slug string, dependsOn ...int) planparser.Card {
	if dependsOn == nil {
		dependsOn = []int{}
	}
	return planparser.Card{
		Number:       number,
		Slug:         slug,
		Title:        slug,
		Intent:       "intent for " + slug,
		HasWhat:      true,
		HasContext:   true,
		ContextFiles: []string{},
		HasEdits:     true,
		EditsFiles:   []string{fmt.Sprintf("pkg/card%d.go", number)},
		HasCreates:   true,
		CreatesFiles: []string{},
		HasDeletes:   true,
		DeletesFiles: []string{},
		HasMoves:     true,
		Moves:        []planparser.MovePair{},
		HasDependsOn: true,
		DependsOn:    dependsOn,
		Commit:       fmt.Sprintf("%d: %s", number, slug),
	}
}

// countFor returns how many of findings carry the given Check name — every
// subtest below asserts on this count (and, for the golden fixture, the total)
// rather than exact Detail text, per the "assert on Check names and
// cardinality" instruction: Detail is for humans, Check is the stable contract.
func countFor(findings []planparser.ValidationError, check string) int {
	n := 0
	for _, f := range findings {
		if f.Check == check {
			n++
		}
	}
	return n
}

// materializeFiles writes an empty placeholder file at each of paths, joined
// under root, creating parent directories as needed — the on-disk half of the
// three existence-dependent checks' hermetic fixtures.
func materializeFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("materialize %s: mkdir: %v", p, err)
		}
		if err := os.WriteFile(full, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("materialize %s: write: %v", p, err)
		}
	}
}

// TestValidate_GoldenFixture_ZeroFindings round-trips the pinned spec's own worked example
// (testdata/goodplan) through Validate with every referenced Edits:/Context: path and Moves: source
// materialized under a t.TempDir() worktreeRoot,
// but deliberately NOT the Moves: destination (rowsjson.go) or any Creates: target (the fixture has
// none) — proving all 14 checks pass simultaneously on the plan-format happy path.
func TestValidate_GoldenFixture_ZeroFindings(t *testing.T) {
	t.Parallel()

	plan, err := planparser.ParsePlan(goodPlanDir())
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
	}

	root := t.TempDir()
	materializeFiles(t, root,
		"internal/boardcli/list.go",
		"internal/boardengine/rows.go",
		"internal/output/envelope.go",
		"internal/boardcli/list_test.go",
		"cmd/lyx/helptree_test.go",
	)

	findings := planparser.Validate(plan, root)
	if len(findings) != 0 {
		t.Errorf("Validate(goldenFixture, materializedRoot) = %+v; want zero findings", findings)
	}
}

// TestValidate_FormatAndApproval covers format-unrecognized and plan-unapproved together, since
// both stem from the same overview frontmatter and loom-plan-spec.md checks them as a pair.
func TestValidate_FormatAndApproval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		format     int
		approved   bool
		wantChecks []string
	}{
		{name: "clean", format: 3, approved: true},
		{name: "unrecognized format", format: 2, approved: true, wantChecks: []string{"format-unrecognized"}},
		{name: "unapproved", format: 3, approved: false, wantChecks: []string{"plan-unapproved"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &planparser.Plan{
				Format:   tt.format,
				Approved: tt.approved,
				Cards:    []planparser.Card{validCard(1, "only")},
			}
			findings := planparser.Validate(plan, t.TempDir())

			for _, check := range []string{"format-unrecognized", "plan-unapproved"} {
				want := 0
				for _, wc := range tt.wantChecks {
					if wc == check {
						want = 1
					}
				}
				if got := countFor(findings, check); got != want {
					t.Errorf("countFor(findings, %q) = %d; want %d", check, got, want)
				}
			}
		})
	}
}

// TestValidate_IndexFileMismatch covers the Card Index numbering-sequence half of
// index-file-mismatch (the orphaned-on-disk-file half is exercised implicitly by every other test's
// clean plan.Dir == "" case, where os.ReadDir fails and that half is silently skipped).
func TestValidate_IndexFileMismatch(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 3, Approved: true,
			Cards: []planparser.Card{validCard(1, "a"), validCard(2, "b")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 0 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 0", got)
		}
	})

	t.Run("numbering gap", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 3, Approved: true,
			// Card Index entries 1, 3 — skipping 2.
			Cards: []planparser.Card{validCard(1, "a"), validCard(3, "b")},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "index-file-mismatch"); got != 1 {
			t.Errorf("countFor(findings, index-file-mismatch) = %d; want 1", got)
		}
	})
}

// TestValidate_CardPathMalformed covers card-path-malformed with an absolute (single leading "/")
// Edits: path — a form that already normalized (there is no root:/// to resolve here) survives as a
// genuine malformed marker.
func TestValidate_CardPathMalformed(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 0 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 0", got)
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.EditsFiles = []string{"/abs/path.go"}
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-path-malformed"); got != 1 {
			t.Errorf("countFor(findings, card-path-malformed) = %d; want 1", got)
		}
	})
}

// TestValidate_MoveFormat covers move-format via Card.MovesRaw, the lenient-card-parse slot a
// non-well-formed "Moves:" bullet lands in.
func TestValidate_MoveFormat(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-format"); got != 0 {
			t.Errorf("countFor(findings, move-format) = %d; want 0", got)
		}
	})

	t.Run("malformed bullet", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.MovesRaw = []string{"this bullet has no arrow at all"}
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-format"); got != 1 {
			t.Errorf("countFor(findings, move-format) = %d; want 1", got)
		}
	})
}

// TestValidate_MoveRedundant covers move-redundant: a path that is both a Moves: endpoint (on one
// card) and a Creates: target (on another) anywhere in the plan — the check's scope is plan-wide
// because there is no batch to scope it to.
func TestValidate_MoveRedundant(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		c1 := validCard(1, "a")
		c2 := validCard(2, "b")
		c2.Moves = []planparser.MovePair{{Old: "old.go", New: "new.go"}}
		plan := &planparser.Plan{Format: 3, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{c1, c2}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-redundant"); got != 0 {
			t.Errorf("countFor(findings, move-redundant) = %d; want 0", got)
		}
	})

	t.Run("moves endpoint also a Creates: target", func(t *testing.T) {
		t.Parallel()
		c1 := validCard(1, "a")
		c1.CreatesFiles = []string{"x.go"}
		c2 := validCard(2, "b")
		c2.Moves = []planparser.MovePair{{Old: "x.go", New: "y.go"}}
		plan := &planparser.Plan{Format: 3, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{c1, c2}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-redundant"); got != 1 {
			t.Errorf("countFor(findings, move-redundant) = %d; want 1", got)
		}
	})
}

// TestValidate_MoveMechanicMissing covers move-mechanic-missing: now plan-level
// (Plan.RenameMechanic), fired only when some card actually parsed a Moves: pair.
func TestValidate_MoveMechanicMissing(t *testing.T) {
	t.Parallel()

	newCardWithMove := func() planparser.Card {
		c := validCard(1, "a")
		c.Moves = []planparser.MovePair{{Old: "old.go", New: "new.go"}}
		return c
	}

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 3, Approved: true, RenameMechanic: "1. git mv old.go new.go first.",
			Cards: []planparser.Card{newCardWithMove()},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-mechanic-missing"); got != 0 {
			t.Errorf("countFor(findings, move-mechanic-missing) = %d; want 0", got)
		}
	})

	t.Run("missing section", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{
			Format: 3, Approved: true,
			Cards: []planparser.Card{newCardWithMove()},
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "move-mechanic-missing"); got != 1 {
			t.Errorf("countFor(findings, move-mechanic-missing) = %d; want 1", got)
		}
	})
}

// TestValidate_MoveSourceMissing covers move-source-missing hermetically: the clean case
// materializes the Moves: source under a t.TempDir() worktreeRoot, the dirty case leaves it absent.
func TestValidate_MoveSourceMissing(t *testing.T) {
	t.Parallel()

	newPlanWithMove := func() (*planparser.Plan, planparser.Card) {
		c := validCard(1, "a")
		c.Moves = []planparser.MovePair{{Old: "pkg/old.go", New: "pkg/new.go"}}
		return &planparser.Plan{Format: 3, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{c}}, c
	}

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		materializeFiles(t, root, "pkg/old.go")
		plan, _ := newPlanWithMove()
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "move-source-missing"); got != 0 {
			t.Errorf("countFor(findings, move-source-missing) = %d; want 0", got)
		}
	})

	t.Run("source absent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		plan, _ := newPlanWithMove()
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "move-source-missing"); got != 1 {
			t.Errorf("countFor(findings, move-source-missing) = %d; want 1", got)
		}
	})
}

// TestValidate_MoveTargetCollision covers move-target-collision's already-exists-on-disk condition
// hermetically.
func TestValidate_MoveTargetCollision(t *testing.T) {
	t.Parallel()

	newPlanWithMove := func() *planparser.Plan {
		c := validCard(1, "a")
		c.Moves = []planparser.MovePair{{Old: "pkg/old.go", New: "pkg/new.go"}}
		return &planparser.Plan{Format: 3, Approved: true, RenameMechanic: "mechanic", Cards: []planparser.Card{c}}
	}

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		materializeFiles(t, root, "pkg/old.go")
		findings := planparser.Validate(newPlanWithMove(), root)
		if got := countFor(findings, "move-target-collision"); got != 0 {
			t.Errorf("countFor(findings, move-target-collision) = %d; want 0", got)
		}
	})

	t.Run("target already exists", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		materializeFiles(t, root, "pkg/old.go", "pkg/new.go")
		findings := planparser.Validate(newPlanWithMove(), root)
		if got := countFor(findings, "move-target-collision"); got != 1 {
			t.Errorf("countFor(findings, move-target-collision) = %d; want 1", got)
		}
	})
}

// TestValidate_CardMissingField covers card-missing-field: a card missing one of the seven required
// labels (What:/Context:/Edits:/Creates:/Deletes:/ Moves:/Depends-on:) yields one finding for that
// field.
func TestValidate_CardMissingField(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 0 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 0", got)
		}
	})

	t.Run("missing Context:", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.HasContext = false
		card.ContextFiles = nil
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-missing-field"); got != 1 {
			t.Errorf("countFor(findings, card-missing-field) = %d; want 1", got)
		}
	})
}

// TestValidate_CardFieldOverlap covers card-field-overlap: the same path in two of a single card's
// fields is a contradiction.
func TestValidate_CardFieldOverlap(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-overlap"); got != 0 {
			t.Errorf("countFor(findings, card-field-overlap) = %d; want 0", got)
		}
	})

	t.Run("path in both Context: and Edits:", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.ContextFiles = []string{"shared.go"}
		card.EditsFiles = []string{"shared.go"}
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-field-overlap"); got != 1 {
			t.Errorf("countFor(findings, card-field-overlap) = %d; want 1", got)
		}
	})
}

// TestValidate_CardNumbering covers card-numbering: the card file's own heading number must match
// the number the Card Index assigned it.
// Unlike most other checks, this one re-reads the card file from plan.Dir, so both cases need a
// real on-disk plan directory rather than a hand-built Plan struct.
func TestValidate_CardNumbering(t *testing.T) {
	t.Parallel()

	t.Run("clean (golden fixture)", func(t *testing.T) {
		t.Parallel()
		plan, err := planparser.ParsePlan(goodPlanDir())
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", goodPlanDir(), err)
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-numbering"); got != 0 {
			t.Errorf("countFor(findings, card-numbering) = %d; want 0", got)
		}
	})

	t.Run("heading number mismatch", func(t *testing.T) {
		t.Parallel()
		dir := writePlanFiles(t, map[string]string{
			"00-overview.md": minimalOverview,
			// The Card Index assigns this file (01-only.md) card number 1,
			// but its own heading declares "# Card 2" — the exact mismatch
			// checkCardNumbering exists to catch.
			"01-only.md": "# Card 2 — only\n\n**What:** placeholder.\n**Context:** none\n**Edits:**\n- `a.go`\n" +
				"**Creates:** none\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n",
		})
		plan, err := planparser.ParsePlan(dir)
		if err != nil {
			t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
		}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "card-numbering"); got != 1 {
			t.Errorf("countFor(findings, card-numbering) = %d; want 1", got)
		}
	})
}

// TestValidate_PathMissing covers path-missing hermetically: the clean case materializes the card's
// sole Edits: path under a t.TempDir() worktreeRoot, the dirty case leaves it absent.
func TestValidate_PathMissing(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		card := validCard(1, "a")
		materializeFiles(t, root, card.EditsFiles...)
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 0 {
			t.Errorf("countFor(findings, path-missing) = %d; want 0", got)
		}
	})

	t.Run("path absent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		card := validCard(1, "a")
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, root)
		if got := countFor(findings, "path-missing"); got != 1 {
			t.Errorf("countFor(findings, path-missing) = %d; want 1", got)
		}
	})
}

// TestValidate_CommitSubjectMismatch covers commit-subject-mismatch: a present Commit: must start
// with the card's own "N: " prefix.
func TestValidate_CommitSubjectMismatch(t *testing.T) {
	t.Parallel()

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{validCard(1, "a")}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "commit-subject-mismatch"); got != 0 {
			t.Errorf("countFor(findings, commit-subject-mismatch) = %d; want 0", got)
		}
	})

	t.Run("wrong prefix", func(t *testing.T) {
		t.Parallel()
		card := validCard(1, "a")
		card.Commit = "2: wrong prefix"
		plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{card}}
		findings := planparser.Validate(plan, t.TempDir())
		if got := countFor(findings, "commit-subject-mismatch"); got != 1 {
			t.Errorf("countFor(findings, commit-subject-mismatch) = %d; want 1", got)
		}
	})
}

// TestValidate_DependsOnOrder covers depends-on-order: a Depends-on: id must name a strictly
// earlier card — naming itself, a later card, or a nonexistent card number are each their own
// trigger.
func TestValidate_DependsOnOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dependsOn []int
		wantCount int
	}{
		{name: "clean (depends on earlier card)", dependsOn: []int{1}, wantCount: 0},
		{name: "depends on itself", dependsOn: []int{2}, wantCount: 1},
		{name: "depends on a later card", dependsOn: []int{3}, wantCount: 1},
		{name: "depends on a nonexistent card", dependsOn: []int{99}, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c1 := validCard(1, "a")
			c2 := validCard(2, "b", tt.dependsOn...)
			c3 := validCard(3, "c")
			plan := &planparser.Plan{Format: 3, Approved: true, Cards: []planparser.Card{c1, c2, c3}}
			findings := planparser.Validate(plan, t.TempDir())
			if got := countFor(findings, "depends-on-order"); got != tt.wantCount {
				t.Errorf("countFor(findings, depends-on-order) = %d; want %d", got, tt.wantCount)
			}
		})
	}
}
