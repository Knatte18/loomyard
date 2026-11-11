// decisionreplace_test.go tests `lym loom decision replace` over the decision fake.
// It also tests the decision lock every Go writer of the record takes.
// It tests the board copy of a stored record and the filed raddle refusal too.

package loomcli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	replaceHead      = "## Goal\n\nKeep the cache.\n\n## Scope\n\n- internal/x\n\n## Decisions\n\n### D1 — Keep the cache\n\nKeep it per run.\n\n"
	replaceAdded     = "### Added after Discussion (parent, 2026-03-01): Drop the flag\n\nDecision: Drop --fast.\n\nRationale: Nobody passes it.\n"
	replaceRisks     = "\n## Constraints\n\n- none\n\n## Auto-mode assumptions\n\n- none\n\n## Open risks\n\n- none\n"
	replaceCarryOver = "\n<!-- lym:carry-over Code-Review -->\n- finding k1\n<!-- /lym:carry-over Code-Review -->\n"
	replaceCriteria  = "\n## Acceptance criteria\n\n- D1: the cache is per run.\n"

	// replaceBaseRecord is the committed record a replace starts from.
	// It holds one decision, one added entry and one carry-over block.
	replaceBaseRecord = replaceHead + replaceAdded + replaceRisks + replaceCarryOver + replaceCriteria
	// replaceFoldedRecord folds the added entry into a main decision, keeping the carry-over block.
	replaceFoldedRecord = replaceHead + "### D2 — Drop the flag\n\nDrop --fast.\n" + replaceRisks + replaceCarryOver +
		"\n## Acceptance criteria\n\n- D1: the cache is per run.\n- D2: no --fast flag.\n"

	replaceTitle  = "Fold the flag call"
	replaceStrand = "hub:task-a:darn"
)

