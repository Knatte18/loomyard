// handoff.go defines the judge-maintained handoff contract — the file a
// progress-judge call writes alongside its verdict so the NEXT judge call can
// bound its read-set to {latest valid handoff + reviews it has not absorbed}
// instead of {every prior round's review}. Like judgeverdict.go, this is a
// two-layer posture: ParseHandoff is fail-loud and never silently defaults —
// a malformed handoff file is an agent defect that must be visible as an
// error to whatever calls this function directly (including tests). The
// fail-safe posture — swallowing that error into a logger.Warn and falling
// back to the uncovered-reviews read-set, never propagating it as an engine
// error and never causing STUCK — lives one layer up, in the round loop
// (run.go) that calls ParseHandoff after a judge call, exactly mirroring how
// judge.go's spawners already treat ParseJudgeVerdict failures.
package treadleengine

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// Handoff is a judge call's maintained state carried into the next judge
// call: which rounds' reviews it has absorbed (CoversRounds), the lossless
// finding-identity ledger (Ledger), and a distilled prose narrative (Prose)
// for everything else. The ledger is deliberately not semantically matched
// in Go — no key canonicalization — because the judge, not this package, is
// the holistic decider of whether two findings are "the same" across
// rounds; Go only validates that the ledger's shape is well-formed.
type Handoff struct {
	CoversRounds []int
	Ledger       []LedgerEntry
	Prose        string
}

// LedgerEntry is one lossless finding-identity record in a Handoff's ledger:
// a short stable Key the judge uses to recognize the same finding recurring
// across rounds, the Rounds it has been seen in, and whether it is currently
// Status "open" or "resolved". The carry-forward rule — every entry from the
// previous handoff must reappear here, never dropped — is enforced at
// prompt level (the judge templates), not by this type or ParseHandoff.
type LedgerEntry struct {
	Key    string
	Rounds []int
	Status string
}

// handoffHeader mirrors a handoff file's YAML frontmatter shape for
// unmarshaling. Unknown extra keys are tolerated (no KnownFields), matching
// judgeHeader's posture: agent-written metadata in the header is harmless
// noise.
type handoffHeader struct {
	CoversRounds []int                `yaml:"covers_rounds"`
	Ledger       []handoffLedgerEntry `yaml:"ledger"`
}

// handoffLedgerEntry mirrors one ledger entry's YAML shape before
// validation promotes it into a LedgerEntry.
type handoffLedgerEntry struct {
	Key    string `yaml:"key"`
	Rounds []int  `yaml:"rounds"`
	Status string `yaml:"status"`
}

// ParseHandoff parses the raw bytes of a judge handoff file into a Handoff.
// The file must open with a "---" line and contain a closing "---" line
// delimiting YAML frontmatter (CRLF line endings are tolerated), exactly
// like ParseJudgeVerdict; everything after the closing delimiter is the
// unconstrained prose narrative. Every rule below is enforced fail-loud with
// a "treadle: "-prefixed error — this function NEVER silently defaults, so a
// self-contradictory or malformed handoff file is always visible as an
// error to its caller (the round loop is the one that turns that error into
// a fail-safe Warn + fallback read-set; see the file-level comment):
//   - the frontmatter must be present, closed, and valid YAML;
//   - covers_rounds must be a non-empty list of positive round numbers;
//   - every ledger entry must have a non-empty key, a non-empty list of
//     positive round numbers, and a status of exactly "open" or "resolved"
//     (case-sensitive) — the ledger list itself MAY be empty (a first
//     handoff has nothing yet to carry forward).
func ParseHandoff(content []byte) (Handoff, error) {
	header, err := splitFrontmatter(content)
	if err != nil {
		return Handoff{}, err
	}

	var parsed handoffHeader
	if err := yaml.Unmarshal([]byte(header), &parsed); err != nil {
		return Handoff{}, fmt.Errorf("treadle: handoff file frontmatter is not valid YAML: %w", err)
	}

	if len(parsed.CoversRounds) == 0 {
		return Handoff{}, fmt.Errorf("treadle: handoff file covers_rounds must be a non-empty list of round numbers")
	}
	for _, round := range parsed.CoversRounds {
		if round <= 0 {
			return Handoff{}, fmt.Errorf("treadle: handoff file covers_rounds contains non-positive round number %d", round)
		}
	}

	ledger, err := validateLedgerEntries(parsed.Ledger)
	if err != nil {
		return Handoff{}, err
	}

	return Handoff{
		CoversRounds: parsed.CoversRounds,
		Ledger:       ledger,
		Prose:        frontmatterProse(content),
	}, nil
}

