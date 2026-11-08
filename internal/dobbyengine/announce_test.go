// announce_test.go pins the beam's dobby announcing landings over fake landing seams.
// It covers one notice per new landing while a pull request is recorded.
// It covers the announced head kept in its file, and a failed read or post retried at the next tick.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// announceCommit is one commit of the fake beam line.
// It holds the object name and the text a landing notice names the commit by.
// It also says whether the commit only records lym's own state.
type announceCommit struct {
	sha, text   string
	recordsOnly bool
}

// announceFake answers the two landing seams from a scripted beam line.
type announceFake struct {
	recorded     bool
	announceHead string
	recordErr    error
	line         []announceCommit
	sinceErr     error
}

func (f *announceFake) landingPullRequest() (string, bool, error) {
	return f.announceHead, f.recorded, f.recordErr
}

// landingsSince lists the landings past from: the commits after it on the line, records-only ones dropped.
// When from is not on the line, it lists every landing on it.
func (f *announceFake) landingsSince(from string) (string, []string, error) {
	if f.sinceErr != nil {
		return "", nil, f.sinceErr
	}
	start := 0
	for i, commit := range f.line {
		if commit.sha == from {
			start = i + 1
		}
	}
	var landed []string
	for _, commit := range f.line[start:] {
		if !commit.recordsOnly {
			landed = append(landed, commit.text)
		}
	}
	return f.line[len(f.line)-1].sha, landed, nil
}

func TestAnnounceLandings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	base := announceCommit{sha: "c0", text: "c0 Open the beam"}
	widgets := announceCommit{sha: "c1", text: "c1 Add widgets"}
	gadgets := announceCommit{sha: "c2", text: "c2 Add gadgets"}
	bookkeeping := announceCommit{sha: "r1", text: "r1 beam: bookkeeping", recordsOnly: true}
	tests := []struct {
		name   string
		stopAt int
		// setup scripts the fake and the harness before the run; between changes the fake after the first tick.
		setup   func(f *announceFake, h *loopHarness)
		between func(f *announceFake)
		// wantNotices holds, per notice posted in order, every substring it names and every substring it must not.
		wantNotices [][2][]string
		// wantFile is the announced-head file's content, empty when none is written.
		wantFile string
	}{
		{
			name:   "OneLandingPastTheRecordsHeadPostsOneNoticeNamingItAndDescribeAndWritesTheHead",
			stopAt: 1,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
			},
			wantNotices: [][2][]string{{{"c1 Add widgets", "lym beam describe b", "updates the pull request"}, {"c0 Open the beam", "lym beam pr update b"}}},
			wantFile:    "c1\n",
		},
		{
			name:   "AnnouncedHeadFileIsReadOverTheRecordsHead",
			stopAt: 1,
			setup: func(f *announceFake, h *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets, gadgets}
				if err := os.MkdirAll(filepath.Dir(h.geom.AnnouncePath), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(h.geom.AnnouncePath), err)
				}
				if err := os.WriteFile(h.geom.AnnouncePath, []byte("c1\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(announced-head) = %v; want nil", err)
				}
			},
			wantNotices: [][2][]string{{{"c2 Add gadgets"}, {"c1 Add widgets"}}},
			wantFile:    "c2\n",
		},
		{
			name:   "ALaterTickWithNoNewCommitPostsNone",
			stopAt: 2,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
			},
			wantNotices: [][2][]string{{{"c1 Add widgets"}, nil}},
			wantFile:    "c1\n",
		},
		{
			name:   "ASecondLandingPostsOneMoreNamingOnlyIt",
			stopAt: 2,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
			},
			between:     func(f *announceFake) { f.line = append(f.line, gadgets) },
			wantNotices: [][2][]string{{{"c1 Add widgets"}, nil}, {{"c2 Add gadgets"}, {"c1 Add widgets"}}},
			wantFile:    "c2\n",
		},
		{
			name:   "AHeadPastOnlyRecordsOnlyCommitsPostsNothingAndWritesTheHead",
			stopAt: 1,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, bookkeeping}
			},
			wantFile: "r1\n",
		},
		{
			name:   "NoPullRequestRecordedPostsNothingAndWritesNothing",
			stopAt: 1,
			setup: func(f *announceFake, _ *loopHarness) {
				f.line = []announceCommit{base, widgets}
			},
		},
		{
			name:   "AFailedPostWritesNothingAndTheNextTickPostsAgain",
			stopAt: 2,
			setup: func(f *announceFake, h *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
				post := h.deps.Notice
				attempts := 0
				h.deps.Notice = func(ctx context.Context, line string) error {
					attempts++
					if attempts == 1 {
						return errors.New("creel is down")
					}
					return post(ctx, line)
				}
			},
			wantNotices: [][2][]string{{{"c1 Add widgets"}, nil}},
			wantFile:    "c1\n",
		},
		{
			name:   "AFailingLandingRecordReadPostsAndWritesNothing",
			stopAt: 1,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
				f.recordErr = errors.New("the landing record is unreadable")
			},
		},
		{
			name:   "AFailingLandingsReadPostsAndWritesNothing",
			stopAt: 1,
			setup: func(f *announceFake, _ *loopHarness) {
				f.recorded, f.announceHead, f.line = true, "c0", []announceCommit{base, widgets}
				f.sinceErr = errors.New("the beam line is unreadable")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newLoopHarness(t, tt.stopAt)
			h.geom.Slug, h.geom.WorktreeRoot = "b", t.TempDir()
			if err := os.WriteFile(filepath.Join(h.geom.WorktreeRoot, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(.git) = %v; want nil", err)
			}
			h.geom.AnnouncePath = filepath.Join(filepath.Dir(h.geom.StatePath), AnnounceFileName)
			fake := &announceFake{}
			h.deps.LandingPullRequest = fake.landingPullRequest
			h.deps.BeamLandingsSince = fake.landingsSince
			tt.setup(fake, h)
			if tt.between != nil {
				h.perTick = func(n int) error {
					if n == 1 {
						tt.between(fake)
					}
					return nil
				}
			}

			if err := h.run(t); err != nil {
				t.Fatalf("Run() = %v; want nil", err)
			}

			notices := h.noticed()
			if len(notices) != len(tt.wantNotices) {
				t.Fatalf("notices = %q; want %d", notices, len(tt.wantNotices))
			}
			for i, want := range tt.wantNotices {
				for _, name := range want[0] {
					if !strings.Contains(notices[i], name) {
						t.Errorf("notice %d = %q; want it to name %q", i, notices[i], name)
					}
				}
				for _, unwanted := range want[1] {
					if strings.Contains(notices[i], unwanted) {
						t.Errorf("notice %d = %q; want it not to name %q", i, notices[i], unwanted)
					}
				}
			}
			written, err := os.ReadFile(h.geom.AnnouncePath)
			switch {
			case tt.wantFile == "" && !errors.Is(err, os.ErrNotExist):
				t.Errorf("announced-head = %q, %v; want no file", written, err)
			case tt.wantFile != "" && string(written) != tt.wantFile:
				t.Errorf("announced-head = %q, %v; want %q", written, err, tt.wantFile)
			}
		})
	}
}