// newReplaceFake is a decision fake of a run past its Discussion approval.
// Its record, working and committed, is replaceBaseRecord.
// One live agent of the run takes notices.
func newReplaceFake(t *testing.T) *decisionFake {
	t.Helper()
	f := newDecisionFake(t)
	if err := os.WriteFile(f.path, []byte(replaceBaseRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	f.committedCopy, f.committedFound = []byte(replaceBaseRecord), true
	f.hasApprovalRow = true
	f.liveStrands = []string{replaceStrand}
	return f.atPastRow("Drift-Baseline")
}

// replaceInputOf writes file as a --from file and returns a replace input of it labelled parent over replaceBaseRecord.
func replaceInputOf(t *testing.T, file string) replaceInput {
	t.Helper()
	path := filepath.Join(t.TempDir(), "record.md")
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	return replaceInput{by: decisionByParent, from: path, title: replaceTitle, baseSHA256: sha256Hex([]byte(replaceBaseRecord))}
}

// TestDecisionReplaceVerb_WritesCommitsAndNotifies asserts each admitted caller's replace writes the file as the record.
// It commits the record under the replace trailer, copies it to the board and tells each live agent to re-read it whole.
func TestDecisionReplaceVerb_WritesCommitsAndNotifies(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		by    string
		setup func(f *decisionFake)
	}{
		{name: "UnnamedShell", by: decisionByParent},
		{name: "ParentOrch", by: decisionByParent, setup: func(f *decisionFake) {
			f.strandName, f.parent = "hub:orch", hubgeom.Parent{Name: "hub:orch"}
		}},
		{name: "RunsArbiter", by: decisionByArbiter, setup: func(f *decisionFake) {
			f.strandName, f.arbiterName = "hub:task-a:arbiter", "hub:task-a:arbiter"
		}},
		{name: "SamplerStrandWithOperatorLabel", by: decisionByOperator, setup: func(f *decisionFake) {
			f.strandName, f.samplerName = "hub:task-a:sampler", "hub:task-a:sampler"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReplaceFake(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			input := replaceInputOf(t, replaceFoldedRecord)
			input.by = tt.by

			var out bytes.Buffer
			if code := decisionReplaceVerb(&out, "task-a", f.deps(), input); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}
			if got := readDecisionRecord(t, f.path); got != replaceFoldedRecord {
				t.Errorf("record = %q; want the --from file", got)
			}
			if string(f.committedCopy) != replaceFoldedRecord || f.replaceCommits != 1 || f.emptyCommits != 0 {
				t.Errorf("HEAD = %q after %d replace and %d empty commits; want the file in one replace commit", f.committedCopy, f.replaceCommits, f.emptyCommits)
			}
			wantTrailer := tt.by + " _lym/discussion/decision-record.md " + sha256Hex([]byte(replaceFoldedRecord))
			if f.headTrailer != wantTrailer {
				t.Errorf("HEAD trailer = %q; want %q", f.headTrailer, wantTrailer)
			}
			if len(f.stored) != 1 || string(f.stored[0]) != replaceFoldedRecord {
				t.Errorf("stored = %q; want one copy of the replaced record", f.stored)
			}
			if len(f.posts) != 1 || f.posts[0].receiver != replaceStrand ||
				!strings.HasPrefix(f.posts[0].line, decisionReplaceNoticeOpening) || !strings.Contains(f.posts[0].line, replaceTitle) {
				t.Errorf("posts = %+v; want one notice to %s opening %q and naming %q", f.posts, replaceStrand, decisionReplaceNoticeOpening, replaceTitle)
			}
			if f.lockCalls != 1 || f.lockReleases != 1 {
				t.Errorf("decision lock taken %d and released %d times; want once each", f.lockCalls, f.lockReleases)
			}
			data := envelope.RequireOK(t, out.String()).Raw
			if data["title"] != replaceTitle || data["sha256"] != sha256Hex([]byte(replaceFoldedRecord)) || data["commit_error"] != nil {
				t.Errorf("envelope = %v; want the title, the written sha256 and no commit_error", data)
			}
		})
	}
}

// TestDecisionReplaceVerb_Refusals asserts every refusal names its way forward.
// Each leaves the record, HEAD and the board as they were.
func TestDecisionReplaceVerb_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const otherSHA = "0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name  string
		file  string
		setup func(f *decisionFake, in *replaceInput)
		want  []string
		// notWant lists text the message must not contain.
		notWant []string
	}{
		{
			name:    "BaseDiffersFromHEAD",
			setup:   func(_ *decisionFake, in *replaceInput) { in.baseSHA256 = otherSHA },
			want:    []string{"loom: decision replace:", "is not the record --base-sha256 names", "way forward:", "re-read the record", "rebuild the --from file"},
			notWant: []string{otherSHA, sha256Hex([]byte(replaceBaseRecord))},
		},
		{
			name:  "WorkingCopyDiffersFromHEAD",
			setup: func(f *decisionFake, _ *replaceInput) { f.workingOverride = []byte(replaceBaseRecord + "stray\n") },
			want:  []string{"differs from its committed copy", "way forward:"},
		},
		{
			name:  "RecordNotCommitted",
			setup: func(f *decisionFake, _ *replaceInput) { f.committedFound = false },
			want:  []string{"not committed at HEAD", "way forward:", "lym fabric commit"},
		},
		{
			name: "BeforeTheDiscussionApproval",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.status.CurrentProducer = "Discussion-Burler-Fix"
			},
			want: []string{"has not passed its Discussion approval", "way forward:", `"lym loom decision add"`},
		},
		{
			name: "AtAWebsterRow",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.atPastRow("Webster").implementerRows = map[string]bool{"Webster": true}
			},
			want: []string{"stands at Webster", "past its plan's approval", "way forward:", `"lym loom decision add"`, "once the run has left its Webster rows"},
		},
		{
			name: "BetweenPlanApprovalAndWebster",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.atPastRow("Drift-Await").implementerRows = map[string]bool{"Drift-Await": true}
			},
			want: []string{"stands at Drift-Await", "way forward:", `"lym loom decision add"`},
		},
		{
			name: "DiscussionWriteRunning",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.status = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NameDiscussionWrite}
			},
			want: []string{"Discussion-Write is running", "way forward:"},
		},
		{
			name: "SectionMissing",
			file: strings.Replace(replaceFoldedRecord, "## Constraints", "## Limits", 1),
			want: []string{"fails the decision record's format check", "discussion-section-missing", "way forward:", "the record is unchanged"},
		},
		{
			name: "DecisionNotInAcceptance",
			file: strings.Replace(replaceFoldedRecord, "- D2: no --fast flag.\n", "", 1),
			want: []string{"fails the decision record's format check", "decision-not-in-acceptance", "way forward:"},
		},
		{
			name: "CarryOverDropped",
			file: strings.Replace(replaceFoldedRecord, replaceCarryOver, "", 1),
			want: []string{"changes the record's carry-over blocks", "Code-Review", "way forward:", "copy every carry-over block"},
		},
		{
			name: "CarryOverChanged",
			file: strings.Replace(replaceFoldedRecord, "finding k1", "finding k9", 1),
			want: []string{"changes the record's carry-over blocks", "way forward:"},
		},
		{
			name: "CarryOverAdded",
			file: strings.Replace(replaceFoldedRecord, replaceCarryOver, replaceCarryOver+"\n<!-- lym:carry-over Plan-Review -->\n- k2\n<!-- /lym:carry-over Plan-Review -->\n", 1),
			want: []string{"changes the record's carry-over blocks", "Plan-Review", "way forward:"},
		},
		{
			name: "AddedEntryRewritten",
			file: replaceHead + strings.Replace(replaceAdded, "Nobody passes it.", "Nobody needs it.", 1) + replaceRisks + replaceCarryOver + replaceCriteria,
			want: []string{"inserts or rewrites an added entry", "way forward:", `"lym loom decision add"`},
		},
		{
			name: "AddedEntryInserted",
			file: replaceHead + replaceAdded + "\n### Added after Discussion (parent, 2026-03-02): Grow the pool\n\nDecision: Grow it.\n" + replaceRisks + replaceCarryOver + replaceCriteria,
			want: []string{"inserts or rewrites an added entry", "Grow the pool", "way forward:"},
		},
		{
			name: "NamedTaskSession",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.strandName, f.parent = replaceStrand, hubgeom.Parent{Name: "hub:orch"}
			},
			want: []string{"is a task session", "ask hub:orch to run the verb"},
		},
		{
			name: "SamplerStrandWithParentLabel",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.strandName, f.samplerName = "hub:task-a:sampler", "hub:task-a:sampler"
			},
			want: []string{"sampler session", "way forward:", "--by operator"},
		},
		{
			name:  "ArbiterLabelFromAnotherStrand",
			setup: func(f *decisionFake, in *replaceInput) { in.by, f.strandName = decisionByArbiter, replaceStrand },
			want:  []string{"is not the run's arbiter", "way forward:"},
		},
		{
			name:  "UnknownLabel",
			setup: func(_ *decisionFake, in *replaceInput) { in.by = "someone" },
			want:  []string{`--by is "someone"`, "way forward:"},
		},
		{
			name:  "EmptyBase",
			setup: func(_ *decisionFake, in *replaceInput) { in.baseSHA256 = " " },
			want:  []string{"--base-sha256 is empty", "way forward:"},
		},
		{
			name:  "UnreadableFile",
			setup: func(_ *decisionFake, in *replaceInput) { in.from = filepath.Join(filepath.Dir(in.from), "absent.md") },
			want:  []string{"read the --from file", "way forward:"},
		},
		{
			name:  "NoticeTooLong",
			setup: func(_ *decisionFake, in *replaceInput) { in.title = strings.Repeat("long title ", 1000) },
			want:  []string{"replace notice would pass the line creel delivers", "way forward:", "shorten --title"},
		},
		{
			name:  "LockFails",
			setup: func(f *decisionFake, _ *replaceInput) { f.lockErr = errors.New("take the run's decision lock: boom") },
			want:  []string{"decision lock: boom", "way forward:"},
		},
		{
			name:  "WriteFailsAndTheRestoreSucceeds",
			setup: func(f *decisionFake, _ *replaceInput) { f.writeFirstErr = errors.New("write boom") },
			want:  []string{"write the decision record: write boom", "way forward:", `re-run "lym loom decision replace"`},
		},
		{
			name:  "WriteAndRestoreBothFail",
			setup: func(f *decisionFake, _ *replaceInput) { f.writeAllErr = errors.New("write boom") },
			want:  []string{"restoring it failed too", "way forward: rewrite the file named above"},
		},
		{
			name: "FiledRaddleRun",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.raddleFileRow, f.raddleFiled = loomshed.NameRaddleFile, []string{"part-a", "part-b"}
			},
			want: []string{"has filed part-a, part-b", "way forward:", `"lym loom decision replace <part-slug>"`, "--record-file"},
		},
		{
			name: "PerchRunAtRaddleFileBeforeTheFiling",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.atRaddleFile(true)
			},
			want: []string{"stands at Raddle-File in perch mode", "way forward:"},
		},
		{
			name: "RecordWriteRunRecordNotCommitted",
			setup: func(f *decisionFake, _ *replaceInput) {
				f.recordRow, f.committedFound = true, false
			},
			want: []string{"the decision record is not committed at HEAD", "way forward:", `"lym loom goto --to Record-Write"`},
		},
		{
			name:  "RecordFileAbsent",
			setup: func(f *decisionFake, _ *replaceInput) { f.readRecordErr = fs.ErrNotExist },
			want:  []string{"the run has no decision record at", "way forward:", "Discussion-Write has written it"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReplaceFake(t)
			file := tt.file
			if file == "" {
				file = replaceFoldedRecord
			}
			input := replaceInputOf(t, file)
			if tt.setup != nil {
				tt.setup(f, &input)
			}

			var out bytes.Buffer
			if code := decisionReplaceVerb(&out, "task-a", f.deps(), input); code == 0 {
				t.Fatalf("exit = 0, out %s; want a refusal", out.String())
			}
			msg := envelope.Decode(t, out.String()).Error
			for _, w := range tt.want {
				if !strings.Contains(msg, w) {
					t.Errorf("message %q lacks %q", msg, w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(msg, w) {
					t.Errorf("message %q contains %q", msg, w)
				}
			}
			if got := readDecisionRecord(t, f.path); got != replaceBaseRecord {
				t.Errorf("record = %q; want it unchanged", got)
			}
			if f.replaceCommits != 0 || f.emptyCommits != 0 || len(f.stored) != 0 || len(f.posts) != 0 {
				t.Errorf("%d replace commits, %d empty commits, stored %q, posts %+v; want none", f.replaceCommits, f.emptyCommits, f.stored, f.posts)
			}
			if f.lockCalls != f.lockReleases {
				t.Errorf("decision lock taken %d and released %d times; want every take released", f.lockCalls, f.lockReleases)
			}
		})
	}
}

