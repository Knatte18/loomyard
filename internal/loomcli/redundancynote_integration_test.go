// redundancynote_integration_test.go drives the redundancy_report note closure over a real hub pair's report file.

package loomcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gatecli"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRedundancyReportNote_ReadsTheWorktreeReport drives the closure over a real pair's report file.
// A pair at the merge base of its parent branch holds a judged report that matches HEAD.
// The rows share one hub and add their pairs one after another, so they are not parallel.
func TestRedundancyReportNote_ReadsTheWorktreeReport(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	written := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	judged, err := json.Marshal(gatecli.RedundancyReport{
		Written: written,
		GateKey: "darn-write",
		Tests:   []gatecli.RedundancyTest{{Package: "alpha", Name: "TestAlphaOwn", File: "alpha/alpha_test.go", Verdict: gatecli.RedundancyUnique}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v; want nil", err)
	}
	tests := []struct {
		name       string
		slug       string
		report     string
		wantPrefix string
		wantExact  string
	}{
		{
			name:       "MalformedReportCouldNotBeRead",
			slug:       "malformed",
			report:     "{not json",
			wantPrefix: "No current redundancy report: the report could not be read: ",
		},
		{
			name:   "ReportOfHeadListsItsTests",
			slug:   "matching",
			report: string(judged),
			wantExact: "The redundancy report of the `darn-write` gate call of 2026-11-09T20:00:00Z matches HEAD's tree.\n" +
				"- `alpha.TestAlphaOwn` in `alpha/alpha_test.go`: unique",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hubforge.AddPair(t, hub, tt.slug)
			location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(tt.slug))
			if err != nil {
				t.Fatalf("ResolveWorktree() error = %v; want nil", err)
			}
			dir := gateslot.WaitDir(location.AnchorPath())
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q) error = %v; want nil", dir, err)
			}
			path := filepath.Join(dir, "redundancy.json")
			if err := os.WriteFile(path, []byte(tt.report), 0o644); err != nil {
				t.Fatalf("WriteFile(%q) error = %v; want nil", path, err)
			}

			got := redundancyReportNote(location)()
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("redundancyReportNote()() = %q; want %q", got, tt.wantExact)
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("redundancyReportNote()() = %q; want it to start with %q", got, tt.wantPrefix)
			}
		})
	}
}
