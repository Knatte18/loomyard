// entries_prreview_test.go covers the PRReview entry: the setting read, its refusals with their way forward, and the unknown-key refusal.

package shedrecipe

import (
	"context"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const prReviewTestRow = "PR-Review"

// newPRReviewTestEnv returns newTestEnv(t) with a setting lookup answering the stub's flag as off.
func newPRReviewTestEnv(t *testing.T) Env {
	t.Helper()
	env := newTestEnv(t)
	env.Setting = fakeSetting(map[string]any{"loom.pr_review": false})
	return env
}

// prReviewTestConfig returns the valid Config of a PRReview row.
func prReviewTestConfig(*testing.T, Env) Config {
	return Config{"pr_review_setting": "loom.pr_review"}
}

// TestPRReviewEntry stays serial: the built row's call is observed through logcapture, which redirects the process-global logger.
func TestPRReviewEntry(t *testing.T) {
	const notBuiltLine = "the PR review is enabled but not built"

	reads := []struct {
		name    string
		value   any
		wantLog bool
	}{
		{"TrueSetting", true, true},
		{"FalseSetting", false, false},
		{"AbsentValue", nil, false},
	}
	for _, tt := range reads {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.Setting = fakeSetting(map[string]any{"loom.pr_review": tt.value})
			logs := logcapture.CaptureVerbose(t)

			producer, err := prReviewEntry(RowIdentity{Name: prReviewTestRow}, Config{"pr_review_setting": "loom.pr_review"}, env)
			if err != nil {
				t.Fatalf("prReviewEntry() = %v; want nil", err)
			}
			outcome, _, err := producer.Call(context.Background())

			if err != nil || outcome != shedengine.Done {
				t.Fatalf("Call() = %q, %v; want Done, nil", outcome, err)
			}
			if got := strings.Contains(logs.String(), notBuiltLine); got != tt.wantLog {
				t.Errorf("Call() logged the not-built line = %v; want %v\n%s", got, tt.wantLog, logs.String())
			}
		})
	}

	refusals := []struct {
		name  string
		cfg   Config
		value any
		want  []string
	}{
		{"MissingKey", Config{}, true, []string{prReviewTestRow, "pr_review_setting", "way forward: set pr_review_setting to a module.key reference such as loom.pr_review"}},
		{"NonBoolValue", Config{"pr_review_setting": "loom.pr_review"}, "yes", []string{prReviewTestRow, "loom.pr_review", "way forward: set loom.pr_review to true or false through `lyx config loom` from the prime"}},
		{"UnknownKey", Config{"pr_review_setting": "loom.pr_review", "bogus_key": "x"}, true, []string{prReviewTestRow, "bogus_key", "way forward: remove the key; a PRReview row takes pr_review_setting"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.Setting = fakeSetting(map[string]any{"loom.pr_review": tt.value})

			_, err := prReviewEntry(RowIdentity{Name: prReviewTestRow}, tt.cfg, env)

			for _, want := range tt.want {
				assertErrContains(t, err, want)
			}
		})
	}
}