// TestDecisionReplaceVerb_CommitOutcomes asserts what a replace does with each outcome of its commit.
// HEAD ends at a commit carrying the replace trailer, or the record is restored.
// Otherwise the refusal names the way to land the trailer.
func TestDecisionReplaceVerb_CommitOutcomes(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	commitBoom := errors.New("commit boom")
	tests := []struct {
		name  string
		setup func(f *decisionFake)
		// wantOK is whether the call succeeds; wantRecord is the record afterwards.
		wantOK     bool
		wantRecord string
		// wantEmpty is the number of empty trailer commits, and wantTrailer whether HEAD ends carrying the call's trailer.
		wantEmpty   int
		wantTrailer bool
		// wantCommitError is whether the envelope reports commit_error; want lists a refusal's fragments.
		wantCommitError bool
		want            []string
	}{
		{
			name:       "HEADAlreadyHoldsTheBytes_LandsAnEmptyTrailerCommit",
			setup:      func(f *decisionFake) { f.replaceUnstaged = true },
			wantOK:     true,
			wantRecord: replaceFoldedRecord, wantEmpty: 1, wantTrailer: true,
		},
		{
			name: "HEADAlreadyHoldsTheBytesAndTheEmptyCommitFails_NamesReplace",
			setup: func(f *decisionFake) {
				f.replaceUnstaged, f.emptyErr = true, errors.New("empty boom")
			},
			wantRecord: replaceFoldedRecord, wantEmpty: 1,
			want: []string{"empty boom", "way forward:", `"lym loom decision replace", which lands the trailer commit`},
		},
		{
			name:       "CommitFailsWithoutLanding_RestoresTheRecord",
			setup:      func(f *decisionFake) { f.replaceCommitErr = commitBoom },
			wantRecord: replaceBaseRecord,
			want:       []string{"commit boom", "the record is restored", `re-run "lym loom decision replace"`, `never "lym fabric commit"`},
		},
		{
			name: "CommitErrorAfterLandingWithTheTrailer_ReportsCommitError",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.replaceErrLands, f.replaceErrTrailer = commitBoom, true, true
			},
			wantOK:     true,
			wantRecord: replaceFoldedRecord, wantTrailer: true, wantCommitError: true,
		},
		{
			name: "CommitErrorAfterLandingWithoutTheTrailer_LandsTheTrailerCommit",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.replaceErrLands = commitBoom, true
			},
			wantOK:     true,
			wantRecord: replaceFoldedRecord, wantEmpty: 1, wantTrailer: true,
		},
		{
			name: "CommitErrorAfterLandingWithoutTheTrailerAndTheEmptyCommitFails_NamesReplace",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.replaceErrLands, f.emptyErr = commitBoom, true, errors.New("empty boom")
			},
			wantRecord: replaceFoldedRecord, wantEmpty: 1,
			want: []string{"empty boom", "way forward:", `"lym loom decision replace", which lands the trailer commit`},
		},
		{
			name: "CommitFailsAndHEADCannotBeRead_KeepsTheWrittenRecord",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.committedAfterErr = commitBoom, errors.New("head boom")
			},
			wantRecord: replaceFoldedRecord,
			want:       []string{"the commit failed: commit boom", "could not be read: head boom", "way forward:", "re-read the record"},
		},
		{
			name: "CommitErrorAfterLandingAndHEADTrailerCannotBeRead_NamesReplace",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.replaceErrLands, f.headCarriesErr = commitBoom, true, errors.New("carries boom")
			},
			wantRecord: replaceFoldedRecord,
			want:       []string{"the commit reported commit boom", "trailer could not be read: carries boom", "way forward:", "which lands the trailer commit"},
		},
		{
			name: "CommitFailsWithoutLandingAndTheRestoreFails_NamesTheRestore",
			setup: func(f *decisionFake) {
				f.replaceCommitErr, f.restoreErr = commitBoom, errors.New("restore boom")
			},
			wantRecord: replaceFoldedRecord,
			want:       []string{"commit boom", "restoring the record failed too: restore boom", "way forward: rewrite the file named above"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReplaceFake(t)
			tt.setup(f)
			input := replaceInputOf(t, replaceFoldedRecord)

			var out bytes.Buffer
			code := decisionReplaceVerb(&out, "task-a", f.deps(), input)
			if (code == 0) != tt.wantOK {
				t.Fatalf("exit = %d, out %s; want success %t", code, out.String(), tt.wantOK)
			}
			if got := readDecisionRecord(t, f.path); got != tt.wantRecord {
				t.Errorf("record = %q; want %q", got, tt.wantRecord)
			}
			if f.emptyCommits != tt.wantEmpty {
				t.Errorf("empty commits = %d; want %d", f.emptyCommits, tt.wantEmpty)
			}
			wantTrailer := recordReplacedValue(decisionByParent, "_lym/discussion/decision-record.md", []byte(replaceFoldedRecord))
			if (f.headTrailer == wantTrailer) != tt.wantTrailer {
				t.Errorf("HEAD trailer = %q; want carrying %q: %t", f.headTrailer, wantTrailer, tt.wantTrailer)
			}
			if !tt.wantOK {
				msg := envelope.Decode(t, out.String()).Error
				for _, w := range tt.want {
					if !strings.Contains(msg, w) {
						t.Errorf("message %q lacks %q", msg, w)
					}
				}
				return
			}
			got, hasError := envelope.RequireOK(t, out.String()).Raw["commit_error"].(string)
			if hasError != tt.wantCommitError || (hasError && got != commitBoom.Error()) {
				t.Errorf("commit_error = %q (present %t); want present %t", got, hasError, tt.wantCommitError)
			}
		})
	}
}

