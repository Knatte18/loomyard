//go:build tier0

// joinparts_test.go covers the join-parts verb's refusal order and envelope over fake deps.

package loomcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

func TestJoinPartsVerb(t *testing.T) {
	t.Parallel()

	const (
		row    = "Parts-Plan-Write"
		chair  = "hub:task:parts-plan"
		writer = "hub:task:parts-plan-core"
	)
	running := func() (shedengine.Status, bool, error) {
		return shedengine.Status{CurrentProducer: row, State: shedengine.StateRunning}, true, nil
	}
	joined := func() (*planparser.JoinResult, error) {
		return &planparser.JoinResult{Cards: []planparser.JoinedCard{{Part: "core", From: 1, To: 1, File: "01-core.md"}, {Part: "wiring", From: 1, To: 2, File: "02-wire.md"}}}, nil
	}

	tests := []struct {
		name       string
		readStatus reviewStatusReader
		caller     string
		join       func() (*planparser.JoinResult, error)
		wantOK     bool
		// wantErr is a substring of the refusal; every refusal carries a way forward.
		wantErr string
	}{
		{name: "TheChairOfARunningRowJoins", readStatus: running, caller: chair, join: joined, wantOK: true},
		{
			name: "ABlockedRunAtTheRowIsRefused", caller: chair, join: joined,
			readStatus: func() (shedengine.Status, bool, error) {
				return shedengine.Status{CurrentProducer: row, State: shedengine.StateBlocked}, true, nil
			},
			wantErr: "the join runs only while " + row + " is the running row; way forward: \"lym loom resume\"",
		},
		{
			name: "ARunAtAnotherRowIsRefused", caller: chair, join: joined,
			readStatus: func() (shedengine.Status, bool, error) {
				return shedengine.Status{CurrentProducer: "Plan-Write", State: shedengine.StateRunning}, true, nil
			},
			wantErr: "the join is the chair's alone, and a running " + row + " row's chair runs it; way forward: past " + row,
		},
		{
			name: "AnAbsentStatusIsRefused", caller: chair, join: joined,
			readStatus: func() (shedengine.Status, bool, error) { return shedengine.Status{}, false, nil },
			wantErr:    "the join is the chair's alone",
		},
		{name: "APartWritersStrandIsRefused", readStatus: running, caller: writer, join: joined, wantErr: writer + " is not the chair"},
		{name: "ASessionWithNoStrandIsRefused", readStatus: running, caller: "", join: joined, wantErr: "no lym strand"},
		{
			name: "AJoinRefusalIsPassedThroughWithItsWayForward", readStatus: running, caller: chair,
			join: func() (*planparser.JoinResult, error) {
				return nil, errors.New("part \"core\" fails its check; way forward: fix it")
			},
			wantErr: "part \"core\" fails its check; way forward: fix it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			exitCode := joinPartsVerb(&out, joinPartsDeps{row: row, planDir: "/plan", readStatus: tt.readStatus, caller: tt.caller, chair: chair, join: tt.join})

			if tt.wantOK {
				if exitCode != 0 {
					t.Fatalf("exit code = %d; want 0 (output: %q)", exitCode, out.String())
				}
				envelope.RequireOK(t, out.String())
				for _, want := range []string{`"plan_dir":"/plan"`, `"part":"wiring"`, `"from":1`, `"to":2`, `"file":"02-wire.md"`} {
					if !strings.Contains(out.String(), want) {
						t.Errorf("output = %q; want it to contain %s", out.String(), want)
					}
				}
				return
			}
			if exitCode != 1 {
				t.Fatalf("exit code = %d; want 1 (output: %q)", exitCode, out.String())
			}
			envelope.RequireErr(t, out.String(), tt.wantErr)
			envelope.RequireErr(t, out.String(), "way forward:")
		})
	}
}
