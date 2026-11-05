// entries_recordwrite_test.go covers recordWriteEntry: its construction-time validation of the three paths and two seams of Env.RecordWrite, and the build over a filled Env.

package shedrecipe

import "testing"

import "github.com/Knatte18/loomyard/internal/testkit/tiergate"

func TestRecordWriteEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		field string
		edit  func(*Env)
	}{
		{"RelativeRecordPath", "RecordWrite.RecordPath", func(e *Env) { e.RecordWrite.RecordPath = "decision-record.md" }},
		{"RelativeSupportLogPath", "RecordWrite.SupportLogPath", func(e *Env) { e.RecordWrite.SupportLogPath = "support-log.md" }},
		{"RelativeDiscussionDir", "RecordWrite.DiscussionDir", func(e *Env) { e.RecordWrite.DiscussionDir = "discussion" }},
		{"NilReadBody", "RecordWrite.ReadBody", func(e *Env) { e.RecordWrite.ReadBody = nil }},
		{"NilReadStored", "RecordWrite.ReadStored", func(e *Env) { e.RecordWrite.ReadStored = nil }},
		{"NilCommit", "RecordWrite.Commit", func(e *Env) { e.RecordWrite.Commit = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.edit(&env)
			_, err := recordWriteEntry(RowIdentity{Name: "Row"}, Config{}, env)
			if err == nil {
				t.Fatalf("recordWriteEntry() error = nil; want a refusal naming %s", tt.field)
			}
			assertErrContains(t, err, tt.field)
		})
	}

	t.Run("BuildsWithAFilledEnv", func(t *testing.T) {
		t.Parallel()
		producer, err := recordWriteEntry(RowIdentity{Name: "Row"}, Config{}, newTestEnv(t))
		if err != nil || producer == nil {
			t.Fatalf("recordWriteEntry() = %v, %v; want a producer and nil", producer, err)
		}
	})
}
