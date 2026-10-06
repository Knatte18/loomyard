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
	appends   int
	commits   int
	lastAdded discussionparser.AddedDecision
	clock     time.Time
}

func newDecisionFake(t *testing.T) *decisionFake {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decision-record.md")
	if err := os.WriteFile(path, []byte(decisionPriorRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	return &decisionFake{path: path, clock: time.Date(2026, 3, 3, 23, 30, 0, 0, time.FixedZone("late", -5*3600))}
}

func (f *decisionFake) deps() decisionDeps {
	return decisionDeps{
		readStatus: func() (shedengine.Status, bool, error) { return f.status, f.found, f.statusErr },
		recordPath: f.path,
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
		commit: func() error { f.commits++; return f.commitErr },
		now:    func() time.Time { return f.clock },
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

// TestDecisionVerb_AppendsAndCommits asserts an entry from the parent or the operator is appended after the prior record byte-identical and committed once, with no status file.
func TestDecisionVerb_AppendsAndCommits(t *testing.T) {
	for _, by := range []string{"parent", "operator"} {
		t.Run(by, func(t *testing.T) {
			f := newDecisionFake(t)
			input := goodDecisionInput()
			input.by = by
			var out bytes.Buffer
			if code := decisionVerb(&out, "task-a", f.deps(), input); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}

			data := envelope.RequireOK(t, out.String()).Raw
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
		// inputRefusal marks a refusal of the input itself, which must not reach the append.
		inputRefusal bool
	}{
		{
			name:  "no decision record",
			setup: func(f *decisionFake, _ *decisionInput) { f.appendErr = fmt.Errorf("read: %w", fs.ErrNotExist) },
			want:  []string{"loom: decision add:", "no decision record", "way forward:", "lyx loom start"},
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
			name: "check finding after the append",
			setup: func(f *decisionFake, _ *decisionInput) {
				f.findings = []discussionparser.Finding{{Check: "sections", Detail: "missing ## Scope"}}
			},
			want: []string{"discussion check flags the record", "sections: missing ## Scope", "way forward:", "the record is restored"},
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
			if got := readDecisionRecord(t, f.path); got != decisionPriorRecord {
				t.Errorf("record = %q; want it untouched", got)
			}
			if f.commits != 0 {
				t.Errorf("commits = %d; want 0 after a refusal", f.commits)
			}
			if tc.inputRefusal && f.appends != 0 {
				t.Errorf("appends = %d; want 0 for an input refusal", f.appends)
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
	if !strings.Contains(msg, "index locked") || !strings.Contains(msg, "way forward:") {
		t.Errorf("message %q; want the commit error and a way forward", msg)
	}
}
