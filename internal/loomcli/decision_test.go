package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/samplerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const decisionPriorRecord = "# Record\n\n## Decisions\n\n### First\n\nDecision: a.\n"

// decisionFake is a decisionDeps over a temp record whose append writes the entry at the end of the file and whose outcomes the test sets.
type decisionFake struct {
	path      string
	status    shedengine.Status
	found     bool
	statusErr error
	findings  []discussionparser.Finding
	appendErr error
	commitErr error
	// programNameErr is what the LOOM program name resolver returns.
	programNameErr error
	// strandName is the caller's exported strand name, empty for an unnamed shell.
	strandName string
	parent     hubgeom.Parent
	parentErr  error
	// arbiterName is the one strand name the run's arbiter check accepts.
	arbiterName string
	// samplerName is the one strand name the run's sampler session check accepts, and samplerErr what that check returns in place of an answer.
	samplerName string
	samplerErr  error
	appends     int
	commits     int
	// lastBatch is every entry of the last append, and lastAdded its last entry.
	lastBatch []discussionparser.AddedDecision
	lastAdded discussionparser.AddedDecision
	clock     time.Time

	// noPlan makes the has-a-Plan-Write-row seam answer false, and recordRow makes the record-written-by-Record-Write seam answer true.
	noPlan    bool
	recordRow bool
	// recipeErr is the error both recipe seams return.
	recipeErr error
	// readRecordErr, when set, is what the record's working-copy read returns in place of the file.
	readRecordErr error
	// workingOverride, when set, is what the record's working-copy read returns in place of the file's bytes.
	workingOverride []byte
	// committedCopy, committedFound and committedErr are the record as committed at HEAD before the commit.
	committedCopy  []byte
	committedFound bool
	committedErr   error
	// committedAfterErr is the committed read's error once the commit has run.
	committedAfterErr error
	// historyVersions and historyErr are the record's committed versions.
	historyVersions [][]byte
	historyErr      error
	// darnCalls counts every call of a seam that reads the recipe or the Record-Write record.
	darnCalls int
	// dropEntryOnCommit makes the commit leave the committed copy without the appended entry.
	dropEntryOnCommit bool
	// commitLeaves, when set, are the bytes the commit leaves as the committed copy.
	commitLeaves []byte
	// liveStrands and liveErr are the answer of the live-run listing.
	liveStrands []string
	liveErr     error
	// notifyErrs maps a receiver to the error its post returns.
	notifyErrs map[string]error
	// posts records every post the verb made, receiver and line.
	posts []decisionPost
	// notifyFailAt, when positive, makes the notify call with that 1-based number fail, and notifyCalls counts every call.
	notifyFailAt int
	notifyCalls  int

	// amendmentPath is the plan amendment record the fake's seams write, in a temp dir.
	amendmentPath string
	// planApproved and planErr are the answer of the approved-plan seam.
	planApproved bool
	planErr      error
	// amendErr makes the amendment append fail before it writes.
	amendErr error
	// amendErrTitle, when set, makes the amendment append fail before it writes for that title alone.
	amendErrTitle string
	// appendClobbers makes a failing record append leave the decision record overwritten, as an append that dies midway would.
	appendClobbers bool
	// amendWrote is the amendment record's bytes the last append wrote.
	amendWrote []byte
	// amendCommittedCopy, amendCommittedFound and amendCommittedErr are the amendment record as committed at HEAD.
	amendCommittedCopy  []byte
	amendCommittedFound bool
	amendCommittedErr   error
	// statusCommitLanded makes HEAD hold the bytes the call wrote, as a status commit that landed them would.
	statusCommitLanded bool
	// amendCommitErr is what the amendment-only commit returns, and amendCommits counts its calls.
	amendCommitErr error
	amendCommits   int
}

// decisionPost is one notice the verb posted.
type decisionPost struct {
	receiver, line string
}

