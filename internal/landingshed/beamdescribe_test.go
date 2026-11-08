// beamdescribe_test.go covers DraftBeamDescription and RecordBeamDescription over told seams and a temp description path.

package landingshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestDraftBeamDescription asserts the draft carries the entry's title and brief and one entry per commit, oldest first.
// It cuts an over-long title and refuses a description already written, leaving it intact.
// It refuses an unset seam and returns a failed seam.
func TestDraftBeamDescription(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

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
		// existing, when set, is a description already at the path.
		existing   string
		entryErr   error
		commitsErr error
		// dropCommits, when set, leaves the Commits seam unset.
		dropCommits bool
		// wantFile is the exact draft written; empty wants the call to fail.
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
			name: "OverLongTitleIsCut", title: strings.Repeat("a", 80), brief: "Groups the work.",
			wantFile: "# " + strings.Repeat("a", 72) + "\n\nGroups the work.\n",
		},
		{
			name: "EmptyBriefAndNoCommitsWriteTheTitleAlone", title: "Ship the beam",
			wantFile: "# Ship the beam\n",
		},
		{
			name: "AnExistingDescriptionIsRefusedAndLeftIntact", title: "Ship the beam", brief: "Groups the work.", existing: "# Old text\n\nThe orch's words.\n",
			wantErr: []string{"a description already stands at", "summary.md", `"lym beam describe my-beam"`, "delete it"},
		},
		{
			name: "EntrySeamErrorIsReturned", entryErr: readErr,
			wantErr: []string{"read the board entry"}, wantIs: readErr,
		},
		{
			name: "CommitsSeamErrorIsReturned", title: "Ship the beam", commitsErr: readErr,
			wantErr: []string{"read the beam's commits"}, wantIs: readErr,
		},
		{
			name: "UnsetSeamIsRefusedNamingIt", title: "Ship the beam", dropCommits: true,
			wantErr: []string{"Commits is empty"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "landing", "summary.md")
			if tt.existing != "" {
				writeDescription(t, path, tt.existing)
			}
			deps := BeamDraftDeps{
				Slug:            "my-beam",
				DescriptionPath: path,
				Entry:           func() (string, string, error) { return tt.title, tt.brief, tt.entryErr },
				Commits:         func() ([]BeamCommit, error) { return tt.commits, tt.commitsErr },
			}
			if tt.dropCommits {
				deps.Commits = nil
			}

			err := DraftBeamDescription(deps)

			if tt.existing != "" {
				if got, readErr := os.ReadFile(path); readErr != nil || string(got) != tt.existing {
					t.Errorf("existing description = %q, %v; want it left as %q", got, readErr, tt.existing)
				}
			}

			if tt.wantFile == "" {
				if err == nil {
					t.Fatalf("DraftBeamDescription() = nil; want an error carrying %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q; want it to carry %q", err.Error(), want)
					}
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("error = %v; want it to wrap %v", err, tt.wantIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("DraftBeamDescription() = %v; want nil", err)
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != tt.wantFile {
				t.Errorf("draft = %q, %v; want %q", got, readErr, tt.wantFile)
			}
		})
	}
}

// TestRecordBeamDescription asserts a valid description is committed once and its bytes are left as the orch wrote them.
// It refuses a missing description naming the draft verb.
// It refuses a description with a finding naming the file and the verb.
// Neither refusal commits, and a failed commit is returned.
func TestRecordBeamDescription(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		slug  = "my-beam"
		valid = "# Ship the beam\n\nThe beam adds the hydraulics model.\n\n## Member pull requests\n\n- #266 Tool-joint pressure loss\n"
	)
	commitErr := errors.New("index locked")
	tests := []struct {
		name string
		// content is the description at the path; empty leaves no file.
		content   string
		commitErr error
		// wantErr are the substrings the failure carries; empty wants the description recorded.
		wantErr     []string
		wantIs      error
		wantCommits int
	}{
		{name: "ValidDescriptionIsCommitted", content: valid, wantCommits: 1},
		{
			name:    "MissingDescriptionIsRefusedNamingTheDraft",
			wantErr: []string{"no description is written at", `"lym beam describe my-beam --draft"`, `"lym beam describe my-beam" again`},
		},
		{
			name: "FindingIsRefusedNamingTheFileAndTheVerb", content: "# Ship the beam\n",
			wantErr: []string{"empty-body: body is empty", "fix the description at", `"lym beam describe my-beam" again`},
		},
		{
			name: "CommitFailureIsReturned", content: valid, commitErr: commitErr,
			wantErr: []string{"commit the description"}, wantIs: commitErr, wantCommits: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "landing", "summary.md")
			if tt.content != "" {
				writeDescription(t, path, tt.content)
			}
			commits := 0
			commit := func() error { commits++; return tt.commitErr }

			err := RecordBeamDescription(slug, path, commit)

			if commits != tt.wantCommits {
				t.Errorf("commits = %d; want %d", commits, tt.wantCommits)
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("RecordBeamDescription() = nil; want an error carrying %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q; want it to carry %q", err.Error(), want)
					}
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("error = %v; want it to wrap %v", err, tt.wantIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("RecordBeamDescription() = %v; want nil", err)
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != tt.content {
				t.Errorf("description = %q, %v; want the orch's bytes %q", got, readErr, tt.content)
			}
		})
	}
}

// writeDescription writes content at path, creating its directory.
func writeDescription(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
}