// orderedEvents records the order in which concurrent writers of one record reach their steps.
type orderedEvents struct {
	mu     sync.Mutex
	events []string
}

func (e *orderedEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *orderedEvents) list() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// TestDecisionLock_SerializesRecordWriters asserts a write started while a replace holds the decision lock runs after it.
// An add's entry and a carry-over block then both land in the record over the replaced bytes.
func TestDecisionLock_SerializesRecordWriters(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// write runs the second writer against the record at path, calling waiting just before it takes the lock.
		write func(t *testing.T, path string, events *orderedEvents, waiting func()) error
		// want is a fragment the record holds once both writers are done.
		want string
	}{
		{
			name: "AddRunsAfterReplace",
			write: func(t *testing.T, path string, events *orderedEvents, waiting func()) error {
				add := newDecisionFake(t)
				add.path = path
				add.lockOverride = func() (func() error, error) {
					waiting()
					return holdDecisionLock(path)
				}
				deps := add.deps()
				appendEntries := deps.append
				deps.append = func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
					events.add("add-append")
					return appendEntries(ds)
				}
				var out bytes.Buffer
				if code := decisionVerb(&out, "task-a", deps, goodDecisionInput()); code != 0 {
					return errors.New(out.String())
				}
				return nil
			},
			want: "### Added after Discussion (parent, 2026-03-04): Keep the cache",
		},
		{
			name: "CarryOverRunsAfterReplace",
			write: func(_ *testing.T, path string, events *orderedEvents, waiting func()) error {
				waiting()
				entry := discussionparser.CarryOver{
					Segment: "Plan-Review", Round: 1, Closing: discussionparser.CarryOverConverged,
					ReviewPath: "r.md", FixerReportPath: "f.md",
					Findings: []discussionparser.CarryOverFinding{{Key: "k2", Class: "c", Severity: "s", Rounds: []int{1}}},
				}
				return writeCarryOverLocked(path, entry, func() error {
					events.add("carry-over-commit")
					return nil
				})
			},
			want: "<!-- lym:carry-over Plan-Review -->",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			events := &orderedEvents{}
			replace := newReplaceFake(t)
			replace.lockOverride = func() (func() error, error) { return holdDecisionLock(replace.path) }
			replace.commitEntered, replace.commitGate = make(chan struct{}), make(chan struct{})
			replace.onReplaceCommit = func() { events.add("replace-commit") }
			input := replaceInputOf(t, replaceFoldedRecord)

			replaceDone := make(chan int)
			go func() {
				var out bytes.Buffer
				replaceDone <- decisionReplaceVerb(&out, "task-a", replace.deps(), input)
			}()
			<-replace.commitEntered

			waiting := make(chan struct{})
			secondDone := make(chan error)
			go func() {
				secondDone <- tt.write(t, replace.path, events, func() { close(waiting) })
			}()
			<-waiting
			close(replace.commitGate)

			if code := <-replaceDone; code != 0 {
				t.Fatalf("replace exit = %d; want 0", code)
			}
			if err := <-secondDone; err != nil {
				t.Fatalf("second writer error = %v; want nil", err)
			}
			if got := events.list(); len(got) != 2 || got[0] != "replace-commit" {
				t.Errorf("events = %v; want the replace commit first, then the second writer", got)
			}
			record := readDecisionRecord(t, replace.path)
			if !strings.Contains(record, "### D2 — Drop the flag") || !strings.Contains(record, tt.want) {
				t.Errorf("record = %q; want the replaced record with %q", record, tt.want)
			}
		})
	}
}

