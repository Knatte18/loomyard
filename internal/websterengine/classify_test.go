// classify_test.go covers Classify's pinned decision order: a present
// Report short-circuits to terminal regardless of every other field, and
// absent a report the dead-reason precedence runs TurnEnded -> timeout ->
// strand-died -> running, in that order. Tier 1: no git, inputs constructed
// directly.

package websterengine

import (
	"testing"
	"time"
)

func TestClassify_ReportPresentShortCircuitsToTerminal(t *testing.T) {
	t.Parallel()

	// Every other field is set to values that would otherwise classify as
	// dead-timeout, to prove Report wins outright regardless.
	in := ClassifyInputs{
		BatchNumber:  4,
		BatchSlug:    "some-batch",
		Report:       &Report{Status: ReportStatusOK, HeadSHA: "abc123"},
		TurnEnded:    true,
		StrandLive:   false,
		Elapsed:      10 * time.Hour,
		BatchTimeout: time.Minute,
	}

	got, terminal := Classify(in)

	if !terminal {
		t.Fatalf("Classify() terminal = false; want true when Report is present")
	}
	if got.Status != DigestStatusDone {
		t.Errorf("Classify().Status = %q; want %q", got.Status, DigestStatusDone)
	}
	if want := "04-some-batch"; got.Batch != want {
		t.Errorf("Classify().Batch = %q; want %q", got.Batch, want)
	}
	if got.HeadSHA != "abc123" {
		t.Errorf("Classify().HeadSHA = %q; want %q", got.HeadSHA, "abc123")
	}
}

func TestClassify_ReportPresentFailedIsStuck(t *testing.T) {
	t.Parallel()

	in := ClassifyInputs{
		BatchNumber: 1,
		BatchSlug:   "x",
		Report:      &Report{Status: ReportStatusFailed, HeadSHA: "def"},
	}

	got, terminal := Classify(in)

	if !terminal {
		t.Fatalf("Classify() terminal = false; want true")
	}
	if got.Status != DigestStatusStuck {
		t.Errorf("Classify().Status = %q; want %q", got.Status, DigestStatusStuck)
	}
}

func TestClassify_NoReport_TurnEndedIsDeadAsking(t *testing.T) {
	t.Parallel()

	in := ClassifyInputs{
		BatchNumber:  2,
		BatchSlug:    "y",
		TurnEnded:    true,
		StrandLive:   true, // must not matter: TurnEnded is checked first.
		Elapsed:      time.Second,
		BatchTimeout: time.Hour,
	}

	got, terminal := Classify(in)

	if !terminal {
		t.Fatalf("Classify() terminal = false; want true")
	}
	if got.Status != DigestStatusDead || got.DeadReason != DeadReasonAsking {
		t.Errorf("Classify() = {Status: %q, DeadReason: %q}; want {%q, %q}", got.Status, got.DeadReason, DigestStatusDead, DeadReasonAsking)
	}
}

func TestClassify_NoReport_TimeoutIsDeadTimeout(t *testing.T) {
	t.Parallel()

	in := ClassifyInputs{
		BatchNumber:  3,
		BatchSlug:    "z",
		TurnEnded:    false,
		StrandLive:   true, // must not matter: timeout is checked before strand liveness.
		Elapsed:      2 * time.Hour,
		BatchTimeout: time.Hour,
	}

	got, terminal := Classify(in)

	if !terminal {
		t.Fatalf("Classify() terminal = false; want true")
	}
	if got.Status != DigestStatusDead || got.DeadReason != DeadReasonTimeout {
		t.Errorf("Classify() = {Status: %q, DeadReason: %q}; want {%q, %q}", got.Status, got.DeadReason, DigestStatusDead, DeadReasonTimeout)
	}
}

func TestClassify_NoReport_StrandDeadIsDeadDied(t *testing.T) {
	t.Parallel()

	in := ClassifyInputs{
		BatchNumber:  5,
		BatchSlug:    "w",
		TurnEnded:    false,
		StrandLive:   false,
		Elapsed:      time.Minute,
		BatchTimeout: time.Hour,
	}

	got, terminal := Classify(in)

	if !terminal {
		t.Fatalf("Classify() terminal = false; want true")
	}
	if got.Status != DigestStatusDead || got.DeadReason != DeadReasonDied {
		t.Errorf("Classify() = {Status: %q, DeadReason: %q}; want {%q, %q}", got.Status, got.DeadReason, DigestStatusDead, DeadReasonDied)
	}
}

func TestClassify_NoReport_StillRunning(t *testing.T) {
	t.Parallel()

	in := ClassifyInputs{
		BatchNumber:  6,
		BatchSlug:    "v",
		TurnEnded:    false,
		StrandLive:   true,
		Elapsed:      42 * time.Second,
		BatchTimeout: time.Hour,
	}

	got, terminal := Classify(in)

	if terminal {
		t.Fatalf("Classify() terminal = true; want false for a still-running batch")
	}
	if got.Status != DigestStatusRunning {
		t.Errorf("Classify().Status = %q; want %q", got.Status, DigestStatusRunning)
	}
	if got.ElapsedS != 42 {
		t.Errorf("Classify().ElapsedS = %d; want 42", got.ElapsedS)
	}
	if want := "06-v"; got.Batch != want {
		t.Errorf("Classify().Batch = %q; want %q", got.Batch, want)
	}
}
