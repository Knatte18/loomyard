// roundusage.go records what a completed burler round cost: round-<N>-usage.yaml, with the fan and lenses the round ran and each half's model, run times and token reading.

package shedadapters

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"

	"gopkg.in/yaml.v3"
)

// roundUsage is the usage record of one completed review round.
// Fan is empty and Lenses nil for a solo round.
type roundUsage struct {
	Round  int       `yaml:"round"`
	Fan    string    `yaml:"fan"`
	Lenses []string  `yaml:"lenses,omitempty"`
	Review halfUsage `yaml:"review"`
	Fix    halfUsage `yaml:"fix"`
}

// halfUsage is one half's model, run times and token reading.
// A token field is nil when the reading is unknown, and WallSeconds is nil when either run time is zero, so an unknown value is never rendered as zero.
// The fork fields are set for the reviewer's half only, when its reading is known.
type halfUsage struct {
	Model           string    `yaml:"model"`
	Effort          string    `yaml:"effort"`
	Start           time.Time `yaml:"start,omitempty"`
	End             time.Time `yaml:"end,omitempty"`
	WallSeconds     *float64  `yaml:"wall_seconds,omitempty"`
	TokensKnown     bool      `yaml:"tokens_known"`
	FreshTokens     *int64    `yaml:"fresh_tokens,omitempty"`
	CacheReadTokens *int64    `yaml:"cache_read_tokens,omitempty"`

	Forks               *int     `yaml:"forks,omitempty"`
	ForkFreshTokens     *int64   `yaml:"fork_fresh_tokens,omitempty"`
	ForkCacheReadTokens *int64   `yaml:"fork_cache_read_tokens,omitempty"`
	ForkFreshShare      *float64 `yaml:"fork_fresh_share,omitempty"`
}

// roundUsagePath returns round n's usage record path under runDir, beside the round's review and fixer report.
func roundUsagePath(runDir string, n int) string {
	return filepath.Join(runDir, fmt.Sprintf("round-%d-usage.yaml", n))
}

// newRoundUsage builds the usage record of the round that ran fan on the review and fix models and produced result.
// The round number is set by writeRoundUsage.
func newRoundUsage(fan string, review, fix burlerengine.ModelChoice, result burlerengine.Result) roundUsage {
	reviewHalf := newHalfUsage(review, result.Review)
	if usage := result.Review.Usage; usage.Known {
		forks := usage.Forks
		reviewHalf.Forks = &forks
		reviewHalf.ForkFreshTokens = &usage.ForkFresh
		reviewHalf.ForkCacheReadTokens = &usage.ForkCacheRead
		if usage.Fresh > 0 {
			share := float64(usage.ForkFresh) / float64(usage.Fresh)
			reviewHalf.ForkFreshShare = &share
		}
	}
	return roundUsage{
		Fan:    fan,
		Lenses: result.Lenses,
		Review: reviewHalf,
		Fix:    newHalfUsage(fix, result.Fix),
	}
}

// newHalfUsage builds one half's record from the model it ran on and the half's run times and token reading.
func newHalfUsage(model burlerengine.ModelChoice, half burlerengine.Half) halfUsage {
	record := halfUsage{
		Model:       model.Model,
		Effort:      model.Effort,
		Start:       half.StartedAt,
		End:         half.EndedAt,
		TokensKnown: half.Usage.Known,
	}
	if !half.StartedAt.IsZero() && !half.EndedAt.IsZero() {
		wall := half.EndedAt.Sub(half.StartedAt).Seconds()
		record.WallSeconds = &wall
	}
	if half.Usage.Known {
		record.FreshTokens = &half.Usage.Fresh
		record.CacheReadTokens = &half.Usage.CacheRead
	}
	return record
}

// writeRoundUsage renders u as round's usage record under runDir.
func writeRoundUsage(runDir string, round int, u roundUsage) error {
	u.Round = round
	content, err := yaml.Marshal(u)
	if err != nil {
		return fmt.Errorf("shedadapters: render round %d usage record: %w", round, err)
	}
	if err := os.WriteFile(roundUsagePath(runDir, round), content, 0o644); err != nil {
		return fmt.Errorf("shedadapters: write round %d usage record: %w", round, err)
	}
	return nil
}

// readRoundUsage decodes round's usage record under runDir.
// A missing or malformed record is an error.
func readRoundUsage(runDir string, round int) (roundUsage, error) {
	path := roundUsagePath(runDir, round)
	content, err := os.ReadFile(path)
	if err != nil {
		return roundUsage{}, fmt.Errorf("shedadapters: read round %d usage record: %w", round, err)
	}
	var u roundUsage
	if err := yaml.Unmarshal(content, &u); err != nil {
		return roundUsage{}, fmt.Errorf("shedadapters: parse round %d usage record %s: %w", round, path, err)
	}
	return u, nil
}
