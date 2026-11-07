// audit_streaming_test.go pins that forEachTranscriptLine streams a transcript rather than materialising it.
// It also pins that two edge cases still decode:
// a final line with no trailing newline, and a single line larger than bufio.Scanner's default token cap.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeTranscript writes body to a new transcript.jsonl under t.TempDir() and returns its path and size in bytes.
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

// TestForEachTranscriptLine_StreamsRatherThanMaterialising pins the memory property of the walk.
// forEachTranscriptLine holds one line at a time rather than the whole transcript.
//
// The threshold is half the file size, which a materialising walk exceeds.
// The margin on either side of it is wide, so the assertion does not depend on allocator or GC detail.
// runtime.GC() before each sample makes the check deterministic:
// it collects everything already unreachable, so HeapAlloc reports what the walk still holds.
// TotalAlloc counts cumulative allocation, which cannot tell a holding walk from a streaming one.
func TestForEachTranscriptLine_StreamsRatherThanMaterialising(t *testing.T) {
	tiergate.Require(t, 0)
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
		// Sampling every 500th line keeps the forced collections cheap while still covering the whole walk.
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
// An abnormally ended session, one whose process died mid-write, leaves a final line with no trailing newline.
// A large tool result can produce a single line far past bufio.Scanner's 64 KiB default token cap.
// That is why the walk reads with bufio.Reader.ReadString, whose line length is unbounded, and not with a Scanner.
func TestForEachTranscriptLine_EdgeCases(t *testing.T) {
	tiergate.Require(t, 0)
	// This line is past bufio.Scanner's 64 KiB default token cap,
	// so a Scanner-based implementation would drop it or fail rather than decode it.
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

// TestAuditForkTranscript_UnterminatedFinalLineStillCounted proves the final line is still audited.
// A transcript that ends without a trailing newline still has that last line's Bash command reported.
func TestAuditForkTranscript_UnterminatedFinalLineStillCounted(t *testing.T) {
	tiergate.Require(t, 0)
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
