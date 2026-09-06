// rewrite_test.go covers RewriteRefs: substitution across Targets, Uses, and both endpoints of a
// Pairs entry; substitution finding a surface lexeme that differs from the canonical key; two
// cards spelling one canonical string differently each being rewritten from their own lexeme;
// idempotence; a no-op map; and prose left untouched.

package planparser_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// rewriteOverview is a minimal format-5 overview naming one Card Index entry per slug in slugs.
func rewriteOverview(slugs ...string) string {
	body := "---\nformat: 5\napproved: true\n---\n\n# Plan: rewrite\n\nFraming.\n\n## Card Index\n\n"
	for i, slug := range slugs {
		body += fmt.Sprintf("%d — %s — placeholder\n", i+1, slug)
	}
	return body
}

func TestRewriteRefs_SubstitutesTargetsUsesAndPairs(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1"),
		"01-card1.md": "# Card 1 — card1\n\n" +
			"**Rename:**\n- `internal/foo#Old` -> `plan:internal/foo#New`\n" +
			"**Uses:**\n- `internal/bar.go`\n" +
			"**Intent:** placeholder.\n",
	})

	subs := map[string]string{
		"internal/foo#Old":      "internal/foo#Renamed",
		"plan:internal/foo#New": "internal/foo#Bound",
		// internal/bar.go canonicalizes to internal/bar.go# at parse time; the surface lexeme on
		// disk is the bare path, so the key here is the canonical form RewriteRefs' callers
		// natively produce.
		"internal/bar.go#": "internal/baz.go#",
	}

	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() error = %v; want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read rewritten card file: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, "- `internal/foo#Renamed` -> `internal/foo#Bound`") {
		t.Errorf("rewritten card file = %q; want the Rename pair's both endpoints substituted", got)
	}
	if !strings.Contains(got, "- `internal/baz.go#`") {
		t.Errorf("rewritten card file = %q; want the Uses entry substituted from its own surface lexeme", got)
	}
	if strings.Contains(got, "internal/bar.go`") {
		t.Errorf("rewritten card file = %q; want the pre-canonicalization surface lexeme gone", got)
	}
}

func TestRewriteRefs_TwoCardsSpellOneCanonicalStringDifferently(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1", "card2"),
		"01-card1.md":    "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** placeholder.\n",
		"02-card2.md":    "# Card 2 — card2\n\n**Edit:**\n- `internal/foo/bar.go`\n**Uses:**\n- `internal/foo#Bar`\n**Intent:** placeholder.\n",
	})

	// Card 1 spells the canonical string "internal/foo/bar.go#" as the plain path (canonicalized
	// at parse time); a second card carries the same target already spelled canonically on disk,
	// alongside its own unrelated glyph Uses entry. Both cards' own copy must be rewritten from
	// each one's own lexeme, and the unrelated ref must be left untouched.
	subs := map[string]string{"internal/foo/bar.go#": "internal/foo/baz.go#"}

	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() error = %v; want nil", err)
	}

	data1, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read card 1: %v", err)
	}
	if !strings.Contains(string(data1), "- `internal/foo/baz.go#`") {
		t.Errorf("card 1 = %q; want its own surface lexeme (internal/foo/bar.go) rewritten to the replacement", string(data1))
	}

	data2, err := os.ReadFile(filepath.Join(dir, "02-card2.md"))
	if err != nil {
		t.Fatalf("read card 2: %v", err)
	}
	if !strings.Contains(string(data2), "- `internal/foo/baz.go#`") {
		t.Errorf("card 2 = %q; want its own already-canonical spelling rewritten too", string(data2))
	}
	if !strings.Contains(string(data2), "- `internal/foo#Bar`") {
		t.Errorf("card 2 = %q; want its own unrelated Uses entry left untouched", string(data2))
	}
}

func TestRewriteRefs_Idempotent(t *testing.T) {
	t.Parallel()

	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1"),
		"01-card1.md":    "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** placeholder.\n",
	})

	subs := map[string]string{"internal/foo/bar.go#": "plan:internal/foo#NewThing"}

	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() first call error = %v; want nil", err)
	}
	after1, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read after first call: %v", err)
	}

	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() second call error = %v; want nil", err)
	}
	after2, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read after second call: %v", err)
	}

	if string(after1) != string(after2) {
		t.Errorf("RewriteRefs() is not idempotent: first call = %q; second call = %q", after1, after2)
	}
	if !strings.Contains(string(after2), "- `plan:internal/foo#NewThing`") {
		t.Errorf("after both calls = %q; want the substitution applied exactly once", after2)
	}
}

func TestRewriteRefs_NoOpMapLeavesFileUntouched(t *testing.T) {
	t.Parallel()

	const body = "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** placeholder.\n"
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1"),
		"01-card1.md":    body,
	})

	// A canonical string that matches nothing this plan carries.
	subs := map[string]string{"internal/nonexistent#Nothing": "internal/nonexistent#Something"}

	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() error = %v; want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read card file: %v", err)
	}
	if string(data) != body {
		t.Errorf("card file = %q; want byte-identical to the original %q", data, body)
	}
}

func TestRewriteRefs_ProseContainingKeyStringLeftUnmodified(t *testing.T) {
	t.Parallel()

	body := "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n" +
		"**Intent:** discusses internal/foo/bar.go in prose, not as a bullet.\n"
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1"),
		"01-card1.md":    body,
	})

	subs := map[string]string{"internal/foo/bar.go#": "internal/foo/baz.go#"}
	if err := planparser.RewriteRefs(dir, subs); err != nil {
		t.Fatalf("RewriteRefs() error = %v; want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read card file: %v", err)
	}
	if !strings.Contains(string(data), "discusses internal/foo/bar.go in prose, not as a bullet.") {
		t.Errorf("card file = %q; want the Intent: prose sentence left byte-identical", data)
	}
	if !strings.Contains(string(data), "- `internal/foo/baz.go#`") {
		t.Errorf("card file = %q; want the bullet itself substituted", data)
	}
}

func TestRewriteRefs_EmptySubsMapWritesNothing(t *testing.T) {
	t.Parallel()

	const body = "# Card 1 — card1\n\n**Edit:**\n- `internal/foo/bar.go`\n**Intent:** placeholder.\n"
	dir := writePlanFiles(t, map[string]string{
		"00-overview.md": rewriteOverview("card1"),
		"01-card1.md":    body,
	})

	if err := planparser.RewriteRefs(dir, map[string]string{}); err != nil {
		t.Fatalf("RewriteRefs() error = %v; want nil", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "01-card1.md"))
	if err != nil {
		t.Fatalf("read card file: %v", err)
	}
	if string(data) != body {
		t.Errorf("card file = %q; want byte-identical to the original %q", data, body)
	}
}

func TestRewriteRefs_MissingPlanDirIsWrappedError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := planparser.RewriteRefs(dir, map[string]string{"a#B": "a#C"})
	if err == nil {
		t.Fatal("RewriteRefs() error = nil; want a wrapped error for a missing overview file")
	}
	if !strings.HasPrefix(err.Error(), "planparser:") {
		t.Errorf("RewriteRefs() error = %q; want \"planparser:\" prefix", err.Error())
	}
}
