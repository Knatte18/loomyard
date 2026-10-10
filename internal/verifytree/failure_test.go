// failure_test.go covers the Publish failure record codec without spawning anything.

package verifytree

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPublishFailureRecord(t *testing.T) {
	t.Parallel()

	newPaths := func(t *testing.T) Paths {
		t.Helper()
		dir := t.TempDir()
		return NewPaths(filepath.Join(dir, "worktree"), dir)
	}

	t.Run("absent reads as none", func(t *testing.T) {
		t.Parallel()
		_, ok, err := ReadPublishFailure(newPaths(t))
		if ok || err != nil {
			t.Fatalf("ReadPublishFailure = (_, %v, %v); want (_, false, nil)", ok, err)
		}
	})

	t.Run("round trip keeps a log copy that survives a rewrite of the log", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		if err := os.WriteFile(p.Log, []byte("first run output"), 0o644); err != nil {
			t.Fatal(err)
		}
		want := PublishFailure{
			Kind:        FailureKindPublishVerify,
			Tests:       []FailedTest{{Package: "example.com/m/a", Test: "TestA/sub"}},
			Head:        "abc123",
			MergeCommit: "def456",
		}
		if err := WritePublishFailure(p, want); err != nil {
			t.Fatalf("WritePublishFailure: %v", err)
		}
		if err := os.WriteFile(p.Log, []byte("later run output"), 0o644); err != nil {
			t.Fatal(err)
		}

		got, ok, err := ReadPublishFailure(p)
		if !ok || err != nil {
			t.Fatalf("ReadPublishFailure = (_, %v, %v); want (_, true, nil)", ok, err)
		}
		want.LogPath = p.PublishFailureLog
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("record = %+v; want %+v", got, want)
		}
		copied, err := os.ReadFile(got.LogPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != "first run output" {
			t.Fatalf("log copy = %q; want the log as it was when the record was written", copied)
		}
	})

	t.Run("malformed record is an error", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		if err := os.WriteFile(p.PublishFailure, []byte("kind: [unclosed"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := ReadPublishFailure(p); ok || err == nil {
			t.Fatalf("ReadPublishFailure = (_, %v, %v); want (_, false, error)", ok, err)
		}
	})

	t.Run("removal clears both files and is idempotent", func(t *testing.T) {
		t.Parallel()
		p := newPaths(t)
		if err := os.WriteFile(p.Log, []byte("output"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WritePublishFailure(p, PublishFailure{Kind: FailureKindPlanVerify, Head: "abc123"}); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := RemovePublishFailure(p); err != nil {
				t.Fatalf("RemovePublishFailure: %v", err)
			}
		}
		for _, path := range []string{p.PublishFailure, p.PublishFailureLog} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("%s still present after removal: %v", path, err)
			}
		}
	})
}
