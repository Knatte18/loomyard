//go:build tier0

// entries_samplersession_test.go covers samplerSessionEntry: the Env.Sampler values and seams it refuses by field name, and the optional Watch seam.

package shedrecipe

import "testing"

// TestSamplerSessionEntry asserts the entry refuses an unset required Env.Sampler value naming its field, and builds when only Watch is nil.
// The built producer and the refused unknown config key are covered by TestEveryEntry_RejectsUnknownConfigKey.
func TestSamplerSessionEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		field string
		edit  func(*Env)
	}{
		{"NilStamp", "Sampler.Stamp", func(e *Env) { e.Sampler.Stamp = nil }},
		{"NilSessions", "Sampler.Sessions", func(e *Env) { e.Sampler.Sessions = nil }},
		{"EmptyRecordPath", "Sampler.RecordPath", func(e *Env) { e.Sampler.RecordPath = "" }},
		{"NilWatchBuilds", "", func(e *Env) { e.Sampler.Watch = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.edit(&env)
			producer, err := samplerSessionEntry(RowIdentity{Name: "Row"}, Config{}, env)
			if tt.field == "" {
				if err != nil || producer == nil {
					t.Fatalf("samplerSessionEntry() = (%v, %v); want a producer and nil", producer, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("samplerSessionEntry() error = nil; want non-nil naming %s", tt.field)
			}
			assertErrContains(t, err, tt.field)
		})
	}
}
