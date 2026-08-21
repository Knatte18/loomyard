// finding_test.go pins the eight Kind constants' exact string values and Finding.String's
// substring contract.

package shedcheck

import (
	"strings"
	"testing"
)

func TestKindConstants(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		want string
	}{
		{"KindBadEntry", KindBadEntry, "bad-entry"},
		{"KindNoTerminals", KindNoTerminals, "no-terminals"},
		{"KindBadTerminal", KindBadTerminal, "bad-terminal"},
		{"KindDanglingTarget", KindDanglingTarget, "dangling-target"},
		{"KindUnreachable", KindUnreachable, "unreachable"},
		{"KindUnexpectedTerminal", KindUnexpectedTerminal, "unexpected-terminal"},
		{"KindDoneCycle", KindDoneCycle, "done-cycle"},
		{"KindBlindGate", KindBlindGate, "blind-gate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.kind) != tt.want {
				t.Errorf("%s = %q; want %q", tt.name, string(tt.kind), tt.want)
			}
		})
	}
}

func TestFinding_String(t *testing.T) {
	tests := []struct {
		name    string
		finding Finding
	}{
		{
			name: "populated",
			finding: Finding{
				Kind:     KindDanglingTarget,
				Producer: "gate",
				Target:   "nowhere",
				Message:  "OnDone names no producer in the list",
			},
		},
		{
			name: "empty producer target and message",
			finding: Finding{
				Kind:     KindBadEntry,
				Producer: "",
				Target:   "",
				Message:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.finding.String()
			if got == "" {
				t.Fatalf("Finding.String() = %q; want non-empty", got)
			}
			if tt.name == "populated" {
				for _, want := range []string{string(tt.finding.Kind), tt.finding.Producer, tt.finding.Target, tt.finding.Message} {
					if !strings.Contains(got, want) {
						t.Errorf("Finding.String() = %q; want substring %q", got, want)
					}
				}
			}
		})
	}
}
