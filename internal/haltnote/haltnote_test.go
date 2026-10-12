// haltnote_test.go pins the halt note's rendered bytes, its tolerant parse and its file-name test.

package haltnote

import (
	"reflect"
	"strings"
	"testing"
)

const blockedGolden = "loom halted blocked at Plan-Write\n\n" +
	"```\nthe plan lacks a card\n```\n\n" +
	"producer: Plan-Write\n" +
	"state: blocked\n" +
	"anomaly: process-signal\n" +
	"history_entries: 2\n" +
	"trace_file: /tmp/trace.log\n" +
	"\nhistory:\n" +
	"- Discussion-Write / done / 2026-01-02T03:04:05Z\n" +
	"- Plan-Write / blocked / 2026-01-02T03:05:06Z\n"

func TestRenderParse(t *testing.T) {
	t.Parallel()

	blocked := Note{
		Producer:       "Plan-Write",
		State:          "blocked",
		Reason:         "the plan lacks a card",
		Anomaly:        "process-signal",
		HistoryEntries: 2,
		TraceFile:      "/tmp/trace.log",
		History: []Row{
			{Producer: "Discussion-Write", Outcome: "done", At: "2026-01-02T03:04:05Z"},
			{Producer: "Plan-Write", Outcome: "blocked", At: "2026-01-02T03:05:06Z"},
		},
	}
	withParent := Note{
		Producer:     "Land",
		State:        "failed",
		Reason:       "worktree is dirty",
		TraceFile:    "t.log",
		ParentStatus: "a.go\nb.go",
	}

	tests := []struct {
		name        string
		note        Note
		golden      string
		wantContain string
	}{
		{name: "blocked note with anomaly and history", note: blocked, golden: blockedGolden},
		{name: "no history", note: Note{Producer: "P", State: "failed", Reason: "r", TraceFile: "t"}, wantContain: "history:\nno history entries\n"},
		{name: "parent status", note: withParent, wantContain: "\nparent fabric status:\na.go\nb.go\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rendered := Render(tc.note)
			if tc.golden != "" && string(rendered) != tc.golden {
				t.Fatalf("Render bytes:\n%q\nwant:\n%q", rendered, tc.golden)
			}
			if tc.wantContain != "" && !strings.Contains(string(rendered), tc.wantContain) {
				t.Fatalf("Render %q lacks %q", rendered, tc.wantContain)
			}
			got, err := Parse(rendered)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.note) {
				t.Fatalf("Parse = %+v, want %+v", got, tc.note)
			}
		})
	}

	t.Run("unknown line is skipped", func(t *testing.T) {
		t.Parallel()
		data := []byte("loom halted blocked at Plan-Write\n\n```\nthe plan lacks a card\n```\n\n" +
			"producer: Plan-Write\nfoo: bar\nstate: blocked\nhistory_entries: 0\ntrace_file: t\n")
		got, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		want := Note{Producer: "Plan-Write", State: "blocked", Reason: "the plan lacks a card", TraceFile: "t"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Parse = %+v, want %+v", got, want)
		}
	})

	for name, data := range map[string]string{
		"missing state":    "producer: P\nhistory_entries: 0\n",
		"missing producer": "state: blocked\nhistory_entries: 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(data)); err == nil {
				t.Fatal("Parse succeeded, want an error")
			}
		})
	}
}

func TestIsNoteName(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"loom-halt.md":         true,
		"loom-halt-2.md":       true,
		"reflection-report.md": false,
		"loom-halt.txt":        false,
		"01-plan-write.md":     false,
		"loom-halt-x.md":       false,
	}
	for name, want := range tests {
		if got := IsNoteName(name); got != want {
			t.Errorf("IsNoteName(%q) = %v, want %v", name, got, want)
		}
	}
}
