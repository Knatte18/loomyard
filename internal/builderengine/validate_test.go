// validate_test.go covers Validate's checks against plan-format v2: the
// plan-valid fixture must yield zero findings, plan-unapproved and
// plan-broken-chain must trip their designed checks, and synthetic
// in-memory plans exercise the checks that need disk state or cap values
// the hand-written fixtures do not exercise (index-file-mismatch,
// verify-missing, batch-oversized, scope-malformed) plus the five move-*
// checks (move-format, move-redundant, move-source-missing,
// move-target-collision, move-mechanic-missing). batch-oversized sums Scope
// plus every card's typed file-op paths and compares len(PlanBatch.Cards)
// against the card cap.

package builderengine_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
)

// generousCaps is large enough that no plan-valid batch's estimated
// context or card count ever trips check 5 by accident.
var generousCaps = builderengine.ValidateCaps{ContextCapTokens: 100_000, CardCap: 10}

func TestValidate_PlanValidFixture_ZeroFindings(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-valid")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	findings := builderengine.Validate(plan, dir, generousCaps)
	if len(findings) != 0 {
		t.Errorf("Validate(plan-valid) = %+v; want zero findings", findings)
	}
}

func TestValidate_PlanUnapproved_TripsCheck1(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-unapproved")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	findings := builderengine.Validate(plan, dir, generousCaps)
	if len(findings) != 1 {
		t.Fatalf("Validate(plan-unapproved) = %+v; want exactly 1 finding", findings)
	}
	if findings[0].Check != "plan-unapproved" {
		t.Errorf("findings[0].Check = %q; want %q", findings[0].Check, "plan-unapproved")
	}
}

func TestValidate_PlanBrokenChain_TripsCheck4Twice(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "plan-broken-chain")
	plan, err := builderengine.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}

	findings := builderengine.Validate(plan, dir, generousCaps)
	var chainFindings []builderengine.ValidationError
	for _, f := range findings {
		if f.Check == "chain-end-dangling" {
			chainFindings = append(chainFindings, f)
		}
	}
	if len(chainFindings) != 2 {
		t.Fatalf("chain-end-dangling findings = %+v; want exactly 2", chainFindings)
	}

	if chainFindings[0].Batch != "01-first" {
		t.Errorf("chainFindings[0].Batch = %q; want %q (dangling target)", chainFindings[0].Batch, "01-first")
	}
	if !strings.Contains(chainFindings[0].Detail, "does not exist") {
		t.Errorf("chainFindings[0].Detail = %q; want it to name the dangling target", chainFindings[0].Detail)
	}

	if chainFindings[1].Batch != "02-second" {
		t.Errorf("chainFindings[1].Batch = %q; want %q (self-deferred target)", chainFindings[1].Batch, "02-second")
	}
	if !strings.Contains(chainFindings[1].Detail, "verify: deferred") && !strings.Contains(chainFindings[1].Detail, "deferred") {
		t.Errorf("chainFindings[1].Detail = %q; want it to name the self-deferred target", chainFindings[1].Detail)
	}
}

// syntheticPlan builds a minimal in-memory Plan for the checks below that
// need shapes the hand-written fixtures deliberately avoid (an unapproved
// or dangling-chain plan cannot also carry a numbering gap, an oversized
// batch, or a malformed scope entry without conflating what each fixture is
// meant to demonstrate).
func syntheticPlan(dir string, batches ...builderengine.PlanBatch) *builderengine.Plan {
	return &builderengine.Plan{
		Dir:      dir,
		Format:   2,
		Approved: true,
		Batches:  batches,
	}
}

// nCards returns a Cards slice of length n (values are irrelevant — the
// synthetic-plan tests below only care about len(b.Cards) for the
// batch-oversized card-count cap).
func nCards(n int) []builderengine.PlanCard {
	return make([]builderengine.PlanCard, n)
}