func newDecisionFake(t *testing.T) *decisionFake {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(path, []byte(decisionPriorRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	return &decisionFake{
		path:          path,
		clock:         time.Date(2026, 3, 3, 23, 30, 0, 0, time.FixedZone("late", -5*3600)),
		amendmentPath: filepath.Join(t.TempDir(), "amendment.yaml"),
	}
}

// newDarnDecisionFake is a fake of a run whose Record-Write row wrote the record, which is committed and unchanged since, with a history holding only that version.
func newDarnDecisionFake(t *testing.T) *decisionFake {
	t.Helper()
	f := newDecisionFake(t)
	f.recordRow = true
	f.committedCopy = []byte(decisionPriorRecord)
	f.committedFound = true
	f.historyVersions = [][]byte{[]byte(decisionPriorRecord)}
	return f
}

func (f *decisionFake) deps() decisionDeps {
	return decisionDeps{
		strandName:     f.strandName,
		programName:    func() (string, error) { return "hub:LOOM", f.programNameErr },
		parent:         func() (hubgeom.Parent, error) { return f.parent, f.parentErr },
		arbiterStrand:  func(name string) bool { return name == f.arbiterName },
		samplerStrand:  func(name string) (bool, error) { return f.samplerName != "" && name == f.samplerName, f.samplerErr },
		readStatus:     func() (shedengine.Status, bool, error) { return f.status, f.found, f.statusErr },
		recordPath:     f.path,
		recordUnderLym: "discussion/decision-record.md",
		append: func(ds []discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
			f.appends++
			f.lastBatch = ds
			f.lastAdded = ds[len(ds)-1]
			if f.appendErr != nil {
				if f.appendClobbers {
					if err := os.WriteFile(f.path, []byte("clobbered"), 0o644); err != nil {
						return nil, err
					}
				}
				return nil, f.appendErr
			}
			if f.findings != nil {
				return f.findings, nil
			}
			record, err := os.ReadFile(f.path)
			if err != nil {
				return nil, err
			}
			for _, d := range ds {
				record = fmt.Appendf(record, "\n### %s\n\nDecision: %s\n", addedDecisionHeading(d), d.Decision)
			}
			return nil, os.WriteFile(f.path, record, 0o644)
		},
		commit: func() error {
			f.commits++
			if f.commitErr != nil {
				return f.commitErr
			}
			switch {
			case f.commitLeaves != nil:
				f.committedCopy = f.commitLeaves
			case f.dropEntryOnCommit:
			default:
				data, err := os.ReadFile(f.path)
				if err != nil {
					return err
				}
				f.committedCopy = data
			}
			f.committedFound = true
			return nil
		},
		now: func() time.Time { return f.clock },
		hasPlan: func() (bool, error) {
			f.darnCalls++
			return !f.noPlan, f.recipeErr
		},
		rowWritten: func() (bool, error) {
			f.darnCalls++
			return f.recordRow, f.recipeErr
		},
		readRecord: func() ([]byte, error) {
			f.darnCalls++
			if f.readRecordErr != nil {
				return nil, f.readRecordErr
			}
			if f.workingOverride != nil {
				return f.workingOverride, nil
			}
			return os.ReadFile(f.path)
		},
		committed: func() ([]byte, bool, error) {
			f.darnCalls++
			if f.commits > 0 && f.committedAfterErr != nil {
				return nil, false, f.committedAfterErr
			}
			return f.committedCopy, f.committedFound, f.committedErr
		},
		history: func() ([][]byte, error) {
			f.darnCalls++
			return f.historyVersions, f.historyErr
		},
		liveStrands: func() ([]string, error) { return f.liveStrands, f.liveErr },
		notify: func(receiver, line string) error {
			f.notifyCalls++
			if f.notifyFailAt == f.notifyCalls {
				return fmt.Errorf("creel down on post %d", f.notifyCalls)
			}
			if err := f.notifyErrs[receiver]; err != nil {
				return err
			}
			f.posts = append(f.posts, decisionPost{receiver: receiver, line: line})
			return nil
		},
		amendmentPath: f.amendmentPath,
		approvedPlan:  func() (bool, error) { return f.planApproved, f.planErr },
		appendAmendment: func(title string, at time.Time) error {
			if f.amendErr != nil {
				return f.amendErr
			}
			if f.amendErrTitle != "" && title == f.amendErrTitle {
				return errors.New("disk boom")
			}
			if err := loomshed.AppendAmendment(f.amendmentPath, title, at); err != nil {
				return err
			}
			var err error
			f.amendWrote, err = os.ReadFile(f.amendmentPath)
			return err
		},
		committedAmendment: func() ([]byte, bool, error) {
			if f.statusCommitLanded {
				return f.amendWrote, true, nil
			}
			return f.amendCommittedCopy, f.amendCommittedFound, f.amendCommittedErr
		},
		commitAmendment: func() error {
			f.amendCommits++
			return f.amendCommitErr
		},
	}
}

// decisionBatchJSON is a --from file of two entries, the second with a PATTERN change.
const decisionBatchJSON = `[
	{"title": "Keep the cache", "decision": "Keep it per run.", "rationale": "A shared cache races."},
	{"title": "Drop the flag", "decision": "Drop --fast.", "rationale": "Nobody passes it.", "pattern_entry": "PATTERN-x", "pattern_action": "adds", "pattern_text": "New rule."}
]`

// writeDecisionBatch writes content as a --from file in a temp dir and returns its path.
func writeDecisionBatch(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decisions.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func goodDecisionInput() decisionInput {
	return decisionInput{by: "parent", title: "Keep the cache", decision: "Keep it per run.", rationale: "A shared cache races."}
}

func readDecisionRecord(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestDecisionVerb_AppendsAndCommits asserts an entry from an unnamed shell or from the run's recorded parent is appended after the prior record byte-identical and committed once, with no status file.
func TestDecisionVerb_AppendsAndCommits(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name        string
		by          string
		strandName  string
		parentName  string
		arbiterName string
		samplerName string
		// samplerErr makes the read of the run's sampler record fail.
		samplerErr error
		// recordRow runs the case on a Record-Write record that is committed and unchanged.
		recordRow bool
		// pattern sets the three PATTERN-change flags.
		pattern bool
		// programNameFails makes the LOOM program name resolver fail.
		programNameFails bool
	}{
		{name: "parent label with the PATTERN-change form", by: "parent", pattern: true},
		{name: "Record-Write record from an unnamed shell", by: "parent", recordRow: true},
		{name: "Record-Write record from the recorded parent", by: "parent", strandName: "hub:orch", parentName: "hub:orch", recordRow: true},
		{name: "Record-Write record with the PATTERN-change form", by: "parent", recordRow: true, pattern: true},
		{name: "parent label from an unnamed shell", by: "parent"},
		{name: "operator label from an unnamed shell", by: "operator"},
		{name: "program name unresolvable", by: "parent", programNameFails: true},
		{name: "recorded parent's own name", by: "parent", strandName: "hub:orch", parentName: "hub:orch"},
		{name: "slug-bearing recorded parent's own name", by: "parent", strandName: "hub:other:orch", parentName: "hub:other:orch"},
		{name: "the run's arbiter strand with the arbiter label", by: "arbiter", strandName: "hub:slug:arbiter", parentName: "hub:orch", arbiterName: "hub:slug:arbiter"},
		{name: "the run's sampler session with the operator label on a Record-Write record", by: "operator", strandName: "hub:slug:sampler", parentName: "hub:orch", samplerName: "hub:slug:sampler", recordRow: true},
		{name: "the recorded parent with an unreadable sampler record", by: "parent", strandName: "hub:orch", parentName: "hub:orch", samplerErr: errors.New("record boom")},
	}
	for _, tc := range tests {
		by := tc.by
		t.Run(tc.name, func(t *testing.T) {
			f := newDecisionFake(t)
			if tc.recordRow {
				f = newDarnDecisionFake(t)
			}
			f.strandName = tc.strandName
			f.arbiterName = tc.arbiterName
			f.samplerName = tc.samplerName
			f.samplerErr = tc.samplerErr
			if tc.programNameFails {
				f.programNameErr = errors.New("geometry boom")
			}
			f.parent = hubgeom.Parent{Name: tc.parentName}
			input := goodDecisionInput()
			input.by = by
			if tc.pattern {
				input.patternEntry, input.patternAction, input.patternText = "PATTERN-x", "adds", "New rule."
			}
			var out bytes.Buffer
			if code := decisionVerb(&out, "task-a", f.deps(), input); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}

			data := envelope.RequireOK(t, out.String()).Raw
			if tc.pattern {
				want := discussionparser.PatternChange{ID: "PATTERN-x", Action: "adds", Text: "New rule."}
				if got := f.lastAdded.PatternChange; got == nil || *got != want {
					t.Errorf("appended PatternChange = %v; want %+v", got, want)
				}
				if data["pattern_change"] != "- `PATTERN-x` — adds: New rule." {
					t.Errorf("envelope pattern_change = %v; want the bullet", data["pattern_change"])
				}
			} else {
				if f.lastAdded.PatternChange != nil {
					t.Errorf("appended PatternChange = %+v; want nil without the form", f.lastAdded.PatternChange)
				}
				if _, ok := data["pattern_change"]; ok {
					t.Errorf("envelope = %v; want no pattern_change without the form", data)
				}
			}
			wantHeading := "Added after Discussion (" + by + ", 2026-03-04): Keep the cache"
			if data["slug"] != "task-a" || data["record"] != f.path || data["heading"] != wantHeading {
				t.Fatalf("envelope = %v; want the slug, the record path and heading %q", data, wantHeading)
			}
			got := readDecisionRecord(t, f.path)
			if !strings.HasPrefix(got, decisionPriorRecord) || !strings.Contains(got, "### "+wantHeading) {
				t.Fatalf("record = %q; want the prior text byte-identical, then the entry", got)
			}
			if f.commits != 1 {
				t.Errorf("commits = %d; want exactly 1", f.commits)
			}
			if f.lastAdded.Date.Location() != time.UTC {
				t.Errorf("entry date zone = %v; want UTC", f.lastAdded.Date.Location())
			}
		})
	}
}

func TestDecisionVerb_Refusals(t *testing.T) {
	tiergate.Require(t, 0)
	type refusalCase struct {
		name string
		// from, when set, is the content of a --from file the call passes in place of the per-entry flags; setup runs after.
		from  string
		setup func(f *decisionFake, in *decisionInput)
		want  []string
		// notWant lists text the message must not contain.
		notWant []string
		// inputRefusal marks a refusal before the append: of the input, the caller or a Record-Write run's record.
		inputRefusal bool
		// noDarnReads marks a refusal that must come before any recipe or record seam is read.
		noDarnReads bool
		// afterCommit marks a refusal that follows the commit, so the record is no longer untouched and one commit has run.
		afterCommit bool
	}
	tests := []refusalCase{
		{
			name:    "no decision record",
			setup:   func(f *decisionFake, _ *decisionInput) { f.appendErr = fmt.Errorf("read: %w", fs.ErrNotExist) },
			want:    []string{"loom: decision add:", "no decision record", "way forward:", "lym loom start", "Discussion-Write has written it"},
			notWant: []string{"never has one"},
		},
		{
			name: "Record-Write record not committed at HEAD with the current row Record-Write",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = false
				f.found, f.status = true, shedengine.Status{CurrentProducer: loomshed.NameRecordWrite}
			},
			inputRefusal: true,
			want:         []string{"not committed at HEAD", "way forward:", `run "lym loom resume", which commits the record and support log`},
			notWant:      []string{"goto"},
		},
		{
			name: "Record-Write record not committed at HEAD with the current row Darn-Write",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = false
				f.found, f.status = true, shedengine.Status{CurrentProducer: loomshed.NameDarnWrite}
			},
			inputRefusal: true,
			want:         []string{"not committed at HEAD", "way forward:", `"lym loom goto --to Record-Write", then "lym loom resume"`, "without overwriting them"},
		},
		{
			name: "Record-Write record with a working copy differing by an appended entry whose commit failed",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyVersions = [][]byte{[]byte(decisionPriorRecord)}
				f.workingOverride = []byte(decisionPriorRecord + "\n### Added after Discussion (parent, 2026-03-01): Failed commit\n\nDecision: d.\n\nRationale: r.\n")
			},
			inputRefusal: true,
			want:         []string{"differs from its committed copy", "way forward:", `"lym fabric commit"`, "re-run the refused call alone, never the failed one", "git -C _lym checkout HEAD -- discussion/decision-record.md"},
		},
		{
			name: "Record-Write record with a working copy differing by an appended PATTERN bullet whose commit failed",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyVersions = [][]byte{[]byte(decisionPriorRecord)}
				f.workingOverride = []byte(decisionPriorRecord + "\n## PATTERN changes\n\n- `PATTERN-x` — adds: New rule.\n")
			},
			inputRefusal: true,
			want:         []string{"differs from its committed copy", "way forward:", `"lym fabric commit"`, "re-run the refused call alone, never the failed one"},
		},
		{
			name: "Record-Write record with a working copy differing otherwise",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord + "\nedited after the commit\n")
			},
			inputRefusal: true,
			want:         []string{"differs from its committed copy", "way forward:", "otherwise restore the record to its committed copy", "git -C _lym checkout HEAD -- discussion/decision-record.md"},
		},
		{
			name: "Record-Write record missing an entry its history held",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyVersions = [][]byte{[]byte(decisionPriorRecord + "\n### Added after Discussion (parent, 2026-03-01): Gone entry\n\nDecision: d.\n\nRationale: r.\n")}
			},
			inputRefusal: true,
			want:         []string{"Added after Discussion (parent, 2026-03-01): Gone entry", "way forward:", "restore the entry and commit the record"},
		},
		{
			name:         "recipe read failure",
			setup:        func(f *decisionFake, _ *decisionInput) { f.recipeErr = errors.New("seed boom") },
			inputRefusal: true,
			want:         []string{"read the run's recipe: seed boom", "way forward:", "nothing is appended"},
		},
		{
			name: "Record-Write record read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.readRecordErr = errors.New("read boom")
			},
			inputRefusal: true,
			want:         []string{"read boom", "way forward:", "nothing is appended"},
		},
		{
			name: "Record-Write committed copy read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedErr = errors.New("committed boom")
			},
			inputRefusal: true,
			want:         []string{"committed boom", "way forward:", "nothing is appended"},
		},
		{
			name: "Record-Write history read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyErr = errors.New("history boom")
			},
			inputRefusal: true,
			want:         []string{"history boom", "way forward:", "nothing is appended"},
		},
		{
			name: "Record-Write entry missing from the committed record after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.dropEntryOnCommit = true
			},
			afterCommit: true,
			want:        []string{"Added after Discussion (parent, 2026-03-04): Keep the cache", "way forward:", `re-run "lym loom decision add"`},
		},
		{
			name: "Record-Write committed record unparseable after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.commitLeaves = []byte("## Decisions\n\n### Added after Discussion (alice): t\n")
			},
			afterCommit: true,
			want:        []string{"could not be read", "way forward:", `re-run "lym loom decision add"`},
		},
		{
			name: "Record-Write committed read failure after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.committedAfterErr = errors.New("after boom")
			},
			afterCommit: true,
			want:        []string{"after boom", "way forward:", `re-run "lym loom decision add"`},
		},
		{
			name: "Discussion-Write running",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.status = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NameDiscussionWrite}
				f.found = true
			},
			want: []string{"loom: decision add:", "Discussion-Write is running", "way forward:", "message the Discussion-Write session"},
		},
		{
			name:         "empty title",
			setup:        func(_ *decisionFake, in *decisionInput) { in.title = "  " },
			inputRefusal: true,
			want:         []string{"--title is empty", "way forward:", "pass --title"},
		},
		{
			name:         "empty decision",
			setup:        func(_ *decisionFake, in *decisionInput) { in.decision = "" },
			inputRefusal: true,
			want:         []string{"--decision is empty", "way forward:", "pass --decision"},
		},
		{
			name:         "empty rationale",
			setup:        func(_ *decisionFake, in *decisionInput) { in.rationale = "" },
			inputRefusal: true,
			want:         []string{"--rationale is empty", "way forward:", "pass --rationale"},
		},
		{
			name:         "invalid by",
			setup:        func(_ *decisionFake, in *decisionInput) { in.by = "agent" },
			inputRefusal: true,
			want:         []string{`--by is "agent"`, "way forward:", "--by parent", "--by operator"},
		},
		{
			name:         "empty by",
			setup:        func(_ *decisionFake, in *decisionInput) { in.by = "" },
			inputRefusal: true,
			want:         []string{"--by is \"\"", "way forward:", "--by parent", "--by operator"},
		},
		{
			name: "named caller differing from the recorded parent",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.strandName = "hub:slug:plan"
				f.parent = hubgeom.Parent{Name: "hub:orch"}
			},
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{`"hub:slug:plan"`, "way forward:", "ask hub:orch to run the verb", "operator's own unnamed shell"},
		},
		{
			name:         "named caller in a run with no recorded parent",
			setup:        func(f *decisionFake, _ *decisionInput) { f.strandName = "hub:slug:plan" },
			inputRefusal: true,
			want:         []string{`"hub:slug:plan"`, "way forward:", "the operator's to record", "operator's own unnamed shell"},
		},
		{
			name: "named caller whose parent cannot be resolved",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.strandName = "hub:slug:plan"
				f.parentErr = errors.New("origin boom")
			},
			inputRefusal: true,
			want:         []string{`"hub:slug:plan"`, "way forward:", "could not be resolved (origin boom)", "operator's own unnamed shell"},
		},
		{
			name:         "arbiter label from the operator's unnamed shell",
			setup:        func(_ *decisionFake, in *decisionInput) { in.by = "arbiter" },
			inputRefusal: true,
			want:         []string{"not the run's arbiter", "way forward:", "--by parent", "--by operator"},
		},
		{
			name: "arbiter label from the run's parent",
			setup: func(f *decisionFake, in *decisionInput) {
				in.by = "arbiter"
				f.strandName = "hub:orch"
				f.parent = hubgeom.Parent{Name: "hub:orch"}
			},
			inputRefusal: true,
			want:         []string{`"hub:orch"`, "not the run's arbiter", "way forward:", "--by parent"},
		},
		{
			name: "arbiter label from another task strand",
			setup: func(f *decisionFake, in *decisionInput) {
				in.by = "arbiter"
				f.strandName = "hub:slug:plan"
				f.arbiterName = "hub:slug:arbiter"
			},
			inputRefusal: true,
			want:         []string{`"hub:slug:plan"`, "not the run's arbiter", "way forward:", "--by operator"},
		},
		{
			name: "arbiter strand passing the parent label",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.strandName = "hub:slug:arbiter"
				f.arbiterName = "hub:slug:arbiter"
				f.parent = hubgeom.Parent{Name: "hub:orch"}
			},
			inputRefusal: true,
			want:         []string{`"hub:slug:arbiter"`, "task session", "way forward:", "ask hub:orch to run the verb"},
		},
		{
			name: "sampler session passing the parent label",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.strandName = "hub:slug:sampler"
				f.samplerName = "hub:slug:sampler"
				f.parent = hubgeom.Parent{Name: "hub:orch"}
			},
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{`"hub:slug:sampler"`, "the run's sampler session", "way forward:", "--by operator"},
		},
		{
			name: "sampler session check failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.strandName = "hub:slug:sampler"
				f.samplerErr = errors.New("record boom")
			},
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"read the run's sampler session: record boom", "way forward:", "nothing is appended"},
		},
		{
			name: "check finding after the append",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.findings = []discussionparser.Finding{{Check: "sections", Detail: "missing ## Scope"}}
			},
			want: []string{"discussion check flags the record", "sections: missing ## Scope", "way forward:", "the record is restored"},
		},
		{
			name:         "pattern action without an entry",
			setup:        func(_ *decisionFake, in *decisionInput) { in.patternAction = "adds" },
			inputRefusal: true,
			want:         []string{"way forward:", "pass --pattern-entry and --pattern-action together", "--pattern-text for adds or changes"},
		},
		{
			name:         "pattern text without an entry",
			setup:        func(_ *decisionFake, in *decisionInput) { in.patternText = "New rule." },
			inputRefusal: true,
			want:         []string{"way forward:", "pass --pattern-entry and --pattern-action together"},
		},
		{
			name:         "pattern entry without an action",
			setup:        func(_ *decisionFake, in *decisionInput) { in.patternEntry = "PATTERN-x" },
			inputRefusal: true,
			want:         []string{"way forward:", "pass --pattern-entry and --pattern-action together"},
		},
		{
			name: "pattern-change finding after the append",
			setup: func(f *decisionFake, in *decisionInput) {
				in.patternEntry, in.patternAction = "PATTERN-x", "adds"
				f.findings = []discussionparser.Finding{{Check: "pattern-change-malformed", Detail: "line 12: adds needs its new text"}}
			},
			want: []string{"discussion check flags the record", "pattern-change-malformed: line 12", "way forward:", "the record is restored"},
		},
		{
			name: "no Decisions heading",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.appendErr = fmt.Errorf("%s: %w", f.path, discussionparser.ErrNoDecisionsHeading)
			},
			want: []string{"## Decisions", "way forward:", "restore the \"## Decisions\" heading"},
		},
		{
			name:  "status read failure",
			setup: func(f *decisionFake, _ *decisionInput) { f.statusErr = errors.New("boom") },
			want:  []string{"read the run status: boom", "way forward:", "re-run"},
		},
		{
			name:  "append I/O failure",
			setup: func(f *decisionFake, _ *decisionInput) { f.appendErr = errors.New("disk full") },
			want:  []string{"disk full", "way forward:", "re-run"},
		},
		{
			name:         "from beside a per-entry flag",
			from:         decisionBatchJSON,
			setup:        func(_ *decisionFake, in *decisionInput) { in.title = "Keep the cache" },
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"--from carries every entry", "way forward:", "drop --title"},
		},
		{
			name:         "from file unreadable",
			setup:        func(f *decisionFake, in *decisionInput) { *in = decisionInput{by: "parent", from: f.path + ".absent"} },
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"read the --from file", "way forward:", "JSON array"},
		},
		{
			name:         "from file not an array",
			from:         `{"title": "a", "decision": "d", "rationale": "r"}`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"is not one JSON array of entries", "way forward:", "JSON array"},
		},
		{
			name:         "from file with data after the array",
			from:         `[{"title": "a", "decision": "d", "rationale": "r"}] []`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"data follows the array", "way forward:"},
		},
		{
			name:         "from file with a stray closing bracket after the array",
			from:         `[{"title": "a", "decision": "d", "rationale": "r"}] ]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"data follows the array", "way forward:"},
		},
		{
			name:         "from entry with an unknown key",
			from:         `[{"title": "a", "decision": "d", "rationale": "r", "pattern_enty": "PATTERN-x"}]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"unknown field", "pattern_enty", "way forward:"},
		},
		{
			name:         "from file with no entry",
			from:         `[]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"holds no entry", "way forward:"},
		},
		{
			name:         "from entry with an empty field",
			from:         `[{"title": "a", "decision": "d", "rationale": "r"}, {"title": "b", "decision": " ", "rationale": "r"}]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"entry 2 of the --from file has an empty decision", "way forward:"},
		},
		{
			name:         "from entry with a partial PATTERN-change form",
			from:         `[{"title": "a", "decision": "d", "rationale": "r", "pattern_text": "x"}]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"entry 1 of the --from file", "one form", "way forward:", "set pattern_entry and pattern_action together"},
		},
		{
			name:         "from entries sharing a title",
			from:         `[{"title": "a", "decision": "d", "rationale": "r"}, {"title": " a ", "decision": "e", "rationale": "r"}]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{`entries 1 and 2 of the --from file share the title "a"`, "way forward:"},
		},
		{
			name: "from entries sharing a PATTERN entry",
			from: `[{"title": "a", "decision": "d", "rationale": "r", "pattern_entry": "PATTERN-x", "pattern_action": "removes"},
				{"title": "b", "decision": "e", "rationale": "r", "pattern_entry": "PATTERN-x", "pattern_action": "adds", "pattern_text": "t"}]`,
			inputRefusal: true,
			noDarnReads:  true,
			want:         []string{"entries 1 and 2 of the --from file both change PATTERN-x", "way forward:"},
		},
		{
			name:         "entry whose notice passes the line creel delivers",
			setup:        func(_ *decisionFake, in *decisionInput) { in.decision = strings.Repeat("d", 9000) },
			inputRefusal: true,
			want:         []string{"would pass the line creel delivers in one hook field", decisionWayNoticeTooLong},
		},
		{
			name: "Record-Write batch entry missing from the committed record after the commit",
			from: decisionBatchJSON,
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.commitLeaves = []byte(decisionPriorRecord + "\n### Added after Discussion (parent, 2026-03-04): Keep the cache\n\nDecision: d.\n")
			},
			afterCommit: true,
			want:        []string{"no entry under \"Added after Discussion (parent, 2026-03-04): Drop the flag\"", "way forward:", `re-run "lym loom decision add"`},
		},
	}
	missingRecord := []struct {
		name, row string
		committed bool
		want      []string
	}{
		{name: "no status file", want: []string{`run "lym loom start"`, "once the run passed Record-Write"}},
		{name: "Preflight", row: loomshed.NamePreflight, want: []string{`run "lym loom resume"`}},
		{name: "Record-Write", row: loomshed.NameRecordWrite, want: []string{`run "lym loom resume"`}},
		{name: "Drift-Baseline", row: "Drift-Baseline", want: []string{`"lym loom goto --to Record-Write", then "lym loom resume"`}},
		{name: "Drift-Await", row: "Drift-Await", want: []string{`"lym loom goto --to Record-Write", then "lym loom resume"`}},
		{name: "Darn-Write", row: loomshed.NameDarnWrite, want: []string{`"lym loom goto --to Record-Write", then "lym loom resume"`}},
		{name: "Darn-Write with a committed record at HEAD", row: loomshed.NameDarnWrite, committed: true, want: []string{"git -C _lym checkout HEAD -- discussion/decision-record.md"}},
	}
	for _, mr := range missingRecord {
		tests = append(tests, refusalCase{
			name: "Record-Write record missing with " + mr.name,
			setup: func(f *decisionFake, _ *decisionInput) {
				f.recordRow = true
				f.readRecordErr = fmt.Errorf("read: %w", fs.ErrNotExist)
				if mr.row != "" {
					f.found, f.status = true, shedengine.Status{CurrentProducer: mr.row}
				}
				f.committedFound = mr.committed
				f.committedCopy = []byte(decisionPriorRecord)
			},
			inputRefusal: true,
			want:         append([]string{"no decision record", "way forward:"}, mr.want...),
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newDecisionFake(t)
			input := goodDecisionInput()
			if tc.from != "" {
				input = decisionInput{by: "parent", from: writeDecisionBatch(t, tc.from)}
			}
			if tc.setup != nil {
				tc.setup(f, &input)
			}

			var out bytes.Buffer
			if code := decisionVerb(&out, "task-a", f.deps(), input); code == 0 {
				t.Fatalf("exit = 0, out %s; want a refusal", out.String())
			}
			msg := envelope.Decode(t, out.String()).Error
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("message %q lacks %q", msg, w)
				}
			}
			for _, w := range append([]string{"Darn-Write"}, tc.notWant...) {
				if strings.Contains(msg, w) {
					t.Errorf("message %q contains %q", msg, w)
				}
			}
			wantCommits := 0
			if tc.afterCommit {
				wantCommits = 1
			} else if got := readDecisionRecord(t, f.path); got != decisionPriorRecord {
				t.Errorf("record = %q; want it untouched", got)
			}
			if f.commits != wantCommits {
				t.Errorf("commits = %d; want %d", f.commits, wantCommits)
			}
			if tc.inputRefusal && f.appends != 0 {
				t.Errorf("appends = %d; want 0 for a refusal before the append", f.appends)
			}
			if tc.noDarnReads && f.darnCalls != 0 {
				t.Errorf("darn seam calls = %d; want 0 for a refusal before any darn read", f.darnCalls)
			}
			if len(f.posts) != 0 {
				t.Errorf("posts = %v; want none for a refusal", f.posts)
			}
		})
	}
}

