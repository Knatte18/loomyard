// burlerreview_assist_test.go pins the assistant reviewer and the attempt record of BurlerReviewProducer:
// the per-round pick handed to the runner;
// the record written before each attempt spawns;
// the probe built from that record;
// and the assistant copy archived before a retry.

package shedadapters

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// lunaChoice is the assistant model choice the assist tests run rounds under.
var lunaChoice = shuttleengine.ModelChoice{Engine: "codex", Model: "luna", Effort: "high", Version: "v1"}

// assistTestSettings returns assistant settings that name lunaChoice for round 1 and no assistant from round 2 on.
func assistTestSettings() AssistSettings {
	return AssistSettings{
		Models:     []shuttleengine.ModelChoice{lunaChoice, {}},
		Fans:       []string{"", "review-code"},
		ModelKey:   shuttleengine.ModelKey{Module: "burler", Key: "assist.model"},
		Timeout:    3 * time.Minute,
		SkipReason: "no time left",
	}
}

// TestBurlerReviewProducer_Call_AssistPick proves each round's review gets its picked assistant;
// it is also given the round token and the settings' key, timeout and skip reason.
// A round whose entry has no model gets the zero assistant.
func TestBurlerReviewProducer_Call_AssistPick(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name       string
		judged     int
		wantRound  string
		wantAssist shuttleengine.ModelChoice
		wantFan    string
	}{
		{name: "round 1 runs the first entry's assistant solo", wantRound: "1", wantAssist: lunaChoice},
		{name: "round 2 runs no assistant", judged: 1, wantRound: "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			for round := 1; round <= tt.judged; round++ {
				writeJudgedRound(t, runDir, round)
			}
			spec := newHalfSpec()
			spec.assist = assistTestSettings()
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
			p := newReviewProducer(t, runDir, runner, spec)

			shedfake.CallOK(t, p)

			if len(runner.GotReviewOpts) != 1 {
				t.Fatalf("Review calls = %d; want 1", len(runner.GotReviewOpts))
			}
			opts := runner.GotReviewOpts[0]
			if opts.Round != tt.wantRound || !reflect.DeepEqual(opts.Assist, tt.wantAssist) {
				t.Errorf("Round = %q, Assist = %+v; want %q and %+v", opts.Round, opts.Assist, tt.wantRound, tt.wantAssist)
			}
			if tt.wantAssist.Model != "" && opts.AssistFan != tt.wantFan {
				t.Errorf("AssistFan = %q; want %q", opts.AssistFan, tt.wantFan)
			}
			if opts.AssistKey != spec.assist.ModelKey || opts.AssistTimeout != spec.assist.Timeout || opts.AssistSkip != spec.assist.SkipReason {
				t.Errorf("AssistKey, AssistTimeout, AssistSkip = %+v, %v, %q; want the settings' %+v, %v, %q", opts.AssistKey, opts.AssistTimeout, opts.AssistSkip, spec.assist.ModelKey, spec.assist.Timeout, spec.assist.SkipReason)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_RecordsAttempt proves each attempt is recorded before it spawns:
// the record holds token 1 with the assistant before attempt 1, and token 1b with the same assistant before attempt 2.
// Its DuringReview hook reads the record, so each token proves the write came first.
// It also proves the assistant's copy beside the review is archived before attempt 2.
// A failed record write is a failure exit that spawns nothing.
func TestBurlerReviewProducer_Call_RecordsAttempt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("each attempt is recorded before it spawns and a retry archives the assistant copy", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		spec := newHalfSpec()
		spec.assist = assistTestSettings()
		copyPath := burlerengine.AssistCopyPath(roundReviewPath(runDir, 1), burlerengine.AssistSeatName)
		var atSpawn []roundUsage
		var copyAtSpawn []bool
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{{Outcome: shuttleengine.OutcomeDied}, reviewDone("d")}}
		runner.DuringReview = func(i int) {
			usage, err := readRoundUsage(runDir, 1)
			if err != nil {
				t.Errorf("readRoundUsage() during attempt %d = %v; want the attempt recorded before the spawn", i+1, err)
			}
			atSpawn = append(atSpawn, usage)
			_, statErr := os.Stat(copyPath)
			copyAtSpawn = append(copyAtSpawn, statErr == nil)
			if i == 0 {
				writeRoundFile(t, roundReviewPath(runDir, 1))
				writeRoundFile(t, copyPath)
			}
		}
		p := newReviewProducer(t, runDir, runner, spec)

		shedfake.CallOK(t, p)

		wantAttempts := []string{"1", "1b"}
		if len(atSpawn) != len(wantAttempts) {
			t.Fatalf("Review calls = %d; want %d", len(atSpawn), len(wantAttempts))
		}
		for i, usage := range atSpawn {
			wantAssist := &assistRecord{Engine: lunaChoice.Engine, Model: lunaChoice.Model, Effort: lunaChoice.Effort, Version: lunaChoice.Version}
			if usage.Attempt != wantAttempts[i] || !reflect.DeepEqual(usage.Assist, wantAssist) || usage.AssistFan != "" {
				t.Errorf("record at spawn %d = attempt %q, assist %+v, assist fan %q; want attempt %q, assist %+v and no fan", i+1, usage.Attempt, usage.Assist, usage.AssistFan, wantAttempts[i], wantAssist)
			}
		}
		if !reflect.DeepEqual(copyAtSpawn, []bool{false, false}) {
			t.Errorf("assistant copy present at spawns = %v; want it absent at both, archived before attempt 2", copyAtSpawn)
		}
		requireArchived(t, runDir, "round-1-review.assist-assist.md", 1)
		requireArchived(t, runDir, "round-1-review.md", 1)
	})

	t.Run("a round with no assistant records its attempt with no assistant keys", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
		p := newReviewProducer(t, runDir, runner, newHalfSpec())

		shedfake.CallOK(t, p)

		usage, err := readRoundUsage(runDir, 1)
		if err != nil || usage.Attempt != "1" || usage.Assist != nil || usage.AssistFan != "" {
			t.Errorf("usage record = %+v, %v; want attempt 1 with no assistant", usage, err)
		}
	})

	t.Run("a failed attempt record write spawns nothing", func(t *testing.T) {
		t.Parallel()
		runDir := t.TempDir()
		writeRoundFile(t, roundReviewPath(runDir, 1))
		if err := os.Mkdir(roundUsagePath(runDir, 1), 0o755); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
		spec := newHalfSpec()
		spec.assist = assistTestSettings()
		runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
		p := newReviewProducer(t, runDir, runner, spec)

		outcome, _, err := p.Call(context.Background())

		if err == nil || outcome == shedengine.Stuck || outcome == shedengine.Done {
			t.Errorf("Call() = %q, %v; want a failure exit", outcome, err)
		}
		if runner.ReviewCalls != 0 {
			t.Errorf("Review calls = %d; want none after a failed attempt record write", runner.ReviewCalls)
		}
	})
}