func TestValidate_IndexFileMismatch(t *testing.T) {
	t.Parallel()

	t.Run("index names a missing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "missing", File: "01-missing.md",
			VerifyCommand: "go build ./...",
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "index-file-mismatch", "01-missing") {
			t.Errorf("Validate() = %+v; want an index-file-mismatch finding for 01-missing", findings)
		}
	})

	t.Run("file on disk not referenced by the index", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"00-overview.md": "unused in this synthetic test",
			"01-first.md":    "content",
			"02-orphan.md":   "never referenced by the index",
		})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		found := false
		for _, f := range findings {
			if f.Check == "index-file-mismatch" && strings.Contains(f.Detail, "02-orphan.md") {
				found = true
			}
		}
		if !found {
			t.Errorf("Validate() = %+v; want an index-file-mismatch finding naming 02-orphan.md", findings)
		}
	})

	t.Run("numbering gap", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"01-first.md": "content",
			"03-third.md": "content",
		})
		plan := syntheticPlan(dir,
			builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md", VerifyCommand: "go build ./..."},
			builderengine.PlanBatch{Number: 3, Slug: "third", File: "03-third.md", VerifyCommand: "go build ./..."},
		)

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "index-file-mismatch", "03-third") {
			t.Errorf("Validate() = %+v; want an index-file-mismatch finding for the numbering gap at 03-third", findings)
		}
	})
}

func TestValidate_VerifyMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"01-first.md": "content"})

	tests := []struct {
		name  string
		batch builderengine.PlanBatch
		want  bool
	}{
		{
			name:  "no command, not deferred",
			batch: builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md"},
			want:  true,
		},
		{
			name:  "deferred without chain-end",
			batch: builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md", VerifyDeferred: true},
			want:  true,
		},
		{
			name:  "has command",
			batch: builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md", VerifyCommand: "go build ./..."},
			want:  false,
		},
		{
			name:  "deferred with chain-end",
			batch: builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md", VerifyDeferred: true, ChainEnd: 2},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := syntheticPlan(dir, tt.batch)
			findings := builderengine.Validate(plan, dir, generousCaps)
			got := hasFinding(findings, "verify-missing", "01-first")
			if got != tt.want {
				t.Errorf("verify-missing finding present = %v; want %v (findings: %+v)", got, tt.want, findings)
			}
		})
	}
}

func TestValidate_BatchOversized(t *testing.T) {
	t.Parallel()

	t.Run("over card cap without oversized flag", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards:         nCards(999),
		})

		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 100_000, CardCap: 10})
		if !hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want a batch-oversized finding for the card-count cap", findings)
		}
	})

	t.Run("over context cap without oversized flag", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"big.go": strings.Repeat("x", 4000)})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Scope:         []string{"big.go"},
		})

		// 4000 bytes / 4 = 1000 estimated tokens, over a cap of 10.
		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 10, CardCap: 10})
		if !hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want a batch-oversized finding for the context-estimate cap", findings)
		}
	})

	t.Run("over cap but oversized: true is clean", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards:         nCards(999),
			Oversized:     true,
		})

		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 100_000, CardCap: 10})
		if hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want no batch-oversized finding once oversized: true is set", findings)
		}
	})

	t.Run("nonexistent scope entries contribute zero bytes", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Scope:         []string{"does/not/exist.go"},
		})

		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 0, CardCap: 10})
		if hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want no batch-oversized finding for a non-existent scope entry", findings)
		}
	})

	t.Run("estimate sums every card's typed file-op paths", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"card-context.go": strings.Repeat("x", 4000)})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards: []builderengine.PlanCard{
				{ContextFiles: []string{"card-context.go"}},
			},
		})

		// 4000 bytes / 4 = 1000 estimated tokens, over a cap of 10.
		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 10, CardCap: 10})
		if !hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want a batch-oversized finding driven by a card's Context: path", findings)
		}
	})

	t.Run("nonexistent Creates target and Moves destination contribute zero bytes", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards: []builderengine.PlanCard{
				{
					CreatesFiles: []string{"does/not/exist-yet.go"},
					Moves:        []builderengine.MovePair{{Old: "does/not/exist.go", New: "does/not/exist-either.go"}},
				},
			},
		})

		findings := builderengine.Validate(plan, dir, builderengine.ValidateCaps{ContextCapTokens: 0, CardCap: 10})
		if hasFinding(findings, "batch-oversized", "01-first") {
			t.Errorf("Validate() = %+v; want no batch-oversized finding for nonexistent Creates:/Moves: paths", findings)
		}
	})
}

