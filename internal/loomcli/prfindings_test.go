// prfindings_test.go drives prFindingsVerb over temporary record, scratch and friction directories and fake seams.

package loomcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writePRFindingsFile writes a review file with one finding per severity and returns its path.
// The verdict is BLOCKING exactly when a finding is BLOCKING.
func writePRFindingsFile(t *testing.T, dir string, severities []burlerengine.Severity) string {
	t.Helper()
	verdict := burlerengine.VerdictApproved
	var findings strings.Builder
	for i, severity := range severities {
		if severity == burlerengine.SeverityBlocking {
			verdict = burlerengine.VerdictBlocking
		}
		fmt.Fprintf(&findings, "  - id: F%d\n    severity: %s\n    class: scope\n    location: a.go\n    summary: summary F%d\n    fix: fix it\n", i+1, severity, i+1)
	}
	body := "findings: []\n"
	if len(severities) > 0 {
		body = "findings:\n" + findings.String()
	}
	path := filepath.Join(dir, "findings.md")
	if err := os.WriteFile(path, []byte("---\nverdict: "+string(verdict)+"\n"+body+"---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
	}
	return path
}

// prFindingsEdgeProducers returns the PR-Review row and its Code-Bouncer target, the target built by mutate.
func prFindingsEdgeProducers(onReroute string, target shedengine.ProducerDef) []shedengine.ProducerDef {
	return []shedengine.ProducerDef{{Name: "PR-Review", OnReroute: onReroute}, target}
}

// namesIn returns the file names in dir, none when it does not exist.
func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v; want nil", dir, err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestPRFindingsVerb pins what the verb decides and writes for a findings set.
// It sends back on a BLOCKING finding, or when the findings plus the fixed-in-place count pass the small maximum.
// It sends back only while the send-back edge can fire, and it answers fix-here and counts otherwise.
// A send-back stores the findings file, writes a friction note and marks the record.
// Every refusal names its way forward and leaves the record, the scratch directory and the friction directory unchanged.
func TestPRFindingsVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const smallMax = 3
	open := prFindingsEdgeProducers("Code-Bouncer", shedengine.ProducerDef{Name: "Code-Bouncer", MaxBounces: 3})
	type call struct {
		commitErr  error
		beforeCall func(rec *pullrequest.Record)
		wantCode   int
		wantOutput []string
	}
	tests := []struct {
		name          string
		severities    []burlerengine.Severity
		record        *pullrequest.Record
		producers     []shedengine.ProducerDef
		smallMaxErr   error
		edgeErr       error
		dirty         []string
		dirtyErr      error
		unparsable    bool
		frictionOff   bool
		priorNote     string
		calls         []call
		wantFixHere   int
		wantSendBacks int
		wantCounted   int
		wantStored    string
		wantFindings  []string
		wantNotes     []string
		wantNote      []string
	}{
		{
			name:          "OneBlockingFindingSendsBack",
			severities:    []burlerengine.Severity{burlerengine.SeverityBlocking, burlerengine.SeverityMedium},
			record:        &pullrequest.Record{Number: 7},
			producers:     open,
			calls:         []call{{wantOutput: []string{`"send_back":true`, "pr-findings-1.md"}}},
			wantSendBacks: 1,
			wantStored:    "pr-findings-1.md",
			wantFindings:  []string{"pr-findings-1.md"},
			wantNotes:     []string{"pr-review-send-back-1.md"},
			wantNote:      []string{"Code-Review approved the diff", "1 BLOCKING, 1 MEDIUM, 0 LOW, 0 NIT", "F1 (BLOCKING): summary F1", "F2 (MEDIUM): summary F2", "pr-findings-1.md"},
		},
		{
			name:          "MoreThanSmallMaxFindingsSendBack",
			severities:    []burlerengine.Severity{burlerengine.SeverityMedium, burlerengine.SeverityLow, burlerengine.SeverityLow, burlerengine.SeverityNit},
			record:        &pullrequest.Record{Number: 7},
			producers:     open,
			calls:         []call{{wantOutput: []string{`"send_back":true`}}},
			wantSendBacks: 1,
			wantStored:    "pr-findings-1.md",
			wantFindings:  []string{"pr-findings-1.md"},
			wantNotes:     []string{"pr-review-send-back-1.md"},
		},
		{
			name:          "FixedInPlaceCountLiftsASmallSetPastSmallMax",
			severities:    []burlerengine.Severity{burlerengine.SeverityLow, burlerengine.SeverityNit},
			record:        &pullrequest.Record{Number: 7, FixHereCount: 2, CountedSets: []string{"earlier"}},
			producers:     open,
			calls:         []call{{wantOutput: []string{`"send_back":true`}}},
			wantFixHere:   2,
			wantSendBacks: 1,
			wantCounted:   1,
			wantStored:    "pr-findings-1.md",
			wantFindings:  []string{"pr-findings-1.md"},
			wantNotes:     []string{"pr-review-send-back-1.md"},
		},
		{
			name:        "SmallSetAnswersFixHereAndCountsIt",
			severities:  []burlerengine.Severity{burlerengine.SeverityLow, burlerengine.SeverityNit},
			record:      &pullrequest.Record{Number: 7},
			producers:   open,
			calls:       []call{{wantOutput: []string{`"send_back":false`, `"fix_here_count":2`}}},
			wantFixHere: 2,
			wantCounted: 1,
		},
		{
			name:        "EdgeWithNoOnRerouteAnswersFixHereOnABlockingSet",
			severities:  []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:      &pullrequest.Record{Number: 7},
			producers:   prFindingsEdgeProducers("", shedengine.ProducerDef{Name: "Code-Bouncer", MaxBounces: 3}),
			calls:       []call{{wantOutput: []string{`"send_back":false`, "the PR-Review row declares no reroute edge"}}},
			wantFixHere: 1,
			wantCounted: 1,
		},
		{
			name:       "EdgeWithAFailingEntryGateAnswersFixHereOnABlockingSet",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers: prFindingsEdgeProducers("Code-Bouncer", shedengine.ProducerDef{Name: "Code-Bouncer", MaxBounces: 3, EntryGate: func() (bool, string, error) {
				return false, "the perch is off", nil
			}}),
			calls:       []call{{wantOutput: []string{`"send_back":false`, "the Code-Bouncer row's entry gate does not hold: the perch is off"}}},
			wantFixHere: 1,
			wantCounted: 1,
		},
		{
			name:        "EdgeWithABudgetOfZeroAnswersFixHereOnABlockingSet",
			severities:  []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:      &pullrequest.Record{Number: 7},
			producers:   prFindingsEdgeProducers("Code-Bouncer", shedengine.ProducerDef{Name: "Code-Bouncer"}),
			calls:       []call{{wantOutput: []string{`"send_back":false`, "the Code-Bouncer row's bounce budget is 0"}}},
			wantFixHere: 1,
			wantCounted: 1,
		},
		{
			name:       "SecondSendBackStoresAndNotesUnderItsOwnNumber",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			calls: []call{
				{wantOutput: []string{"pr-findings-1.md"}},
				{
					beforeCall: func(rec *pullrequest.Record) { rec.SendBack = "" },
					wantOutput: []string{"pr-findings-2.md"},
				},
			},
			wantSendBacks: 2,
			wantStored:    "pr-findings-2.md",
			wantFindings:  []string{"pr-findings-1.md", "pr-findings-2.md"},
			wantNotes:     []string{"pr-review-send-back-1.md", "pr-review-send-back-2.md"},
		},
		{
			name:          "EmptyFrictionDirectorySendsBackAndWritesNoNote",
			severities:    []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:        &pullrequest.Record{Number: 7},
			producers:     open,
			frictionOff:   true,
			calls:         []call{{wantOutput: []string{`"send_back":true`}}},
			wantSendBacks: 1,
			wantStored:    "pr-findings-1.md",
			wantFindings:  []string{"pr-findings-1.md"},
		},
		{
			name:       "DirtyWorktreeIsRefused",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			dirty:      []string{"a.go", "b.go"},
			calls: []call{{wantCode: 1, wantOutput: []string{
				"the task worktree has uncommitted paths: a.go, b.go",
				"way forward: commit or discard them, keeping the findings file under the task worktree's .scratch/ directory",
			}}},
		},
		{
			name:       "UnreadableDirtyPathsAreAnError",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			dirtyErr:   errors.New("git failed"),
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read the task worktree's uncommitted paths: git failed",
				"way forward: run lym loom pr-findings",
			}}},
		},
		{
			name:       "UnparsableFileIsRefused",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			unparsable: true,
			calls: []call{{wantCode: 1, wantOutput: []string{
				"is not a findings file: burler: review file must open with",
				"way forward: write the findings in burler's review format",
				"burler-review-format.md defines",
			}}},
		},
		{
			name:       "RunWithNoRecordIsRefused",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			producers:  open,
			calls: []call{{wantCode: 1, wantOutput: []string{
				"this run has no pull request record",
				"way forward: wait until the PR-Review row has opened the pull request, then run lym loom pr-findings",
			}}},
		},
		{
			name:        "FailingSmallMaxReadIsAnError",
			severities:  []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:      &pullrequest.Record{Number: 7},
			producers:   open,
			smallMaxErr: errors.New("config broken"),
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read pr.review_small_max: config broken",
				"way forward: run lym loom pr-findings",
				"tell the operator when it keeps failing",
			}}},
		},
		{
			name:       "FailingEdgeReadIsAnError",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			edgeErr:    errors.New("recipe unreadable"),
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read whether the send-back edge can fire: recipe unreadable",
				"tell the operator when it keeps failing",
			}}},
		},
		{
			name:       "EdgeOverARecipeWithoutTheCodeBouncerRowIsAnError",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers:  []shedengine.ProducerDef{{Name: "PR-Review", OnReroute: "Code-Bouncer"}},
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read whether the send-back edge can fire: the run's recipe has no Code-Bouncer row",
				"tell the operator when it keeps failing",
			}}},
		},
		{
			name:       "EdgeWithAnEntryGateThatErrorsIsAnError",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			producers: prFindingsEdgeProducers("Code-Bouncer", shedengine.ProducerDef{Name: "Code-Bouncer", MaxBounces: 3, EntryGate: func() (bool, string, error) {
				return false, "", errors.New("gate unreadable")
			}}),
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read whether the send-back edge can fire: read the Code-Bouncer row's entry gate: gate unreadable",
				"tell the operator when it keeps failing",
			}}},
		},
		{
			name:          "SendBackWhoseNoteAlreadyExistsKeepsTheFirstNote",
			severities:    []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:        &pullrequest.Record{Number: 7},
			producers:     open,
			priorNote:     "pr-review-send-back-1.md",
			calls:         []call{{wantOutput: []string{`"send_back":true`}}},
			wantSendBacks: 1,
			wantStored:    "pr-findings-1.md",
			wantFindings:  []string{"pr-findings-1.md"},
			wantNotes:     []string{"pr-review-send-back-1.md"},
			wantNote:      []string{"a note an interrupted send-back left"},
		},
		{
			name:       "EdgeOverARecipeWithoutThePRReviewRowIsAnError",
			severities: []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:     &pullrequest.Record{Number: 7},
			calls: []call{{wantCode: 1, wantOutput: []string{
				"read whether the send-back edge can fire: the run's recipe has no PR-Review row",
			}}},
		},
		{
			name:          "MarkedRecordAnswersTheSendBackAgainAndWritesNothing",
			severities:    []burlerengine.Severity{burlerengine.SeverityBlocking},
			record:        &pullrequest.Record{Number: 7, SendBack: "/run/pr-findings-1.md", SendBacks: 1},
			producers:     open,
			calls:         []call{{wantOutput: []string{`"send_back":true`, "/run/pr-findings-1.md"}}},
			wantSendBacks: 1,
			wantStored:    "/run/pr-findings-1.md",
		},
		{
			name:       "FailedCommitIsAnErrorAndTheRerunCountsTheSetOnce",
			severities: []burlerengine.Severity{burlerengine.SeverityLow, burlerengine.SeverityNit},
			record:     &pullrequest.Record{Number: 7},
			producers:  open,
			calls: []call{
				{commitErr: errors.New("push stopped"), wantCode: 1, wantOutput: []string{
					"commit the pull request record: push stopped",
					"way forward: run lym loom pr-findings",
				}},
				{wantOutput: []string{`"send_back":false`, `"fix_here_count":2`}},
			},
			wantFixHere: 2,
			wantCounted: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			recordPath, lockPath := filepath.Join(dir, "pull-request.json"), filepath.Join(dir, "pull-request.json.lock")
			scratch, frictionDir := filepath.Join(dir, "scratch"), filepath.Join(dir, "friction")
			if tt.frictionOff {
				frictionDir = ""
			}
			if tt.record != nil {
				if err := pullrequest.WriteRecord(recordPath, lockPath, *tt.record); err != nil {
					t.Fatalf("WriteRecord() = %v; want nil", err)
				}
			}
			if tt.priorNote != "" {
				if err := os.MkdirAll(frictionDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", frictionDir, err)
				}
				if err := os.WriteFile(filepath.Join(frictionDir, tt.priorNote), []byte("a note an interrupted send-back left"), 0o644); err != nil {
					t.Fatalf("WriteFile(prior note) = %v; want nil", err)
				}
			}
			file := writePRFindingsFile(t, dir, tt.severities)
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%q) = %v; want nil", file, err)
			}
			if tt.unparsable {
				if err := os.WriteFile(file, []byte("not a review\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", file, err)
				}
			}

			var commitErr error
			deps := prFindingsDeps{
				dirty:          func() ([]string, error) { return tt.dirty, tt.dirtyErr },
				recordPath:     recordPath,
				recordLockPath: lockPath,
				scratchDir:     scratch,
				frictionDir:    frictionDir,
				stencilsDir:    "/hub/stencils",
				smallMax:       func() (int, error) { return smallMax, tt.smallMaxErr },
				edge: func() (bool, string, error) {
					if tt.edgeErr != nil {
						return false, "", tt.edgeErr
					}
					return sendBackEdge(tt.producers, "PR-Review")
				},
				commit: func() error { return commitErr },
			}
			for i, c := range tt.calls {
				if c.beforeCall != nil {
					if err := pullrequest.UpdateRecord(recordPath, lockPath, func(rec *pullrequest.Record) error {
						c.beforeCall(rec)
						return nil
					}); err != nil {
						t.Fatalf("UpdateRecord() = %v; want nil", err)
					}
				}
				commitErr = c.commitErr
				var out bytes.Buffer
				if code := prFindingsVerb(&out, file, deps); code != c.wantCode {
					t.Errorf("call %d: prFindingsVerb() = %d, %s; want exit %d", i+1, code, out.String(), c.wantCode)
				}
				for _, want := range c.wantOutput {
					if !strings.Contains(out.String(), want) {
						t.Errorf("call %d: prFindingsVerb() output = %s; want it to contain %q", i+1, out.String(), want)
					}
				}
			}

			got, found, err := pullrequest.ReadRecord(recordPath, lockPath)
			if err != nil || found != (tt.record != nil) {
				t.Fatalf("ReadRecord() = %v, %v; want found %v, nil", found, err, tt.record != nil)
			}
			wantStored := tt.wantStored
			if wantStored != "" && !filepath.IsAbs(wantStored) {
				wantStored = filepath.Join(scratch, wantStored)
			}
			if got.SendBack != wantStored || got.FixHereCount != tt.wantFixHere || got.SendBacks != tt.wantSendBacks || len(got.CountedSets) != tt.wantCounted {
				t.Errorf("record = {SendBack: %q, FixHereCount: %d, SendBacks: %d, CountedSets: %d}; want {%q, %d, %d, %d}",
					got.SendBack, got.FixHereCount, got.SendBacks, len(got.CountedSets), wantStored, tt.wantFixHere, tt.wantSendBacks, tt.wantCounted)
			}
			if tt.wantCounted == 1 && tt.record != nil && len(tt.record.CountedSets) == 0 {
				sum := sha256.Sum256(content)
				if got.CountedSets[0] != hex.EncodeToString(sum[:]) {
					t.Errorf("CountedSets = %v; want the findings file's sha256 %x", got.CountedSets, sum)
				}
			}

			if names := namesIn(t, scratch); !equalNames(names, tt.wantFindings) {
				t.Errorf("scratch directory holds %v; want %v", names, tt.wantFindings)
			}
			if names := namesIn(t, frictionDir); !equalNames(names, tt.wantNotes) {
				t.Errorf("friction directory holds %v; want %v", names, tt.wantNotes)
			}
			if len(tt.wantNote) > 0 {
				note, err := os.ReadFile(filepath.Join(frictionDir, tt.wantNotes[0]))
				if err != nil {
					t.Fatalf("ReadFile(first note) = %v; want nil", err)
				}
				for _, want := range tt.wantNote {
					if !strings.Contains(string(note), want) {
						t.Errorf("first note = %s; want it to contain %q", note, want)
					}
				}
			}
		})
	}
}

// equalNames reports whether got and want hold the same names in order, a nil and an empty list being equal.
func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestPRReviewSmallMax_ReadsThePairsLayers asserts what the small-maximum seam reads.
// It reads pr.review_small_max over the pair's layers, so the task's local layer, where run_settings land, wins.
func TestPRReviewSmallMax_ReadsThePairsLayers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		localLayer string
		want       int
	}{
		{name: "no local value reads the hub's", want: 3},
		{name: "the task's local layer wins over the hub's", localLayer: "pr: {review_small_max: 1}\n", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			seedLoomConfigWithKeys(t, loc.AnchorPath(), map[string]string{"pr.review_small_max": "3"})
			if tt.localLayer != "" {
				localConfig := configengine.LocalFile(loc.AnchorPath(), "loom")
				if err := os.MkdirAll(filepath.Dir(localConfig), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(localConfig), err)
				}
				if err := os.WriteFile(localConfig, []byte(tt.localLayer), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", localConfig, err)
				}
			}

			got, err := prReviewSmallMax(loc)()
			if err != nil || got != tt.want {
				t.Errorf("prReviewSmallMax() = %d, %v; want %d, nil", got, err, tt.want)
			}
		})
	}
}
