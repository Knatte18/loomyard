// upgrade_test.go pins the first read after an upgrade.
// It covers which standing pause entries become pause intents and a hold, and when the read drops them.

package dobbyengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fixtries"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeUpgradeDeps returns first-read seams over an empty hub whose pause record lists entries.
// Its other seams succeed and do nothing.
func fakeUpgradeDeps(t *testing.T, entries func() ([]pauserecord.Entry, error)) UpgradeDeps {
	t.Helper()
	return UpgradeDeps{
		Lock:        filepath.Join(t.TempDir(), "upgrade.lock"),
		Now:         time.Now,
		Entries:     entries,
		PullBolt:    func() error { return nil },
		Classify:    func(string) (UpgradeRun, error) { return UpgradeRun{}, nil },
		PauseIntent: func(string) (bool, error) { return false, nil },
		Machine:     func() (string, error) { return "m1", nil },
		OpenFixTask: func(string) (string, bool, error) { return "", false, nil },
		AdoptDir:    t.TempDir(),
		WriteHold: func(boardengine.Hold) (boardengine.HoldWrite, error) {
			return boardengine.HoldWrite{Wrote: true}, nil
		},
		Drop:   func(pauserecord.Entry) error { return nil },
		Notice: func(string) error { return nil },
	}
}

// upgradeHub is a fake hub the first read runs over, and what the read did to it.
type upgradeHub struct {
	// entries are the live entries of the pause record; Drop removes the entry it drops.
	entries []pauserecord.Entry
	// runs are the answers of Classify by slug.
	runs map[string]UpgradeRun
	// pullErr, intentErr, writeErr and dropErr fail the seam of their name.
	pullErr, intentErr, writeErr, dropErr error
	// handedOver names the runs whose pause intent push meets a takeover.
	handedOver map[string]bool
	// fixTasks maps a subject to its open fix task.
	fixTasks map[string]string
	// standing is the hold of another machine that WriteHold finds standing.
	standing *boardengine.Hold
	// ownHoldStands makes WriteHold find this machine's identical hold standing: nothing written and no Standing.
	ownHoldStands bool

	pulls   int
	intents []string
	holds   []boardengine.Hold
	notices []string
}

// deps returns the first-read seams over the hub, with the fix-try counts of tries written under its adopt directory.
func (h *upgradeHub) deps(t *testing.T, tries map[string]int) UpgradeDeps {
	t.Helper()
	deps := fakeUpgradeDeps(t, func() ([]pauserecord.Entry, error) { return slices.Clone(h.entries), nil })
	for subject, count := range tries {
		dir := filepath.Join(deps.AdoptDir, subject)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) = %v; want nil", dir, err)
		}
		file := filepath.Join(dir, fixtries.FileName)
		if err := os.WriteFile(file, []byte(fmt.Sprintf(`{"count": %d}`, count)), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) = %v; want nil", file, err)
		}
	}
	deps.PullBolt = func() error { h.pulls++; return h.pullErr }
	deps.Classify = func(slug string) (UpgradeRun, error) { return h.runs[slug], nil }
	deps.PauseIntent = func(slug string) (bool, error) {
		if h.intentErr != nil {
			return false, h.intentErr
		}
		if h.handedOver[slug] {
			return true, nil
		}
		h.intents = append(h.intents, slug)
		return false, nil
	}
	deps.OpenFixTask = func(subject string) (string, bool, error) {
		task, found := h.fixTasks[subject]
		return task, found, nil
	}
	deps.WriteHold = func(hold boardengine.Hold) (boardengine.HoldWrite, error) {
		if h.writeErr != nil {
			return boardengine.HoldWrite{}, h.writeErr
		}
		if h.standing != nil {
			return boardengine.HoldWrite{Standing: h.standing}, nil
		}
		if h.ownHoldStands {
			return boardengine.HoldWrite{}, nil
		}
		h.holds = append(h.holds, hold)
		return boardengine.HoldWrite{Wrote: true}, nil
	}
	deps.Drop = func(entry pauserecord.Entry) error {
		if h.dropErr != nil {
			return h.dropErr
		}
		h.entries = slices.DeleteFunc(h.entries, func(live pauserecord.Entry) bool {
			return live.Scope == entry.Scope && live.Caller == entry.Caller
		})
		return nil
	}
	deps.Notice = func(line string) error { h.notices = append(h.notices, line); return nil }
	return deps
}

