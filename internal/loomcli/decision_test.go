package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
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
	// strandName is the caller's exported strand name, empty for an unnamed shell.
	strandName string
	parent     hubgeom.Parent
	parentErr  error
	// arbiterName is the one strand name the run's arbiter check accepts.
	arbiterName string
	appends     int
	commits     int
	lastAdded   discussionparser.AddedDecision
	clock       time.Time

	// darn and darnErr are the answer of the run-is-darn seam.
	darn    bool
	darnErr error
	// readRecordErr, when set, is what the record's working-copy read returns in place of the file.
	readRecordErr error
	// committedCopy, committedFound and committedErr are the record as committed at HEAD before the commit.
	committedCopy  []byte
	committedFound bool
	committedErr   error
	// committedAfterErr is the committed read's error once the commit has run.
	committedAfterErr error
	// historyVersions and historyErr are the record's committed versions.
	historyVersions [][]byte
	historyErr      error
	// darnCalls counts every call of a darn seam.
	darnCalls int
	// dropEntryOnCommit makes the commit leave the committed copy without the appended entry.
	dropEntryOnCommit bool
	// commitLeaves, when set, are the bytes the commit leaves as the committed copy.
	commitLeaves []byte
}

func newDecisionFake(t *testing.T) *decisionFake {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(path, []byte(decisionPriorRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	return &decisionFake{path: path, clock: time.Date(2026, 3, 3, 23, 30, 0, 0, time.FixedZone("late", -5*3600))}
}

// newDarnDecisionFake is a fake of a darn run whose record is committed and unchanged since, with a history holding only that version.
func newDarnDecisionFake(t *testing.T) *decisionFake {
	t.Helper()
	f := newDecisionFake(t)
	f.darn = true
	f.committedCopy = []byte(decisionPriorRecord)
	f.committedFound = true
	f.historyVersions = [][]byte{[]byte(decisionPriorRecord)}
	return f
}

func (f *decisionFake) deps() decisionDeps {
	return decisionDeps{
		strandName:    f.strandName,
		parent:        func() (hubgeom.Parent, error) { return f.parent, f.parentErr },
		arbiterStrand: func(name string) bool { return name == f.arbiterName },
		readStatus:    func() (shedengine.Status, bool, error) { return f.status, f.found, f.statusErr },
		recordPath:    f.path,
		append: func(d discussionparser.AddedDecision) ([]discussionparser.Finding, error) {
			f.appends++
			f.lastAdded = d
			if f.appendErr != nil {
				return nil, f.appendErr
			}
			if f.findings != nil {
				return f.findings, nil
			}
			prior, err := os.ReadFile(f.path)
			if err != nil {
				return nil, err
			}
			entry := fmt.Sprintf("\n### %s\n\nDecision: %s\n", addedDecisionHeading(d), d.Decision)
			return nil, os.WriteFile(f.path, append(prior, entry...), 0o644)
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
		darn: func() (bool, error) {
			f.darnCalls++
			return f.darn, f.darnErr
		},
		readRecord: func() ([]byte, error) {
			f.darnCalls++
			if f.readRecordErr != nil {
				return nil, f.readRecordErr
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
	}
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
	tests := []struct {
		name        string
		by          string
		strandName  string
		parentName  string
		arbiterName string
		// darn runs the case on a darn run whose record is committed and unchanged; running also sets the status running at Darn-Write.
		darn    bool
		running bool
		// pattern sets the three PATTERN-change flags.
		pattern bool
	}{
		{name: "parent label with the PATTERN-change form", by: "parent", pattern: true},
		{name: "darn run from an unnamed shell", by: "parent", darn: true},
		{name: "darn run from the recorded parent", by: "parent", strandName: "hub:orch", parentName: "hub:orch", darn: true},
		{name: "darn run while Darn-Write runs", by: "parent", darn: true, running: true},
		{name: "parent label from an unnamed shell", by: "parent"},
		{name: "operator label from an unnamed shell", by: "operator"},
		{name: "recorded parent's own name", by: "parent", strandName: "hub:orch", parentName: "hub:orch"},
		{name: "slug-bearing recorded parent's own name", by: "parent", strandName: "hub:other:orch", parentName: "hub:other:orch"},
		{name: "the run's arbiter strand with the arbiter label", by: "arbiter", strandName: "hub:slug:arbiter", parentName: "hub:orch", arbiterName: "hub:slug:arbiter"},
	}
	for _, tc := range tests {
		by := tc.by
		t.Run(tc.name, func(t *testing.T) {
			f := newDecisionFake(t)
			if tc.darn {
				f = newDarnDecisionFake(t)
			}
			if tc.running {
				f.status = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NameDarnWrite}
				f.found = true
			}
			f.strandName = tc.strandName
			f.arbiterName = tc.arbiterName
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
	tests := []struct {
		name  string
		setup func(f *decisionFake, in *decisionInput)
		want  []string
		// notWant lists text the message must not contain.
		notWant []string
		// inputRefusal marks a refusal before the append: of the input, the caller or a darn run's record.
		inputRefusal bool
		// noDarnReads marks a refusal that must come before any darn seam is read.
		noDarnReads bool
		// afterCommit marks a refusal that follows the commit, so the record is no longer untouched and one commit has run.
		afterCommit bool
	}{
		{
			name:    "no decision record",
			setup:   func(f *decisionFake, _ *decisionInput) { f.appendErr = fmt.Errorf("read: %w", fs.ErrNotExist) },
			want:    []string{"loom: decision add:", "no decision record", "way forward:", "lyx loom start", "Darn-Write has validated it"},
			notWant: []string{"never has one"},
		},
		{
			name: "darn record missing",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.readRecordErr = fmt.Errorf("read: %w", fs.ErrNotExist)
			},
			inputRefusal: true,
			want:         []string{"no decision record", "way forward:", "message the running Darn-Write session", "board note naming the task"},
		},
		{
			name: "darn record not committed",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = false
			},
			inputRefusal: true,
			want:         []string{"not committed yet", "way forward:", "message the running Darn-Write session"},
		},
		{
			name: "darn working copy differing from its committed copy",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord + "\nedited after validation\n")
			},
			inputRefusal: true,
			want:         []string{"differs from its committed copy", "way forward:", "once the committed record is unchanged"},
		},
		{
			name: "darn record missing an entry its history held",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyVersions = [][]byte{[]byte(decisionPriorRecord + "\n### Added after Discussion (parent, 2026-03-01): Gone entry\n\nDecision: d.\n\nRationale: r.\n")}
			},
			inputRefusal: true,
			want:         []string{"Added after Discussion (parent, 2026-03-01): Gone entry", "way forward:", "message the running Darn-Write session"},
		},
		{
			name:         "darn run read failure",
			setup:        func(f *decisionFake, _ *decisionInput) { f.darnErr = errors.New("seed boom") },
			inputRefusal: true,
			want:         []string{"read the run's recipe: seed boom", "way forward:", "nothing is appended"},
		},
		{
			name: "darn record read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.readRecordErr = errors.New("read boom")
			},
			inputRefusal: true,
			want:         []string{"read boom", "way forward:", "nothing is appended"},
		},
		{
			name: "darn committed copy read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedErr = errors.New("committed boom")
			},
			inputRefusal: true,
			want:         []string{"committed boom", "way forward:", "nothing is appended"},
		},
		{
			name: "darn history read failure",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.historyErr = errors.New("history boom")
			},
			inputRefusal: true,
			want:         []string{"history boom", "way forward:", "nothing is appended"},
		},
		{
			name: "darn entry missing from the committed record after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.dropEntryOnCommit = true
			},
			afterCommit: true,
			want:        []string{"Added after Discussion (parent, 2026-03-04): Keep the cache", "way forward:", "a Darn-Write write landed during the append"},
		},
		{
			name: "darn committed record unparseable after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.commitLeaves = []byte("## Decisions\n\n### Added after Discussion (alice): t\n")
			},
			afterCommit: true,
			want:        []string{"could not be read", "way forward:", "a Darn-Write write landed during the append"},
		},
		{
			name: "darn committed read failure after the commit",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.darn = true
				f.committedFound = true
				f.committedCopy = []byte(decisionPriorRecord)
				f.committedAfterErr = errors.New("after boom")
			},
			afterCommit: true,
			want:        []string{"after boom", "way forward:", "a Darn-Write write landed during the append"},
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
				f.darn = true
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newDecisionFake(t)
			input := goodDecisionInput()
			tc.setup(f, &input)

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
			for _, w := range tc.notWant {
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
		})
	}
}

func TestDecisionVerb_CommitFailureNamesWayForward(t *testing.T) {
	f := newDecisionFake(t)
	f.commitErr = errors.New("index locked")
	var out bytes.Buffer
	if code := decisionVerb(&out, "task-a", f.deps(), goodDecisionInput()); code == 0 {
		t.Fatalf("exit = 0; want a refusal when the commit fails")
	}
	msg := envelope.Decode(t, out.String()).Error
	if !strings.Contains(msg, "index locked") || !strings.Contains(msg, "way forward:") || !strings.Contains(msg, "a session lyx refuses the verb from reports status: FAILED and the orch runs it") {
		t.Errorf("message %q; want the commit error and a way forward naming the orch", msg)
	}
}