// TestDecisionVerb_NotifiesLiveAgents asserts a committed entry is announced to each live agent of the run but the tackler, the arbiter and the caller,
// and that a failed listing or post is reported on a success envelope.
func TestDecisionVerb_NotifiesLiveAgents(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	// Two entries whose joined inline notice passes the line cap while each one-entry notice fits.
	oversizedBatch := fmt.Sprintf(`[{"title": "First", "decision": %q, "rationale": "r"}, {"title": "Second", "decision": %q, "rationale": "r"}]`, strings.Repeat("a", 4500), strings.Repeat("b", 4500))
	tests := []struct {
		name        string
		strandName  string
		arbiter     bool
		liveStrands []string
		liveErr     error
		notifyErrs  map[string]error
		// from, when set, is the content of a --from file the call passes in place of the per-entry flags; the posts then split per entry and wantPosts counts them.
		from      string
		failPost  int
		wantPosts int
		want      []string
		// wantErrors are the strands of the reported failures, "" for a failed listing.
		wantErrors []string
	}{
		{
			name:        "Orch_SkipsTacklerArbiterAndUnnamedRun",
			strandName:  "hub:orch",
			liveStrands: []string{"hub:task-a:darn", "hub:task-a:tackler-2", "hub:task-a:arbiter", "0f3a9c", "hub:task-a:plan"},
			want:        []string{"hub:task-a:darn", "hub:task-a:plan"},
		},
		{
			name:        "CallerIsLiveRun_SkipsItself",
			strandName:  "hub:task-a:lead",
			liveStrands: []string{"hub:task-a:lead", "hub:task-a:darn"},
			want:        []string{"hub:task-a:darn"},
		},
		{
			name:        "Arbiter_SkipsItself",
			strandName:  "hub:task-a:arbiter",
			arbiter:     true,
			liveStrands: []string{"hub:task-a:arbiter", "hub:task-a:darn"},
			want:        []string{"hub:task-a:darn"},
		},
		{
			name: "NoLiveRun_SendsNothing",
		},
		{
			name:       "ListingFails_ReportedNotRefused",
			liveErr:    errors.New("run dir unreadable"),
			wantErrors: []string{""},
		},
		{
			name:        "OnePostFails_OthersStillSent",
			liveStrands: []string{"hub:task-a:darn", "hub:task-a:plan"},
			notifyErrs:  map[string]error{"hub:task-a:darn": errors.New("creel down")},
			want:        []string{"hub:task-a:plan"},
			wantErrors:  []string{"hub:task-a:darn"},
		},
		{
			name:        "InlineBatchOverTheCap_SplitsIntoOneNoticePerEntry",
			liveStrands: []string{"hub:task-a:webster"},
			from:        oversizedBatch,
			want:        []string{"hub:task-a:webster"},
			wantPosts:   2,
		},
		{
			name:        "InlineBatchOverTheCap_SecondPostFails_NotReportedNotified",
			liveStrands: []string{"hub:task-a:webster"},
			from:        oversizedBatch,
			failPost:    2,
			wantErrors:  []string{"hub:task-a:webster"},
			wantPosts:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newDecisionFake(t)
			f.strandName = tt.strandName
			f.parent = hubgeom.Parent{Name: "hub:orch"}
			if tt.strandName != "" && !tt.arbiter {
				f.parent.Name = tt.strandName
			}
			f.liveStrands, f.liveErr, f.notifyErrs = tt.liveStrands, tt.liveErr, tt.notifyErrs
			input := goodDecisionInput()
			if tt.arbiter {
				f.arbiterName = tt.strandName
				input.by = "arbiter"
			}
			if tt.from != "" {
				input = decisionInput{by: "parent", from: writeDecisionBatch(t, tt.from)}
			}
			f.notifyFailAt = tt.failPost
			var out bytes.Buffer
			if code := decisionVerb(&out, "task-a", f.deps(), input); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}
			if f.commits != 1 {
				t.Errorf("commits = %d; want 1", f.commits)
			}

			data := envelope.RequireOK(t, out.String()).Raw
			if got := fmt.Sprint(data["notified"]); got != fmt.Sprint(tt.want) {
				t.Errorf("envelope notified = %s; want %v", got, tt.want)
			}
			gotErrors := []string{}
			for _, item := range data["notice_errors"].([]any) {
				entry := item.(map[string]any)
				if entry["error"] == "" {
					t.Errorf("notice error %v; want its error text", entry)
				}
				gotErrors = append(gotErrors, entry["strand"].(string))
			}
			if fmt.Sprint(gotErrors) != fmt.Sprint(append([]string{}, tt.wantErrors...)) {
				t.Errorf("envelope notice_errors strands = %v; want %v", gotErrors, tt.wantErrors)
			}

			if tt.from != "" {
				if len(f.posts) != tt.wantPosts {
					t.Fatalf("posts = %v; want %d, one per entry in entry order", f.posts, tt.wantPosts)
				}
				for i, post := range f.posts {
					if !strings.HasPrefix(post.line, decisionNoticeOpening) || !strings.Contains(post.line, addedDecisionHeading(f.lastBatch[i])) {
						t.Errorf("post %d line = %q; want the opening and entry %d's heading", i, post.line, i+1)
					}
					if other := addedDecisionHeading(f.lastBatch[1-i]); strings.Contains(post.line, other) {
						t.Errorf("post %d line = %q; want it to carry only entry %d", i, post.line, i+1)
					}
				}
				if tt.failPost != 0 {
					if got := data["notice_errors"].([]any)[0].(map[string]any)["error"]; got != fmt.Sprintf("creel down on post %d", tt.failPost) {
						t.Errorf("notice error = %v; want the failed post's error", got)
					}
				}
				return
			}
			if len(f.posts) != len(tt.want) {
				t.Fatalf("posts = %v; want one to each of %v", f.posts, tt.want)
			}
			heading := addedDecisionHeading(f.lastAdded)
			for i, post := range f.posts {
				if post.receiver != tt.want[i] {
					t.Errorf("post %d receiver = %q; want %q", i, post.receiver, tt.want[i])
				}
				if !strings.HasPrefix(post.line, decisionNoticeOpening) {
					t.Errorf("post line = %q; want it to open with %q", post.line, decisionNoticeOpening)
				}
				for _, want := range []string{f.path, heading, input.decision, input.rationale} {
					if !strings.Contains(post.line, want) {
						t.Errorf("post line = %q; want it to carry %q", post.line, want)
					}
				}
			}
		})
	}
}