// TestBurlerReviewProducer_Call_RecordsAssistOutcome proves an accepted round records its assistant's ending;
// it also records the lenses the assistant returned no report for, and a round with no assistant records neither.
func TestBurlerReviewProducer_Call_RecordsAssistOutcome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name         string
		assists      []burlerengine.AssistOutcome
		wantEnding   string
		wantMissing  []string
		staleEnding  string
		staleMissing []string
	}{
		{name: "a fan that returned every lens", assists: []burlerengine.AssistOutcome{{Seat: "assist", Ending: "done"}}, wantEnding: "done"},
		{name: "a fanned seat that went solo", assists: []burlerengine.AssistOutcome{{Seat: "assist", Ending: "done", MissingLenses: []string{"a", "b"}}}, wantEnding: "done", wantMissing: []string{"a", "b"}},
		{name: "an assistant that timed out", assists: []burlerengine.AssistOutcome{{Seat: "assist", Ending: "timeout"}}, wantEnding: "timeout"},
		{name: "no assistant"},
		{name: "a later attempt with no assistant clears an earlier attempt's outcome", staleEnding: "done", staleMissing: []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			if tt.staleEnding != "" {
				stale := roundUsage{AssistEnding: tt.staleEnding, AssistLensesMissing: tt.staleMissing}
				if err := writeRoundUsage(runDir, 1, stale); err != nil {
					t.Fatalf("writeRoundUsage: %v", err)
				}
			}
			result := reviewDone("d")
			result.Assists = tt.assists
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{result}}
			p := newReviewProducer(t, runDir, runner, newHalfSpec())

			shedfake.CallOK(t, p)

			usage, err := readRoundUsage(runDir, 1)
			if err != nil {
				t.Fatalf("readRoundUsage() = %v; want nil", err)
			}
			if usage.AssistEnding != tt.wantEnding || !reflect.DeepEqual(usage.AssistLensesMissing, tt.wantMissing) {
				t.Errorf("record assist ending %q, lenses missing %v; want %q and %v", usage.AssistEnding, usage.AssistLensesMissing, tt.wantEnding, tt.wantMissing)
			}
		})
	}
}