func TestValidate_ScopeMalformed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		entry string
		want  bool
	}{
		{name: "empty entry", entry: "", want: true},
		{name: "absolute path", entry: "/etc/passwd", want: true},
		{name: "dot-dot escape", entry: "../escape.go", want: true},
		{name: "unclean double slash", entry: "internal//list.go", want: true},
		{name: "well-formed relative path", entry: "internal/boardcli/list.go", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			plan := syntheticPlan(dir, builderengine.PlanBatch{
				Number: 1, Slug: "first", File: "01-first.md",
				VerifyCommand: "go build ./...",
				Scope:         []string{tt.entry},
			})

			findings := builderengine.Validate(plan, dir, generousCaps)
			got := hasFinding(findings, "scope-malformed", "01-first")
			if got != tt.want {
				t.Errorf("scope-malformed finding present for entry %q = %v; want %v (findings: %+v)", tt.entry, got, tt.want, findings)
			}
		})
	}
}

func TestValidate_FormatUnrecognized(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	plan := &builderengine.Plan{Dir: dir, Format: 1, Approved: true}

	findings := builderengine.Validate(plan, dir, generousCaps)
	if !hasFinding(findings, "format-unrecognized", "") {
		t.Errorf("Validate() = %+v; want a format-unrecognized finding", findings)
	}
}

// TestValidate_MoveFormat covers move-format: a malformed Moves: bullet
// (retained verbatim in PlanCard.MovesRaw by the parser) yields one finding
// citing the card and quoting the raw entry; a well-formed pair produces
// none.
func TestValidate_MoveFormat(t *testing.T) {
	t.Parallel()

	t.Run("malformed raw entry flags the citing card", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards: []builderengine.PlanCard{
				{BatchPrefix: 1, Number: 1, MovesRaw: []string{"no arrow here"}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		found := false
		for _, f := range findings {
			if f.Check == "move-format" && f.Batch == "01-first" &&
				strings.Contains(f.Detail, "01.1") && strings.Contains(f.Detail, "no arrow here") {
				found = true
			}
		}
		if !found {
			t.Errorf("Validate() = %+v; want a move-format finding citing card 01.1 and quoting the raw entry", findings)
		}
	})

	t.Run("well-formed moves produce no finding", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"a.go": "content"})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{{Old: "a.go", New: "b.go"}}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-format", "01-first") {
			t.Errorf("Validate() = %+v; want no move-format finding for a well-formed pair", findings)
		}
	})
}

// TestValidate_MoveRedundant covers move-redundant: an endpoint duplicated
// into the same batch's Creates: is flagged, but a rename plus a DIFFERENT
// Creates: path (extraction alongside a rename) is not.
func TestValidate_MoveRedundant(t *testing.T) {
	t.Parallel()

	t.Run("endpoint duplicated into Creates flags move-redundant", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"old.go": "content"})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{
					Moves:        []builderengine.MovePair{{Old: "old.go", New: "new.go"}},
					CreatesFiles: []string{"new.go"},
				},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-redundant", "01-first") {
			t.Errorf("Validate() = %+v; want a move-redundant finding", findings)
		}
	})

	t.Run("rename plus a different Creates path is not redundant", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"old.go": "content"})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{
					Moves:        []builderengine.MovePair{{Old: "old.go", New: "new.go"}},
					CreatesFiles: []string{"other.go"},
				},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-redundant", "01-first") {
			t.Errorf("Validate() = %+v; want no move-redundant finding for a different Creates path", findings)
		}
	})
}

