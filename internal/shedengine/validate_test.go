// validate_test.go is a table test over (*Shed).validate's rules: one case per rule, each
// asserting a distinct error-message substring, plus one case asserting a fully-valid Shed
// returns nil.

package shedengine

import (
	"context"
	"strings"
	"testing"
)

// stubProducer is a trivial in-test ShedProducer stub used only for validate_test.go's non-nil
// Producer fields. The shared funcProducer fake arrives in batch 2 and must not be anticipated
// here.
type stubProducer struct{}

func (stubProducer) Call(ctx context.Context) (Outcome, OutputPointer, error) {
	return Done, OutputPointer{}, nil
}

func validShed() *Shed {
	return &Shed{
		Producers: []ProducerDef{
			{Name: "Preflight", Producer: stubProducer{}},
			{Name: "Plan-Write", Producer: stubProducer{}},
		},
		StatusPath:     "/tmp/status.json",
		LockPath:       "/tmp/run.lock",
		StatusLockPath: "/tmp/status.json.lock",
		MaxBounces:     3,
	}
}

func TestShed_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Shed)
		wantErr string
	}{
		{
			name:    "empty StatusPath",
			mutate:  func(s *Shed) { s.StatusPath = "" },
			wantErr: "StatusPath",
		},
		{
			name:    "empty LockPath",
			mutate:  func(s *Shed) { s.LockPath = "" },
			wantErr: "LockPath",
		},
		{
			name:    "empty StatusLockPath",
			mutate:  func(s *Shed) { s.StatusLockPath = "" },
			wantErr: "StatusLockPath",
		},
		{
			name: "LockPath equal to StatusLockPath",
			mutate: func(s *Shed) {
				s.StatusLockPath = s.LockPath
			},
			wantErr: "same file",
		},
		{
			name:    "negative MaxBounces",
			mutate:  func(s *Shed) { s.MaxBounces = -1 },
			wantErr: "MaxBounces",
		},
		{
			name:    "empty Producers",
			mutate:  func(s *Shed) { s.Producers = nil },
			wantErr: "Producers",
		},
		{
			name: "empty Name",
			mutate: func(s *Shed) {
				s.Producers[0].Name = ""
			},
			wantErr: "name",
		},
		{
			name: "nil Producer",
			mutate: func(s *Shed) {
				s.Producers[0].Producer = nil
			},
			wantErr: "Producer",
		},
		{
			name: "duplicate Name",
			mutate: func(s *Shed) {
				s.Producers[1].Name = s.Producers[0].Name
			},
			wantErr: "duplicate",
		},
		{
			name: "OnStuck names no Name in the list",
			mutate: func(s *Shed) {
				s.Producers[0].OnStuck = "No-Such-Producer"
			},
			wantErr: "OnStuck",
		},
		{
			// Both rows keep the base fixture's default Segment: "", so this
			// also pins that an OnStuck pairing between two producers that
			// both carry the empty Segment is legal — the existing loom
			// shape, preserved as one implicit standalone group.
			name: "forward OnStuck reference is accepted",
			mutate: func(s *Shed) {
				s.Producers[0].OnStuck = s.Producers[1].Name
			},
			wantErr: "",
		},
		{
			name: "OnStuck naming its own producer stays legal",
			mutate: func(s *Shed) {
				s.Producers[0].OnStuck = s.Producers[0].Name
			},
			wantErr: "",
		},
		{
			name: "negative ProducerDef.MaxBounces",
			mutate: func(s *Shed) {
				s.Producers[0].MaxBounces = -1
			},
			wantErr: "has a negative MaxBounces",
		},
		{
			name: "OnDone names no producer in the list",
			mutate: func(s *Shed) {
				s.Producers[0].OnDone = "No-Such-Producer"
			},
			wantErr: "OnDone \"No-Such-Producer\" naming no producer in the list",
		},
		{
			name: "OnDone naming a later producer is accepted",
			mutate: func(s *Shed) {
				s.Producers[0].OnDone = s.Producers[1].Name
			},
			wantErr: "",
		},
		{
			name: "OnDone naming itself",
			mutate: func(s *Shed) {
				s.Producers[0].OnDone = s.Producers[0].Name
			},
			wantErr: "OnDone naming itself",
		},
		{
			name: "OnStuck names a producer in a different Segment",
			mutate: func(s *Shed) {
				s.Producers[0].Segment = "seg-a"
				s.Producers[0].OnStuck = s.Producers[1].Name
			},
			wantErr: "different Segment",
		},
		{
			name: "OnStuck between producers sharing the same non-empty Segment is accepted",
			mutate: func(s *Shed) {
				s.Producers[0].Segment = "seg-a"
				s.Producers[1].Segment = "seg-a"
				s.Producers[0].OnStuck = s.Producers[1].Name
			},
			wantErr: "",
		},
		{
			name:    "fully valid Shed",
			mutate:  func(s *Shed) {},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validShed()
			tt.mutate(s)
			err := s.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil; want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validate() = %q; want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
