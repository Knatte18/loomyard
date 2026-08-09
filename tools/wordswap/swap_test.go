package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestSwapText_CasePreservingSubstitution covers the lower/Title/UPPER forms and their
// embedded-token equivalents.
func TestSwapText_CasePreservingSubstitution(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lower", "host", "warp"},
		{"title", "Host", "Warp"},
		{"upper", "HOST", "WARP"},
		{"embedded_lower_camel", "hostBranch", "warpBranch"},
		{"embedded_title_camel", "HostJunctions", "WarpJunctions"},
		{"embedded_upper_snake", "HOST_BRANCH", "WARP_BRANCH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := swapText(tt.input, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", tt.input, err)
			}
			if got.Out != tt.want {
				t.Errorf("swapText(%q).Out = %q; want %q", tt.input, got.Out, tt.want)
			}
			if len(got.Ambiguous) != 0 {
				t.Errorf("swapText(%q).Ambiguous = %v; want empty", tt.input, got.Ambiguous)
			}
			if got.Mismatch {
				t.Errorf("swapText(%q).Mismatch = true; want false", tt.input)
			}
		})
	}
}

// TestSwapText_TokenBoundaryRejection verifies that a lowercase letter immediately preceding
// the matched form means no match at all -- not AMBIGUOUS, not swapped, not reported.
func TestSwapText_TokenBoundaryRejection(t *testing.T) {
	tests := []string{"ghost", "localhost", "conhost"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := swapText(in, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", in, err)
			}
			if got.Out != in {
				t.Errorf("swapText(%q).Out = %q; want unchanged %q", in, got.Out, in)
			}
			if len(got.Ambiguous) != 0 {
				t.Errorf("swapText(%q).Ambiguous = %v; want empty", in, got.Ambiguous)
			}
			if len(got.Skipped) != 0 {
				t.Errorf("swapText(%q).Skipped = %v; want empty", in, got.Skipped)
			}
		})
	}
}

// TestSwapText_CamelStartAcceptance verifies that a matched form starting uppercase swaps even
// though a lowercase letter precedes it in the surrounding text -- the camelCase start case.
func TestSwapText_CamelStartAcceptance(t *testing.T) {
	got, err := swapText("myHostPath", "host", "warp", nil)
	if err != nil {
		t.Fatalf("swapText returned error: %v", err)
	}
	if got.Out != "myWarpPath" {
		t.Errorf("swapText(%q).Out = %q; want %q", "myHostPath", got.Out, "myWarpPath")
	}
}

// TestSwapText_MixedCaseRejection verifies that a form matching neither the lower, Title, nor
// UPPER shape is left unchanged and unreported.
func TestSwapText_MixedCaseRejection(t *testing.T) {
	for _, in := range []string{"hOst", "HoSt"} {
		t.Run(in, func(t *testing.T) {
			got, err := swapText(in, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", in, err)
			}
			if got.Out != in {
				t.Errorf("swapText(%q).Out = %q; want unchanged %q", in, got.Out, in)
			}
			if len(got.Ambiguous) != 0 {
				t.Errorf("swapText(%q).Ambiguous = %v; want empty", in, got.Ambiguous)
			}
			if len(got.Skipped) != 0 {
				t.Errorf("swapText(%q).Skipped = %v; want empty", in, got.Skipped)
			}
		})
	}
}

// TestSwapText_AmbiguityClassification verifies that host + lowercase at a token start is left
// byte-unchanged and reported in Ambiguous with the correct 1-based line.
func TestSwapText_AmbiguityClassification(t *testing.T) {
	tests := []string{"hostclean", "hostlayout", "hosthub", "hostname"}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			got, err := swapText(in, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", in, err)
			}
			if got.Out != in {
				t.Errorf("swapText(%q).Out = %q; want byte-unchanged %q", in, got.Out, in)
			}
			if len(got.Ambiguous) != 1 {
				t.Fatalf("swapText(%q).Ambiguous = %v; want exactly one entry", in, got.Ambiguous)
			}
			if got.Ambiguous[0].Line != 1 {
				t.Errorf("swapText(%q).Ambiguous[0].Line = %d; want 1", in, got.Ambiguous[0].Line)
			}
			if got.Ambiguous[0].Text != in {
				t.Errorf("swapText(%q).Ambiguous[0].Text = %q; want %q", in, got.Ambiguous[0].Text, in)
			}
		})
	}
}

