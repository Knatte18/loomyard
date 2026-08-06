// index_test.go — Tier-1 unit coverage for parseTrailerScanRecord, the pure per-record parser
// scanWarpSHATrailers's git-log scan loop calls.
// No git spawn: every case here operates on hand-built record strings, never a real commit history,
// so this file carries no build constraint and must never import "os/exec" or reference a
// git-spawning helper.

package fabricengine

import (
	"testing"
)

// TestParseTrailerScanRecord_MultipleTagsSplitOnNewline is the TDD case for this card: a commit
// carrying several "Snapshot:" trailers renders as a MULTI-LINE value for the one Snapshot
// placeholder (one line per trailer occurrence), so the parser must split within that field on "\n"
// rather than treating the whole multi-line value as a single tag.
// Written first and run against the pre-split implementation to confirm it fails before the split
// is added.
func TestParseTrailerScanRecord_MultipleTagsSplitOnNewline(t *testing.T) {
	t.Parallel()

	record := "abc123" + warpSHATrailerFormatUnitSep +
		"def456" + warpSHATrailerFormatUnitSep +
		"raddle\ntrace"

	weftSHA, warpSHA, snapshotTags, ok := parseTrailerScanRecord(record)
	if !ok {
		t.Fatalf("parseTrailerScanRecord(%q) ok = false; want true", record)
	}
	if weftSHA != "abc123" {
		t.Errorf("parseTrailerScanRecord(%q) weftSHA = %q; want %q", record, weftSHA, "abc123")
	}
	if warpSHA != "def456" {
		t.Errorf("parseTrailerScanRecord(%q) warpSHA = %q; want %q", record, warpSHA, "def456")
	}
	wantTags := []string{"raddle", "trace"}
	if len(snapshotTags) != len(wantTags) {
		t.Fatalf("parseTrailerScanRecord(%q) snapshotTags = %v; want %v", record, snapshotTags, wantTags)
	}
	for i, tag := range wantTags {
		if snapshotTags[i] != tag {
			t.Errorf("parseTrailerScanRecord(%q) snapshotTags[%d] = %q; want %q", record, i, snapshotTags[i], tag)
		}
	}
}

// TestParseTrailerScanRecord covers the remaining record shapes as a table: no Snapshot field at
// all, exactly one tag, a Warp-SHA-less record (skipped even though it carries a Snapshot value),
// an empty record, and surrounding-newline trimming.
func TestParseTrailerScanRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		record      string
		wantWeftSHA string
		wantWarpSHA string
		wantTags    []string
		wantOK      bool
	}{
		{
			name:        "NoSnapshotField",
			record:      "abc123" + warpSHATrailerFormatUnitSep + "def456",
			wantWeftSHA: "abc123",
			wantWarpSHA: "def456",
			wantTags:    nil,
			wantOK:      true,
		},
		{
			name:        "ExactlyOneTag",
			record:      "abc123" + warpSHATrailerFormatUnitSep + "def456" + warpSHATrailerFormatUnitSep + "raddle",
			wantWeftSHA: "abc123",
			wantWarpSHA: "def456",
			wantTags:    []string{"raddle"},
			wantOK:      true,
		},
		{
			name:        "SnapshotWithNoWarpSHAIsSkipped",
			record:      "abc123" + warpSHATrailerFormatUnitSep + "" + warpSHATrailerFormatUnitSep + "raddle",
			wantWeftSHA: "",
			wantWarpSHA: "",
			wantTags:    nil,
			wantOK:      false,
		},
		{
			name:        "EmptyRecord",
			record:      "",
			wantWeftSHA: "",
			wantWarpSHA: "",
			wantTags:    nil,
			wantOK:      false,
		},
		{
			name:        "SurroundingNewlinesAreTrimmed",
			record:      "\nabc123" + warpSHATrailerFormatUnitSep + "def456" + warpSHATrailerFormatUnitSep + "raddle\n",
			wantWeftSHA: "abc123",
			wantWarpSHA: "def456",
			wantTags:    []string{"raddle"},
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			weftSHA, warpSHA, snapshotTags, ok := parseTrailerScanRecord(tt.record)
			if ok != tt.wantOK {
				t.Fatalf("parseTrailerScanRecord(%q) ok = %v; want %v", tt.record, ok, tt.wantOK)
			}
			if weftSHA != tt.wantWeftSHA {
				t.Errorf("parseTrailerScanRecord(%q) weftSHA = %q; want %q", tt.record, weftSHA, tt.wantWeftSHA)
			}
			if warpSHA != tt.wantWarpSHA {
				t.Errorf("parseTrailerScanRecord(%q) warpSHA = %q; want %q", tt.record, warpSHA, tt.wantWarpSHA)
			}
			if len(snapshotTags) != len(tt.wantTags) {
				t.Fatalf("parseTrailerScanRecord(%q) snapshotTags = %v; want %v", tt.record, snapshotTags, tt.wantTags)
			}
			for i, tag := range tt.wantTags {
				if snapshotTags[i] != tag {
					t.Errorf("parseTrailerScanRecord(%q) snapshotTags[%d] = %q; want %q", tt.record, i, snapshotTags[i], tag)
				}
			}
		})
	}
}