// TestValidate_MoveSourceMissing covers move-source-missing: a source path
// with no on-disk file and no plan-wide Creates:/Moves-target suppression is
// flagged; a source satisfied by another batch's Creates: is suppressed; and
// a chained rename across two batches (A: x -> y, B: y -> z) is suppressed
// regardless of batch order.
func TestValidate_MoveSourceMissing(t *testing.T) {
	t.Parallel()

	t.Run("missing source is flagged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{{Old: "missing.go", New: "renamed.go"}}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-source-missing", "01-first") {
			t.Errorf("Validate() = %+v; want a move-source-missing finding", findings)
		}
	})

	t.Run("source satisfied by another batch's Creates is suppressed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir,
			builderengine.PlanBatch{
				Number: 1, Slug: "creator", File: "01-creator.md",
				VerifyCommand: "go build ./...",
				Cards: []builderengine.PlanCard{
					{CreatesFiles: []string{"generated.go"}},
				},
			},
			builderengine.PlanBatch{
				Number: 2, Slug: "renamer", File: "02-renamer.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "generated.go", New: "renamed.go"}}},
				},
			},
		)

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-source-missing", "02-renamer") {
			t.Errorf("Validate() = %+v; want no move-source-missing finding (source is created by another batch)", findings)
		}
	})

	t.Run("chained rename across batches is suppressed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"x.go": "content"})
		plan := syntheticPlan(dir,
			builderengine.PlanBatch{
				Number: 1, Slug: "first", File: "01-first.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "x.go", New: "y.go"}}},
				},
			},
			builderengine.PlanBatch{
				Number: 2, Slug: "second", File: "02-second.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "y.go", New: "z.go"}}},
				},
			},
		)

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-source-missing", "02-second") {
			t.Errorf("Validate() = %+v; want no move-source-missing finding for a chained rename (batch A: x->y, batch B: y->z)", findings)
		}
	})
}

// TestValidate_MoveTargetCollision covers move-target-collision's three
// OR'd conditions: an existing on-disk target, two batches targeting the
// same path, and a cross-batch Creates: collision (same-batch overlap is
// move-redundant's job, deliberately not re-flagged here).
func TestValidate_MoveTargetCollision(t *testing.T) {
	t.Parallel()

	t.Run("existing target on disk is flagged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"already-there.go": "content"})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{{Old: "src.go", New: "already-there.go"}}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-target-collision", "01-first") {
			t.Errorf("Validate() = %+v; want a move-target-collision finding for an existing on-disk target", findings)
		}
	})

	t.Run("two batches targeting the same path are both flagged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir,
			builderengine.PlanBatch{
				Number: 1, Slug: "first", File: "01-first.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "a.go", New: "shared.go"}}},
				},
			},
			builderengine.PlanBatch{
				Number: 2, Slug: "second", File: "02-second.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "b.go", New: "shared.go"}}},
				},
			},
		)

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-target-collision", "01-first") || !hasFinding(findings, "move-target-collision", "02-second") {
			t.Errorf("Validate() = %+v; want a move-target-collision finding for both batches targeting shared.go", findings)
		}
	})

	t.Run("cross-batch Creates collision is flagged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir,
			builderengine.PlanBatch{
				Number: 1, Slug: "first", File: "01-first.md",
				VerifyCommand:     "go build ./...",
				HasRenameMechanic: true,
				Cards: []builderengine.PlanCard{
					{Moves: []builderengine.MovePair{{Old: "a.go", New: "target.go"}}},
				},
			},
			builderengine.PlanBatch{
				Number: 2, Slug: "second", File: "02-second.md",
				VerifyCommand: "go build ./...",
				Cards: []builderengine.PlanCard{
					{CreatesFiles: []string{"target.go"}},
				},
			},
		)

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-target-collision", "01-first") {
			t.Errorf("Validate() = %+v; want a move-target-collision finding for the cross-batch Creates: collision", findings)
		}
	})
}