// TestBurlerReviewProducer_Call_ProbeFromAttemptRecord proves the probe takes its options from the usage record;
// never from the deps' current assistant settings.
// A recorded retry token and assistant are probed as recorded.
// A record without the attempt key, as an older binary wrote it, is probed as the bare round with no assistant.
func TestBurlerReviewProducer_Call_ProbeFromAttemptRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	recordedAssist := &assistRecord{Engine: "codex", Model: "recorded-luna", Effort: "low", Version: "v9"}
	tests := []struct {
		name       string
		usage      roundUsage
		wantRound  string
		wantAssist shuttleengine.ModelChoice
		wantFan    string
	}{
		{
			name:      "a recorded retry and assistant win over the deps",
			usage:     roundUsage{Attempt: "2b", Assist: recordedAssist, AssistFan: "review-code"},
			wantRound: "2b", wantAssist: shuttleengine.ModelChoice{Engine: "codex", Model: "recorded-luna", Effort: "low", Version: "v9"}, wantFan: "review-code",
		},
		{
			name:      "a record without the attempt key is probed as the bare round with no assistant",
			usage:     roundUsage{Assist: recordedAssist, AssistFan: "review-code"},
			wantRound: "2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDir := t.TempDir()
			writeJudgedRound(t, runDir, 1)
			if err := writeRoundUsage(runDir, 2, tt.usage); err != nil {
				t.Fatalf("writeRoundUsage: %v", err)
			}
			spec := newHalfSpec()
			// The deps name another assistant for every round.
			spec.assist = AssistSettings{Models: []shuttleengine.ModelChoice{{Model: "current-luna"}}, Fans: []string{"current-fan"}}
			runner := &shedfake.BurlerHalfRunner{ReviewResults: []burlerengine.ReviewResult{reviewDone("d")}}
			p := newReviewProducer(t, runDir, runner, spec)

			shedfake.CallOK(t, p)

			if len(runner.GotProbeReviewOpts) != 1 {
				t.Fatalf("ProbeReview calls = %d; want 1", len(runner.GotProbeReviewOpts))
			}
			opts := runner.GotProbeReviewOpts[0]
			if opts.Round != tt.wantRound || !reflect.DeepEqual(opts.Assist, tt.wantAssist) || opts.AssistFan != tt.wantFan {
				t.Errorf("probe opts Round = %q, Assist = %+v, AssistFan = %q; want %q, %+v, %q", opts.Round, opts.Assist, opts.AssistFan, tt.wantRound, tt.wantAssist, tt.wantFan)
			}
		})
	}
}
