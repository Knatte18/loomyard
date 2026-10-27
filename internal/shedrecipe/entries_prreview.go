// entries_prreview.go implements the PRReview registry entry: it reads a row's one setting reference and builds the PR-Review stub.

package shedrecipe

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	// prReviewEngine is the registry name and error label of the PRReview entry.
	prReviewEngine = "PRReview"
	// prReviewSettingKey is the row's one config key, a module.key reference to the stub's on/off setting.
	prReviewSettingKey = "pr_review_setting"

	prReviewKeyWayForward     = "way forward: remove the key; a PRReview row takes pr_review_setting"
	prReviewMissingWayForward = "way forward: set pr_review_setting to a module.key reference such as loom.pr_review"
)

// prReviewEntry is the Constructor for the "PRReview" registry row.
// The row's required pr_review_setting key is a module.key reference, read through Env.Setting as a bool, that says whether the stub logs on each call.
func prReviewEntry(row RowIdentity, cfg Config, env Env) (shedengine.ShedProducer, error) {
	if err := configRejectUnknown(cfg, prReviewSettingKey); err != nil {
		return nil, fmt.Errorf("%w; row %q: %s", err, row.Name, prReviewKeyWayForward)
	}
	if _, err := configString(cfg, prReviewSettingKey, true); err != nil {
		return nil, fmt.Errorf("shedrecipe: %s: row %q: %w; %s", prReviewEngine, row.Name, err, prReviewMissingWayForward)
	}
	value, ref, _, err := readSetting(prReviewEngine, row, cfg, prReviewSettingKey, env)
	if err != nil {
		return nil, err
	}
	switch enabled := value.(type) {
	case nil:
		return landingshed.NewPRReview(row.Name, false), nil
	case bool:
		return landingshed.NewPRReview(row.Name, enabled), nil
	default:
		return nil, fmt.Errorf("shedrecipe: %s: row %q: setting %q (%s) holds a %T, want a bool; way forward: set %s to true or false through `lym config %s` from the prime", prReviewEngine, row.Name, ref.Text, ref, value, ref.Text, ref.Module)
	}
}
