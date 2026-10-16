// roundusage.go records what a review round cost: round-<N>-usage.yaml, with the fan and lenses the round ran, each Burler half's model, run times and token reading, and the Bouncer's seed and judge passes.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"

	"gopkg.in/yaml.v3"
)

// RoundFans holds one segment's per-round fan lists: Review the reviewer's and Fix the fixer's, entry N for round N.
// A round past the last entry takes the last entry, and an empty entry is a solo or single round.
type RoundFans struct {
	Review []string
	Fix    []string
}

// roundFan returns the entry of fans for round, the last entry for every later round, and "" for an empty list.
func roundFan(fans []string, round int) string {
	switch {
	case len(fans) == 0:
		return ""
	case round > len(fans):
		return fans[len(fans)-1]
	}
	return fans[round-1]
}

// roundUsage is the usage record of one review round.
// Fan and FixFan are the reviewer's and fixer's fans the round was spawned under, empty for a solo or single round; FansRecorded says they were written at the spawn, so an empty name is told from an absent one.
// Lenses is nil for a solo round, and FixLenses and FixForks are set only for a twill fixer, FixForks a pointer so a single round carries no count.
// Review and Fix are nil until the round's Burler completes, so a record is a completed round's exactly when its review half is present.
// Seed and Judge list the Bouncer's passes for the round, one entry per pass.
type roundUsage struct {
	Round        int         `yaml:"round"`
	Fan          string      `yaml:"fan"`
	FixFan       string      `yaml:"fix_fan"`
	FansRecorded bool        `yaml:"fans_recorded,omitempty"`
	Lenses       []string    `yaml:"lenses,omitempty"`
	FixLenses    []string    `yaml:"fix_lenses,omitempty"`
	FixForks     *int        `yaml:"fix_forks,omitempty"`
	Review       *halfUsage  `yaml:"review,omitempty"`
	Fix          *halfUsage  `yaml:"fix,omitempty"`
	Seed         []halfUsage `yaml:"seed,omitempty"`
	Judge        []halfUsage `yaml:"judge,omitempty"`
}

// halfUsage is one half's engine, model, run times and token reading.
// Engine is the engine the half's run used, absent for a half with no run record.
// A token field is nil when the reading is unknown, and WallSeconds is nil when either run time is zero, so an unknown value is never rendered as zero.
// The fork fields are set for the reviewer's half only, when its reading is known.
type halfUsage struct {
	Engine          string    `yaml:"engine,omitempty"`
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

// newHalfUsage builds one half's record from the model it ran on and the half's engine, run times and token reading.
// The engine comes from the half, not the choice, so it names the engine that ran even when the choice left it empty.
func newHalfUsage(model shuttleengine.ModelChoice, half burlerengine.Half) halfUsage {
	record := halfUsage{
		Engine:      half.Engine,
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

// newPassUsage builds the entry of one Bouncer pass from the model it ran on and the shuttle result's run times and token reading.
// The fork fields stay unset.
func newPassUsage(model shuttleengine.ModelChoice, result shuttleengine.Result) halfUsage {
	return newHalfUsage(model, burlerengine.Half{StartedAt: result.StartedAt, EndedAt: result.EndedAt, Usage: result.Usage, Engine: result.Engine})
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

// mergeRoundUsage is the one write path of a round's usage record, shared by every writer of a part of it:
// it reads the record if present, calls apply on it and writes the result.
// A missing record starts from the zero value, and a record that does not decode is archived aside, logged and replaced by a fresh record holding only this writer's part.
func mergeRoundUsage(runDir string, round int, now func() time.Time, apply func(u *roundUsage)) error {
	path := roundUsagePath(runDir, round)
	var u roundUsage
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("shedadapters: read round %d usage record: %w", round, err)
	default:
		if err := yaml.Unmarshal(content, &u); err != nil {
			logger.Warn("shedadapters: round usage record does not decode; archiving it and starting a fresh one", "round", round, "path", path, "error", err)
			if err := archiveStaleOutputs([]string{path}, now); err != nil {
				return err
			}
			u = roundUsage{}
		}
	}
	apply(&u)
	return writeRoundUsage(runDir, round, u)
}

// recordRoundFans writes the fan names round's reviewer and fixer are spawned under, with the flag that says they are there.
// It is the only writer of the three keys; the empty names of a solo or single round are recorded like any other.
func recordRoundFans(runDir string, round int, now func() time.Time, fan, fixFan string) error {
	return mergeRoundUsage(runDir, round, now, func(u *roundUsage) {
		u.Fan, u.FixFan, u.FansRecorded = fan, fixFan, true
	})
}

// readRecordedFans returns the reviewer and fixer fan names recorded for round.
// A missing record, one that does not decode and one without the flag are errors, since a record written by a binary that predates the names also decodes the reviewer's fan as "".
func readRecordedFans(runDir string, round int) (fan, fixFan string, err error) {
	u, err := readRoundUsage(runDir, round)
	if err != nil {
		return "", "", err
	}
	if !u.FansRecorded {
		return "", "", errors.New("the record carries no fans_recorded flag")
	}
	return u.Fan, u.FixFan, nil
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

// readCompletedRoundUsage is readRoundUsage for a reader that needs the Burler's part of the record:
// a record with no review half is an error naming the round, since only the Bouncer has written to it.
func readCompletedRoundUsage(runDir string, round int) (roundUsage, error) {
	u, err := readRoundUsage(runDir, round)
	if err != nil {
		return roundUsage{}, err
	}
	if u.Review == nil {
		return roundUsage{}, fmt.Errorf("shedadapters: round %d usage record has no review half", round)
	}
	return u, nil
}
