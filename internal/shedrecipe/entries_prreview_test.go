//go:build tier0

// entries_prreview_test.go covers the PRReview entry: it builds the PR-Review row from an empty config and refuses every config key naming the way forward.

package shedrecipe

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/landingshed"
)

const prReviewTestRow = "PR-Review"

// newPRReviewTestEnv returns the plain test Env: the PRReview row reads no setting.
func newPRReviewTestEnv(t *testing.T) Env {
	t.Helper()
	return newTestEnv(t)
}

// prReviewTestConfig returns the valid Config of a PRReview row, which is empty.
func prReviewTestConfig(*testing.T, Env) Config {
	return Config{}
}

func TestPRReviewEntry(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfigBuildsThePRReviewRow", func(t *testing.T) {
		t.Parallel()
		producer, err := prReviewEntry(RowIdentity{Name: prReviewTestRow}, Config{}, newPRReviewTestEnv(t))
		if err != nil {
			t.Fatalf("prReviewEntry() = %v; want nil", err)
		}
		if _, ok := producer.(*landingshed.PRReview); !ok {
			t.Errorf("prReviewEntry() built a %T; want a *landingshed.PRReview", producer)
		}
	})

	refusals := []struct {
		name string
		cfg  Config
		want string
	}{
		{"GateKey", Config{"gate_key": "pr-review"}, "gate_key"},
		{"RetiredSettingKey", Config{"pr_review_setting": "loom.pr_review"}, "pr_review_setting"},
	}
	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := prReviewEntry(RowIdentity{Name: prReviewTestRow}, tt.cfg, newPRReviewTestEnv(t))

			assertErrContains(t, err, prReviewTestRow)
			assertErrContains(t, err, tt.want)
			assertErrContains(t, err, "way forward: remove the key; a PRReview row takes no config")
		})
	}
}
