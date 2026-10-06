package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// TestSwapText covers swapText's classification of every occurrence of "host":
//   - lower/Title/UPPER forms and their embedded-token equivalents swap with the case preserved;
//   - a lowercase letter before the match, or a form matching no case shape, is no match at all;
//   - a match starting uppercase swaps after a lowercase letter (the camelCase start);
//   - host + lowercase at a token start is left byte-unchanged and reported in Ambiguous;
//   - a -skip regexp matching an occurrence's line leaves it unchanged and reports it in Skipped, including one that claims an otherwise-AMBIGUOUS occurrence, which lets a run reach exit zero;
//   - substitution is language-agnostic, and reverting the recorded spans always reproduces the input, including when the target word already occurs in it.
//
// Every row also asserts the reversibility check did not report a mismatch.
func TestSwapText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		in            string
		skips         []*regexp.Regexp
		want          string
		wantAmbiguous []Occurrence
		wantSkipped   []Occurrence
	}{
		{name: "lower", in: "host", want: "pair"},
		{name: "title", in: "Host", want: "Pair"},
		{name: "upper", in: "HOST", want: "PAIR"},
		{name: "embedded lower camel", in: "hostBranch", want: "pairBranch"},
		{name: "embedded title camel", in: "HostJunctions", want: "PairJunctions"},
		{name: "embedded upper snake", in: "HOST_BRANCH", want: "PAIR_BRANCH"},
		{name: "camel start after lowercase", in: "myHostPath", want: "myPairPath"},
		{name: "token boundary ghost", in: "ghost", want: "ghost"},
		{name: "token boundary localhost", in: "localhost", want: "localhost"},
		{name: "token boundary conhost", in: "conhost", want: "conhost"},
		{name: "mixed case hOst", in: "hOst", want: "hOst"},
		{name: "mixed case HoSt", in: "HoSt", want: "HoSt"},
		{name: "ambiguous hostclean", in: "hostclean", want: "hostclean", wantAmbiguous: []Occurrence{{Line: 1, Text: "hostclean"}}},
		{name: "ambiguous hostlayout", in: "hostlayout", want: "hostlayout", wantAmbiguous: []Occurrence{{Line: 1, Text: "hostlayout"}}},
		{name: "ambiguous hosthub", in: "hosthub", want: "hosthub", wantAmbiguous: []Occurrence{{Line: 1, Text: "hosthub"}}},
		{name: "ambiguous hostname", in: "hostname", want: "hostname", wantAmbiguous: []Occurrence{{Line: 1, Text: "hostname"}}},
		{
			name: "several occurrences of different forms on one line",
			in:   "hostBranch talks to HOST_BRANCH and bare host on one line",
			want: "pairBranch talks to PAIR_BRANCH and bare pair on one line",
		},
		{
			name:        "skip claims one line, another still swaps",
			in:          "a live pane hosting an idle agent\nplain host on this line\n",
			skips:       []*regexp.Regexp{regexp.MustCompile("pane hosting an idle agent")},
			want:        "a live pane hosting an idle agent\nplain pair on this line\n",
			wantSkipped: []Occurrence{{Line: 1, Text: "a live pane hosting an idle agent"}},
		},
		{name: "reversible simple", in: "the host repo", want: "the pair repo"},
		{
			name: "reversible with target already present",
			in:   "pair and host both appear, and Host too",
			want: "pair and pair both appear, and Pair too",
		},
		{
			name: "reversible with embedded forms",
			in:   "hostBranch, HOST_BRANCH, and pairClean already present",
			want: "pairBranch, PAIR_BRANCH, and pairClean already present",
		},
		{
			name: "shell fragment",
			in:   `HOST_BRANCH="$(git rev-parse --abbrev-ref HEAD)"`,
			want: `PAIR_BRANCH="$(git rev-parse --abbrev-ref HEAD)"`,
		},
		{name: "markdown fragment", in: "the **host repo** holds ...", want: "the **pair repo** holds ..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := swapText(tt.in, "host", "pair", tt.skips)
			if err != nil {
				t.Fatalf("swapText(%q) returned error: %v", tt.in, err)
			}
			if got.Out != tt.want {
				t.Errorf("swapText(%q).Out = %q; want %q", tt.in, got.Out, tt.want)
			}
			if !slices.Equal(got.Ambiguous, tt.wantAmbiguous) {
				t.Errorf("swapText(%q).Ambiguous = %v; want %v", tt.in, got.Ambiguous, tt.wantAmbiguous)
			}
			if !slices.Equal(got.Skipped, tt.wantSkipped) {
				t.Errorf("swapText(%q).Skipped = %v; want %v", tt.in, got.Skipped, tt.wantSkipped)
			}
			if got.Mismatch {
				t.Errorf("swapText(%q).Mismatch = true; want false", tt.in)
			}
		})
	}
}

// TestProcessFile_DryRunWritesNothing verifies that processFile with dryRun=true reports the
// change status without writing the file.
func TestProcessFile_DryRunWritesNothing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "the host repo\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	status, result, err := processFile(path, "host", "pair", nil, true)
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

// TestProcessFile_MismatchLeavesFileUntouched verifies that a Mismatch result from swapText leaves the on-disk file byte-for-byte unchanged and is reported as "mismatch".
// The failure is injected through the package-level revertSpans hook rather than by contriving input, since no genuine input can fail the real reversibility check.
// It does not call t.Parallel because it rewrites the package-level revertSpans hook; the parallel tests of this package resume only after it has restored the hook.
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

	status, result, err := processFile(path, "host", "pair", nil, false)
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
