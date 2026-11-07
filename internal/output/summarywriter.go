// summarywriter.go implements SummaryWriter, a stream filter that prints a verb's JSON envelope as a readable summary.
// It passes every other byte through as written.

package output

import (
	"bytes"
	"io"
)

// SummaryWriter writes everything written to it through to an underlying writer.
// A line that opens with '{' and holds one envelope is written as its readable summary instead.
// Text that does not open a line with '{', such as a prompt with no line break, reaches the underlying writer at once.
// A line that opens with '{' is held until its line break or Flush.
// A SummaryWriter is not safe for concurrent use.
type SummaryWriter struct {
	out         io.Writer
	atLineStart bool
	holding     bool
	held        []byte
}

// NewSummaryWriter returns a SummaryWriter over out, at the start of a line.
func NewSummaryWriter(out io.Writer) *SummaryWriter {
	return &SummaryWriter{out: out, atLineStart: true}
}

// Write implements io.Writer, holding a line that opens with '{' and passing any other text through.
func (s *SummaryWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		if !s.holding && s.atLineStart && p[0] == '{' {
			s.holding = true
		}
		end := bytes.IndexByte(p, '\n')
		chunk := p
		if end >= 0 {
			chunk = p[:end+1]
		}
		if s.holding {
			s.held = append(s.held, chunk...)
			p = p[len(chunk):]
			if end < 0 {
				return total, nil
			}
			if err := s.writeHeld(); err != nil {
				return total - len(p), err
			}
			continue
		}
		if _, err := s.out.Write(chunk); err != nil {
			return total - len(p), err
		}
		s.atLineStart = end >= 0
		p = p[len(chunk):]
	}
	return total, nil
}

// Flush writes a held line that never got its line break.
// It writes the line as its summary when it holds an envelope, and as written otherwise.
func (s *SummaryWriter) Flush() error {
	if !s.holding {
		return nil
	}
	return s.writeHeld()
}

// writeHeld writes the held line as its summary, or as it was written when it holds no envelope, and starts a new line.
func (s *SummaryWriter) writeHeld() error {
	held := s.held
	s.held = nil
	s.holding = false
	s.atLineStart = true
	if summary, ok := Summary(bytes.TrimSpace(held)); ok {
		_, err := io.WriteString(s.out, summary)
		return err
	}
	_, err := s.out.Write(held)
	return err
}