// TestSwapText_MultipleAndMixedOccurrencesOnOneLine verifies that several occurrences of
// different case forms on a single line all swap in a single pass.
func TestSwapText_MultipleAndMixedOccurrencesOnOneLine(t *testing.T) {
	in := "hostBranch talks to HOST_BRANCH and bare host on one line"
	want := "warpBranch talks to WARP_BRANCH and bare warp on one line"
	got, err := swapText(in, "host", "warp", nil)
	if err != nil {
		t.Fatalf("swapText returned error: %v", err)
	}
	if got.Out != want {
		t.Errorf("swapText(%q).Out = %q; want %q", in, got.Out, want)
	}
	if len(got.Ambiguous) != 0 {
		t.Errorf("swapText(%q).Ambiguous = %v; want empty", in, got.Ambiguous)
	}
}

// TestSwapText_SkipBehavior verifies that a -skip regexp matching an occurrence's line leaves it
// unchanged and reports it in Skipped rather than Ambiguous, while a non-matching occurrence on
// another line still swaps -- including the case where a skip claims an otherwise-AMBIGUOUS
// occurrence, which is what lets a run reach exit zero.
func TestSwapText_SkipBehavior(t *testing.T) {
	in := "a live pane hosting an idle agent\nplain host on this line\n"
	skips := []*regexp.Regexp{regexp.MustCompile("pane hosting an idle agent")}
	got, err := swapText(in, "host", "warp", skips)
	if err != nil {
		t.Fatalf("swapText returned error: %v", err)
	}
	wantOut := "a live pane hosting an idle agent\nplain warp on this line\n"
	if got.Out != wantOut {
		t.Errorf("swapText(...).Out = %q; want %q", got.Out, wantOut)
	}
	if len(got.Ambiguous) != 0 {
		t.Errorf("swapText(...).Ambiguous = %v; want empty", got.Ambiguous)
	}
	if len(got.Skipped) != 1 {
		t.Fatalf("swapText(...).Skipped = %v; want exactly one entry", got.Skipped)
	}
	if got.Skipped[0].Line != 1 {
		t.Errorf("swapText(...).Skipped[0].Line = %d; want 1", got.Skipped[0].Line)
	}
}

// TestSwapText_ReversibilityInvariant verifies that reverting the recorded substitution spans
// reproduces the input byte-for-byte, including the critical case where the target word already
// occurs in the input.
func TestSwapText_ReversibilityInvariant(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple", "the host repo"},
		{"target_already_present", "warp and host both appear, and Host too"},
		{"embedded_forms", "hostBranch, HOST_BRANCH, and warpClean already present"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := swapText(tt.input, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", tt.input, err)
			}
			if got.Mismatch {
				t.Errorf("swapText(%q).Mismatch = true; want false", tt.input)
			}
		})
	}
}

// TestSwapText_LanguageAgnosticism verifies substitution works identically over a shell fragment
// and a markdown fragment -- the tool has no language-specific parsing.
func TestSwapText_LanguageAgnosticism(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "shell",
			input: `HOST_BRANCH="$(git rev-parse --abbrev-ref HEAD)"`,
			want:  `WARP_BRANCH="$(git rev-parse --abbrev-ref HEAD)"`,
		},
		{
			name:  "markdown",
			input: "the **host repo** holds ...",
			want:  "the **warp repo** holds ...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := swapText(tt.input, "host", "warp", nil)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", tt.input, err)
			}
			if got.Out != tt.want {
				t.Errorf("swapText(%q).Out = %q; want %q", tt.input, got.Out, tt.want)
			}
		})
	}
}

// TestProcessFile_DryRunWritesNothing verifies that processFile with dryRun=true reports the
// change status without writing the file.
func TestProcessFile_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "the host repo\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	status, result, err := processFile(path, "host", "warp", nil, true)
	if err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}
	if status != "changed" {
		t.Errorf("processFile status = %q; want %q", status, "changed")
	}
	if result.Out == original {
		t.Errorf("processFile result.Out unexpectedly equals the unswapped original")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != original {
		t.Errorf("dry-run wrote the file: got %q; want unchanged %q", string(after), original)
	}
}

// TestProcessFile_MismatchLeavesFileUntouched verifies that a Mismatch result from swapText
// leaves the on-disk file byte-for-byte unchanged and is reported as "mismatch".
// The failure is injected through the package-level revertSpans hook rather than by contriving
// input, since no genuine input can fail the real reversibility check.
func TestProcessFile_MismatchLeavesFileUntouched(t *testing.T) {
	original := revertSpans
	t.Cleanup(func() { revertSpans = original })
	revertSpans = func(out string, spans []span) string {
		// Deliberately broken: never reproduces the input, forcing Result.Mismatch = true.
		return out + "!corrupted!"
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "the host repo\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	status, result, err := processFile(path, "host", "warp", nil, false)
	if err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}
	if status != "mismatch" {
		t.Errorf("processFile status = %q; want %q", status, "mismatch")
	}
	if !result.Mismatch {
		t.Errorf("processFile result.Mismatch = false; want true")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != content {
		t.Errorf("mismatch case wrote the file: got %q; want unchanged %q", string(after), content)
	}
}
