// handoff.go holds what the two halves of a round share: ErrHalfNotStopped, acceptReview with the digest it hands the fix half, and halfOf, which names a half by its terminal result.

package burlerengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrHalfNotStopped reports that a seat of the round could not be stopped, so it may still be writing the round's files.
// It wraps the seats engine's seatengine.ErrSeatNotStopped, whose message ends with the way forward.
// The round never archives over or retries past it.
var ErrHalfNotStopped = errors.New("burler: a half of the round could not be stopped")

// reviewHandoff is what acceptReview took from a finished review: the bytes it parsed, the parse, and the cluster audit's warnings.
type reviewHandoff struct {
	content  []byte
	verdict  Verdict
	findings []Finding
	warnings []string
}

// reviewDigest returns the hex SHA-256 of a review file's bytes, the identity the fix half checks the review against.
func reviewDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// acceptReview accepts the review a done reviewer left at p.ReviewPath.
// For a cluster round it audits the reviewer's forks, then it parses the review file strictly with ParseCurrentReview.
// It touches nothing on disk.
func acceptReview(p *Profile, reviewer shuttleengine.Result) (reviewHandoff, error) {
	var handoff reviewHandoff
	if p.ClusterFan != "" {
		warnings, err := auditClusterRound(reviewer.ForkAudit, len(p.clusterLenses))
		if err != nil {
			return reviewHandoff{}, err
		}
		handoff.warnings = warnings
	}

	content, err := os.ReadFile(p.ReviewPath)
	if err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: read review file %q: %w", p.ReviewPath, err)
	}
	verdict, findings, err := ParseCurrentReview(content)
	if err != nil {
		return reviewHandoff{}, fmt.Errorf("burler: round reached done but its review file is invalid: %w", err)
	}
	handoff.content, handoff.verdict, handoff.findings = content, verdict, findings
	return handoff, nil
}

// halfOf names a half by the identity of its terminal result, falling back to strandGUID and runDir when the result carries none.
func halfOf(result shuttleengine.Result, strandGUID, runDir string) Half {
	half := Half{
		SessionID:            result.SessionID,
		StrandGUID:           result.StrandGUID,
		LastAssistantMessage: result.LastAssistantMessage,
		RunDir:               result.RunDir,
		StartedAt:            result.StartedAt,
		EndedAt:              result.EndedAt,
		Usage:                result.Usage,
		Engine:               result.Engine,
	}
	if half.StrandGUID == "" {
		half.StrandGUID = strandGUID
	}
	if half.RunDir == "" {
		half.RunDir = runDir
	}
	return half
}
