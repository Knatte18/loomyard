// burler_test.go holds the run directory writers and the minimal profile the Burler row producers' tests share.

package shedadapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/burlerengine"
)

// simpleBurlerProfile returns a minimal burlerengine.Profile suitable for passing through Call --
// its content fields are irrelevant since the row producers never invoke burlerengine's own validation.
func simpleBurlerProfile() burlerengine.Profile {
	return burlerengine.Profile{
		Rubric:   "rubric text",
		FixScope: burlerengine.FixScopeOverlay,
	}
}

// writeRoundFile writes a placeholder file at path, creating its parent directory.
func writeRoundFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// writeRoundPair writes round n's review, fixer-report and acceptance record under runDir, making it complete -- but not
// judged, which is a separate condition writeJudgedRound adds on top.
func writeRoundPair(t *testing.T, runDir string, n int) {
	t.Helper()
	writeRoundFile(t, roundReviewPath(runDir, n))
	writeRoundFile(t, roundFixerReportPath(runDir, n))
	if err := writeAcceptance(runDir, n, "digest"); err != nil {
		t.Fatalf("writeAcceptance(round %d): %v", n, err)
	}
}

// writeJudgedRound writes round n's own files plus the CONTINUE verdict and ledger the segment's Bouncer writes when it rejects that round -- the complete on-disk state of a round the producer may advance past.
// The verdict is CONTINUE rather than CONVERGED because a CONVERGED round is one the segment left on a Done, never one the round producer is called after.
func writeJudgedRound(t *testing.T, runDir string, n int) {
	t.Helper()
	writeRoundPair(t, runDir, n)
	if err := os.WriteFile(verdictPath(runDir, n), []byte(bouncerVerdictContent("CONTINUE")), 0o644); err != nil {
		t.Fatalf("WriteFile(verdict round %d): %v", n, err)
	}
	if err := os.WriteFile(ledgerPath(runDir, n), []byte(bouncerLedgerContent(n)), 0o644); err != nil {
		t.Fatalf("WriteFile(ledger round %d): %v", n, err)
	}
}
