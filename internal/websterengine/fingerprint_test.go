// fingerprint_test.go covers fingerprint's identity properties: identical directories fingerprint
// identically,
// and a rename, a one-byte content edit, or an added batch file each change the result, while
// non-.md entries, subdirectories and the amendment log are ignored entirely.
// Tier 1: no git, only t.TempDir().

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// overviewWithIndex is a minimal valid 00-overview.md whose Card Index holds indexLines, for fixtures that restamp a plan directory.
func overviewWithIndex(indexLines string) string {
	return "---\nformat: 5\napproved: true\n---\n\n# Plan: fixture\n\nFraming.\n\n## Card Index\n\n" + indexLines + "\n\n## verify:\n\ngo test ./...\n"
}

// fingerprintWriteFiles writes every entry of files (keyed by relative
// path) into dir, creating any needed subdirectories.
func fingerprintWriteFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

//testtiming:keep pins which plan-directory changes move the fingerprint and which (non-markdown files, subdirectories, the amendment log) do not; the covering record-batch test only observes a refusal on one edit
func TestFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("identical directories match", func(t *testing.T) {
		t.Parallel()

		files := map[string]string{
			"00-overview.md": "overview content",
			"01-first.md":    "first content",
		}

		dirA := t.TempDir()
		dirB := t.TempDir()
		fingerprintWriteFiles(t, dirA, files)
		fingerprintWriteFiles(t, dirB, files)

		fpA, err := fingerprint(dirA)
		if err != nil {
			t.Fatalf("fingerprint(dirA) error = %v; want nil", err)
		}
		fpB, err := fingerprint(dirB)
		if err != nil {
			t.Fatalf("fingerprint(dirB) error = %v; want nil", err)
		}

		if fpA != fpB {
			t.Errorf("fingerprint(dirA) = %q; fingerprint(dirB) = %q; want equal for identical content", fpA, fpB)
		}
	})

	t.Run("the overview frame hash follows everything but the Card Index", func(t *testing.T) {
		t.Parallel()

		frameOf := func(overview string) string {
			dir := t.TempDir()
			fingerprintWriteFiles(t, dir, map[string]string{"00-overview.md": overview})
			frame, err := overviewFrameHash(dir)
			if err != nil {
				t.Fatalf("overviewFrameHash() error = %v", err)
			}
			return frame
		}
		base := frameOf(overviewWithIndex("1 — a — card a"))
		if got := frameOf(overviewWithIndex("1 — a — reworded\n2 — b — added")); got != base {
			t.Errorf("frame hash moved with the Card Index: %q, want %q", got, base)
		}
		if got := frameOf(strings.Replace(overviewWithIndex("1 — a — card a"), "Framing.", "Reframed.", 1)); got == base {
			t.Error("frame hash did not move with the framing")
		}
		if got, err := overviewFrameHash(t.TempDir()); err != nil || got != "" {
			t.Errorf("frame hash of an absent overview = %q, %v; want empty and no error", got, err)
		}
	})

	// Each row changes a plan directory holding 00-overview.md and 01-first.md and states whether
	// the fingerprint must follow.
	tests := []struct {
		name       string
		mutate     func(t *testing.T, dir string)
		wantChange bool
	}{
		{
			name: "changes on a rename",
			mutate: func(t *testing.T, dir string) {
				if err := os.Rename(filepath.Join(dir, "01-first.md"), filepath.Join(dir, "01-renamed.md")); err != nil {
					t.Fatalf("rename: %v", err)
				}
			},
			wantChange: true,
		},
		{
			name: "changes on a one-byte edit",
			mutate: func(t *testing.T, dir string) {
				fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "first contenu"})
			},
			wantChange: true,
		},
		{
			name: "changes on an added batch file",
			mutate: func(t *testing.T, dir string) {
				fingerprintWriteFiles(t, dir, map[string]string{"02-second.md": "more content"})
			},
			wantChange: true,
		},
		{
			// Only top-level *.md files count.
			name: "ignores a non-markdown file and a subdirectory",
			mutate: func(t *testing.T, dir string) {
				fingerprintWriteFiles(t, dir, map[string]string{
					"notes.txt":           "ignored",
					"reports/report-1.md": "also ignored: this is inside a subdirectory",
				})
			},
		},
		{
			// DetectDrift's exact-tier repair creates planparser.AmendmentsFileName inside the plan
			// directory, so folding it into the fingerprint made webster's own repair invalidate
			// the plan it had just repaired.
			name: "ignores the amendment log",
			mutate: func(t *testing.T, dir string) {
				fingerprintWriteFiles(t, dir, map[string]string{
					planparser.AmendmentsFileName: "# Amendments\n- Timestamp: t, Card: 1-card, OldGlyph: a#B, NewGlyph: a#C, Tier: exact, SHA: deadbeef\n",
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			fingerprintWriteFiles(t, dir, map[string]string{
				"00-overview.md": "overview content",
				"01-first.md":    "first content",
			})
			before, err := fingerprint(dir)
			if err != nil {
				t.Fatalf("fingerprint(before) error = %v; want nil", err)
			}

			tt.mutate(t, dir)

			after, err := fingerprint(dir)
			if err != nil {
				t.Fatalf("fingerprint(after) error = %v; want nil", err)
			}
			if changed := before != after; changed != tt.wantChange {
				t.Errorf("fingerprint changed = %v (before %q, after %q); want %v", changed, before, after, tt.wantChange)
			}
		})
	}
}

