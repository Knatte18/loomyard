package vscode

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSettings(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		dir := t.TempDir()
		want := []byte("\xEF\xBB\xBF{\n  // c\n  \"a\": 1,\n}\n")
		if err := os.MkdirAll(filepath.Join(dir, ".vscode"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".vscode", "settings.json"), want, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ReadSettings(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
	t.Run("absent", func(t *testing.T) {
		got, err := ReadSettings(t.TempDir())
		if err != nil || got != nil {
			t.Fatalf("got %q, %v; want nil, nil", got, err)
		}
	})
	t.Run("directory at path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".vscode", "settings.json")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := ReadSettings(dir)
		if err == nil {
			t.Fatalf("got %q, nil; want error", got)
		}
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("error %q does not name %s", err, path)
		}
	})
}
