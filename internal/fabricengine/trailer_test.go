// trailer_test.go — unit tests for the Warp-SHA trailer format/parse helpers.

package fabricengine

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestAppendParseWarpSHATrailer_RoundTrip covers append-then-parse round trips
// for a single-line subject and a multi-paragraph message.
func TestAppendParseWarpSHATrailer_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		message string
		warpSHA string
	}{
		{"single_line", "raddle: sync module docs", "a3f9c21e8b7d4f10"},
		{
			"multi_paragraph",
			"raddle: sync module docs\n\nExplains why the docs needed syncing across\nmultiple lines of body text.",
			"a3f9c21e8b7d4f10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appended := appendWarpSHATrailer(tt.message, tt.warpSHA)
			gotSHA, ok := parseWarpSHATrailer(appended)
			if !ok {
				t.Fatalf("parseWarpSHATrailer(%q) ok = false; want true", appended)
			}
			if gotSHA != tt.warpSHA {
				t.Errorf("parseWarpSHATrailer(%q) = %q; want %q", appended, gotSHA, tt.warpSHA)
			}
		})
	}
}

// TestAppendWarpSHATrailer_SubjectIsNeverATrailerBlock asserts that a
// single-line message -- even one shaped exactly like a trailer line, the
// "builder: <label>"/"webster: <label>" form every builder/webster weft
// commit uses -- gets its Warp-SHA trailer in a NEW blank-line-separated
// paragraph, keeping the git subject clean. Joining instead would fold the
// trailer into the subject paragraph and pollute `git log --oneline` for
// every such commit (the round fable-r1 regression).
func TestAppendWarpSHATrailer_SubjectIsNeverATrailerBlock(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			"builder_style_subject",
			"builder: poll 01-json-flag done",
			"builder: poll 01-json-flag done\n\nWarp-SHA: abc123",
		},
		{
			"plain_subject",
			"weft sync",
			"weft sync\n\nWarp-SHA: abc123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendWarpSHATrailer(tt.message, "abc123")
			if got != tt.want {
				t.Errorf("appendWarpSHATrailer(%q) = %q; want %q", tt.message, got, tt.want)
			}
		})
	}
}

// TestAppendWarpSHATrailer_JoinsExistingTrailerBlock asserts that appending to
// a message already ending in a trailer block (e.g. a prior Co-authored-by:)
// joins the new line directly, without introducing a stray blank line.
func TestAppendWarpSHATrailer_JoinsExistingTrailerBlock(t *testing.T) {
	message := "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>"
	want := "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>\nWarp-SHA: abc123"

	got := appendWarpSHATrailer(message, "abc123")
	if got != want {
		t.Errorf("appendWarpSHATrailer() = %q; want %q", got, want)
	}
}

// TestParseWarpSHATrailer_Absent asserts that parsing a message with no
// Warp-SHA trailer reports ok=false.
func TestParseWarpSHATrailer_Absent(t *testing.T) {
	message := "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>"
	sha, ok := parseWarpSHATrailer(message)
	if ok {
		t.Errorf("parseWarpSHATrailer(%q) = (%q, true); want ok=false", message, sha)
	}
}

// TestParseWarpSHATrailer_MultipleTrailersLastWins asserts that when a message
// carries more than one Warp-SHA trailer line, the last one wins.
func TestParseWarpSHATrailer_MultipleTrailersLastWins(t *testing.T) {
	message := "raddle: sync module docs\n\nWarp-SHA: first000\nWarp-SHA: second111"

	gotSHA, ok := parseWarpSHATrailer(message)
	if !ok {
		t.Fatalf("parseWarpSHATrailer(%q) ok = false; want true", message)
	}
	if gotSHA != "second111" {
		t.Errorf("parseWarpSHATrailer(%q) = %q; want %q", message, gotSHA, "second111")
	}
}

// TestParseWarpSHATrailer_TolerantOfSurroundingWhitespace asserts that
// leading/trailing whitespace around the trailer line and its value does not
// prevent extraction.
func TestParseWarpSHATrailer_TolerantOfSurroundingWhitespace(t *testing.T) {
	message := "raddle: sync module docs\n\n  Warp-SHA:   abc123   "

	gotSHA, ok := parseWarpSHATrailer(message)
	if !ok {
		t.Fatalf("parseWarpSHATrailer(%q) ok = false; want true", message)
	}
	if gotSHA != "abc123" {
		t.Errorf("parseWarpSHATrailer(%q) = %q; want %q", message, gotSHA, "abc123")
	}
}

// parseSnapshotTags scans message for every "Snapshot: <tag>" trailer line
// and returns the tags in the order they appear. It is a test-only helper
// mirroring parseWarpSHATrailer's scan, used to assert appendSnapshotTrailers
// round-trips.
func parseSnapshotTags(message string) []string {
	var tags []string
	prefix := SnapshotTrailerKey + ":"
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if value == "" {
			continue
		}
		tags = append(tags, value)
	}
	return tags
}

// TestAppendSnapshotTrailers_SingleTag asserts that a single tag appends one
// Snapshot: line in a new trailer-block paragraph.
func TestAppendSnapshotTrailers_SingleTag(t *testing.T) {
	message := "weft sync"
	want := "weft sync\n\nSnapshot: build-42"

	got, err := appendSnapshotTrailers(message, []string{"build-42"})
	if err != nil {
		t.Fatalf("appendSnapshotTrailers(%q, [build-42]) unexpected error: %v", message, err)
	}
	if got != want {
		t.Errorf("appendSnapshotTrailers(%q, [build-42]) = %q; want %q", message, got, want)
	}
}

