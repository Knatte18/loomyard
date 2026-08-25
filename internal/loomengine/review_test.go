// review_test.go — untagged Tier-1 unit tests for ResolveReview and LoomReviewsDir.
// ResolveReview's tests mirror discussion_test.go's shape: pure Go over an in-memory Config and a
// temp-dir modelspec registry, no live hub, reed, or network involved.
// LoomReviewsDir's test mirrors discussionpath_test.go's shape: pure path arithmetic over a
// hand-built lyxcwd.Location.

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

// TestLoomReviewsDir verifies LoomReviewsDir's returned path is AnchorPath-anchored, sits under the
// ephemeral .lyx tree rather than the durable one, and mirrors the loom subdirectory LoomScratchDir
// already names.
func TestLoomReviewsDir(t *testing.T) {
	l := &lyxcwd.Location{
		HubPath:      filepath.Join("home", "user", "repo-HUB"),
		WorktreeName: "repo",
		// AnchorRel deliberately differs from "." to prove the accessor follows the
		// anchored subpath, not the bare worktree root.
		AnchorRel: filepath.Join("sub", "dir"),
	}

	want := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName, "loom", "reviews")
	if got := LoomReviewsDir(l); got != want {
		t.Errorf("LoomReviewsDir() = %q; want %q", got, want)
	}

	if got := LoomReviewsDir(l); filepath.Dir(got) != LoomScratchDir(l) {
		t.Errorf("LoomReviewsDir() = %q; want its parent to equal LoomScratchDir() = %q", got, LoomScratchDir(l))
	}
}
