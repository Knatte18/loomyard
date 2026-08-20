// round.go is the single place a round number becomes a concrete path, mirroring
// internal/treadleengine/roundfiles.go's discipline: every artifact path this package writes or
// reads is built here, never assembled ad hoc at the call site.
// The Bouncer has no attempt concept — unlike treadle's round/attempt pair, a retry here is a
// Shed bounce (a fresh Call after Stuck) rather than an in-producer retry, so a round number alone
// is enough to name every file.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ResolveRound returns the highest round N for which reportName(N) names an existing file inside
// runDir, scanning upward from 1 and stopping at the first absent round.
// It returns 0 when reportName(1) does not exist, which is a legal non-error answer meaning no
// round has been run yet.
// This resolves both halves of a segment: the Bouncer's "round to judge" is the return value, and
// the round producer's "round to write" is the return value plus one.
func ResolveRound(runDir string, reportName func(round int) string) (int, error) {
	info, err := os.Stat(runDir)
	if err != nil {
		return 0, fmt.Errorf("shedadapters: stat run dir %s: %w", runDir, err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("shedadapters: run dir %s is not a directory", runDir)
	}

	for n := 1; ; n++ {
		path := filepath.Join(runDir, reportName(n))
		if _, err := os.Stat(path); err != nil {
			// Only fs.ErrNotExist means "this round's report is absent" — any
			// other stat error (permission, I/O) must be returned rather than
			// read as absence, which would otherwise truncate the scan mid-
			// sequence or re-seed a segment that has already judged rounds.
			if errors.Is(err, fs.ErrNotExist) {
				return n - 1, nil
			}
			return 0, fmt.Errorf("shedadapters: stat report file %s: %w", path, err)
		}
	}
}

// verdictPath returns the path of the bouncer verdict file for round inside runDir.
func verdictPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-bouncer-verdict.md", round))
}

// ledgerPath returns the path of the bouncer ledger file for round inside runDir.
func ledgerPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-bouncer-ledger.md", round))
}

// focusPath returns the path of the focus file for round inside runDir.
// round is the round the file targets, not the round that wrote it — a judge call for round N
// writes round-<N+1>-focus.md, so the round producer reads its own round's focus file with no
// off-by-one reasoning.
func focusPath(runDir string, round int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-focus.md", round))
}
