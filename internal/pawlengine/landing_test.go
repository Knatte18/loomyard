// landing_test.go covers a beam's landing record: its round trip, which records belong to which beam, and what a drop and a failed update leave on disk.

package pawlengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landingPaths returns the record path and lock path of a fresh temp directory, laid out as a worktree's tracked record and never-tracked lock.
func landingPaths(t *testing.T) (path, lockPath string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "_lym", "landing", "beam-landing.json"), filepath.Join(dir, ".lym", "locks", "_lym", "landing", "beam-landing.json.lock")
}

func TestLandingRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	describedAt := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	withPullRequest := func(rec *pawlengine.LandingRecord) error {
		rec.DescribedAt = describedAt
		rec.PullRequest = &pawlengine.LandingPullRequest{Number: 7, URL: "https://example.test/pull/7", PushedHead: "aaa"}
		return nil
	}

	t.Run("RoundTripWithAndWithoutPullRequest", func(t *testing.T) {
		t.Parallel()
		path, lockPath := landingPaths(t)
		if err := pawlengine.UpdateLanding(path, lockPath, "b", func(rec *pawlengine.LandingRecord) error {
			rec.DescribedAt = describedAt
			return nil
		}); err != nil {
			t.Fatalf("UpdateLanding() = %v; want nil", err)
		}
		got, found, err := pawlengine.ReadLanding(path, lockPath, "b")
		if err != nil || !found {
			t.Fatalf("ReadLanding(b) = _, %t, %v; want a record", found, err)
		}
		if want := (pawlengine.LandingRecord{Slug: "b", DescribedAt: describedAt}); !reflect.DeepEqual(got, want) {
			t.Errorf("ReadLanding(b) = %+v; want %+v", got, want)
		}

		if err := pawlengine.UpdateLanding(path, lockPath, "b", withPullRequest); err != nil {
			t.Fatalf("UpdateLanding() = %v; want nil", err)
		}
		got, _, err = pawlengine.ReadLanding(path, lockPath, "b")
		if err != nil {
			t.Fatalf("ReadLanding(b) = _, _, %v; want nil", err)
		}
		want := pawlengine.LandingRecord{Slug: "b", DescribedAt: describedAt, PullRequest: &pawlengine.LandingPullRequest{Number: 7, URL: "https://example.test/pull/7", PushedHead: "aaa"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ReadLanding(b) = %+v; want %+v", got, want)
		}
	})

	t.Run("UpdateOverAbsentFileStartsFromTheSlug", func(t *testing.T) {
		t.Parallel()
		path, lockPath := landingPaths(t)
		var seen pawlengine.LandingRecord
		if err := pawlengine.UpdateLanding(path, lockPath, "b", func(rec *pawlengine.LandingRecord) error {
			seen = *rec
			return nil
		}); err != nil {
			t.Fatalf("UpdateLanding() = %v; want nil", err)
		}
		if want := (pawlengine.LandingRecord{Slug: "b"}); !reflect.DeepEqual(seen, want) {
			t.Errorf("mutate saw %+v; want %+v", seen, want)
		}
		if _, found, err := pawlengine.ReadLanding(path, lockPath, "b"); err != nil || !found {
			t.Errorf("ReadLanding(b) after the update = _, %t, %v; want a record", found, err)
		}
	})

	t.Run("UpdateOverAnotherSlugsRecordReplacesIt", func(t *testing.T) {
		t.Parallel()
		path, lockPath := landingPaths(t)
		if err := pawlengine.UpdateLanding(path, lockPath, "outer", withPullRequest); err != nil {
			t.Fatalf("UpdateLanding(outer) = %v; want nil", err)
		}
		var seen pawlengine.LandingRecord
		if err := pawlengine.UpdateLanding(path, lockPath, "b", func(rec *pawlengine.LandingRecord) error {
			seen = *rec
			return nil
		}); err != nil {
			t.Fatalf("UpdateLanding(b) = %v; want nil", err)
		}
		want := pawlengine.LandingRecord{Slug: "b"}
		if !reflect.DeepEqual(seen, want) {
			t.Errorf("mutate saw %+v; want %+v, the inherited record replaced", seen, want)
		}
		got, found, err := pawlengine.ReadLanding(path, lockPath, "b")
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Errorf("ReadLanding(b) = %+v, %t, %v; want %+v, true, nil", got, found, err, want)
		}
	})

	t.Run("ReadOfAnotherSlugsRecordIsNotFoundAndKeepsTheFile", func(t *testing.T) {
		t.Parallel()
		path, lockPath := landingPaths(t)
		if err := pawlengine.UpdateLanding(path, lockPath, "outer", withPullRequest); err != nil {
			t.Fatalf("UpdateLanding(outer) = %v; want nil", err)
		}
		before := readFile(t, path)
		if _, found, err := pawlengine.ReadLanding(path, lockPath, "b"); err != nil || found {
			t.Errorf("ReadLanding(b) = _, %t, %v; want not found", found, err)
		}
		if after := readFile(t, path); string(after) != string(before) {
			t.Errorf("record bytes after the read = %q; want %q", after, before)
		}
	})

	t.Run("AMutateErrorWritesNothing", func(t *testing.T) {
		t.Parallel()
		path, lockPath := landingPaths(t)
		refusal := errors.New("refused")
		if err := pawlengine.UpdateLanding(path, lockPath, "b", func(*pawlengine.LandingRecord) error { return refusal }); !errors.Is(err, refusal) {
			t.Errorf("UpdateLanding() over no record = %v; want the mutate error", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("stat of the record after a failed first update = %v; want it absent", err)
		}

		if err := pawlengine.UpdateLanding(path, lockPath, "b", withPullRequest); err != nil {
			t.Fatalf("UpdateLanding() = %v; want nil", err)
		}
		before := readFile(t, path)
		if err := pawlengine.UpdateLanding(path, lockPath, "b", func(rec *pawlengine.LandingRecord) error {
			rec.PullRequest = nil
			return refusal
		}); !errors.Is(err, refusal) {
			t.Errorf("UpdateLanding() over a record = %v; want the mutate error", err)
		}
		if after := readFile(t, path); string(after) != string(before) {
			t.Errorf("record bytes after a failed update = %q; want %q", after, before)
		}
	})

	t.Run("DropClearsOnlyTheBeamsPullRequest", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// written is the slug of the record written first; empty writes none.
			written     string
			wantDropped bool
		}{
			{name: "OwnRecord", written: "b", wantDropped: true},
			{name: "AnotherSlugsRecord", written: "outer"},
			{name: "AbsentFile"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				path, lockPath := landingPaths(t)
				var before []byte
				if tt.written != "" {
					if err := pawlengine.UpdateLanding(path, lockPath, tt.written, withPullRequest); err != nil {
						t.Fatalf("UpdateLanding(%s) = %v; want nil", tt.written, err)
					}
					before = readFile(t, path)
				}
				dropped, err := pawlengine.DropLandingPullRequest(path, lockPath, "b")
				if err != nil || dropped != tt.wantDropped {
					t.Fatalf("DropLandingPullRequest(b) after a record of %q = %t, %v; want %t, nil", tt.written, dropped, err, tt.wantDropped)
				}
				if tt.wantDropped {
					got, _, err := pawlengine.ReadLanding(path, lockPath, "b")
					if err != nil {
						t.Fatalf("ReadLanding(b) = _, _, %v; want nil", err)
					}
					if want := (pawlengine.LandingRecord{Slug: "b", DescribedAt: describedAt}); !reflect.DeepEqual(got, want) {
						t.Errorf("ReadLanding(b) after the drop = %+v; want %+v, the description time kept", got, want)
					}
					return
				}
				if tt.written == "" {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("stat of the record after a drop over no record = %v; want it absent", err)
					}
					return
				}
				if after := readFile(t, path); string(after) != string(before) {
					t.Errorf("record bytes after a drop over another slug's record = %q; want %q", after, before)
				}
			})
		}
	})
}

// readFile returns the bytes of the file at path, failing the test when it cannot be read.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