// TestDecisionCalls_RefreshEveryStoredRecord asserts which runs' calls copy the committed record onto the board entry.
// A run of a recipe with no Discussion approval copies when its entry carries a stored record,
// and a run of a recipe with one copies only past it.
func TestDecisionCalls_RefreshEveryStoredRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		setup func(f *decisionFake)
		// verb is add, replace or store.
		verb       string
		wantStored bool
	}{
		{name: "AddOnARaddleRunOfAPromotedSketch", verb: "add", setup: func(f *decisionFake) {
			f.raddleFileRow, f.boardStored = loomshed.NameRaddleFile, true
		}, wantStored: true},
		{name: "ReplaceOnARaddleRunOfAPromotedSketch", verb: "replace", setup: func(f *decisionFake) {
			f.raddleFileRow, f.boardStored = loomshed.NameRaddleFile, true
		}, wantStored: true},
		{name: "StoreOnARaddleRunOfAPromotedSketch", verb: "store", setup: func(f *decisionFake) {
			f.raddleFileRow, f.boardStored = loomshed.NameRaddleFile, true
		}, wantStored: true},
		{name: "AddOnARunWithNoStoredRecord", verb: "add"},
		{name: "AddOnARunReEnteredIntoItsDiscussion", verb: "add", setup: func(f *decisionFake) {
			f.hasApprovalRow, f.boardStored = true, true
			f.status.CurrentProducer, f.found = "Discussion-Burler-Fix", true
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newDecisionFake(t)
			if err := os.WriteFile(f.path, []byte(replaceBaseRecord), 0o644); err != nil {
				t.Fatal(err)
			}
			f.committedCopy, f.committedFound = []byte(replaceBaseRecord), true
			if tt.setup != nil {
				tt.setup(f)
			}

			var out bytes.Buffer
			var code int
			switch tt.verb {
			case "add":
				code = decisionVerb(&out, "task-a", f.deps(), goodDecisionInput())
			case "replace":
				code = decisionReplaceVerb(&out, "task-a", f.deps(), replaceInputOf(t, replaceFoldedRecord))
			default:
				code = decisionStoreVerb(&out, "task-a", f.deps())
			}
			if code != 0 {
				t.Fatalf("%s exit = %d, out %s; want 0", tt.verb, code, out.String())
			}
			if tt.wantStored != (len(f.stored) == 1) {
				t.Errorf("stored = %q; want stored %t", f.stored, tt.wantStored)
			}
			if tt.wantStored && !bytes.Equal(f.stored[0], f.committedCopy) {
				t.Errorf("stored = %q; want the committed record %q", f.stored[0], f.committedCopy)
			}
		})
	}
}

