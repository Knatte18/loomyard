// filetail.go reads the bounded tail of a session record and counts a file's lines up to a limit.
// These are the two file reads an engine's batching nudge makes on every judged tool call.

package shuttleengine

import (
	"bytes"
	"io"
	"os"
)

// ReadFileTail returns the complete lines of the last budget bytes of the file at path.
// It returns the whole file when it is smaller.
// The window's first line is dropped unless the window starts the file, since it may be cut;
// a window holding no line end yields nil.
// A caller reads a session record this way so a call at a large context costs one bounded read.
func ReadFileTail(path string, budget int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := info.Size() - budget
	if start < 0 {
		start = 0
	}
	tail := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(tail, start); err != nil && err != io.EOF {
		return nil, err
	}
	if start > 0 {
		nl := bytes.IndexByte(tail, '\n')
		if nl < 0 {
			return nil, nil
		}
		tail = tail[nl+1:]
	}
	return tail, nil
}

// CountLinesUpTo counts the lines of the file at path, stopping once the count reaches limit and returning at most limit;
// a last line without its newline counts.
func CountLinesUpTo(path string, limit int) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	lines, unterminated := 0, false
	for lines < limit {
		n, err := f.Read(buf)
		lines += bytes.Count(buf[:n], []byte{'\n'})
		if n > 0 {
			unterminated = buf[n-1] != '\n'
		}
		if err == io.EOF {
			if unterminated {
				lines++
			}
			return min(lines, limit), nil
		}
		if err != nil {
			return 0, err
		}
	}
	return min(lines, limit), nil
}