// TestDecisionVerb_FromFileRecordsBatch asserts a --from batch is appended in one write, in file order, with its PATTERN change.
// The batch is committed once with every title in the amendment record.
// Each live agent gets one notice: a short one naming every heading, and Merriam's carrying every entry's text.
func TestDecisionVerb_FromFileRecordsBatch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	f := newDecisionFake(t)
	f.planApproved = true
	f.liveStrands = []string{"hub:task-a:plan", "hub:task-a:webster"}
	input := decisionInput{by: "parent", from: writeDecisionBatch(t, decisionBatchJSON), amendsPlan: true}

	var out bytes.Buffer
	if code := decisionVerb(&out, "task-a", f.deps(), input); code != 0 {
		t.Fatalf("exit = %d, out %s; want 0", code, out.String())
	}
	if f.appends != 1 || f.commits != 1 {
		t.Errorf("appends, commits = %d, %d; want 1, 1", f.appends, f.commits)
	}
	wantPattern := discussionparser.PatternChange{ID: "PATTERN-x", Action: "adds", Text: "New rule."}
	if len(f.lastBatch) != 2 || f.lastBatch[0].Title != "Keep the cache" || f.lastBatch[0].PatternChange != nil ||
		f.lastBatch[1].Title != "Drop the flag" || f.lastBatch[1].PatternChange == nil || *f.lastBatch[1].PatternChange != wantPattern {
		t.Errorf("appended batch = %+v; want both entries in file order, the second with %+v", f.lastBatch, wantPattern)
	}
	headings := []string{"Added after Discussion (parent, 2026-03-04): Keep the cache", "Added after Discussion (parent, 2026-03-04): Drop the flag"}

	data := envelope.RequireOK(t, out.String()).Raw
	if got, want := fmt.Sprint(data["headings"]), fmt.Sprint(headings); got != want {
		t.Errorf("envelope headings = %s; want %s", got, want)
	}
	if got := fmt.Sprint(data["pattern_changes"]); got != "[- `PATTERN-x` — adds: New rule.]" {
		t.Errorf("envelope pattern_changes = %s; want the second entry's bullet", got)
	}
	for _, key := range []string{"heading", "pattern_change"} {
		if _, ok := data[key]; ok {
			t.Errorf("envelope = %v; want no %s for a batch", data, key)
		}
	}
	amendment, _, err := loomshed.ReadAmendment(f.amendmentPath)
	if err != nil || !reflect.DeepEqual(amendment.Titles, []string{"Keep the cache", "Drop the flag"}) {
		t.Errorf("amendment titles = %v, %v; want both titles in order", amendment.Titles, err)
	}

	if len(f.posts) != 2 {
		t.Fatalf("posts = %v; want one to each live agent", f.posts)
	}
	texts := []string{"Keep it per run.", "A shared cache races.", "Drop --fast.", "Nobody passes it."}
	for _, post := range f.posts {
		if !strings.HasPrefix(post.line, decisionNoticeOpening) || !strings.Contains(post.line, f.path) {
			t.Errorf("post line = %q; want the opening and the record path", post.line)
		}
		for _, heading := range headings {
			if !strings.Contains(post.line, heading) {
				t.Errorf("post line to %s = %q; want heading %q", post.receiver, post.line, heading)
			}
		}
		inline := post.receiver == "hub:task-a:webster"
		for _, text := range texts {
			if strings.Contains(post.line, text) != inline {
				t.Errorf("post line to %s = %q; carrying %q is %v, want %v", post.receiver, post.line, text, !inline, inline)
			}
		}
	}
}

