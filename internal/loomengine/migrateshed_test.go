// migrateshed_test.go pins MigrateConfig's step that moves the driver and tackler keys off the retired template's defaults.

package loomengine

import (
	"strings"
	"testing"
)

// TestMigrateConfig_ShedOverrides verifies, over whole documents with a second pass on each output, that the retired template's four lines become the inherit values with the current comments.
// A pinned value keeps its value and gains the comment, and a hand-edited comment is kept.
// A commentless run_settings document and a current-template document are returned unchanged.
func TestMigrateConfig_ShedOverrides(t *testing.T) {
	t.Parallel()

	retiredLine := func(key, value string) string {
		for _, retired := range retiredShedKeyComments {
			if retired.key == key {
				return key + ": " + value + "  " + retired.comment + "\n"
			}
		}
		t.Fatalf("no retired comment for key %q", key)
		return ""
	}
	// currentLine is key's line in the current template with value, in the single-space comment form the migration's encoder writes.
	currentLine := func(key, value string) string {
		for _, line := range strings.SplitAfter(ConfigTemplate(), "\n") {
			if strings.HasPrefix(line, key+": ") {
				return key + ": " + value + " " + line[strings.Index(line, "# "):]
			}
		}
		t.Fatalf("no template line for key %q", key)
		return ""
	}
	retiredDocument := func(spawns string) string {
		return retiredLine("driver_kind", "llm") + retiredLine("tackler", "haiku") + retiredLine("tackler_timeout_min", "10") + retiredLine("tackler_max_spawns", spawns)
	}
	inheritDocument := currentLine("driver_kind", `""`) + currentLine("tackler", `""`) + currentLine("tackler_timeout_min", "0") + currentLine("tackler_max_spawns", "~")
	handEdited := "driver_kind: llm # kept by hand\n"

	tests := []struct {
		name      string
		input     string
		want      string
		wantLines int
	}{
		{name: "the retired lines become the inherit values with the current comments", input: retiredDocument("4"), want: inheritDocument, wantLines: 5},
		{
			name:      "a tackler_max_spawns of 0 keeps every value and every key gains the current comment",
			input:     retiredDocument("0"),
			want:      currentLine("driver_kind", "llm") + currentLine("tackler", "haiku") + currentLine("tackler_timeout_min", "10") + currentLine("tackler_max_spawns", "0"),
			wantLines: 4,
		},
		{
			name:      "a hand-edited driver_kind comment keeps every value and the comment, and the other three gain the current comment",
			input:     handEdited + retiredLine("tackler", "haiku") + retiredLine("tackler_timeout_min", "10") + retiredLine("tackler_max_spawns", "4"),
			want:      handEdited + currentLine("tackler", "haiku") + currentLine("tackler_timeout_min", "10") + currentLine("tackler_max_spawns", "4"),
			wantLines: 3,
		},
		{name: "old values without comments are returned unchanged", input: "driver_kind: llm\ntackler: haiku\ntackler_timeout_min: 10\ntackler_max_spawns: 4\n", want: "driver_kind: llm\ntackler: haiku\ntackler_timeout_min: 10\ntackler_max_spawns: 4\n"},
		{name: "a document of the current template's lines is returned unchanged", input: inheritDocument, want: inheritDocument},
		{name: "a document holding none of the keys is returned unchanged", input: "step_idle_timeout_min: 60\n", want: "step_idle_timeout_min: 60\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, lines, err := migrateShedOverrides([]byte(tt.input))
			if err != nil {
				t.Fatalf("migrateShedOverrides() = _, _, %v; want nil error", err)
			}
			if string(got) != tt.want {
				t.Errorf("migrateShedOverrides() = %q; want %q", got, tt.want)
			}
			if len(lines) != tt.wantLines {
				t.Errorf("migrateShedOverrides() reported %d lines %q; want %d", len(lines), lines, tt.wantLines)
			}
			again, againLines, err := migrateShedOverrides(got)
			if err != nil {
				t.Fatalf("second migrateShedOverrides() = _, _, %v; want nil error", err)
			}
			if string(again) != string(got) || len(againLines) != 0 {
				t.Errorf("second migrateShedOverrides() = %q with lines %q; want the document unchanged and no lines", again, againLines)
			}
		})
	}
}
