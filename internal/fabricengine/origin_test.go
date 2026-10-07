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
// and parent_worktree wire keys, and that a record without the parent_worktree key decodes with it
// empty.
//
//testtiming:keep Origin's parent_branch and parent_worktree wire keys round-tripping and a legacy record decoding with an empty ParentWorktree; coverage of its blocks by other tests does not show an assertion of this
func TestOrigin_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		want    Origin
		wantKey string
	}{
		{"parent_branch", Origin{ParentBranch: "main"}, `"parent_branch":"main"`},
		{"parent_worktree", Origin{ParentBranch: "main", ParentWorktree: "prime"}, `"parent_worktree":"prime"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.want)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if !strings.Contains(string(data), tt.wantKey) {
				t.Errorf("Marshal() = %s; want it to contain %s", data, tt.wantKey)
			}

			var got Origin
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal() = %+v; want %+v", got, tt.want)
			}
		})
	}

	var legacy Origin
	if err := json.Unmarshal([]byte(`{"parent_branch":"main"}`), &legacy); err != nil {
		t.Fatalf("Unmarshal(legacy) error = %v", err)
	}
	if legacy.ParentWorktree != "" {
		t.Errorf("legacy ParentWorktree = %q; want empty", legacy.ParentWorktree)
	}
}

// TestOriginRecordPaths asserts that OriginRecordPath joins the anchor path with OriginRecordRel and
// OriginRecordPathFor joins RecordsWorktreePath with AnchorRel and OriginRecordRel, at both
// AnchorRel == "." and a subpath anchor — proving the subpath case moves the record down by the
// anchor, with the anchor segment present in OriginRecordPathFor — and that both end in
// OriginRecordRel(), the anchor-relative form both accessors are built from.
//
//testtiming:keep OriginRecordPath and OriginRecordPathFor joining the anchor and OriginRecordRel at a root and a subpath anchor, both ending in the shared suffix; coverage of its blocks by other tests does not show an assertion of this
func TestOriginRecordPaths(t *testing.T) {
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
			rel := OriginRecordRel()

			got := OriginRecordPath(l)
			want := filepath.Join(l.AnchorPath(), rel)
			if got != want {
				t.Errorf("OriginRecordPath() = %q; want %q", got, want)
			}
			if !strings.HasSuffix(got, rel) {
				t.Errorf("OriginRecordPath() = %q; want it to end in OriginRecordRel() = %q", got, rel)
			}

			gotFor := OriginRecordPathFor(l, slug)
			wantFor := filepath.Join(RecordsWorktreePath(l, slug), l.AnchorRel, rel)
			if gotFor != wantFor {
				t.Errorf("OriginRecordPathFor(%q) = %q; want %q", slug, gotFor, wantFor)
			}
			if tt.anchorRel != "." && !strings.Contains(gotFor, tt.anchorRel) {
				t.Errorf("OriginRecordPathFor(%q) = %q; want it to contain anchor segment %q", slug, gotFor, tt.anchorRel)
			}
			if !strings.HasSuffix(gotFor, rel) {
				t.Errorf("OriginRecordPathFor(%q) = %q; want it to end in OriginRecordRel() = %q", slug, gotFor, rel)
			}
		})
	}
}
