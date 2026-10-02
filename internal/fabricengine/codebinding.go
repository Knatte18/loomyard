// codebinding.go owns the .lyx-code record: a plain single-line file at the board root holding the
// repo's short agent-name code, recorded once on weft:main beside .lyx-warp.
// Like the warp binding, it is written to disk here but committed onto weft:main by the CLI layer
// through Bolt; this file spawns no git and never calls Bolt itself.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// CodeFileName is the filename of the recorded repo code at the board root (<boardDir>/.lyx-code).
// It holds only the code, plus a trailing newline.
const CodeFileName = ".lyx-code"

// ReadCode reads the recorded repo code from <boardDir>/.lyx-code and reports whether a usable one
// was found.
// Any read error and an empty-after-trim value report not found, like readWarpBinding;
// a value failing agentname.ValidateCode also reports not found and logs a named warning,
// so a damaged record reads as an uncoded hub that `lyx fabric code <code>` or `clone --code` records over.
func ReadCode(boardDir string) (code string, found bool) {
	data, err := os.ReadFile(filepath.Join(boardDir, CodeFileName))
	if err != nil {
		return "", false
	}
	return usableCode(string(data), boardDir)
}

// usableCode applies the blank-and-grammar rule shared by ReadCode and the weft probe.
// where names the record's location in the warning.
func usableCode(raw, where string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if err := agentname.ValidateCode(trimmed); err != nil {
		logger.Warn("fabricengine: recorded repo code is invalid; treating the hub as uncoded", "file", CodeFileName, "where", where, "value", trimmed, "error", err)
		return "", false
	}
	return trimmed, true
}

// WriteCode writes code plus a trailing newline to <boardDir>/.lyx-code, replacing any existing content.
// The caller commits the result onto weft:main through Bolt; this function only touches the working tree.
func WriteCode(boardDir, code string) error {
	path := filepath.Join(boardDir, CodeFileName)
	if err := os.WriteFile(path, []byte(code+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// resolveEffectiveCode encodes the whole code-record rule, pure and git-free.
// recorded and found are the record at the probed weft, supplied is the --code value,
// and freshBind is true when the weft carries neither the warp binding nor the anchor marker.
// It returns the effective code, whether the caller must write a new record, a warning for a
// bound weft left uncoded, or a refusal.
func resolveEffectiveCode(recorded string, found bool, supplied string, freshBind bool) (effective string, writeRecord bool, warning string, err error) {
	if supplied != "" {
		if err := agentname.ValidateCode(supplied); err != nil {
			return "", false, "", err
		}
	}
	if found {
		if supplied == "" || supplied == recorded {
			return recorded, false, "", nil
		}
		return "", false, "", fmt.Errorf("this repo's recorded code is %q, not %q; refusing to change it (`lyx fabric code` cannot change a recorded code either) — drop --code, or pass --code %s", recorded, supplied, recorded)
	}
	if supplied != "" {
		return supplied, true, "", nil
	}
	if freshBind {
		return "", false, "", fmt.Errorf("a fresh clone needs the repo's short code: pass --code <code>, 2-6 characters matching [a-z][a-z0-9]{1,5}, e.g. lyx fabric clone --code lx <weft-url> <warp-url>")
	}
	return "", false, "this weft records no repo code, so no strand can be named until one is set; way forward: run `lyx fabric code <code>` in the hub", nil
}
