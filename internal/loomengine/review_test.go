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
	cfg := Config{Review: "opus[effort=high]", ReviewTimeoutMin: 240}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	settings, err := ResolveReview(cfg, reg)
	if err != nil {
		t.Fatalf("ResolveReview(...) = _, %v; want nil error", err)
	}
	if settings.Model == "" {
		t.Error("ResolveReview(...).Model = \"\"; want non-empty")
	}
	if settings.Effort != "high" {
		t.Errorf("ResolveReview(...).Effort = %q; want %q", settings.Effort, "high")
	}
	wantTimeout := 240 * time.Minute
	if settings.Timeout != wantTimeout {
		t.Errorf("ResolveReview(...).Timeout = %s; want %s", settings.Timeout, wantTimeout)
	}
}

// TestResolveReview_MalformedSpec verifies an ungrammatical review model-spec returns an error
// naming the review role, rather than being silently carried into a review producer's spawn site.
func TestResolveReview_MalformedSpec(t *testing.T) {
	cfg := Config{Review: "opus[effort", ReviewTimeoutMin: 240}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	_, err = ResolveReview(cfg, reg)
	if err == nil {
		t.Fatal("ResolveReview(...) = _, nil; want non-nil error for malformed review spec")
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("ResolveReview(...) error = %q; want it to name the review role", err.Error())
	}
}

// TestResolveJudge verifies ResolveJudge resolves the template's judge value to the sonnet model
// with effort medium.
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

// TestResolveJudge_MalformedSpec verifies an ungrammatical judge model-spec returns an error
// naming the judge role.
func TestResolveJudge_MalformedSpec(t *testing.T) {
	cfg := Config{Judge: "sonnet[medium"}

	reg, err := modelspec.LoadRegistry(t.TempDir())
	if err != nil {
		t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
	}

	_, err = ResolveJudge(cfg, reg)
	if err == nil {
		t.Fatal("ResolveJudge(...) = _, nil; want non-nil error for malformed judge spec")
	}
	if !strings.Contains(err.Error(), "judge") {
		t.Errorf("ResolveJudge(...) error = %q; want it to name the judge role", err.Error())
	}
}

// TestLoomReviewsDirRel verifies LoomReviewsDirRel's exact relative value under the durable tree.
func TestLoomReviewsDirRel(t *testing.T) {
	want := filepath.Join(lyxdirs.LyxDirName, "reviews")
	if got := LoomReviewsDirRel(); got != want {
		t.Errorf("LoomReviewsDirRel() = %q; want %q", got, want)
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

// TestLoomReviewsDir verifies LoomReviewsDir's returned path is AnchorPath-anchored, sits under the durable _lyx tree rather than the ephemeral one, and equals the anchor joined with its Rel form.
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
}