// TestValidate_MoveMechanicMissing covers move-mechanic-missing: a batch
// with a Moves: pair but no "## Rename mechanic" section is flagged; the
// same batch with the section is clean; and a batch whose every Moves:
// field is "none" (zero pairs) is clean even without the section.
func TestValidate_MoveMechanicMissing(t *testing.T) {
	t.Parallel()

	t.Run("moves without the section is flagged", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{{Old: "a.go", New: "b.go"}}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if !hasFinding(findings, "move-mechanic-missing", "01-first") {
			t.Errorf("Validate() = %+v; want a move-mechanic-missing finding", findings)
		}
	})

	t.Run("moves with the section is clean", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"a.go": "content"})
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand:     "go build ./...",
			HasRenameMechanic: true,
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{{Old: "a.go", New: "b.go"}}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-mechanic-missing", "01-first") {
			t.Errorf("Validate() = %+v; want no move-mechanic-missing finding when the section is present", findings)
		}
	})

	t.Run("none-only batch without the section is clean", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		plan := syntheticPlan(dir, builderengine.PlanBatch{
			Number: 1, Slug: "first", File: "01-first.md",
			VerifyCommand: "go build ./...",
			Cards: []builderengine.PlanCard{
				{Moves: []builderengine.MovePair{}},
			},
		})

		findings := builderengine.Validate(plan, dir, generousCaps)
		if hasFinding(findings, "move-mechanic-missing", "01-first") {
			t.Errorf("Validate() = %+v; want no move-mechanic-missing finding for a none-only Moves: field", findings)
		}
	})
}

// hasFinding reports whether findings contains an entry matching both check
// and batch (an empty batch matches a plan-level finding).
func hasFinding(findings []builderengine.ValidationError, check, batch string) bool {
	for _, f := range findings {
		if f.Check == check && f.Batch == batch {
			return true
		}
	}
	return false
}

// TestValidate_FindingsOrderedByCheckThenBatch pins the deterministic
// ordering Validate promises: findings are grouped by check (in check-1..6
// order) and, within a check, by ascending batch number.
func TestValidate_FindingsOrderedByCheckThenBatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"01-first.md":  "content",
		"02-second.md": "content",
	})
	// Both batches are missing a verify: (check 3) and batch 2 additionally
	// has a malformed scope entry (check 6) — check 3's findings must all
	// precede check 6's, and within check 3, batch 1 must precede batch 2.
	plan := syntheticPlan(dir,
		builderengine.PlanBatch{Number: 1, Slug: "first", File: "01-first.md"},
		builderengine.PlanBatch{Number: 2, Slug: "second", File: "02-second.md", Scope: []string{"../escape.go"}},
	)

	findings := builderengine.Validate(plan, dir, generousCaps)

	var order []string
	for _, f := range findings {
		order = append(order, f.Check+"/"+f.Batch)
	}
	want := []string{"verify-missing/01-first", "verify-missing/02-second", "scope-malformed/02-second"}
	if len(order) != len(want) {
		t.Fatalf("finding order = %v; want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("finding order = %v; want %v", order, want)
			break
		}
	}
}

// TestValidate_ValidationErrorImplementsError pins ValidationError's Error()
// formatting, which lets a finding be used directly wherever a single error
// value is expected (e.g. an errors.New-style substring assertion).
func TestValidate_ValidationErrorImplementsError(t *testing.T) {
	t.Parallel()

	planLevel := builderengine.ValidationError{Check: "plan-unapproved", Detail: "approved: is not true"}
	if got, want := planLevel.Error(), "plan-unapproved: approved: is not true"; got != want {
		t.Errorf("planLevel.Error() = %q; want %q", got, want)
	}

	batchLevel := builderengine.ValidationError{Check: "verify-missing", Batch: "01-first", Detail: "no verify: command"}
	if got, want := batchLevel.Error(), "verify-missing/01-first: no verify: command"; got != want {
		t.Errorf("batchLevel.Error() = %q; want %q", got, want)
	}
}
