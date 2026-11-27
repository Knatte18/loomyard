// dobbydeps_test.go pins how the dobby reads a hub reconcile's report on the ended runs it asked to tear down.
// It also pins the gate the describer agent's run carries over its draft, and the findings read of a description.

package beamcli

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pushledger"
	"github.com/Knatte18/loomyard/internal/runpusher"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRestartHubPushers pins that the hub's own targets are started in order:
// the prime's, the board's, the landing and each beam pair with parent work.
// It also pins that a failing start does not stop the others.
// A refused parent target is settled instead of started.
func TestRestartHubPushers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// refused names the targets that read refused, by their ledger slug: "landing" or the beam's.
		refused     bool
		wantStarted int
		wantSettled []string
	}{
		{name: "EveryHubTargetIsStarted", wantStarted: 4},
		{name: "ARefusedParentTargetIsSettledAndNotStarted", refused: true, wantStarted: 2, wantSettled: []string{"landing", "beam"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prime := &lymcwd.Location{RepoName: "repo", HubPath: t.TempDir(), WorktreeName: "prime", AnchorRel: "."}
			entry := pushledger.Entry{Worktree: filepath.Join(prime.HubPath, "beam"), Slug: "beam", Dropped: map[string]string{"member": "abc"}}
			if err := os.Mkdir(entry.Worktree, 0o755); err != nil {
				t.Fatalf("Mkdir() = %v; want nil", err)
			}
			err := pushledger.Update(prime.HubPath, pushledger.Key("beam-records"), time.Unix(1, 0), func(pushledger.Entry, bool) (pushledger.Entry, bool, error) {
				return entry, true, nil
			})
			if err != nil {
				t.Fatalf("Update() = %v; want nil", err)
			}
			var started []runpusher.Target
			var settled []string
			restartHubPushers(prime, func(target runpusher.Target) (bool, error) {
				started = append(started, target)
				if len(started) == 1 {
					return false, errors.New("start failed")
				}
				return true, nil
			}, func(runpusher.Target) (bool, error) { return tt.refused, nil }, func(target runpusher.Target) error {
				if target.Pair {
					settled = append(settled, target.Slug)
					return nil
				}
				settled = append(settled, string(target.Kind))
				return nil
			})

			hubTargets := []runpusher.Target{{Location: prime}, runpusher.BoardTarget(prime), runpusher.LandingTarget(prime)}
			// A refused landing target and beam pair leave the first two hub targets alone.
			wantHub := min(tt.wantStarted, len(hubTargets))
			if len(started) != tt.wantStarted || !slices.Equal(started[:wantHub], hubTargets[:wantHub]) {
				t.Fatalf("targets started = %+v; want %d, opening with %+v", started, tt.wantStarted, hubTargets[:wantHub])
			}
			for _, target := range started[wantHub:] {
				if !target.Pair || target.Slug != "beam" || target.Location.WorktreePath() != entry.Worktree {
					t.Errorf("pair target started = %+v; want the beam's at %s", target, entry.Worktree)
				}
			}
			if !slices.Equal(settled, tt.wantSettled) {
				t.Errorf("targets settled = %v; want %v", settled, tt.wantSettled)
			}
		})
	}
}

// validBeamDescription is a change description that passes validation.
const validBeamDescription = "# Ship the beam\n\nGroups the work.\n\n## Merge danger\n\n**Door:** two-way\n**Blast radius:** beam\n"

func TestDescribeGate_JudgesTheSpecsOneOutputFile(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// content is the draft's content; empty leaves no draft.
		content    string
		forbidden  []*regexp.Regexp
		wantPassed bool
	}{
		{name: "ValidDraftPasses", content: validBeamDescription, wantPassed: true},
		{name: "DraftWithAFindingFails", content: "# Ship the beam\n"},
		{name: "DraftWithAForbiddenTermFails", content: validBeamDescription, forbidden: []*regexp.Regexp{regexp.MustCompile(`Groups`)}},
		{name: "MissingDraftFails"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			draft := filepath.Join(t.TempDir(), "draft.md")
			if tt.content != "" {
				if err := os.WriteFile(draft, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", draft, err)
				}
			}
			gate := describeGate(tt.forbidden)(shuttleengine.Spec{OutputFiles: []string{draft}})
			if len(gate) != 1 || gate[0].Name != describeGateName || gate[0].Attempts != describeGateAttempts {
				t.Fatalf("describeGate() = %+v; want the one %s entry with %d attempts", gate, describeGateName, describeGateAttempts)
			}
			result, err := gate[0].Gate()
			if err != nil || result.Passed != tt.wantPassed {
				t.Errorf("gate() = %+v, %v; want passed %v", result, err, tt.wantPassed)
			}
		})
	}
}