// TestBoardRecordStoredOf asserts a beam's project record is never reported as stored, without reading its entry.
// Any other run reports what its board entry says, or the read's error.
func TestBoardRecordStoredOf(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	readBoom := errors.New("read boom")
	tests := []struct {
		name    string
		project bool
		stored  bool
		readErr error
		// want is the reported answer, wantRead whether the entry was read and wantErr the error returned.
		want, wantRead bool
		wantErr        error
	}{
		{name: "BeamProjectRecordWhoseEntryHoldsOne", project: true, stored: true},
		{name: "EntryCarriesARecord", stored: true, want: true, wantRead: true},
		{name: "EntryCarriesNoRecord", wantRead: true},
		{name: "EntryReadFails", readErr: readBoom, wantRead: true, wantErr: readBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			read := false
			seam := boardRecordStoredOf(runRecord{project: tt.project}, func() (bool, error) {
				read = true
				return tt.stored, tt.readErr
			})
			got, err := seam()
			if got != tt.want || !errors.Is(err, tt.wantErr) || read != tt.wantRead {
				t.Errorf("boardRecordStoredOf(project %t)() = %t, %v with the entry read %t; want %t, %v with it read %t",
					tt.project, got, err, read, tt.want, tt.wantErr, tt.wantRead)
			}
		})
	}
}