// TestDecisionNoticeOpening_NamedByParentDirective asserts the rendered parent directive quotes the opening every decision notice starts with.
// That quote is what makes a spawned role admit the notice as its parent's.
func TestDecisionNoticeOpening_NamedByParentDirective(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	directive, err := parentdirective.Directive(stencilkit.Seed(t), "hub:orch", false)
	if err != nil {
		t.Fatalf("Directive = _, %v; want nil error", err)
	}
	if !strings.Contains(directive, "`"+decisionNoticeOpening+"`") {
		t.Errorf("Directive = %q; want it to quote the decision notice's opening %q", directive, decisionNoticeOpening)
	}
}

// recordingNoticePoster is a noticePoster that records its kind on every post and answers with err.
type recordingNoticePoster struct {
	kind  string
	err   error
	posts *[]string
}

func (p recordingNoticePoster) Post(receiver, body string) (int64, error) {
	*p.posts = append(*p.posts, p.kind)
	return 0, p.err
}

func TestPostDecisionNotice_SignsByCaller(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		caller    string
		strandErr error
		want      []string
		wantErr   bool
	}{
		{name: "Orch_PostsAsProgram", caller: "hub:orch", want: []string{"program"}},
		{name: "Unnamed_PostsAsProgram", caller: "", want: []string{"program"}},
		{name: "Unparsable_PostsAsProgram", caller: "0f3a9c", want: []string{"program"}},
		{name: "Arbiter_PostsAsStrand", caller: "hub:task-a:arbiter", want: []string{"strand"}},
		{
			name:      "StrandOutsidePane_FallsBackToProgramOnce",
			caller:    "hub:task-a:arbiter",
			strandErr: fmt.Errorf("refused: %w", creelengine.ErrStrandClaimOutsidePane),
			want:      []string{"strand", "program"},
		},
		{
			name:      "StrandOtherFailure_IsNotRetried",
			caller:    "hub:task-a:arbiter",
			strandErr: errors.New("creel down"),
			want:      []string{"strand"},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var posts []string
			clients := noticeClients{
				strand: func(string) (noticePoster, error) {
					return recordingNoticePoster{kind: "strand", err: tt.strandErr, posts: &posts}, nil
				},
				program: func() (noticePoster, error) {
					return recordingNoticePoster{kind: "program", posts: &posts}, nil
				},
			}
			err := postDecisionNotice(clients, tt.caller, "hub:task-a:darn", "line")
			if (err != nil) != tt.wantErr {
				t.Errorf("postDecisionNotice() error = %v; want error %t", err, tt.wantErr)
			}
			if fmt.Sprint(posts) != fmt.Sprint(tt.want) {
				t.Errorf("postDecisionNotice() posted as %v; want %v", posts, tt.want)
			}
		})
	}
}