// validateLedgerEntries promotes the raw YAML ledger entries into
// LedgerEntry values, enforcing the per-entry shape rules. An empty input
// list is legal (a first handoff carries no prior findings) and returns an
// empty, non-nil slice.
func validateLedgerEntries(raw []handoffLedgerEntry) ([]LedgerEntry, error) {
	ledger := make([]LedgerEntry, 0, len(raw))
	for _, entry := range raw {
		if strings.TrimSpace(entry.Key) == "" {
			return nil, fmt.Errorf("treadle: handoff file has a ledger entry with an empty key")
		}
		if len(entry.Rounds) == 0 {
			return nil, fmt.Errorf("treadle: handoff file ledger entry %q has an empty rounds list", entry.Key)
		}
		for _, round := range entry.Rounds {
			if round <= 0 {
				return nil, fmt.Errorf("treadle: handoff file ledger entry %q contains non-positive round number %d", entry.Key, round)
			}
		}
		switch entry.Status {
		case "open", "resolved":
			// within vocabulary
		default:
			return nil, fmt.Errorf("treadle: handoff file ledger entry %q has status %q; want exactly \"open\" or \"resolved\"", entry.Key, entry.Status)
		}
		ledger = append(ledger, LedgerEntry{Key: entry.Key, Rounds: entry.Rounds, Status: entry.Status})
	}
	return ledger, nil
}

// frontmatterProse returns everything after a handoff file's closing "---"
// frontmatter delimiter line, CRLF-normalized and trimmed. It is called only
// after splitFrontmatter has already validated that the opening and closing
// delimiters exist, so the closing line is guaranteed to be found here; this
// is a separate, minimal scan rather than a splitFrontmatter return value
// because splitFrontmatter's contract (shared with judgeverdict.go) is
// header-only and prose is a handoff-specific need.
func frontmatterProse(content []byte) string {
	lines := strings.Split(string(content), "\n")
	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			closingIdx = i
			break
		}
	}
	if closingIdx == -1 || closingIdx+1 >= len(lines) {
		return ""
	}
	prose := strings.Join(lines[closingIdx+1:], "\n")
	return strings.TrimSpace(strings.ReplaceAll(prose, "\r", ""))
}

// latestValidHandoff walks rounds newest-to-oldest looking for the most
// recent round record whose HandoffPath is non-empty AND whose file both
// reads and ParseHandoffs cleanly. An unreadable or unparseable recorded
// handoff is a fail-safe skip, not a hard stop: it logs a logger.Warn
// prefixed with name — the calling engine's own name, threaded down from
// Engine.Run exactly like every other Warn this package emits, since these
// lines reach an operator's stderr at logger's default threshold during an
// ordinary run and must not label a perch block with a foreign module name
// — then the walk continues to the next older round, so a single corrupted
// handoff degrades to the next older valid one instead of taking every
// future judge call down with it. ok is false only when no round in rounds
// carries a handoff that reads and parses cleanly (including the
// fresh-block case of zero rounds). Apart from name this helper depends on
// nothing but rounds — it must not assume a current-round review exists,
// since pre-round targeting reuses it before any round has run.
func latestValidHandoff(name string, rounds []roundRecord) (path string, h Handoff, ok bool) {
	for i := len(rounds) - 1; i >= 0; i-- {
		round := rounds[i]
		if round.HandoffPath == "" {
			continue
		}
		content, err := os.ReadFile(round.HandoffPath)
		if err != nil {
			logger.Warn(name+": recorded handoff file unreadable, falling back to an older handoff", "round", round.Round, "cause", err)
			continue
		}
		parsed, err := ParseHandoff(content)
		if err != nil {
			logger.Warn(name+": recorded handoff file unparseable, falling back to an older handoff", "round", round.Round, "cause", err)
			continue
		}
		return round.HandoffPath, parsed, true
	}
	return "", Handoff{}, false
}

// judgeReadSet builds the review-file read-set a progress-judge call is fed,
// replacing the unbounded collectJudgeReviews call at both judge call sites
// (run.go). With a valid handoff (see latestValidHandoff), readSet is the
// reviews of every completed round in rounds whose number is NOT already in
// that handoff's CoversRounds, in round order, plus currentReviewPath — the
// rounds it omits are exactly the ones the handoff has already absorbed —
// and prevHandoffPath is that handoff's own path, to thread into the next
// judge call's previous_handoff input. With no valid handoff at all (a
// fresh block, or every recorded handoff failed to read/parse), readSet
// degrades to exactly today's all-reviews behavior via collectJudgeReviews
// and prevHandoffPath is "". Rounds where no judge ran at all (round 1, a
// round right after an approved round, or a round whose judge call itself
// failed) carry no HandoffPath and so never appear in any handoff's
// CoversRounds — their reviews are therefore always present in some future
// call's readSet, which is what closes the judge-gap hole. name is the
// calling engine's own name, passed straight through to latestValidHandoff
// so its fail-safe Warns carry the caller's prefix.
func judgeReadSet(name string, rounds []roundRecord, currentReviewPath string) (readSet []string, prevHandoffPath string) {
	path, handoff, ok := latestValidHandoff(name, rounds)
	if !ok {
		return collectJudgeReviews(rounds, currentReviewPath), ""
	}

	covered := make(map[int]bool, len(handoff.CoversRounds))
	for _, round := range handoff.CoversRounds {
		covered[round] = true
	}

	reviews := make([]string, 0, len(rounds)+1)
	for _, r := range rounds {
		if !covered[r.Round] {
			reviews = append(reviews, r.ReviewPath)
		}
	}
	return append(reviews, currentReviewPath), path
}
