// triage_test.go drives the verify gate's flaky-failure helpers: the warning and the friction note.

package websterengine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// failOutput renders go test output failing the named tests in pkg, each at its subtest nesting depth.
func failOutput(pkg string, tests ...string) string {
	var b strings.Builder
	for _, name := range tests {
		indent := strings.Repeat("    ", strings.Count(name, "/"))
		b.WriteString(indent + "--- FAIL: " + name + " (0.00s)\n" + indent + "    x_test.go:1: boom " + name + "\n")
	}
	b.WriteString("FAIL\nFAIL\t" + pkg + "\t0.01s\n")
	return b.String()
}

func TestTriageWarnings(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"flaky", []string{"p.A", "p.B"}, []string{"integration verify: flaky failures passed or vanished on rerun: p.A, p.B"}},
		{"none", nil, nil},
	}
	for _, c := range cases {
		if got := triageWarnings(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q; want %q", c.name, got, c.want)
		}
	}
}

func TestWriteTriageFrictionNote(t *testing.T) {
	t.Run("content", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := writeTriageFrictionNote(dir, []string{"p.A", "p.B"}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "webster-verify-triage.md"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Integration-suite triage: non-regression verify failures\n\n" +
			"These identities failed the first verify run and passed or vanished on rerun (flaky):\n\n- p.A\n- p.B\n\n"
		if string(got) != want {
			t.Errorf("note = %q; want %q", got, want)
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		cwd := t.TempDir()
		t.Chdir(cwd)
		if err := writeTriageFrictionNote("", []string{"p.A"}); err != nil {
			t.Fatal(err)
		}
		if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
			t.Errorf("wrote %d entries; want none", len(entries))
		}
	})
	t.Run("no flaky", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "friction")
		if err := writeTriageFrictionNote(dir, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("friction dir exists (err = %v); want nothing written", err)
		}
	})
}