// TestDecisionStoreVerb_TakesTheDecisionLock asserts a store holds the run's decision lock while it copies the record.
// A lock that cannot be taken refuses the call with the store's retry way forward and copies nothing.
func TestDecisionStoreVerb_TakesTheDecisionLock(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		lock   error
		wantOK bool
	}{
		{name: "LockTaken_ReleasedAfterTheCopy", wantOK: true},
		{name: "LockFails_RefusesWithTheRetryWay", lock: errors.New("take the run's decision lock: boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newReplaceFake(t)
			f.lockErr = tt.lock

			var out bytes.Buffer
			code := decisionStoreVerb(&out, "task-a", f.deps())
			if (code == 0) != tt.wantOK {
				t.Fatalf("exit = %d, out %s; want success %t", code, out.String(), tt.wantOK)
			}
			if tt.wantOK {
				if f.lockCalls != 1 || f.lockReleases != 1 || len(f.stored) != 1 {
					t.Errorf("lock taken %d and released %d times, stored %d records; want once each and one copy", f.lockCalls, f.lockReleases, len(f.stored))
				}
				return
			}
			msg := envelope.Decode(t, out.String()).Error
			for _, w := range []string{"decision lock: boom", decisionStoreRetry} {
				if !strings.Contains(msg, w) {
					t.Errorf("message %q lacks %q", msg, w)
				}
			}
			if len(f.stored) != 0 {
				t.Errorf("stored = %q; want nothing copied", f.stored)
			}
		})
	}
}

