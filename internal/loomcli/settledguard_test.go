package loomcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestSettledSegmentGuard covers which gotos the guard admits and refuses over a loom producer list and run directories holding records.
func TestSettledSegmentGuard(t *testing.T) {
	t.Parallel()

	producers := []shedengine.ProducerDef{
		{Name: "Plan-Write", OnDone: "Plan-Bouncer"},
		{Name: "Plan-Bouncer", Segment: "Plan-Review", OnStuck: "Plan-Burler-Review", OnDone: "Webster"},
		{Name: "Plan-Burler-Review", Segment: "Plan-Review", OnStuck: "Plan-Burler-Fix", OnDone: "Plan-Burler-Fix"},
		{Name: "Plan-Burler-Fix", Segment: "Plan-Review", OnStuck: "Plan-Bouncer", OnDone: "Plan-Bouncer"},
		{Name: "Webster", OnDone: "Webster-Bouncer"},
		{Name: "Webster-Bouncer", Segment: "Webster-Review", OnStuck: "Webster-Burler-Review", OnDone: "Describe"},
		{Name: "Webster-Burler-Review", Segment: "Webster-Review", OnStuck: "Webster-Burler-Fix", OnDone: "Webster-Burler-Fix"},
		{Name: "Webster-Burler-Fix", Segment: "Webster-Review", OnStuck: "Webster-Bouncer", OnDone: "Webster-Bouncer"},
		{Name: "Merge-In", OnDone: "Merge-Bouncer"},
		{Name: "Merge-Bouncer", Segment: "Merge-Review", OnStuck: "Merge-Burler-Review", OnDone: "Publish"},
		{Name: "Merge-Burler-Review", Segment: "Merge-Review", OnStuck: "Merge-Burler-Fix", OnDone: "Merge-Burler-Fix"},
		{Name: "Merge-Burler-Fix", Segment: "Merge-Review", OnStuck: "Merge-Bouncer", OnDone: "Merge-Bouncer"},
		{Name: "PR-Gate", Segment: "PR-Review", OnStuck: "PR-Rework", OnDone: "Finalize"},
		{Name: "PR-Rework", Segment: "PR-Review", OnDone: "Plan-Bouncer"},
	}

	const (
		settled     = `{"subject":{"kind":"document"},"generation":1,"round":1,"settled_at":"2026-10-20T10:00:00Z","settled":true}`
		interrupted = `{"subject":{"kind":"document"},"generation":1,"round":1,"settled_at":"2026-10-20T10:00:00Z","settled":false}`
		amendment   = `{"number":2,"amendment":true}`
		before      = "2026-10-20T09:00:00Z"
		after       = "2026-10-20T11:00:00Z"
		// roundStarted is a settle whose judged round started at 09:30, between before and after.
		roundStarted = `{"subject":{"kind":"document"},"generation":1,"round":1,"settled_at":"2026-10-20T10:00:00Z","settled":true,"round_started_at":"2026-10-20T09:30:00Z"}`
	)
	amendmentAt := func(writtenAt string) *loomshed.AmendmentRecord {
		parsed, err := time.Parse(time.RFC3339, writtenAt)
		if err != nil {
			t.Fatal(err)
		}
		return &loomshed.AmendmentRecord{Titles: []string{"Keep the cache"}, WrittenAt: parsed}
	}

	tests := []struct {
		name string
		// bouncer is the Bouncer row whose run directory holds the files below.
		bouncer string
		// files maps a file name in that run directory to its content.
		files map[string]string
		// amendment is the plan amendment record, nil when none exists; amendmentErr is its read error.
		amendment    *loomshed.AmendmentRecord
		amendmentErr error
		target       string
		// wantRefusal lists the substrings of the refusal; empty means the target is admitted.
		wantRefusal []string
		wantErr     bool
	}{
		{name: "Bouncer under a live record", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled}, target: "Plan-Bouncer"},
		{
			name: "Plan-Review review row", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled},
			target: "Plan-Burler-Review", wantRefusal: []string{"the Plan-Review verdict is settled", "goto --to Plan-Bouncer", "goto --to Plan-Write"},
		},
		{
			name: "Plan-Review fix row", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled},
			target: "Plan-Burler-Fix", wantRefusal: []string{"goto --to Plan-Bouncer", "goto --to Plan-Write"},
		},
		{
			name: "Merge-Review review row", bouncer: "Merge-Bouncer", files: map[string]string{"settle-record.json": settled},
			target: "Merge-Burler-Review", wantRefusal: []string{"goto --to Merge-Bouncer", "goto --to Merge-In"},
		},
		{
			name: "interrupted live record", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": interrupted},
			target: "Plan-Burler-Review", wantRefusal: []string{"goto --to Plan-Bouncer", "goto --to Plan-Write"},
		},
		{
			name: "record made not live by a later round's report", bouncer: "Plan-Bouncer",
			files:  map[string]string{"settle-record.json": settled, "round-2-review.md": "review"},
			target: "Plan-Burler-Review",
		},
		{
			name: "amendment generation that has not settled", bouncer: "Plan-Bouncer",
			files:  map[string]string{"settle-record.json": settled, "generation-record.json": amendment},
			target: "Plan-Burler-Review",
		},
		{name: "segment with no record", bouncer: "Plan-Bouncer", target: "Plan-Burler-Review"},
		{name: "row outside every review segment", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled}, target: "Plan-Write"},
		{name: "segment holding no Bouncer", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled}, target: "PR-Gate"},
		{
			name: "settle record unreadable as JSON", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": "{"},
			target: "Plan-Burler-Review", wantErr: true,
		},
		{
			name: "amendment written after the judged round started", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": roundStarted},
			amendment: amendmentAt("2026-10-20T09:45:00Z"), target: "Plan-Burler-Review",
		},
		{
			name: "amendment written before the judged round started", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": roundStarted},
			amendment: amendmentAt(before), target: "Plan-Burler-Review", wantRefusal: []string{"the Plan-Review verdict is settled", "goto --to Plan-Bouncer"},
		},
		{
			name: "amendment over a settle with no round start", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": settled},
			amendment: amendmentAt(before), target: "Plan-Burler-Review",
		},
		{
			name: "amendment does not admit the fix row", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": roundStarted},
			amendment: amendmentAt(after), target: "Plan-Burler-Fix", wantRefusal: []string{"goto --to Plan-Bouncer"},
		},
		{
			name: "amendment does not admit another segment's review row", bouncer: "Merge-Bouncer", files: map[string]string{"settle-record.json": roundStarted},
			amendment: amendmentAt(after), target: "Merge-Burler-Review", wantRefusal: []string{"goto --to Merge-Bouncer"},
		},
		{
			name: "amendment record unreadable", bouncer: "Plan-Bouncer", files: map[string]string{"settle-record.json": roundStarted},
			amendmentErr: errors.New("decode amendment"), target: "Plan-Burler-Review", wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reviewsDir := t.TempDir()
			subdir, isBouncer, err := loomrecipe.BouncerRunSubdir(tt.bouncer)
			if err != nil || !isBouncer {
				t.Fatalf("BouncerRunSubdir(%q) = %q, %v, %v; want a Bouncer row", tt.bouncer, subdir, isBouncer, err)
			}
			runDir := filepath.Join(reviewsDir, subdir)
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			guard := settledSegmentGuard(producers, reviewsDir, func() (loomshed.AmendmentRecord, bool, error) {
				if tt.amendment == nil {
					return loomshed.AmendmentRecord{}, false, tt.amendmentErr
				}
				return *tt.amendment, true, nil
			})

			refusal, err := guard(tt.target)

			if (err != nil) != tt.wantErr {
				t.Fatalf("guard(%q) error = %v; want error %v", tt.target, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(tt.wantRefusal) == 0 && refusal != "" {
				t.Fatalf("guard(%q) = %q; want the target admitted", tt.target, refusal)
			}
			for _, want := range tt.wantRefusal {
				if !strings.Contains(refusal, want) {
					t.Errorf("guard(%q) = %q; want it to contain %q", tt.target, refusal, want)
				}
			}
			if strings.Contains(refusal, "PR-Rework") {
				t.Errorf("guard(%q) = %q; want the writer to be the row before the segment, never PR-Rework", tt.target, refusal)
			}
		})
	}
}
