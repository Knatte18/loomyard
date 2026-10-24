//go:build tier0

// failure_test.go covers the landing failure record codec without spawning anything.

package verifytree

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLandingFailureRecord(t *testing.T) {
	t.Parallel()

	newPaths := func(t *testing.T) Paths {
		t.Helper()
		dir := t.TempDir()
		return NewPaths(filepath.Join(dir, "worktree"), dir)
	}

	t.Run("absent reads as none", func(t *testing.T) {
		t.Parallel()
		_, ok, err := ReadLandingFailure(newPaths(t))
		if ok || err != nil {
			t.Fatalf("ReadLandingFailure = (_, %v, %v); want (_, false, nil)", ok, err)
		}
	})

	for _, tier := range []int{0, 2} {
		t.Run(fmt.Sprintf("round trip keeps the caller's log path and tier%d", tier), func(t *testing.T) {
			t.Parallel()
			p := newPaths(t)
			want := LandingFailure{
				Kind:        FailureKindPublish,
				Tier:        tier,
				Tests:       []FailedTest{{Package: "example.com/m/a", Test: "TestA/sub"}},
				LogPath:     filepath.Join(p.Dir, "verify-3.log"),
				Head:        "abc123",
				MergeCommit: "def456",
				WrittenAt:   "2026-01-02T03:04:05Z",
			}
			if err := WriteLandingFailure(p, want); err != nil {
				t.Fatalf("WriteLandingFailure: %v", err)
			}
			written, err := os.ReadFile(p.LandingFailure)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(written), fmt.Sprintf("tier: %d\n", tier)) {
				t.Fatalf("record file = %q; want a tier: %d line even for tier 0", written, tier)
			}

			got, ok, err := ReadLandingFailure(p)
			if !ok || err != nil {
				t.Fatalf("ReadLandingFailure = (_, %v, %v); want (_, true, nil)", ok, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("record = %+v; want %+v", got, want)
			}
		})
	}

	t.Run("trace fields round-trip and an untraced record writes none of their keys", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		base := LandingFailure{Kind: FailureKindPublish, LogPath: filepath.Join(p.Dir, "verify-1.log"), Head: "abc123"}
		if err := WriteLandingFailure(p, base); err != nil {
			t.Fatal(err)
		}
		written, err := os.ReadFile(p.LandingFailure)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"cards", "later_range", "untraced"} {
			if strings.Contains(string(written), key+":") {
				t.Errorf("record without a trace = %q; want no %q key", written, key)
			}
		}

		want := base
		want.Cards = []TracedCard{
			{Number: 3, Glyphs: []string{"internal/a#F"}, Locations: []string{"internal/a/a.go:12"}, Batch: 2, Range: CommitRange{From: "aaa", To: "bbb"}},
			{Number: 5, Glyphs: []string{"internal/b#G"}, Locations: []string{"internal/b/b.go:7"}},
		}
		want.LaterRange = CommitRange{From: "ccc", To: "ddd"}
		want.Untraced = []string{"internal/c/c.go:4"}
		if err := WriteLandingFailure(p, want); err != nil {
			t.Fatal(err)
		}
		got, ok, err := ReadLandingFailure(p)
		if !ok || err != nil {
			t.Fatalf("ReadLandingFailure = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("record = %+v; want %+v", got, want)
		}
	})

	t.Run("a record written before the tier field decodes with tier 0, and its row and target keys are ignored", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		if err := os.WriteFile(p.LandingFailure, []byte("kind: plan_verify\nrow: Merge-In\ntarget: Webster-Burler\nlog_path: verify-1.log\nhead: abc123\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok, err := ReadLandingFailure(p)
		if !ok || err != nil {
			t.Fatalf("ReadLandingFailure = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		if got.Tier != 0 || got.Kind != FailureKindPlanVerify {
			t.Fatalf("record = %+v; want tier 0 and kind %q", got, FailureKindPlanVerify)
		}
		if got.Cards != nil || got.LaterRange != (CommitRange{}) || got.Untraced != nil {
			t.Fatalf("record = %+v; want empty trace fields", got)
		}
	})

	t.Run("malformed record is an error", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		if err := os.WriteFile(p.LandingFailure, []byte("kind: [unclosed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := ReadLandingFailure(p); ok || err == nil {
			t.Fatalf("ReadLandingFailure = (_, %v, %v); want (_, false, error)", ok, err)
		}
	})

	t.Run("removal clears the record, leaves its log and is idempotent", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		log := filepath.Join(p.Dir, "verify-1.log")
		if err := os.WriteFile(log, []byte("output"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteLandingFailure(p, LandingFailure{Kind: FailureKindPlanVerify, LogPath: log, Head: "abc123"}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := RemoveLandingFailure(p); err != nil {
				t.Fatalf("RemoveLandingFailure: %v", err)
			}
		}
		if _, err := os.Stat(p.LandingFailure); !os.IsNotExist(err) {
			t.Fatalf("%s still present after removal: %v", p.LandingFailure, err)
		}
		if _, err := os.Stat(log); err != nil {
			t.Fatalf("the record's log was removed with it: %v", err)
		}
	})
}
