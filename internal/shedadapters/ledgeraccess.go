// ledgeraccess.go is how another package consumes the ledger contract without re-declaring either
// the filename convention ledgerPath owns or the parse grammar parseLedger owns: an exported
// predicate recognizing a ledger file's path, an exported accessor reading and parsing one, and the
// exported model both hand back.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LedgerEntry is the exported mirror of ledgerEntry: one finding-identity record in a ledger file.
type LedgerEntry struct {
	Key    string
	Rounds []int
	Status string
}

// Ledger is the exported mirror of ledgerFile, carrying no Prose field: no consumer this package
// serves today reads it, and omitting it keeps the exported surface minimal.
type Ledger struct {
	Round   int
	Entries []LedgerEntry
}

// ledgerRound reports whether path's base name is a ledger file's, and if so which round it names.
// It derives its answer from ledgerPath itself -- extracting the digit run from path's base name and
// reconstructing the same round's filename via ledgerPath, accepting only a byte-for-byte match --
// rather than from a re-typed round-%d-bouncer-ledger.md literal. That derivation is what makes this
// predicate reject the four same-directory round-%d- siblings (verdictPath, focusPath,
// roundReviewPath, roundFixerReportPath produce different filenames for the same round) without
// naming any of their literal shapes here.
func ledgerRound(path string) (int, bool) {
	base := filepath.Base(path)

	start := strings.IndexFunc(base, func(r rune) bool { return r >= '0' && r <= '9' })
	if start < 0 {
		return 0, false
	}
	end := start
	for end < len(base) && base[end] >= '0' && base[end] <= '9' {
		end++
	}
	digits := base[start:end]
	if len(digits) > 1 && digits[0] == '0' {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 {
		return 0, false
	}

	if filepath.Base(ledgerPath("", n)) != base {
		return 0, false
	}
	return n, true
}

// IsLedgerPath reports whether path's base name is the ledger spelling ledgerPath formats.
// It accepts a ledgerPath(dir, n) result for any positive n and rejects everything else, including
// the empty string, a bare directory path, and the four same-directory round-%d- siblings
// verdictPath, focusPath, roundReviewPath, and roundFixerReportPath produce.
func IsLedgerPath(path string) bool {
	_, ok := ledgerRound(path)
	return ok
}

// ReadLedger reads path, parses it via parseLedger, and derives the round path's own base name
// encodes through the same helper IsLedgerPath uses.
// It returns a zero Ledger and a non-nil error on a read failure, a parse failure, or when the
// parsed frontmatter's round disagrees with the round the filename encodes -- inheriting the
// fail-closed check recordedVerdict already applies -- rather than returning a Ledger claiming the
// frontmatter's value.
// Deriving the round inside the accessor rather than requiring the caller to pass it is what keeps
// round extraction inside the package that owns the filename.
func ReadLedger(path string) (Ledger, error) {
	round, ok := ledgerRound(path)
	if !ok {
		return Ledger{}, fmt.Errorf("shedadapters: %q is not a ledger file path", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Ledger{}, fmt.Errorf("shedadapters: read ledger file %s: %w", path, err)
	}

	parsed, err := parseLedger(raw)
	if err != nil {
		return Ledger{}, err
	}
	if parsed.Round != round {
		return Ledger{}, fmt.Errorf("shedadapters: ledger file %s names round %d in its filename but round %d in its frontmatter", path, round, parsed.Round)
	}

	entries := make([]LedgerEntry, len(parsed.Entries))
	for i, e := range parsed.Entries {
		entries[i] = LedgerEntry{Key: e.Key, Rounds: e.Rounds, Status: e.Status}
	}
	return Ledger{Round: parsed.Round, Entries: entries}, nil
}
