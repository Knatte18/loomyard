// roundusage.go holds the usage record of each review round, round-<N>-usage.yaml.
// The record names the round's fan and lenses, each Burler half's model, run times and token reading.
// It also holds the Bouncer's seed and judge passes.

package shedadapters

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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
// Fan and FixFan are the reviewer's and fixer's fans the round was spawned under, empty for a solo or single round.
// FansRecorded says they were written at the spawn, so an empty name is told from an absent one.
// Lenses is nil for a solo round.
// FixLenses and FixForks are set only for a twill fixer; FixForks is a pointer so a single round carries no count.
// Review and Fix stay nil until the round's Burler completes;
// a record has a review half exactly when its round is complete.
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
	// Attempt is the token the round's review table last spawned under: the round number, plus b on a retry.
	// Assist and AssistFan are the assistant's model choice and fan it spawned with; Assist is nil with no assistant.
	// A record without Attempt, from a binary that predates the field, reads as no assistant and the bare round.
	Attempt   string        `yaml:"attempt,omitempty"`
	Assist    *assistRecord `yaml:"assist,omitempty"`
	AssistFan string        `yaml:"assist_fan,omitempty"`
	// AssistEnding is the assistant's end marker once the round is accepted; AssistLensesMissing lists unreported lenses.
	// A round whose AssistEnding reads done and names no lens ran its whole fan; AssistFan alone does not show that.
	AssistEnding        string   `yaml:"assist_ending,omitempty"`
	AssistLensesMissing []string `yaml:"assist_lenses_missing,omitempty"`
}

// assistRecord is the assistant model choice a round's review table spawned with.
type assistRecord struct {
	Engine  string `yaml:"engine,omitempty"`
	Model   string `yaml:"model"`
	Effort  string `yaml:"effort,omitempty"`
	Version string `yaml:"version,omitempty"`
}

// roundAttempt is what a round's review table spawns under: the attempt token and the assistant choice and fan.
// The zero choice runs no assistant.
type roundAttempt struct {
	token     string
	assist    shuttleengine.ModelChoice
	assistFan string
}

// recordRoundAttempt writes the attempt a round's review table is about to spawn under.
// A probe or resume after an interruption then builds the same table, whatever the config says by then.
// It is the only writer of the Attempt, Assist and AssistFan fields.
func recordRoundAttempt(runDir string, round int, now func() time.Time, attempt roundAttempt) error {
	return mergeRoundUsage(runDir, round, now, func(u *roundUsage) {
		u.Attempt, u.Assist, u.AssistFan = attempt.token, nil, ""
		if attempt.assist.Model != "" {
			u.Assist = &assistRecord{Engine: attempt.assist.Engine, Model: attempt.assist.Model, Effort: attempt.assist.Effort, Version: attempt.assist.Version}
			u.AssistFan = attempt.assistFan
		}
	})
}

// readRoundAttempt returns the attempt round's review table last spawned under.
// A missing or undecodable record, and one without an attempt token, read as the bare round with no assistant.
func readRoundAttempt(runDir string, round int) roundAttempt {
	bare := roundAttempt{token: strconv.Itoa(round)}
	u, err := readRoundUsage(runDir, round)
	if err != nil || u.Attempt == "" {
		return bare
	}
	attempt := roundAttempt{token: u.Attempt}
	if u.Assist != nil {
		attempt.assist = shuttleengine.ModelChoice{Engine: u.Assist.Engine, Model: u.Assist.Model, Effort: u.Assist.Effort, Version: u.Assist.Version}
		attempt.assistFan = u.AssistFan
	}
	return attempt
}

// halfUsage is one half's engine, model, run times and token reading.
// Engine is the engine the half's run used; it is absent for a half with no run record.
// A token field is nil when the reading is unknown.
// WallSeconds is nil when either run time is zero, so an unknown value is never rendered as zero.
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

// newPassUsage builds one Bouncer pass's entry from the shuttle result's run times and token reading.
// The model is the choice the pass ran on; the fork fields stay unset.
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
// A missing record starts from the zero value.
// A record that does not decode is archived aside and logged,
// and the write starts over with only this writer's part.
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

// recordRoundFans writes the fan names round's reviewer and fixer are spawned under.
// It writes the FansRecorded flag with them, so the names are known to be there.
// It is the only writer of Fan, FixFan and FansRecorded.
// The empty names of a solo or single round are recorded like any other name.
func recordRoundFans(runDir string, round int, now func() time.Time, fan, fixFan string) error {
	return mergeRoundUsage(runDir, round, now, func(u *roundUsage) {
		u.Fan, u.FixFan, u.FansRecorded = fan, fixFan, true
	})
}

// readRecordedFans returns the reviewer and fixer fan names recorded for round.
// A missing record, an undecodable one and one without the flag are errors:
// a record a binary wrote before the names existed decodes the reviewer's fan as the empty string.
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

// roundReviewStart returns the start of round's review half from its usage record under runDir.
// It returns the zero time for round 0, an absent or undecodable record, a record with no review half and a zero start;
// a read or decode failure is logged at Warn, since a missing start only means the start is unknown.
func roundReviewStart(runDir string, round int) time.Time {
	if round == 0 {
		return time.Time{}
	}
	u, err := readRoundUsage(runDir, round)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("shedadapters: round usage record unreadable; reading the round start as unknown", "round", round, "error", err)
		}
		return time.Time{}
	}
	if u.Review == nil {
		return time.Time{}
	}
	return u.Review.Start
}

// readCompletedRoundUsage decodes round's usage record under runDir and requires its Burler half.
// A record with no review half is an error naming the round, since only the Bouncer has written to it.
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
