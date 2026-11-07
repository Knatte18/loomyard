// handoff.go holds what the two halves of a burler round share.
// It covers the stop error of a half, the acceptance of a finished review, and the identity of a half.

package burlerengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// ErrHalfNotStopped reports that a seat of the round could not be stopped.
// The seat may still be writing the round's files.
// It wraps the seat engine's stop error, whose message names the way forward.
// The round never archives over or retries past it.
var ErrHalfNotStopped = errors.New("burler: a half of the round could not be stopped")

// reviewHandoff holds a finished review as the round accepted it.
// It carries the bytes that were parsed, the verdict and findings, and the cluster audit's warnings.
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
// For a cluster round it first audits the reviewer's forks.
// It then parses the review file strictly.
// It writes nothing to disk.
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
