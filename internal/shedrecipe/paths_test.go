package shedrecipe

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveUnderRoot(t *testing.T) {
	t.Run("PlainRelativeValueJoinsUnderRoot", func(t *testing.T) {
		root := t.TempDir()
		got, err := resolveUnderRoot("MyEntry", "mykey", root, "sub/file.txt")
		if err != nil {
			t.Fatalf("resolveUnderRoot() error = %v; want nil", err)
		}
		want := filepath.Join(root, "sub/file.txt")
		if got != want {
			t.Errorf("resolveUnderRoot() = %q; want %q", got, want)
		}
	})

	t.Run("NestedRelativeValueJoinsUnderRoot", func(t *testing.T) {
		root := t.TempDir()
		got, err := resolveUnderRoot("MyEntry", "mykey", root, "a/b/c.txt")
		if err != nil {
			t.Fatalf("resolveUnderRoot() error = %v; want nil", err)
		}
		want := filepath.Join(root, "a/b/c.txt")
		if got != want {
			t.Errorf("resolveUnderRoot() = %q; want %q", got, want)
		}
	})

	t.Run("EmptyValueErrors", func(t *testing.T) {
		root := t.TempDir()
		_, err := resolveUnderRoot("MyEntry", "mykey", root, "")
		if err == nil {
			t.Fatalf("resolveUnderRoot() error = nil; want non-nil")
		}
	})

	t.Run("AbsoluteValueErrorsAndNamesKey", func(t *testing.T) {
		root := t.TempDir()
		_, err := resolveUnderRoot("MyEntry", "mykey", root, "/abs/path")
		if err == nil {
			t.Fatalf("resolveUnderRoot() error = nil; want non-nil")
		}
		if !strings.Contains(err.Error(), "mykey") {
			t.Errorf("resolveUnderRoot() error = %v; want it to name key %q", err, "mykey")
		}
	})

	t.Run("EscapingValueErrors", func(t *testing.T) {
		root := t.TempDir()
		_, err := resolveUnderRoot("MyEntry", "mykey", root, "../escape")
		if err == nil {
			t.Fatalf("resolveUnderRoot() error = nil; want non-nil")
		}
	})

	t.Run("DotDotStayingInsideRootResolves", func(t *testing.T) {
		root := t.TempDir()
		got, err := resolveUnderRoot("MyEntry", "mykey", root, "sub/../file.txt")
		if err != nil {
			t.Fatalf("resolveUnderRoot() error = %v; want nil", err)
		}
		want := filepath.Join(root, "file.txt")
		if got != want {
			t.Errorf("resolveUnderRoot() = %q; want %q", got, want)
		}
	})
}
