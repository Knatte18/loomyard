// review_test.go — untagged Tier-1 unit tests for ResolveReview and LoomReviewsDir.
// ResolveReview's tests mirror discussion_test.go's shape: pure Go over an in-memory Config and a
// temp-dir modelspec registry, no live hub, reed, or network involved.
// LoomReviewsDir's test mirrors discussionpath_test.go's shape: pure path arithmetic over a hand-built lyxcwd.Location;
// the accessor is durable, rooted under _lyx.

package loomengine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
)

// TestResolveReview verifies ResolveReview returns the expected model/effort/version triple and
// timeout for the embedded template's own review values.
func TestResolveReview(t *testing.T) {
	cfg := Config{Review: ModelSpecList{"opus[effort=high]", "sonnet[medium]"}, Fix: ModelSpecList{"opus[effort=low]"}, ReviewTimeoutMin: 240}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveReview(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
	}
	if len(settings.Models.Review) != 2 || len(settings.Models.Fix) != 1 {
		t.Fatalf("ResolveReview(...).Models = %+v; want two review entries and one fix entry", settings.Models)
	}
	if settings.Models.Review[0].Model == "" {
		t.Error("ResolveReview(...).Models.Review[0].Model = \"\"; want non-empty")
	}
	if settings.Models.Review[0].Effort != "high" || settings.Models.Review[1].Effort != "medium" {
		t.Errorf("ResolveReview(...).Models.Review efforts = %q, %q; want high, medium", settings.Models.Review[0].Effort, settings.Models.Review[1].Effort)
	}
	if settings.Models.Fix[0].Effort != "low" {
		t.Errorf("ResolveReview(...).Models.Fix[0].Effort = %q; want %q", settings.Models.Fix[0].Effort, "low")
	}
	wantTimeout := 240 * time.Minute
	if settings.Timeout != wantTimeout {
		t.Errorf("ResolveReview(...).Timeout = %s; want %s", settings.Timeout, wantTimeout)
	}
}

// TestResolveJudge verifies ResolveJudge resolves the template's judge value to the sonnet model with effort medium.
func TestResolveJudge(t *testing.T) {
	cfg := Config{Judge: "sonnet[medium]"}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveJudge(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveJudge(...) = _, %v; want nil error", err)
	}
	if !strings.Contains(settings.Model, "sonnet") {
		t.Errorf("ResolveJudge(...).Model = %q; want a sonnet model", settings.Model)
	}
	if settings.Effort != "medium" {
		t.Errorf("ResolveJudge(...).Effort = %q; want %q", settings.Effort, "medium")
	}
}

// TestResolveReviewAndJudge_MalformedSpec verifies an ungrammatical review, fix or judge model-spec returns an error naming its role, rather than being silently carried into a producer's spawn site, and that a bad later review entry or a bad fix entry also names its entry index and the way forward.
func TestResolveReviewAndJudge_MalformedSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		role    string
		resolve func(Config, modelspec.Registry) error
		cfg     Config
		wantIn  []string
	}{
		{
			name: "review",
			role: "review",
			cfg:  Config{Review: ModelSpecList{"opus[effort"}, ReviewTimeoutMin: 240},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "later review entry",
			role:   "review",
			cfg:    Config{Review: ModelSpecList{"sonnet[medium]", "opus[effort"}, Fix: ModelSpecList{"opus[medium]"}},
			wantIn: []string{"entry 2", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name:   "fix entry",
			role:   "fix",
			cfg:    Config{Review: ModelSpecList{"sonnet[medium]"}, Fix: ModelSpecList{"opus[medium]", "opus[effort"}},
			wantIn: []string{"entry 2", "a model-spec the registry defines"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveReview(cfg, reg)
				return err
			},
		},
		{
			name: "judge",
			role: "judge",
			cfg:  Config{Judge: "sonnet[medium"},
			resolve: func(cfg Config, reg modelspec.Registry) error {
				_, err := ResolveJudge(cfg, reg)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg, err := modelspec.LoadRegistry(t.TempDir())
			if err != nil {
				t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
			}

			err = tt.resolve(tt.cfg, reg)
			if err == nil {
				t.Fatalf("resolving the %s role = nil error; want non-nil error for a malformed spec", tt.role)
			}
			if !strings.Contains(err.Error(), tt.role) {
				t.Errorf("resolving the %s role error = %q; want it to name the role", tt.role, err.Error())
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("resolving the %s role error = %q; want it to contain %q", tt.role, err.Error(), want)
				}
			}
		})
	}
}

// TestLoomParentReviewAccessors pins the three parent-review accessors: the durable relative and anchored forms sit beside the Discussion-Review run directory, and the lock directory is the .lyx mirror.
func TestLoomParentReviewAccessors(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		AnchorRel:    filepath.Join("sub", "dir"),
	}

	wantRel := filepath.Join(lyxdirs.LyxDirName, "reviews", "parent-review")
	if got := LoomParentReviewDirRel(); got != wantRel {
		t.Errorf("LoomParentReviewDirRel() = %q; want %q", got, wantRel)
	}
	if got, want := LoomParentReviewDir(l), filepath.Join(l.AnchorPath(), wantRel); got != want {
		t.Errorf("LoomParentReviewDir() = %q; want %q", got, want)
	}
	wantLock := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "reviews", "parent-review")
	if got := LoomParentReviewLockDir(l); got != wantLock {
		t.Errorf("LoomParentReviewLockDir() = %q; want %q", got, wantLock)
	}
}

// TestLoomReviewsDir verifies LoomReviewsDir's returned path is AnchorPath-anchored, sits under the durable _lyx tree rather than the ephemeral one, and equals the anchor joined with its Rel form, whose exact value is pinned too.
func TestLoomReviewsDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-LYXHUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor follows the
		// anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName, "reviews")
	if got := LoomReviewsDir(l); got != want {
		t.Errorf("LoomReviewsDir() = %q; want %q", got, want)
	}

	if got, rel := LoomReviewsDir(l), filepath.Join(l.AnchorPath(), LoomReviewsDirRel()); got != rel {
		t.Errorf("LoomReviewsDir() = %q; want it to equal %q", got, rel)
	}
	if got := LoomReviewsDirRel(); got != filepath.Join(lyxdirs.LyxDirName, "reviews") {
		t.Errorf("LoomReviewsDirRel() = %q; want %q", got, filepath.Join(lyxdirs.LyxDirName, "reviews"))
	}
}