// TestMoveBegunCardHashes proves a begun card's recorded hash moves only when it equals the pre-rewrite file hash,
// a mismatching one keeps its old hash, a file absent from either map is left alone, and empty PlanFileHashes is a no-op.
func TestMoveBegunCardHashes(t *testing.T) {
	t.Parallel()

	newState := func() *State {
		return &State{Batches: map[int]*BatchState{
			1: {CardHashes: map[string]string{"01-a": "a0"}},
			2: {CardHashes: map[string]string{"02-b": "stale"}},
			3: {CardHashes: map[string]string{"03-c": "c0"}},
			4: {CardHashes: map[string]string{"04-d": "d0"}},
			5: nil,
			6: {},
		}}
	}
	before := map[string]string{"01-a.md": "a0", "02-b.md": "b0", "03-c.md": "c0"}
	after := map[string]string{"01-a.md": "a1", "02-b.md": "b1", "04-d.md": "d1"}

	t.Run("moves only an exact match", func(t *testing.T) {
		t.Parallel()
		st := newState()
		moveBegunCardHashes(st, before, after)
		if got := st.Batches[1].CardHashes["01-a"]; got != "a1" {
			t.Errorf("matching hash = %q; want it moved to a1", got)
		}
		if got := st.Batches[2].CardHashes["02-b"]; got != "stale" {
			t.Errorf("mismatching hash = %q; want it kept as stale", got)
		}
		if got := st.Batches[3].CardHashes["03-c"]; got != "c0" {
			t.Errorf("hash for a file absent from after = %q; want it left at c0", got)
		}
		if got := st.Batches[4].CardHashes["04-d"]; got != "d0" {
			t.Errorf("hash for a file absent from before = %q; want it left at d0", got)
		}
	})

	t.Run("empty before is a no-op", func(t *testing.T) {
		t.Parallel()
		st := newState()
		moveBegunCardHashes(st, nil, after)
		if got := st.Batches[1].CardHashes["01-a"]; got != "a0" {
			t.Errorf("hash = %q; want it unchanged with no PlanFileHashes", got)
		}
	})
}

// TestRestamp proves both re-baselines record the plan directory's current fingerprint and store one
// baseline copy per recorded hash, so a sanctioned plan rewrite never trips the staleness guard;
// restampFingerprint also adopts a rewrite of a begun card into its recorded hash, and
// restampBaseline, which Rebaseline uses, never does.
//
//testtiming:keep pins both re-baselines' fingerprint and stored copies and that only restampFingerprint moves a begun card's hash; the covering regression test observes one begin
func TestRestamp(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		restamp   func(st *State, planDir, websterDir string) error
		wantMoved bool
	}{
		{"restampFingerprint", restampFingerprint, true},
		{"restampBaseline", restampBaseline, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planDir := t.TempDir()
			fingerprintWriteFiles(t, planDir, map[string]string{"00-overview.md": overviewWithIndex("1 — a — card a"), "01-a.md": "card a\n"})
			st := &State{}
			if err := restampFingerprint(st, planDir, t.TempDir()); err != nil {
				t.Fatalf("restampFingerprint() error = %v", err)
			}
			original := st.PlanFingerprint
			begun := batcher.Batch{Cards: []planparser.Card{{Number: 1, Slug: "a"}}}
			hashes, err := batchCardHashes(begun, planDir)
			if err != nil {
				t.Fatalf("batchCardHashes() error = %v", err)
			}
			st.Batches = map[int]*BatchState{1: {CardHashes: hashes}}

			// Stand in for BindHandles' own RewriteRefs pass.
			fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "card a, rewritten\n"})
			websterDir := t.TempDir()
			if err := tc.restamp(st, planDir, websterDir); err != nil {
				t.Fatalf("restamp() error = %v", err)
			}

			current, err := fingerprint(planDir)
			if err != nil {
				t.Fatalf("fingerprint() error = %v", err)
			}
			if st.PlanFingerprint == original || st.PlanFingerprint != current {
				t.Errorf("State.PlanFingerprint = %s; want the plan directory's current fingerprint %s", st.PlanFingerprint, current)
			}
			frame, err := overviewFrameHash(planDir)
			if err != nil || frame == "" || st.PlanOverviewFrameHash != frame {
				t.Errorf("State.PlanOverviewFrameHash = %q; want the overview's frame hash %q (err %v)", st.PlanOverviewFrameHash, frame, err)
			}
			stored, err := os.ReadDir(filepath.Join(websterDir, planBaselineDirName))
			if err != nil {
				t.Fatalf("read plan baseline store: %v", err)
			}
			if len(stored) != len(st.PlanFileHashes) {
				t.Errorf("stored copies = %d; want one per recorded hash (%d)", len(stored), len(st.PlanFileHashes))
			}

			now, err := batchCardHashes(begun, planDir)
			if err != nil {
				t.Fatalf("batchCardHashes() error = %v", err)
			}
			moved := st.Batches[1].CardHashes["01-a"] == now["01-a"]
			if moved != tc.wantMoved {
				t.Errorf("recorded hash moved = %v; want %v", moved, tc.wantMoved)
			}
		})
	}
}

