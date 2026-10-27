//go:build tier0

// beamdescribe_test.go covers NewBeamDescribe over told seams and a temp description path.

package landingshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestBeamDescribe asserts the beam description carries the entry's title, brief and one bullet per commit subject, cuts an over-long title, refuses an empty body naming the way out, returns a failed seam, and rewrites the same bytes on a re-run.
func TestBeamDescribe(t *testing.T) {
	t.Parallel()

	const slug = "my-beam"
	readErr := errors.New("board unreadable")
	tests := []struct {
		name     string
		title    string
		brief    string
		subjects []string
		entryErr error
		subjErr  error
		// wantFile is the exact description written; empty wants the call to fail.
		wantFile string
		// wantErr are the substrings the failure carries; a seam failure wants wantIs as well.
		wantErr []string
		wantIs  error
	}{
		{
			name: "TitleBriefAndTwoSubjects", title: "Ship the beam", brief: "Groups the work.", subjects: []string{"first change", "second change"},
			wantFile: "# Ship the beam\n\nGroups the work.\n\n- first change\n- second change\n",
		},
		{
			name: "OverLongTitleIsCut", title: strings.Repeat("a", 80), brief: "Groups the work.",
			wantFile: "# " + strings.Repeat("a", 72) + "\n\nGroups the work.\n",
		},
		{
			name: "SubjectsAloneFillTheBody", title: "Ship the beam", subjects: []string{"only change"},
			wantFile: "# Ship the beam\n\n- only change\n",
		},
		{
			name: "EmptyBriefAndNoSubjectsAreRefusedNamingTheWayOut", title: "Ship the beam",
			wantErr: []string{"body is empty", `"lym batten cancel my-beam"`, `"lym board upsert"`, `"lym beam land my-beam"`},
		},
		{
			name: "EntrySeamErrorIsReturned", entryErr: readErr,
			wantErr: []string{"read the board entry"}, wantIs: readErr,
		},
		{
			name: "SubjectsSeamErrorIsReturned", title: "Ship the beam", brief: "Groups the work.", subjErr: readErr,
			wantErr: []string{"commit subjects"}, wantIs: readErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "landing", "summary.md")
			commits := 0
			producer, err := NewBeamDescribe("Beam-Describe", BeamDescribeDeps{
				Slug:            slug,
				DescriptionPath: path,
				Entry:           func() (string, string, error) { return tt.title, tt.brief, tt.entryErr },
				CommitSubjects:  func() ([]string, error) { return tt.subjects, tt.subjErr },
				Commit:          func() error { commits++; return nil },
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
