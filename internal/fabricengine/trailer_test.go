// trailer_test.go — unit tests for the Warp-SHA trailer format/parse helpers.

package fabricengine

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestAppendParseWarpSHATrailer_RoundTrip covers append-then-parse round trips for a single-line
// subject and a multi-paragraph message.
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

// TestAppendWarpSHATrailer_SubjectIsNeverATrailerBlock asserts that a single-line message -- even
// one shaped exactly like a trailer line, the "webster: <label>" form every
// webster weft commit uses -- gets its Warp-SHA trailer in a NEW blank-line-separated
// paragraph, keeping the git subject clean.
// Joining instead would fold the trailer into the subject paragraph and pollute `git log --oneline`
// for every such commit (the round fable-r1 regression).
//
//testtiming:keep a single-line message, even one shaped like a trailer, getting its Warp-SHA trailer in a new paragraph; coverage of its blocks by other tests does not show an assertion of this
func TestAppendWarpSHATrailer_SubjectIsNeverATrailerBlock(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			"webster_style_subject",
			"webster: record-batch 01 done",
			"webster: record-batch 01 done\n\nWarp-SHA: abc123",
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

// TestAppendWarpSHATrailer_JoinsExistingTrailerBlock asserts that appending to a message already
// ending in a trailer block (e.g.
// a prior Co-authored-by:) joins the new line directly, without introducing a stray blank line.
func TestAppendWarpSHATrailer_JoinsExistingTrailerBlock(t *testing.T) {
	message := "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>"
	want := "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>\nWarp-SHA: abc123"

	got := appendWarpSHATrailer(message, "abc123")
	if got != want {
		t.Errorf("appendWarpSHATrailer() = %q; want %q", got, want)
	}
}

// TestParseWarpSHATrailer covers a message with no Warp-SHA trailer (ok=false), several Warp-SHA
// trailer lines (the last wins) and whitespace around the trailer line and its value (tolerated).
//
//testtiming:keep parseWarpSHATrailer returning ok=false when absent, the last trailer winning, and whitespace being tolerated; coverage of its blocks by other tests does not show an assertion of this
func TestParseWarpSHATrailer(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantSHA string
		wantOK  bool
	}{
		{"absent", "raddle: sync module docs\n\nCo-authored-by: Someone <someone@example.com>", "", false},
		{"multiple_trailers_last_wins", "raddle: sync module docs\n\nWarp-SHA: first000\nWarp-SHA: second111", "second111", true},
		{"tolerant_of_surrounding_whitespace", "raddle: sync module docs\n\n  Warp-SHA:   abc123   ", "abc123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSHA, ok := parseWarpSHATrailer(tt.message)
			if ok != tt.wantOK || gotSHA != tt.wantSHA {
				t.Errorf("parseWarpSHATrailer(%q) = (%q, %v); want (%q, %v)", tt.message, gotSHA, ok, tt.wantSHA, tt.wantOK)
			}
		})
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

// TestAppendSnapshotTrailers_Tags covers a single tag (one Snapshot: line in a new trailer-block
// paragraph), several tags (one line per tag, all in the same trailer block, parsing back in order)
// and an empty tags slice (message returned unchanged with a nil error).
func TestAppendSnapshotTrailers_Tags(t *testing.T) {
	tests := []struct {
		name    string
		message string
		tags    []string
		want    string
	}{
		{"single_tag", "weft sync", []string{"build-42"}, "weft sync\n\nSnapshot: build-42"},
		{
			"multiple_tags",
			"weft sync",
			[]string{"build-42", "release-1.0", "nightly"},
			"weft sync\n\nSnapshot: build-42\nSnapshot: release-1.0\nSnapshot: nightly",
		},
		{"empty_tags_leave_message_unchanged", "weft sync\n\nWarp-SHA: abc123", nil, "weft sync\n\nWarp-SHA: abc123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := appendSnapshotTrailers(tt.message, tt.tags)
			if err != nil {
				t.Fatalf("appendSnapshotTrailers(%q, %v) unexpected error: %v", tt.message, tt.tags, err)
			}
			if got != tt.want {
				t.Errorf("appendSnapshotTrailers(%q, %v) = %q; want %q", tt.message, tt.tags, got, tt.want)
			}
			if len(tt.tags) > 0 {
				if gotTags := parseSnapshotTags(got); !reflect.DeepEqual(gotTags, tt.tags) {
					t.Errorf("parseSnapshotTags(%q) = %v; want %v", got, gotTags, tt.tags)
				}
			}
		})
	}
}

// TestAppendSnapshotTrailers_CoexistsWithWarpSHATrailer asserts that Snapshot trailers appended
// after a Warp-SHA trailer already appended by the caller join the same trailer block,
// and that both parse back independently.
//
//testtiming:keep Snapshot trailers joining an existing Warp-SHA trailer block and both parsing back; coverage of its blocks by other tests does not show an assertion of this
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

// TestAppendSnapshotTrailers_RejectsInvalidTags asserts that a tag containing a newline, carriage
// return, colon, or any other out-of-charset character is rejected, and that rejection happens
// before anything is written (the returned message is empty on error).
// A valid tag preceding an invalid one fails the whole call too: a caller must never end up with a
// partial trailer block.
//
//testtiming:keep out-of-charset tags, and a valid tag before an invalid one, being rejected with *ErrInvalidSnapshotTag and an empty message; coverage of its blocks by other tests does not show an assertion of this
func TestAppendSnapshotTrailers_RejectsInvalidTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		wantBadTag string
	}{
		{"newline", []string{"build\n42"}, "build\n42"},
		{"carriage_return", []string{"build\r42"}, "build\r42"},
		{"colon", []string{"build:42"}, "build:42"},
		{"out_of_charset_space", []string{"build 42"}, "build 42"},
		{"out_of_charset_slash", []string{"build/42"}, "build/42"},
		{"valid_tag_before_invalid_tag", []string{"build-42", "bad:tag"}, "bad:tag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := appendSnapshotTrailers("weft sync", tt.tags)
			if err == nil {
				t.Fatalf("appendSnapshotTrailers(%q, %q) err = nil; want error", "weft sync", tt.tags)
			}
			if got != "" {
				t.Errorf("appendSnapshotTrailers(%q, %q) message = %q; want empty on error", "weft sync", tt.tags, got)
			}
			var invalidTagErr *ErrInvalidSnapshotTag
			if !errors.As(err, &invalidTagErr) {
				t.Fatalf("appendSnapshotTrailers(%q, %q) error = %v; want *ErrInvalidSnapshotTag", "weft sync", tt.tags, err)
			}
			if invalidTagErr.Tag != tt.wantBadTag {
				t.Errorf("ErrInvalidSnapshotTag.Tag = %q; want %q", invalidTagErr.Tag, tt.wantBadTag)
			}
		})
	}
}