func TestDescriptionFindings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// content is the description's content; empty leaves no file.
		content   string
		forbidden []*regexp.Regexp
		wantCheck string
	}{
		{name: "ValidDescriptionHasNone", content: validBeamDescription},
		{name: "ForbiddenTermIsNamed", content: validBeamDescription, forbidden: []*regexp.Regexp{regexp.MustCompile(`Groups`)}, wantCheck: summaryparser.CheckForbiddenTerm},
		{name: "EmptyBodyIsNamed", content: "# Ship the beam\n", wantCheck: summaryparser.CheckEmptyBody},
		{name: "MissingFileIsAFinding", wantCheck: summaryparser.CheckMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "summary.md")
			if tt.content != "" {
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
				}
			}
			findings, err := descriptionFindings(path, tt.forbidden)
			if err != nil {
				t.Fatalf("descriptionFindings() = _, %v; want nil", err)
			}
			if tt.wantCheck == "" {
				if len(findings) != 0 {
					t.Errorf("descriptionFindings() = %v; want none", findings)
				}
				return
			}
			if len(findings) != 1 || !strings.Contains(findings[0], tt.wantCheck) {
				t.Errorf("descriptionFindings() = %v; want the one %s finding", findings, tt.wantCheck)
			}
		})
	}
}

// The describer's gate and findings both refuse what the hub's landing.yaml forbids.
func TestBeamDescribeDeps_CarriesTheForbiddenTerms(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	board := t.TempDir()
	configPath := configengine.ConfigFile(board, "landing")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v; want nil", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte(landingshed.ConfigTemplate()), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", configPath, err)
	}
	draft := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(draft, []byte(strings.Replace(validBeamDescription, "Groups the work.", "Run lym.", 1)), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v; want nil", draft, err)
	}

	deps, err := beamDescribeDeps(nil, nil, "pay", configengine.Dirs{Board: board})
	if err != nil {
		t.Fatalf("beamDescribeDeps() = _, %v; want nil", err)
	}
	findings, err := deps.Findings(draft)
	if err != nil || len(findings) != 1 || !strings.Contains(findings[0], summaryparser.CheckForbiddenTerm) {
		t.Errorf("Findings() = %v, %v; want the one %s finding", findings, err, summaryparser.CheckForbiddenTerm)
	}
	shuttle, ok := deps.Shuttle.(*runnerAgentShuttle)
	if !ok {
		t.Fatalf("deps.Shuttle = %T; want *runnerAgentShuttle", deps.Shuttle)
	}
	result, err := shuttle.gate(shuttleengine.Spec{OutputFiles: []string{draft}})[0].Gate()
	if err != nil || result.Passed {
		t.Errorf("gate() = %+v, %v; want a failure on the forbidden term", result, err)
	}
}

func TestRunNoticeTags(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := runNoticeTags("pay")
	want := creelengine.PostTags{Topic: "dobby-run-notice/pay", Run: "pay"}
	if got != want {
		t.Errorf("runNoticeTags(pay) = %+v; want %+v", got, want)
	}
	gotHold := gateHoldTags("pay")
	wantHold := creelengine.PostTags{Topic: "dobby-gate-hold/pay", Run: "pay"}
	if gotHold != wantHold {
		t.Errorf("gateHoldTags(pay) = %+v; want %+v", gotHold, wantHold)
	}
}

func TestEndedTeardownResults(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	ended := func(mutate func(entry *fabriclayout.EntryOutcome)) fabriclayout.OriginReport {
		entry := fabriclayout.EntryOutcome{Slug: "x", State: fabriclayout.EntryEnded, How: "done", Local: true}
		mutate(&entry)
		return fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{entry}}
	}
	notEnded := "origin's board does not show x ended, so the reconcile left its pair; way forward: read lym batten status x"
	tests := []struct {
		name         string
		report       fabriclayout.OriginReport
		reconcileErr error
		want         string
	}{
		{"RemovedPairIsNoFailure", ended(func(e *fabriclayout.EntryOutcome) { e.Removed = true }), nil, ""},
		{"BusyPairNamesItsWayForward", ended(func(e *fabriclayout.EntryOutcome) { e.Busy, e.WayForward = true, "wait for the driver" }), nil, "its pair is busy; wait for the driver"},
		{"FailedTeardownNamesItsError", ended(func(e *fabriclayout.EntryOutcome) { e.Error = "boom" }), nil, "its pair's teardown failed: boom"},
		{"AnotherRunsRemovalLeavesThisOne", ended(func(e *fabriclayout.EntryOutcome) { e.Slug, e.Removed = "y", true }), nil, notEnded},
		{"ActiveEntryIsLeft", fabriclayout.OriginReport{Entries: []fabriclayout.EntryOutcome{{Slug: "x", State: fabriclayout.EntryActive}}}, nil, notEnded},
		{"UnreachableOriginJudgesNothing", fabriclayout.OriginReport{Unreachable: true}, nil, "origin was unreachable, so the reconcile judged no run ended"},
		{"ReconcileErrorFailsEveryRun", fabriclayout.OriginReport{}, errors.New("ls-remote failed"), "the hub's origin reconcile failed: ls-remote failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			results := endedTeardownResults([]string{"x"}, tt.report, tt.reconcileErr)
			got, reported := results["x"]
			if !reported || len(results) != 1 {
				t.Fatalf("endedTeardownResults() = %v; want one result for x", results)
			}
			gotText := ""
			if got != nil {
				gotText = got.Error()
			}
			if gotText != tt.want {
				t.Errorf("endedTeardownResults()[x] = %q; want %q", gotText, tt.want)
			}
		})
	}
}