func TestDecisionVerb_CommitFailureNamesWayForward(t *testing.T) {
	tiergate.Require(t, 0)
	f := newDecisionFake(t)
	f.commitErr = errors.New("index locked")
	var out bytes.Buffer
	if code := decisionVerb(&out, "task-a", f.deps(), goodDecisionInput()); code == 0 {
		t.Fatalf("exit = 0; want a refusal when the commit fails")
	}
	msg := envelope.Decode(t, out.String()).Error
	if !strings.Contains(msg, "index locked") || !strings.Contains(msg, "way forward:") || !strings.Contains(msg, "a session lym refuses the verb from reports status: FAILED and the orch runs it") {
		t.Errorf("message %q; want the commit error and a way forward naming the orch", msg)
	}
}

// decisionEntryRecord is the decision record holding the entry the fake appends for goodDecisionInput on the fake's clock.
const decisionEntryRecord = decisionPriorRecord + "\n### Added after Discussion (parent, 2026-03-04): Keep the cache\n\nDecision: Keep it per run.\n"

// seedAmendment writes an amendment record holding titles at the fake's amendment path, as earlier --amends-plan calls would have left it.
func seedAmendment(t *testing.T, f *decisionFake, titles ...string) {
	t.Helper()
	for _, title := range titles {
		if err := loomshed.AppendAmendment(f.amendmentPath, title, f.clock.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
}

// amendmentBytesAfterCall is the amendment record's bytes a call on the fake's clock leaves holding titles.
func amendmentBytesAfterCall(t *testing.T, f *decisionFake, titles ...string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "amendment.yaml")
	for _, title := range titles {
		if err := loomshed.AppendAmendment(path, title, f.clock); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestDecisionVerb_AmendsPlan asserts a call with --amends-plan records the entry and the amendment title together.
// It refuses where there is no plan to amend, and leaves both records as they were on every failed append or commit that did not land.
// It also asserts the approved-plan reader's answers over a plan directory.
func TestDecisionVerb_AmendsPlan(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const title = "Keep the cache"
	tests := []struct {
		name string
		// from, when set, is the content of a --from file the call passes in place of the per-entry flags.
		from  string
		setup func(t *testing.T, f *decisionFake)
		// want lists the fragments of a refusal; empty means the call succeeds.
		want    []string
		notWant []string
		// wantTitles are the amendment record's titles afterwards, nil when no record exists.
		wantTitles []string
		// entry reports whether the decision record holds the entry afterwards; decisionAbsent that it does not exist.
		entry          bool
		decisionAbsent bool
		// wantCommits and wantAmendCommits are the record commits and the amendment-only commits.
		wantCommits      int
		wantAmendCommits int
		wantCommitError  bool
	}{
		{
			name:       "ApprovedPlan_LandsEntryAndTitleInOneCommit",
			wantTitles: []string{title}, entry: true, wantCommits: 1,
		},
		{
			name:       "SecondCall_AppendsItsTitle",
			setup:      func(t *testing.T, f *decisionFake) { seedAmendment(t, f, "First") },
			wantTitles: []string{"First", title}, entry: true, wantCommits: 1,
		},
		{
			name:  "NoPlanWriteRow_Refused",
			setup: func(_ *testing.T, f *decisionFake) { f.noPlan = true },
			want:  []string{"--amends-plan has no plan to amend on this run's recipe, which has no Plan-Write row", "way forward:", "without --amends-plan"},
		},
		{
			name:  "UnapprovedPlan_Refused",
			setup: func(_ *testing.T, f *decisionFake) { f.planApproved = false },
			want:  []string{"no approved plan to amend", "way forward:", "without --amends-plan", "Plan-Write and Plan-Review read the decision record"},
		},
		{
			name:  "PlanReadError_RefusedWithRetry",
			setup: func(_ *testing.T, f *decisionFake) { f.planErr = errors.New("stat boom") },
			want:  []string{"read the plan: stat boom", "way forward:", "nothing is appended"},
		},
		{
			name:  "PlanUnparsed_RefusedNamingValidatePlan",
			setup: func(_ *testing.T, f *decisionFake) { f.planErr = fmt.Errorf("%w: bad frontmatter", errPlanUnparsed) },
			want:  []string{"bad frontmatter", "way forward:", "lym loom validate-plan", "without --amends-plan"},
		},
		{
			name: "AbsentDecisionRecord_RefusedLeavingBothAbsent",
			setup: func(t *testing.T, f *decisionFake) {
				if err := os.Remove(f.path); err != nil {
					t.Fatal(err)
				}
				f.appendErr = fmt.Errorf("read: %w", fs.ErrNotExist)
			},
			want:           []string{"no decision record", "way forward:", "lym loom start"},
			decisionAbsent: true,
		},
		{
			name: "AmendmentAppendFails_RefusedLeavingBothRecords",
			setup: func(t *testing.T, f *decisionFake) {
				seedAmendment(t, f, "First")
				f.amendErr = errors.New("disk boom")
			},
			want:       []string{"write the amendment record: disk boom", "way forward:", "nothing is appended"},
			wantTitles: []string{"First"},
		},
		{
			name: "BatchSecondTitleFails_RestoresTheAmendmentRecord",
			from: decisionBatchJSON,
			setup: func(t *testing.T, f *decisionFake) {
				seedAmendment(t, f, "First")
				f.amendErrTitle = "Drop the flag"
			},
			want:       []string{"write the amendment record: disk boom", "way forward:", "nothing is appended"},
			wantTitles: []string{"First"},
		},
		{
			name: "CheckFindings_KeepsTheFirstTitle",
			setup: func(t *testing.T, f *decisionFake) {
				seedAmendment(t, f, "First")
				f.findings = []discussionparser.Finding{{Check: "sections", Detail: "missing ## Scope"}}
			},
			want:       []string{"discussion check flags the record", "sections: missing ## Scope", "the record is restored"},
			wantTitles: []string{"First"},
		},
		{
			name: "AppendError_RestoresBothRecords",
			setup: func(t *testing.T, f *decisionFake) {
				seedAmendment(t, f, "First")
				f.appendErr, f.appendClobbers = errors.New("disk full"), true
			},
			want:       []string{"disk full", "way forward:", "nothing is appended"},
			wantTitles: []string{"First"},
		},
		{
			name: "FindingsAfterStatusCommit_CommitsTheRestoredRecord",
			setup: func(_ *testing.T, f *decisionFake) {
				f.statusCommitLanded = true
				f.findings = []discussionparser.Finding{{Check: "sections", Detail: "missing ## Scope"}}
			},
			want:             []string{"discussion check flags the record"},
			wantAmendCommits: 1,
		},
		{
			name: "FindingsAfterStatusCommit_CommitFailureNamesFabricCommit",
			setup: func(_ *testing.T, f *decisionFake) {
				f.statusCommitLanded = true
				f.amendCommitErr = errors.New("index locked")
				f.findings = []discussionparser.Finding{{Check: "sections", Detail: "missing ## Scope"}}
			},
			want:             []string{"index locked", "way forward:", "lym fabric commit"},
			wantAmendCommits: 1,
		},
		{
			name: "CommitErrorWithHeadHoldingBoth_SucceedsWithCommitError",
			setup: func(t *testing.T, f *decisionFake) {
				f.commitErr = errors.New("index locked")
				f.committedCopy, f.committedFound = []byte(decisionEntryRecord), true
				f.amendCommittedCopy, f.amendCommittedFound = amendmentBytesAfterCall(t, f, title), true
			},
			wantTitles: []string{title}, entry: true, wantCommits: 1, wantCommitError: true,
		},
		{
			name: "CommitErrorWithHeadHoldingRecordAlone_KeepsBothAndNamesFabricCommit",
			setup: func(t *testing.T, f *decisionFake) {
				f.commitErr = errors.New("index locked")
				f.committedCopy, f.committedFound = []byte(decisionPriorRecord), true
				f.amendCommittedCopy, f.amendCommittedFound = amendmentBytesAfterCall(t, f, title), true
			},
			want:       []string{"index locked", "without the entry", "way forward:", "lym fabric commit"},
			wantTitles: []string{title}, entry: true, wantCommits: 1,
		},
		{
			name: "CommitErrorWithHeadDiffering_RestoresBothAndNamesTheRerun",
			setup: func(t *testing.T, f *decisionFake) {
				seedAmendment(t, f, "First")
				f.commitErr = errors.New("index locked")
				f.amendCommittedCopy, f.amendCommittedFound = amendmentBytesAfterCall(t, f, "First"), true
			},
			want:       []string{"index locked", "way forward:", "re-run \"lym loom decision add ... --amends-plan\""},
			notWant:    []string{"lym fabric commit"},
			wantTitles: []string{"First"}, wantCommits: 1,
		},
		{
			name: "CommitErrorWithUnreadableHead_KeepsBothAndNamesFabricCommit",
			setup: func(_ *testing.T, f *decisionFake) {
				f.commitErr = errors.New("index locked")
				f.amendCommittedErr = errors.New("head boom")
			},
			want:       []string{"index locked", "head boom", "way forward:", "lym fabric commit"},
			wantTitles: []string{title}, entry: true, wantCommits: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newDecisionFake(t)
			f.planApproved = true
			if tt.setup != nil {
				tt.setup(t, f)
			}
			input := goodDecisionInput()
			if tt.from != "" {
				input = decisionInput{by: "parent", from: writeDecisionBatch(t, tt.from)}
			}
			input.amendsPlan = true

			var out bytes.Buffer
			code := decisionVerb(&out, "task-a", f.deps(), input)
			if len(tt.want) == 0 {
				if code != 0 {
					t.Fatalf("exit = %d, out %s; want 0", code, out.String())
				}
				_, hasCommitError := envelope.RequireOK(t, out.String()).Raw["commit_error"]
				if hasCommitError != tt.wantCommitError {
					t.Errorf("envelope has commit_error = %t; want %t", hasCommitError, tt.wantCommitError)
				}
			} else {
				if code == 0 {
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
			}

			record, found, err := loomshed.ReadAmendment(f.amendmentPath)
			if err != nil {
				t.Fatal(err)
			}
			if found != (tt.wantTitles != nil) || !reflect.DeepEqual(record.Titles, tt.wantTitles) {
				t.Errorf("amendment record titles = %v (found %t); want %v", record.Titles, found, tt.wantTitles)
			}
			switch data, err := os.ReadFile(f.path); {
			case tt.decisionAbsent:
				if !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("decision record read error = %v; want it absent", err)
				}
			case err != nil:
				t.Fatal(err)
			case tt.entry:
				if !strings.HasPrefix(string(data), decisionPriorRecord) || !strings.Contains(string(data), "### Added after Discussion (parent, 2026-03-04): Keep the cache") {
					t.Errorf("decision record = %q; want the prior text and the entry", data)
				}
			default:
				if string(data) != decisionPriorRecord {
					t.Errorf("decision record = %q; want it as before", data)
				}
			}
			if f.commits != tt.wantCommits || f.amendCommits != tt.wantAmendCommits {
				t.Errorf("record commits = %d, amendment commits = %d; want %d and %d", f.commits, f.amendCommits, tt.wantCommits, tt.wantAmendCommits)
			}
		})
	}

	readerTests := []struct {
		name    string
		write   func(t *testing.T, anchor string)
		want    bool
		wantErr error
	}{
		{name: "AbsentOverview_AnswersFalse", write: func(*testing.T, string) {}},
		{
			name:  "ApprovedPlan_AnswersTrue",
			write: func(t *testing.T, anchor string) { writeDecisionPlan(t, anchor, true) },
			want:  true,
		},
		{
			name:  "UnapprovedPlan_AnswersFalse",
			write: func(t *testing.T, anchor string) { writeDecisionPlan(t, anchor, false) },
		},
		{
			name: "PlanThatDoesNotParse_WrapsErrPlanUnparsed",
			write: func(t *testing.T, anchor string) {
				plankit.WriteTree(t, anchor, map[string]string{"_lym/plan/00-overview.md": "not a plan\n"})
			},
			wantErr: errPlanUnparsed,
		},
	}
	for _, tt := range readerTests {
		t.Run("PlanApprovedReader_"+tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			tt.write(t, anchor)
			got, err := planApprovedReader(anchor)()
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("planApprovedReader() = %t, %v; want %t, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

// writeDecisionPlan writes a one-card plan under the plan directory of the worktree anchored at anchor.
func writeDecisionPlan(t *testing.T, anchor string, approved bool) {
	t.Helper()
	plankit.Write(t, planparser.PlanDir(anchor), plankit.Plan{
		Approved: approved,
		Language: "none",
		Cards: []plankit.Card{{
			Number:        1,
			Slug:          "first-card",
			Summary:       "the first card",
			Groups:        []plankit.Group{{Label: "Edit", Targets: []string{"docs/a.md"}}},
			Intent:        "change the doc.",
			ImpactSummary: "none",
		}},
	})
}

// TestSamplerStrandReader pins which strand the sampler session check admits: the one the Sampler-Session row recorded, while the run stands at that row, running or halted, and not a darn run's.
func TestSamplerStrandReader(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const sampler = "hub:slug:sampler"
	type ask struct {
		name string
		want bool
	}
	tests := []struct {
		name   string
		recipe string
		// status is the run's status file, absent when nil, and record the sampler record, absent when nil.
		status *shedengine.Status
		record *samplerengine.Record
		// rerecord, when set, replaces the record's strand name before the asks, as a resume under a new strand does.
		rerecord string
		asks     []ask
	}{
		{name: "the recorded strand at a running session row is admitted", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameSamplerSession, State: shedengine.StateRunning},
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{sampler, true}}},
		{name: "the recorded strand at a blocked session row stays admitted", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameSamplerSession, State: shedengine.StateBlocked},
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{sampler, true}}},
		{name: "the strand is refused once the current row is Code-Bouncer", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameCodeBouncer, State: shedengine.StateRunning},
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{sampler, false}}},
		{name: "a different name is refused", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameSamplerSession, State: shedengine.StateRunning},
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{"hub:slug:plan", false}}},
		{name: "a new recorded strand is admitted and the old one is refused", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameSamplerSession, State: shedengine.StateRunning},
			record: &samplerengine.Record{StrandName: sampler}, rerecord: "hub:slug:sampler-2", asks: []ask{{"hub:slug:sampler-2", true}, {sampler, false}}},
		{name: "an absent record admits nobody", recipe: shedrun.RecipeSampler,
			status: &shedengine.Status{CurrentProducer: loomshed.NameSamplerSession, State: shedengine.StateRunning}, asks: []ask{{sampler, false}}},
		{name: "an absent status file admits nobody", recipe: shedrun.RecipeSampler,
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{sampler, false}}},
		{name: "a darn run admits nobody", recipe: shedrun.RecipeDarn,
			status: &shedengine.Status{CurrentProducer: loomshed.NameDarnWrite, State: shedengine.StateRunning},
			record: &samplerengine.Record{StrandName: sampler}, asks: []ask{{sampler, false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			if tt.status != nil {
				statusPath, statusLock := shedrun.StatusFile(loc, shedrun.SelfRunID), shedrun.StatusLock(loc, shedrun.SelfRunID)
				if err := os.MkdirAll(filepath.Dir(statusLock), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := state.WriteJSON(statusPath, statusLock, *tt.status); err != nil {
					t.Fatalf("state.WriteJSON() = %v; want nil", err)
				}
			}
			recordPath, recordLock := shedrun.SamplerRecord(loc, shedrun.SelfRunID), shedrun.SamplerRecordLock(loc, shedrun.SelfRunID)
			if tt.record != nil {
				seed := *tt.record
				if err := samplerengine.UpdateRecord(recordPath, recordLock, func(stored *samplerengine.Record) { *stored = seed }); err != nil {
					t.Fatalf("seed the record: %v", err)
				}
			}
			if tt.rerecord != "" {
				if err := samplerengine.UpdateRecord(recordPath, recordLock, func(stored *samplerengine.Record) { stored.StrandName = tt.rerecord }); err != nil {
					t.Fatalf("rewrite the record: %v", err)
				}
			}

			admits := samplerStrandReader(loc, tt.recipe)
			for _, a := range tt.asks {
				if got, err := admits(a.name); err != nil || got != a.want {
					t.Errorf("samplerStrandReader(%q)(%q) = %v, %v; want %v, nil", tt.recipe, a.name, got, err, a.want)
				}
			}
		})
	}
}
