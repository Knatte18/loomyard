// gateattempts_test.go covers HistoryEntry.GateAttempts end to end: a producer's OutputPointer
// reaches Step's persisted HistoryEntry, and the status file on disk actually shows the count, per
// the producer-gate contract's "recorded in the row's envelope/history so the status file
// shows it" requirement. It reuses newTestShed, seedStatus, readStatus, and commonSeed from
// testsupport_test.go rather than redeclaring any of them.

package shedengine

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// gatedOutcomeProducer returns a *funcProducer that always reports outcome and an OutputPointer
// naming outputPath, gateAttempts and gateSpans, so a test can drive Step through a gated producer's exact
// return shape without a real shuttleengine/shedadapters dependency.
func gatedOutcomeProducer(outcome Outcome, outputPath string, gateAttempts *int, gateSpans []GateSpan) *funcProducer {
	return &funcProducer{
		fn: func(ctx context.Context) (Outcome, OutputPointer, error) {
			return outcome, OutputPointer{Path: outputPath, GateAttempts: gateAttempts, GateSpans: gateSpans}, nil
		},
	}
}

// TestStep_GateAttempts pins what Step records of a producer's gate attempts, in the returned history and in the status file:
// an exhausted gate's count, a pass-after-retry count, and a first-try pass shown as a non-nil zero (a distinct verdict from an ungated call), while an ungated producer gains no gate_attempts or gate_spans field.
// A gated call's spans appear on the returned and the persisted entry.
func TestStep_GateAttempts(t *testing.T) {
	t.Parallel()
	spans := []GateSpan{{Gate: "plan-parse", Kind: GateSpanKindGate, Start: "2026-08-15T09:00:00.5Z", End: "2026-08-15T09:00:01Z"}}
	tests := []struct {
		name      string
		producer  *funcProducer
		want      *int
		wantSpans []GateSpan
	}{
		{"stuck exhaustion persists the count", gatedOutcomeProducer(Stuck, "", intPtr(3), nil), intPtr(3), nil},
		{"pass after retry persists the count", gatedOutcomeProducer(Done, "out.md", intPtr(2), nil), intPtr(2), nil},
		{"passed first try shows zero, not omitted", gatedOutcomeProducer(Done, "out.md", intPtr(0), nil), intPtr(0), nil},
		{"a gated call's spans persist", gatedOutcomeProducer(Done, "out.md", intPtr(1), spans), intPtr(1), spans},
		{"ungated producer omits the field", fixedOutcomeProducer(Done, "out.md"), nil, nil},
	}
	describe := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return "pointer to " + strconv.Itoa(*p)
	}
	same := func(got, want *int) bool {
		if got == nil || want == nil {
			return got == want
		}
		return *got == *want
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shed, statusPath, _, statusLockPath := newTestShed(t)
			shed.Producers = []ProducerDef{{Name: "A", Producer: tt.producer}}
			seedStatus(t, statusPath, statusLockPath, commonSeed("A"))

			res, err := shed.Step(context.Background())
			if err != nil {
				t.Fatalf("Step(...) = _, %v; want nil error", err)
			}
			if len(res.History) != 1 {
				t.Fatalf("len(History) = %d; want 1", len(res.History))
			}
			if got := res.History[0].GateAttempts; !same(got, tt.want) {
				t.Errorf("History[0].GateAttempts = %s; want %s", describe(got), describe(tt.want))
			}

			persisted := readStatus(t, statusPath, statusLockPath)
			if len(persisted.History) != 1 {
				t.Fatalf("persisted len(History) = %d; want 1", len(persisted.History))
			}
			if got := persisted.History[0].GateAttempts; !same(got, tt.want) {
				t.Errorf("persisted History[0].GateAttempts = %s; want %s -- the status file must show the attempt count", describe(got), describe(tt.want))
			}
			if !reflect.DeepEqual(res.History[0].GateSpans, tt.wantSpans) {
				t.Errorf("History[0].GateSpans = %+v; want %+v", res.History[0].GateSpans, tt.wantSpans)
			}
			if !reflect.DeepEqual(persisted.History[0].GateSpans, tt.wantSpans) {
				t.Errorf("persisted History[0].GateSpans = %+v; want %+v", persisted.History[0].GateSpans, tt.wantSpans)
			}
			if tt.wantSpans == nil {
				// An old entry reads with no list, so the raw bytes must carry no key; the decoded Status cannot tell an absent list from an empty one.
				raw, err := os.ReadFile(statusPath)
				if err != nil {
					t.Fatalf("os.ReadFile(status) = _, %v", err)
				}
				var onDisk struct {
					History []map[string]any `json:"history"`
				}
				if err := json.Unmarshal(raw, &onDisk); err != nil || len(onDisk.History) != 1 {
					t.Fatalf("decoding the raw status = %v with %d entries; want one history entry", err, len(onDisk.History))
				}
				if _, present := onDisk.History[0]["gate_spans"]; present {
					t.Errorf("raw history entry %v has a gate_spans key; want it omitted", onDisk.History[0])
				}
			}
		})
	}
}
