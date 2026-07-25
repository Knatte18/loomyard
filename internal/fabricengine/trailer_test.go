// trailer_test.go — unit tests for the Warp-SHA trailer format/parse helpers.

package fabricengine

import "testing"

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
