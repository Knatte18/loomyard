// audit_streaming_test.go is R2-F5's regression guard: it pins that forEachTranscriptLine walks a
// session transcript as a STREAM rather than materialising it, and that the two edge cases a naive
// streaming port breaks — a final line with no trailing newline, and a single line larger than
// bufio.Scanner's default token cap — both still decode.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeTranscript writes body to a fresh .jsonl file under t.TempDir() and returns its path and
// size in bytes.
func writeTranscript(t *testing.T, body string) (path string, sizeBytes int64) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat transcript: %v", err)
	}
	return path, info.Size()
}

// assistantBashLine renders one assistant tool_use line carrying a Bash command of the given size.
func assistantBashLine(commandBody string) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","id":"t1","input":{"command":"%s"}}]}}`, commandBody)
}

// TestForEachTranscriptLine_StreamsRatherThanMaterialising pins the memory property the streaming
// rewrite exists for.
//
// The previous implementation was os.ReadFile plus strings.Split(string(data), "\n"), which keeps
// the raw bytes, a full string copy of them, a per-line slice header, and every decoded line ALIVE
// AT ONCE — so live heap during the walk could never drop below roughly twice the file size. A
// streaming walk holds one line. The threshold below is half the file size: measured, the streaming
// implementation peaks around 0.02x on this fixture and the materialising one cannot get under 2x,
// so there are roughly two orders of magnitude of margin either side of it and the assertion is not
// sensitive to allocator or GC detail.
//
// runtime.GC() before each sample is what makes this deterministic: it collects everything already
// unreachable, so HeapAlloc reports what the walk is actually still HOLDING rather than what it has
// cumulatively allocated (TotalAlloc, which does not separate the two implementations at all).
func TestForEachTranscriptLine_StreamsRatherThanMaterialising(t *testing.T) {
	const (
		lineCount        = 2000
		commandBodyBytes = 4000
	)
	var body strings.Builder
	commandBody := strings.Repeat("x", commandBodyBytes)
	for i := 0; i < lineCount; i++ {
		body.WriteString(assistantBashLine(commandBody))
		body.WriteString("\n")
	}
	path, sizeBytes := writeTranscript(t, body.String())

	visited := 0
	var peakLiveBytes uint64
	err := forEachTranscriptLine(path, func(transcriptLine) {
		visited++
		// Sampling every 500th line keeps the forced collections cheap while still covering the
		// whole walk, including its final quarter where a materialising implementation is holding
		// the most.
		if visited%500 != 0 {
			return
		}
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if stats.HeapAlloc > peakLiveBytes {
			peakLiveBytes = stats.HeapAlloc
		}
	})
	if err != nil {
		t.Fatalf("forEachTranscriptLine() error: %v", err)
	}
	if visited != lineCount {
		t.Fatalf("visited %d lines; want %d", visited, lineCount)
	}

	limitBytes := uint64(sizeBytes / 2)
	if peakLiveBytes > limitBytes {
		t.Errorf("peak live heap during the walk = %d bytes for a %d-byte transcript (%.2fx); want under %d (0.5x) — anything at or above the file size means the transcript is being materialised rather than streamed",
			peakLiveBytes, sizeBytes, float64(peakLiveBytes)/float64(sizeBytes), limitBytes)
	}
}

// TestForEachTranscriptLine_EdgeCases pins the two shapes a naive streaming port silently breaks.
//
// Both are ordinary here rather than exotic: an abnormally ended session (the process died
// mid-write, which is exactly when an operator wants the audit) leaves a final line with no
// trailing newline, and a large tool result routinely produces a single line far past
// bufio.Scanner's 64 KiB default token cap — which is why this reads with bufio.Reader.ReadString,
// whose line length is unbounded, and not with a Scanner.
func TestForEachTranscriptLine_EdgeCases(t *testing.T) {
	// Comfortably past bufio.Scanner's 64 KiB default token cap, so a Scanner-based implementation
	// would drop this line (or error) rather than decode it.
	const oversizedCommandBytes = 256 * 1024

	tests := []struct {
		name        string
		body        string
		wantVisited int
	}{
		{
			name:        "final_line_without_trailing_newline",
			body:        assistantBashLine("first") + "\n" + assistantBashLine("last-no-newline"),
			wantVisited: 2,
		},
		{
			name:        "single_line_far_past_the_scanner_token_cap",
			body:        assistantBashLine(strings.Repeat("y", oversizedCommandBytes)) + "\n",
			wantVisited: 1,
		},
		{
			name:        "oversized_line_last_and_unterminated",
			body:        assistantBashLine("first") + "\n" + assistantBashLine(strings.Repeat("z", oversizedCommandBytes)),
			wantVisited: 2,
		},
		{
			name:        "blank_and_undecodable_lines_are_skipped_not_fatal",
			body:        "\n   \n{ not json\n" + assistantBashLine("only-real-one") + "\n\n",
			wantVisited: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, _ := writeTranscript(t, tt.body)

			visited := 0
			if err := forEachTranscriptLine(path, func(transcriptLine) { visited++ }); err != nil {
				t.Fatalf("forEachTranscriptLine() error: %v", err)
			}
			if visited != tt.wantVisited {
				t.Errorf("visited %d lines; want %d", visited, tt.wantVisited)
			}
		})
	}
}

// TestAuditForkTranscript_UnterminatedFinalLineStillCounted proves the edge case above reaches the
// audit's own output rather than only forEachTranscriptLine's call count: a fork whose transcript
// ends without a trailing newline still has that last line's Bash command reported.
func TestAuditForkTranscript_UnterminatedFinalLineStillCounted(t *testing.T) {
	path, _ := writeTranscript(t, assistantBashLine("git status")+"\n"+assistantBashLine("git push"))

	report, err := auditForkTranscript(path)
	if err != nil {
		t.Fatalf("auditForkTranscript() error: %v", err)
	}
	want := []string{"git status", "git push"}
	if len(report.BashCommands) != len(want) {
		t.Fatalf("BashCommands = %v; want %v — the unterminated final line must still be audited", report.BashCommands, want)
	}
	for i, cmd := range want {
		if report.BashCommands[i] != cmd {
			t.Errorf("BashCommands[%d] = %q; want %q", i, report.BashCommands[i], cmd)
		}
	}
}