// TestDecisionCalls_RefusedOnAFiledRaddleRun asserts add, replace and store are refused on a raddle run once it filed.
// It holds in every mode and at any row, the board copy is left alone,
// and a filing the raddle store has not marked done names a resume first.
func TestDecisionCalls_RefusedOnAFiledRaddleRun(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	filings := []struct {
		name  string
		setup func(f *decisionFake)
		want  []string
	}{
		{name: "FamilyFilingMarkedDone", setup: func(f *decisionFake) {
			f.raddleFiled = []string{"part-a", "part-b"}
		}, want: []string{"has filed part-a, part-b", "family's project", `"lym loom decision add <part-slug>"`, `"lym board upsert <project-slug> --record-file <file>"`}},
		{name: "BoardHoldsTheFilingWithoutTheStoreMarker", setup: func(f *decisionFake) {
			f.boardFilingParts, f.boardFilingHeld = []string{"part-a"}, true
		}, want: []string{"has filed part-a", `run "lym loom resume" on this run first`, "--record-file"}},
		{name: "EntryNoLongerATask", setup: func(f *decisionFake) {
			f.entryNotTask, f.boardFilingParts = true, []string{"part-a"}
		}, want: []string{"has filed part-a", `"lym loom resume"`, "--record-file"}},
		{name: "PerchFilingAtALaterRow", setup: func(f *decisionFake) {
			f.raddlePerch, f.raddleFiled = true, []string{"part-a"}
		}, want: []string{"in perch mode has filed part-a", `"lym loom decision replace <part-slug>"`}},
	}
	for _, filing := range filings {
		for _, verb := range []string{"add", "replace", "store"} {
			t.Run(filing.name+"_"+verb, func(t *testing.T) {
				t.Parallel()
				f := newReplaceFake(t)
				f.raddleFileRow, f.boardStored = loomshed.NameRaddleFile, true
				f.status.CurrentProducer = "Finalize"
				f.pastRows = map[string]bool{"Finalize": true}
				filing.setup(f)

				var out bytes.Buffer
				var code int
				switch verb {
				case "add":
					code = decisionVerb(&out, "task-a", f.deps(), goodDecisionInput())
				case "replace":
					code = decisionReplaceVerb(&out, "task-a", f.deps(), replaceInputOf(t, replaceFoldedRecord))
				default:
					code = decisionStoreVerb(&out, "task-a", f.deps())
				}
				if code == 0 {
					t.Fatalf("%s exit = 0, out %s; want a refusal", verb, out.String())
				}
				msg := envelope.Decode(t, out.String()).Error
				for _, w := range filing.want {
					if !strings.Contains(msg, w) {
						t.Errorf("message %q lacks %q", msg, w)
					}
				}
				if len(f.stored) != 0 {
					t.Errorf("stored = %q; want the project's stored record left alone", f.stored)
				}
				if got := readDecisionRecord(t, f.path); got != replaceBaseRecord {
					t.Errorf("record = %q; want it unchanged", got)
				}
			})
		}
	}
}
