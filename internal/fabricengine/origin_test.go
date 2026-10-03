// origin_test.go — unit tests for the Origin provenance record type and its three path accessors.
// Every Location here comes from locationkit.Location, per the Test
// Tier Purity Invariant: no git spawn, no resolution.

package fabricengine

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
)

// TestOrigin_JSONRoundTrip asserts that Origin marshals and unmarshals through the parent_branch
// wire key.
func TestOrigin_JSONRoundTrip(t *testing.T) {
	want := Origin{ParentBranch: "main"}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"parent_branch":"main"`) {
		t.Errorf("Marshal() = %s; want it to contain the parent_branch wire key", data)
	}

	var got Origin
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Errorf("Unmarshal() = %+v; want %+v", got, want)
	}
}

// TestOrigin_ParentWorktreeRoundTrip asserts that ParentWorktree marshals and unmarshals through the parent_worktree wire key,
// and that a record without the key decodes with it empty.
func TestOrigin_ParentWorktreeRoundTrip(t *testing.T) {
	want := Origin{ParentBranch: "main", ParentWorktree: "prime"}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"parent_worktree":"prime"`) {
		t.Errorf("Marshal() = %s; want it to contain the parent_worktree wire key", data)
	}

	var got Origin
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got != want {
		t.Errorf("Unmarshal() = %+v; want %+v", got, want)
	}

	var legacy Origin
	if err := json.Unmarshal([]byte(`{"parent_branch":"main"}`), &legacy); err != nil {
		t.Fatalf("Unmarshal(legacy) error = %v", err)
	}
	if legacy.ParentWorktree != "" {
		t.Errorf("legacy ParentWorktree = %q; want empty", legacy.ParentWorktree)
	}
}

// TestOriginRecordPath_BothAnchors asserts that OriginRecordPath joins the anchor path with
// OriginRecordRel at both AnchorRel == "." and a subpath anchor, proving the subpath case moves
// the record down by the anchor.
func TestOriginRecordPath_BothAnchors(t *testing.T) {
	hub := filepath.Join("repos", "loomyard-LYXHUB")
	const worktreeName = "loomyard"

	tests := []struct {
		name      string
		anchorRel string
	}{
		{"root", "."},
		{"subpath", "backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := locationkit.Location(hub, worktreeName, tt.anchorRel)
			got := OriginRecordPath(l)
			want := filepath.Join(l.AnchorPath(), OriginRecordRel())
			if got != want {
				t.Errorf("OriginRecordPath() = %q; want %q", got, want)
			}
		})
	}
}

// TestOriginRecordPathFor_BothAnchors asserts that OriginRecordPathFor joins WeftWorktreePath with
// AnchorRel and OriginRecordRel at both AnchorRel == "." and a subpath anchor, asserting the anchor
// segment is present in the subpath case.
func TestOriginRecordPathFor_BothAnchors(t *testing.T) {
	hub := filepath.Join("repos", "loomyard-LYXHUB")
	const worktreeName = "loomyard"
	const slug = "test-wt"

	tests := []struct {
		name      string
		anchorRel string
	}{
		{"root", "."},
		{"subpath", "backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := locationkit.Location(hub, worktreeName, tt.anchorRel)
			got := OriginRecordPathFor(l, slug)
			want := filepath.Join(WeftWorktreePath(l, slug), l.AnchorRel, OriginRecordRel())
			if got != want {
				t.Errorf("OriginRecordPathFor(%q) = %q; want %q", slug, got, want)
			}
			if tt.anchorRel != "." && !strings.Contains(got, tt.anchorRel) {
				t.Errorf("OriginRecordPathFor(%q) = %q; want it to contain anchor segment %q", slug, got, tt.anchorRel)
			}
		})
	}
}

// TestOriginRecordRel_IsTheSharedSuffix asserts that both OriginRecordPath and OriginRecordPathFor
// end in OriginRecordRel(), the anchor-relative form both accessors are built from.
func TestOriginRecordRel_IsTheSharedSuffix(t *testing.T) {
	hub := filepath.Join("repos", "loomyard-LYXHUB")
	const worktreeName = "loomyard"
	const slug = "test-wt"
	l := locationkit.Location(hub, worktreeName, ".")

	rel := OriginRecordRel()

	if got := OriginRecordPath(l); !strings.HasSuffix(got, rel) {
		t.Errorf("OriginRecordPath() = %q; want it to end in OriginRecordRel() = %q", got, rel)
	}
	if got := OriginRecordPathFor(l, slug); !strings.HasSuffix(got, rel) {
		t.Errorf("OriginRecordPathFor(%q) = %q; want it to end in OriginRecordRel() = %q", slug, got, rel)
	}
}