// holdSummary renders a hold as kind, fix task and subjects with their try counts.
func holdSummary(hold boardengine.Hold) string {
	subjects := make([]string, len(hold.Subjects))
	for i, subject := range hold.Subjects {
		subjects[i] = fmt.Sprintf("%s:%d", subject.Subject, subject.Tries)
	}
	return hold.Kind + "/" + hold.FixTask + "/" + strings.Join(subjects, ",")
}

func TestUpgradeRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	entry := func(slug, caller string) pauserecord.Entry {
		return pauserecord.Entry{Scope: slug, Caller: caller, PausedAt: time.Unix(1, 0)}
	}
	ownedRun := func(subject string) UpgradeRun {
		return UpgradeRun{OwnedHere: true, HasRecord: true, Subject: subject}
	}

	tests := []struct {
		name  string
		hub   upgradeHub
		tries map[string]int
		// wantErr is a target errors.Is finds in the read's error, nil when the read succeeds.
		wantErr     error
		wantFailure bool
		wantPulls   int
		wantIntents []string
		wantHolds   []string
		// wantNotice lists the fragments the one notice carries, none when no notice goes out.
		wantNotice []string
		// wantLeft lists the "scope caller" of the entries the read leaves.
		wantLeft []string
	}{
		{
			name: "AdoptEntryBecomesAPauseIntentAndOneHold",
			hub: upgradeHub{
				entries: []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:    map[string]UpgradeRun{"r1": ownedRun("beamA")},
			},
			wantPulls: 1, wantIntents: []string{"r1"}, wantHolds: []string{"adopt//beamA:0"},
			wantNotice: []string{"adopt hold of machine m1", "lym adopt beamA", "lym tender resume <slug>", "r1"},
		},
		{
			name: "OpenFixTaskMakesAnAdoptFixHoldWithTheLocalTries",
			hub: upgradeHub{
				entries:  []pauserecord.Entry{entry("r1", pauserecord.HoldCaller)},
				runs:     map[string]UpgradeRun{"r1": ownedRun("beamA")},
				fixTasks: map[string]string{"beamA": "fix-1"},
			},
			tries:     map[string]int{"beamA": 2},
			wantPulls: 1, wantIntents: []string{"r1"}, wantHolds: []string{"adopt-fix/fix-1/beamA:2"},
			wantNotice: []string{"adopt-fix hold of machine m1", "lym adopt beamA"},
		},
		{
			name: "OwnerLandedAndOwnedElsewhereEntriesDropWithNoHold",
			hub: upgradeHub{
				entries: []pauserecord.Entry{
					entry("r1", pauserecord.OwnerCaller), entry("r2", pauserecord.AdoptCaller), entry("r3", pauserecord.AdoptCaller),
				},
				runs: map[string]UpgradeRun{
					"r2": {OwnedHere: true, HasRecord: true, Terminal: true, Subject: "beamA"},
					"r3": {HasRecord: true, Subject: "beamA"},
				},
			},
			wantPulls: 1,
		},
		{
			name: "ReleasedQueuedTaskJoinsTheHoldSubjects",
			hub: upgradeHub{
				entries: []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller), entry("q1", pauserecord.AdoptCaller)},
				runs: map[string]UpgradeRun{
					"r1": ownedRun("beamA"),
					"q1": {OwnedHere: true, Queued: true, Subject: "beamB"},
				},
			},
			wantPulls: 1, wantIntents: []string{"r1"}, wantHolds: []string{"adopt//beamA:0,beamB:0"},
			wantNotice: []string{"lym adopt beamA beamB", "r1"},
		},
		{
			name: "AnotherMachinesStandingHoldWritesNoneAndTheNoticeNamesIt",
			hub: upgradeHub{
				entries:  []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:     map[string]UpgradeRun{"r1": ownedRun("beamA")},
				standing: &boardengine.Hold{Kind: boardengine.HoldKindAdopt, Machine: "m2"},
			},
			wantPulls: 1, wantIntents: []string{"r1"},
			wantNotice: []string{"machine m2's adopt hold stands", "lym adopt beamA"},
		},
		{
			name: "AnIdenticalHoldOfThisMachineStandingPostsNoSecondNoticeAndDropsTheEntry",
			hub: upgradeHub{
				entries:       []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:          map[string]UpgradeRun{"r1": ownedRun("beamA")},
				ownHoldStands: true,
			},
			wantPulls: 1, wantIntents: []string{"r1"},
		},
		{
			name: "HandedOverRunDropsItsEntryWithNoIntentAndNoHold",
			hub: upgradeHub{
				entries:    []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:       map[string]UpgradeRun{"r1": ownedRun("beamA")},
				handedOver: map[string]bool{"r1": true},
			},
			wantPulls: 1,
		},
		{
			name: "UnreachableOriginWritesAndDropsNothing",
			hub: upgradeHub{
				entries: []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:    map[string]UpgradeRun{"r1": ownedRun("beamA")},
				pullErr: fmt.Errorf("pull: %w", ErrOriginUnreachable),
			},
			wantErr: ErrOriginUnreachable, wantPulls: 1, wantLeft: []string{"r1 adopt"},
		},
		{
			name: "FailedIntentPushDropsNothing",
			hub: upgradeHub{
				entries:   []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:      map[string]UpgradeRun{"r1": ownedRun("beamA")},
				intentErr: errors.New("the push is owed"),
			},
			wantFailure: true, wantPulls: 1, wantLeft: []string{"r1 adopt"},
		},
		{
			name: "FailedHoldWriteDropsNothing",
			hub: upgradeHub{
				entries:  []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
				runs:     map[string]UpgradeRun{"r1": ownedRun("beamA")},
				writeErr: errors.New("board diverged"),
			},
			wantFailure: true, wantPulls: 1, wantIntents: []string{"r1"}, wantLeft: []string{"r1 adopt"},
		},
		{
			name: "NoStandingEntryReadsNothing",
			hub: upgradeHub{
				entries: []pauserecord.Entry{entry("r1", pauserecord.BoltHoldCaller)},
			},
			wantLeft: []string{"r1 " + pauserecord.BoltHoldCaller},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := tt.hub

			_, err := UpgradeRead(hub.deps(t, tt.tries))

			switch {
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Errorf("UpgradeRead() error = %v; want it to wrap %v", err, tt.wantErr)
			case tt.wantFailure && err == nil:
				t.Errorf("UpgradeRead() = nil; want an error")
			case tt.wantErr == nil && !tt.wantFailure && err != nil:
				t.Errorf("UpgradeRead() = %v; want nil", err)
			}
			if hub.pulls != tt.wantPulls {
				t.Errorf("Bolt pulls = %d; want %d", hub.pulls, tt.wantPulls)
			}
			if !slices.Equal(hub.intents, tt.wantIntents) {
				t.Errorf("pause intents = %q; want %q", hub.intents, tt.wantIntents)
			}
			var holds []string
			for _, hold := range hub.holds {
				holds = append(holds, holdSummary(hold))
			}
			if !slices.Equal(holds, tt.wantHolds) {
				t.Errorf("holds = %q; want %q", holds, tt.wantHolds)
			}
			if len(hub.notices) != min(len(tt.wantNotice), 1) {
				t.Fatalf("notices = %q; want %d", hub.notices, min(len(tt.wantNotice), 1))
			}
			for _, fragment := range tt.wantNotice {
				if !strings.Contains(hub.notices[0], fragment) {
					t.Errorf("notice = %q; want it to carry %q", hub.notices[0], fragment)
				}
			}
			if len(hub.notices) == 1 && strings.Contains(hub.notices[0], "--take-hold") {
				t.Errorf("notice = %q; want no mention of --take-hold", hub.notices[0])
			}
			var left []string
			for _, live := range hub.entries {
				left = append(left, live.Scope+" "+live.Caller)
			}
			if !slices.Equal(left, tt.wantLeft) {
				t.Errorf("entries left = %q; want %q", left, tt.wantLeft)
			}
		})
	}

	t.Run("AReRunAfterItsHoldWasClearedWritesTheHoldAgain", func(t *testing.T) {
		t.Parallel()
		hub := upgradeHub{
			entries: []pauserecord.Entry{entry("r1", pauserecord.AdoptCaller)},
			runs:    map[string]UpgradeRun{"r1": ownedRun("beamA")},
			dropErr: errors.New("pause record locked"),
		}
		deps := hub.deps(t, nil)

		if _, err := UpgradeRead(deps); err == nil {
			t.Fatalf("UpgradeRead() = nil; want the failed drop")
		}
		hub.holds, hub.dropErr = nil, nil
		if _, err := UpgradeRead(deps); err != nil {
			t.Fatalf("UpgradeRead() on the re-run = %v; want nil", err)
		}

		if len(hub.holds) != 1 || len(hub.entries) != 0 {
			t.Errorf("holds, entries left after the re-run = %d, %d; want the hold written again and no entry left", len(hub.holds), len(hub.entries))
		}
	})
}