// editFixture writes a two-file plan, restamps a state over it, and returns the state and the batch holding card 01-a.
func editFixture(t *testing.T) (*State, *BatchState, batcher.Batch, string) {
	t.Helper()
	planDir := t.TempDir()
	fingerprintWriteFiles(t, planDir, map[string]string{
		"00-overview.md": overviewWithIndex("1 — a — card a"),
		"01-a.md":        "card a\n",
	})
	st := &State{}
	if err := restampFingerprint(st, planDir, t.TempDir()); err != nil {
		t.Fatalf("restampFingerprint() error = %v", err)
	}
	b := batcher.Batch{Cards: []planparser.Card{{Number: 1, Slug: "a"}}}
	hashes, err := batchCardHashes(b, planDir)
	if err != nil {
		t.Fatalf("batchCardHashes() error = %v", err)
	}
	return st, &BatchState{CardHashes: hashes}, b, planDir
}

// TestPlanEditErrors proves an unchanged plan passes both guards, a card edit makes PlanEditError wrap ErrFingerprintMismatch naming rebaseline and restore-plan,
// and a begun card edited since its batch began is named by batchCardEditError.
func TestPlanEditErrors(t *testing.T) {
	t.Parallel()

	t.Run("PlanEditError is nil on an unchanged plan and names the way forward after an edit", func(t *testing.T) {
		t.Parallel()

		st, _, _, planDir := editFixture(t)

		if err := PlanEditError(st, planDir); err != nil {
			t.Fatalf("PlanEditError() on an unchanged plan = %v; want nil", err)
		}

		fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "card a, edited\n"})
		err := PlanEditError(st, planDir)
		if !errors.Is(err, ErrFingerprintMismatch) {
			t.Fatalf("PlanEditError() after a card edit = %v; want ErrFingerprintMismatch", err)
		}
		for _, want := range []string{"rebaseline --card 01", "restore-plan"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("PlanEditError() = %q; want it to name %q", err, want)
			}
		}
	})

	t.Run("batchCardEditError names the edited begun card", func(t *testing.T) {
		t.Parallel()

		st, bs, b, planDir := editFixture(t)

		if err := batchCardEditError(st, bs, b, planDir); err != nil {
			t.Fatalf("batchCardEditError() on unedited cards = %v; want nil", err)
		}

		fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "card a, edited\n"})
		err := batchCardEditError(st, bs, b, planDir)
		if !errors.Is(err, ErrFingerprintMismatch) {
			t.Fatalf("batchCardEditError() after a card edit = %v; want ErrFingerprintMismatch", err)
		}
		if want := "batch 01 card 01-a changed since it was begun"; !strings.Contains(err.Error(), want) {
			t.Errorf("batchCardEditError() = %q; want it to name %q", err, want)
		}
		if !strings.Contains(err.Error(), "restore-plan") {
			t.Errorf("batchCardEditError() = %q; want the restore-plan way forward", err)
		}

		if err := batchCardEditError(st, &BatchState{}, b, planDir); err != nil {
			t.Errorf("batchCardEditError() on a record without card hashes = %v; want nil", err)
		}
	})
}