// TestAppendSnapshotTrailers_MultipleTags asserts that multiple tags each get
// their own Snapshot: line, one per tag, all inside the same trailer block.
func TestAppendSnapshotTrailers_MultipleTags(t *testing.T) {
	message := "weft sync"
	tags := []string{"build-42", "release-1.0", "nightly"}
	want := "weft sync\n\nSnapshot: build-42\nSnapshot: release-1.0\nSnapshot: nightly"

	got, err := appendSnapshotTrailers(message, tags)
	if err != nil {
		t.Fatalf("appendSnapshotTrailers(%q, %v) unexpected error: %v", message, tags, err)
	}
	if got != want {
		t.Errorf("appendSnapshotTrailers(%q, %v) = %q; want %q", message, tags, got, want)
	}
	if gotTags := parseSnapshotTags(got); !reflect.DeepEqual(gotTags, tags) {
		t.Errorf("parseSnapshotTags(%q) = %v; want %v", got, gotTags, tags)
	}
}

// TestAppendSnapshotTrailers_CoexistsWithWarpSHATrailer asserts that Snapshot
// trailers appended after a Warp-SHA trailer already appended by the caller
// join the same trailer block, and that both parse back independently.
func TestAppendSnapshotTrailers_CoexistsWithWarpSHATrailer(t *testing.T) {
	message := "weft sync"
	withWarpSHA := appendWarpSHATrailer(message, "abc123")
	want := "weft sync\n\nWarp-SHA: abc123\nSnapshot: build-42\nSnapshot: release-1.0"

	got, err := appendSnapshotTrailers(withWarpSHA, []string{"build-42", "release-1.0"})
	if err != nil {
		t.Fatalf("appendSnapshotTrailers(%q, ...) unexpected error: %v", withWarpSHA, err)
	}
	if got != want {
		t.Errorf("appendSnapshotTrailers(%q, ...) = %q; want %q", withWarpSHA, got, want)
	}

	gotSHA, ok := parseWarpSHATrailer(got)
	if !ok || gotSHA != "abc123" {
		t.Errorf("parseWarpSHATrailer(%q) = (%q, %v); want (%q, true)", got, gotSHA, ok, "abc123")
	}
	wantTags := []string{"build-42", "release-1.0"}
	if gotTags := parseSnapshotTags(got); !reflect.DeepEqual(gotTags, wantTags) {
		t.Errorf("parseSnapshotTags(%q) = %v; want %v", got, gotTags, wantTags)
	}
}

// TestAppendSnapshotTrailers_EmptyTagsReturnsMessageUnchanged asserts that an
// empty tags slice returns message unchanged with a nil error.
func TestAppendSnapshotTrailers_EmptyTagsReturnsMessageUnchanged(t *testing.T) {
	message := "weft sync\n\nWarp-SHA: abc123"

	got, err := appendSnapshotTrailers(message, nil)
	if err != nil {
		t.Fatalf("appendSnapshotTrailers(%q, nil) unexpected error: %v", message, err)
	}
	if got != message {
		t.Errorf("appendSnapshotTrailers(%q, nil) = %q; want unchanged %q", message, got, message)
	}
}

// TestAppendSnapshotTrailers_RejectsInvalidTags asserts that a tag containing
// a newline, carriage return, colon, or any other out-of-charset character is
// rejected, and that rejection happens before anything is written (the
// returned message is empty on error).
func TestAppendSnapshotTrailers_RejectsInvalidTags(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{"newline", "build\n42"},
		{"carriage_return", "build\r42"},
		{"colon", "build:42"},
		{"out_of_charset_space", "build 42"},
		{"out_of_charset_slash", "build/42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := appendSnapshotTrailers("weft sync", []string{tt.tag})
			if err == nil {
				t.Fatalf("appendSnapshotTrailers(%q, [%q]) err = nil; want error", "weft sync", tt.tag)
			}
			if got != "" {
				t.Errorf("appendSnapshotTrailers(%q, [%q]) message = %q; want empty on error", "weft sync", tt.tag, got)
			}
			var invalidTagErr *ErrInvalidSnapshotTag
			if !errors.As(err, &invalidTagErr) {
				t.Fatalf("appendSnapshotTrailers(%q, [%q]) error = %v; want *ErrInvalidSnapshotTag", "weft sync", tt.tag, err)
			}
			if invalidTagErr.Tag != tt.tag {
				t.Errorf("ErrInvalidSnapshotTag.Tag = %q; want %q", invalidTagErr.Tag, tt.tag)
			}
		})
	}
}

// TestAppendSnapshotTrailers_FailsFastOnAnyInvalidTagInTheList asserts that
// when a valid tag precedes an invalid one in the list, the whole call fails
// with no message written -- a caller must never end up with a partial
// trailer block.
func TestAppendSnapshotTrailers_FailsFastOnAnyInvalidTagInTheList(t *testing.T) {
	got, err := appendSnapshotTrailers("weft sync", []string{"build-42", "bad:tag"})
	if err == nil {
		t.Fatal("appendSnapshotTrailers with a trailing invalid tag err = nil; want error")
	}
	if got != "" {
		t.Errorf("appendSnapshotTrailers with a trailing invalid tag message = %q; want empty on error", got)
	}
}
