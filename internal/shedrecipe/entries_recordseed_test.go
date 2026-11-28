// entries_recordseed_test.go covers recordSeedEntry.
// It checks the construction-time validation of the paths, slug and seams of Env.RecordSeed.
// It also checks the build over a filled Env.

package shedrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRecordSeedEntry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		field string
		edit  func(*Env)
	}{
		{"RelativeRecordPath", "RecordSeed.RecordPath", func(e *Env) { e.RecordSeed.RecordPath = "decision-record.md" }},
		{"RelativeSupportLogPath", "RecordSeed.SupportLogPath", func(e *Env) { e.RecordSeed.SupportLogPath = "support-log.md" }},
		{"RelativeDiscussionDir", "RecordSeed.DiscussionDir", func(e *Env) { e.RecordSeed.DiscussionDir = "discussion" }},
		{"RelativeSeedPath", "RecordSeed.SeedPath", func(e *Env) { e.RecordSeed.SeedPath = "seed.json" }},
		{"EmptySlug", "RecordSeed.Slug", func(e *Env) { e.RecordSeed.Slug = "" }},
		{"NilReadStored", "RecordSeed.ReadStored", func(e *Env) { e.RecordSeed.ReadStored = nil }},
		{"NilCommit", "RecordSeed.Commit", func(e *Env) { e.RecordSeed.Commit = nil }},
		{"NilReadSeed", "RecordSeed.ReadSeed", func(e *Env) { e.RecordSeed.ReadSeed = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.edit(&env)
			_, err := recordSeedEntry(RowIdentity{Name: "Row"}, Config{}, env)
			if err == nil {
				t.Fatalf("recordSeedEntry() error = nil; want a refusal naming %s", tt.field)
			}
			assertErrContains(t, err, tt.field)
		})
	}

	t.Run("BuildsWithAFilledEnv", func(t *testing.T) {
		t.Parallel()
		producer, err := recordSeedEntry(RowIdentity{Name: "Row"}, Config{}, newTestEnv(t))
		if err != nil || producer == nil {
			t.Fatalf("recordSeedEntry() = %v, %v; want a producer and nil", producer, err)
		}
	})
}
