// wiring_terminal_test.go covers the production hidden read of wiring.go for input that is not a terminal.

package slackcli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestReadHiddenTerminalLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A pipe's read end is an *os.File that is not a terminal.
	pipeRead, pipeWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() = %v", err)
	}
	t.Cleanup(func() {
		pipeRead.Close()
		pipeWrite.Close()
	})
	if _, err := pipeWrite.WriteString("xoxb-piped\n"); err != nil {
		t.Fatalf("write to the pipe: %v", err)
	}

	tests := []struct {
		name string
		in   io.Reader
	}{
		{"ANonFileReaderIsNoTerminal", strings.NewReader("xoxb-piped\n")},
		{"APipeFileIsNoTerminal", pipeRead},
	}
	// The group returns only after its parallel subtests finish, so the unread check below sees their effect.
	t.Run("Inputs", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				line, onTerminal, err := readHiddenTerminalLine(tt.in, tokenPrompt)
				if line != "" || onTerminal || err != nil {
					t.Errorf("readHiddenTerminalLine() = (%q, %v, %v); want (\"\", false, nil)", line, onTerminal, err)
				}
			})
		}
	})

	// The input is left unread for the caller to read to its end.
	rest, err := io.ReadAll(io.LimitReader(pipeRead, int64(len("xoxb-piped\n"))))
	if err != nil || string(rest) != "xoxb-piped\n" {
		t.Errorf("pipe after the read = (%q, %v); want the whole input unread", rest, err)
	}
}
