package websterengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteRefusalNote(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		n := RefusalNote{
			Verb:    "begin-batch",
			Args:    "begin-batch 03",
			Message: "plan drifted: run rebaseline",
			Fields:  map[string]any{"plan_drifted": true, "batch": "03-x"},
		}
		if err := WriteRefusalNote(dir, n); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-refusal-begin-batch.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "lyx webster begin-batch refused\n\n" +
			"Arguments: begin-batch 03\n\n" +
			"```\nplan drifted: run rebaseline\n```\n\n" +
			"batch: 03-x\nplan_drifted: true\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := WriteRefusalNote("", RefusalNote{Verb: "begin-batch", Message: "m"}); err != nil {
			t.Fatal(err)
		}
		if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
			t.Errorf("wrote %d entries; want none", len(entries))
		}
	})
	t.Run("same verb twice", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		n := RefusalNote{Verb: "record-batch", Args: "record-batch 01", Message: "m"}
		for range 2 {
			if err := WriteRefusalNote(dir, n); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"webster-refusal-record-batch.md", "webster-refusal-record-batch-2.md"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("missing %s: %v", name, err)
			}
		}
	})
	t.Run("no fields", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := WriteRefusalNote(dir, RefusalNote{Verb: "status", Args: "status", Message: "boom"}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-refusal-status.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "lyx webster status refused\n\nArguments: status\n\n```\nboom\n```\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
}
