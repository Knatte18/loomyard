// overlay.go reads and words the stdin side of `lyx loom validate-plan --overlay`: the proposed plan file contents the verb checks in memory, and the refusal for each way that input or its names are unusable.

package loomcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// errOverlayNoInput is the cause readOverlay reports for an empty or whitespace-only input, and the verb for a terminal stdin.
var errOverlayNoInput = errors.New("no input")

// errOverlayMalformed is the sentinel readOverlay wraps when the input is not exactly one JSON object of distinct names mapped to string values.
var errOverlayMalformed = errors.New("malformed overlay")

// inputIsTerminal reports whether in is a character device such as a terminal.
// Checked before any read, so a terminal never blocks the verb.
func inputIsTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// readOverlay reads all of in as one JSON object mapping plan file names to their full proposed content.
// Empty or whitespace-only input is errOverlayNoInput.
// Input that is not exactly one JSON object, a name that occurs twice and a value that is not a string wrap errOverlayMalformed, naming the offending name where there is one.
func readOverlay(in io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("read overlay: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errOverlayNoInput
	}

	malformed := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", errOverlayMalformed, fmt.Sprintf(format, args...))
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if open, err := dec.Token(); err != nil {
		return nil, malformed("%v", err)
	} else if delim, ok := open.(json.Delim); !ok || delim != '{' {
		return nil, malformed("want one JSON object, got %v", open)
	}

	files := make(map[string]string)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, malformed("%v", err)
		}
		name, _ := key.(string)
		if _, seen := files[name]; seen {
			return nil, malformed("name %q occurs twice", name)
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, malformed("%v", err)
		}
		content, isString := value.(string)
		if !isString {
			return nil, malformed("the value of %q is not a string", name)
		}
		files[name] = content
	}
	if _, err := dec.Token(); err != nil {
		return nil, malformed("%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, malformed("want one JSON object, got a second value after it")
	}
	return files, nil
}

// overlaidPlan parses the plan under planDir with the proposed contents read from in laid over it.
// A refusal is the wording of an unusable input or name, and err is a plan that does not parse; at most one is set.
func overlaidPlan(in io.Reader, planDir string) (plan *planparser.Plan, refusal string, err error) {
	if inputIsTerminal(in) {
		return nil, overlayRefusal(errOverlayNoInput), nil
	}
	files, err := readOverlay(in)
	if err != nil {
		if refusal := overlayRefusal(err); refusal != "" {
			return nil, refusal, nil
		}
		return nil, "loom: validate-plan --overlay: " + err.Error(), nil
	}
	overlay, err := planparser.NewOverlay(planDir, files)
	if err == nil {
		plan, err = overlay.Parse()
	}
	if refusal = overlayRefusal(err); refusal != "" {
		return nil, refusal, nil
	}
	return plan, "", err
}

// overlayRefusal words the refusal for an error of readOverlay or of planparser.NewOverlay, as `loom: validate-plan --overlay: <cause>; way forward: <step>`.
// It returns "" for any other error, which is a plan that does not parse.
func overlayRefusal(err error) string {
	const prefix = "loom: validate-plan --overlay: "
	const objectForm = "pipe one JSON object mapping plan file names to their full proposed content on stdin, as `lyx loom validate-plan --help` shows"
	switch {
	case errors.Is(err, errOverlayNoInput):
		return prefix + err.Error() + "; way forward: " + objectForm
	case errors.Is(err, errOverlayMalformed):
		return prefix + err.Error() + "; way forward: " + objectForm + ", with distinct names and string values"
	case errors.Is(err, planparser.ErrOverlayName):
		return prefix + err.Error() + "; way forward: name only `00-overview.md` or a card file the Card Index lists, and add or remove a card with `lyx loom insert-card` or `lyx loom remove-card`"
	case errors.Is(err, planparser.ErrOverlayFileMissing):
		return prefix + err.Error() + "; way forward: write the card file the Card Index names, then re-run"
	}
	return ""
}
