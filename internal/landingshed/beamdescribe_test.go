// beamdescribe_test.go covers NewBeamDescribe over told seams and a temp description path.

package landingshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBeamDescribe asserts the beam description carries the entry's title, brief, one entry per commit and the project record's added entries, cuts an over-long title, refuses an empty body naming the way out, returns a failed seam, and rewrites the same bytes on a re-run.
func TestBeamDescribe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "my-beam"
	readErr := errors.New("board unreadable")
	at := func(hour int) time.Time { return time.Date(2026, 10, 30, hour, 5, 0, 0, time.FixedZone("CET", 3600)) }
	pullRequest := BeamCommit{SHA: "1111111111111111111111111111111111111111", Message: "Tool-joint pressure loss (#266)\n\nAdds the tool-joint loss to the hydraulics. Tests cover the annulus.\n\nCreated using\n - LoomYard", Committed: at(14)}
	parentMerge := BeamCommit{SHA: "2222222222222222222222222222222222222222", Message: "Merge branch 'main' into my-beam\n\n# Conflicts:\n#\tsrc/a.go\n", Committed: at(15)}
	direct := BeamCommit{SHA: "3333333333333333333333333333333333333333", Message: "Fix a typo", Committed: at(16)}
	const (
		wantPullRequestEntry = "- #266 Tool-joint pressure loss, merged 2026-10-30 13:05 UTC\n  Adds the tool-joint loss to the hydraulics."
		wantParentMergeEntry = "- 222222222222 Merge branch 'main' into my-beam, committed 2026-10-30 14:05 UTC"
		wantDirectEntry      = "- 333333333333 Fix a typo, committed 2026-10-30 15:05 UTC"
	)
	tests := []struct {
		name    string
		title   string
		brief   string
		commits []BeamCommit
		// decisions are the project record's added entries the seam returns.
		decisions  []string
		decErr     error
		entryErr   error
		commitsErr error
		// wantFile is the exact description written; empty wants the call to fail.
		wantFile string
		// wantErr are the substrings the failure carries; a seam failure wants wantIs as well.
		wantErr []string
		wantIs  error
	}{
		{
			name: "PullRequestsAndDirectCommitsAreListedOldestFirst", title: "Ship the beam", brief: "Groups the work.", commits: []BeamCommit{pullRequest, parentMerge, direct},
			wantFile: "# Ship the beam\n\nGroups the work.\n\n## Changes\n\n" + wantPullRequestEntry + "\n" + wantParentMergeEntry + "\n" + wantDirectEntry + "\n",
		},
		{
			name: "ALongSummaryIsCutWithAnEllipsis", title: "Ship the beam",
			commits:  []BeamCommit{{SHA: "4444", Message: "Long change\n\n" + strings.Repeat("word ", 50), Committed: at(12)}},
			wantFile: "# Ship the beam\n\n## Changes\n\n- 4444 Long change, committed 2026-10-30 11:05 UTC\n  " + strings.TrimSpace(strings.Repeat("word ", 40)) + "…\n",
		},
		{
			name: "ProjectDecisionsFollowTheChanges", title: "Ship the beam", brief: "Groups the work.", commits: []BeamCommit{direct},
			decisions: []string{"### Added after Discussion (parent, 2026-10-31): Narrow the scope\n\n- **Decision:** Drop the CLI half.\n- **Rationale:** It belongs to a later beam."},
			wantFile:  "# Ship the beam\n\nGroups the work.\n\n## Changes\n\n" + wantDirectEntry + "\n\n## Project decisions\n\n### Added after Discussion (parent, 2026-10-31): Narrow the scope\n\n- **Decision:** Drop the CLI half.\n- **Rationale:** It belongs to a later beam.\n",
		},
		{
			name: "ProjectDecisionsSeamErrorIsReturned", title: "Ship the beam", brief: "Groups the work.", decErr: readErr,
			wantErr: []string{"read the project record"}, wantIs: readErr,
		},
		{
			name: "OverLongTitleIsCut", title: strings.Repeat("a", 80), brief: "Groups the work.",
			wantFile: "# " + strings.Repeat("a", 72) + "\n\nGroups the work.\n",
		},
		{
			name: "EmptyBriefAndNoCommitsAreRefusedNamingTheWayOut", title: "Ship the beam",
			wantErr: []string{"body is empty", `"lym batten cancel my-beam"`, `"lym board upsert"`, `"lym beam land my-beam"`},
		},
		{
			name: "EntrySeamErrorIsReturned", entryErr: readErr,
			wantErr: []string{"read the board entry"}, wantIs: readErr,
		},
		{
			name: "CommitsSeamErrorIsReturned", title: "Ship the beam", brief: "Groups the work.", commitsErr: readErr,
			wantErr: []string{"read the beam's commits"}, wantIs: readErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "landing", "summary.md")
			commits := 0
			producer, err := NewBeamDescribe("Beam-Describe", BeamDescribeDeps{
				Slug:             slug,
				DescriptionPath:  path,
				ProjectDecisions: func() ([]string, error) { return tt.decisions, tt.decErr },
				Entry:            func() (string, string, error) { return tt.title, tt.brief, tt.entryErr },
				Commits:          func() ([]BeamCommit, error) { return tt.commits, tt.commitsErr },
				Commit:           func() error { commits++; return nil },
			})
			if err != nil {
				t.Fatalf("NewBeamDescribe() error = %v; want nil", err)
			}

			outcome, _, err := producer.Call(context.Background())

			if tt.wantFile == "" {
				if err == nil {
					t.Fatalf("Call() = %q, nil; want an error carrying %q", outcome, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Call() error = %q; want it to carry %q", err.Error(), want)
					}
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("Call() error = %v; want it to wrap %v", err, tt.wantIs)
				}
				if commits != 0 {
					t.Errorf("commits = %d; want none for a refused description", commits)
				}
				return
			}
			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != tt.wantFile {
				t.Errorf("description = %q, %v; want %q", got, readErr, tt.wantFile)
			}
			if commits != 1 {
				t.Errorf("commits = %d; want 1", commits)
			}

			if outcome, _, err := producer.Call(context.Background()); err != nil || outcome != shedengine.Done {
				t.Fatalf("re-run Call() = %q, %v; want Done, nil", outcome, err)
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != tt.wantFile {
				t.Errorf("description after a re-run = %q, %v; want the same bytes", got, readErr)
			}
		})
	}
}
