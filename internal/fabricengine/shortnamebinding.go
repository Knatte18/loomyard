// shortnamebinding.go owns the .lym-shortname record, a plain single-line file at the board root.
// It holds the repo's shortname, recorded once on weft:main beside .lym-warp.
// Like the warp binding, it is written to disk here but committed onto weft:main by the CLI layer through Bolt;
// this file spawns no git and never calls Bolt itself.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
)

// ShortnameFileName is the filename of the recorded repo shortname at the board root (<boardDir>/.lym-shortname).
// It holds only the shortname, plus a trailing newline.
const ShortnameFileName = ".lym-shortname"

// ReadShortname reads the recorded repo shortname from <boardDir>/.lym-shortname.
// It reports whether a usable one was found.
// A read error, or a value that is empty after trimming, reports not found.
// A value that fails the shortname grammar also reports not found and logs a named warning,
// so a damaged record reads as a hub with no shortname.
// `lym fabric shortname <shortname>` or `clone --shortname` records a new one over it.
func ReadShortname(boardDir string) (shortname string, found bool) {
	data, err := os.ReadFile(filepath.Join(boardDir, ShortnameFileName))
	if err != nil {
		return "", false
	}
	return usableShortname(string(data), boardDir)
}

// usableShortname returns the trimmed raw value when it is a non-blank, valid shortname.
// A blank value reports not found; an invalid one reports not found and logs a warning.
// where names the record's location in that warning.
func usableShortname(raw, where string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if err := agentname.ValidateShortname(trimmed); err != nil {
		logger.Warn("fabricengine: recorded repo shortname is invalid; treating the hub as having no shortname", "file", ShortnameFileName, "where", where, "value", trimmed, "error", err)
		return "", false
	}
	return trimmed, true
}

// WriteShortname writes shortname plus a trailing newline to <boardDir>/.lym-shortname, replacing any existing content.
// The caller commits the result onto weft:main through Bolt; this function only touches the working tree.
func WriteShortname(boardDir, shortname string) error {
	path := filepath.Join(boardDir, ShortnameFileName)
	if err := os.WriteFile(path, []byte(shortname+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// resolveEffectiveShortname decides the effective repo shortname from the record and the supplied value, without git.
// recorded and found are the record at the probed weft, and supplied is the --shortname value.
// freshBind is true when the weft carries neither the warp binding nor the anchor marker.
// It returns the effective shortname and whether the caller must write a new record.
// It also returns a warning for a bound weft that has no shortname, and an error when the request is refused.
func resolveEffectiveShortname(recorded string, found bool, supplied string, freshBind bool) (effective string, writeRecord bool, warning string, err error) {
	if supplied != "" {
		if err := agentname.ValidateShortname(supplied); err != nil {
			return "", false, "", err
		}
	}
	if found {
		if supplied == "" || supplied == recorded {
			return recorded, false, "", nil
		}
		return "", false, "", fmt.Errorf("this repo's recorded shortname is %q, not %q; refusing to change it (`lym fabric shortname` cannot change a recorded shortname either) — drop --shortname, or pass --shortname %s", recorded, supplied, recorded)
	}
	if supplied != "" {
		return supplied, true, "", nil
	}
	if freshBind {
		return "", false, "", fmt.Errorf("a fresh clone needs the repo's shortname: pass --shortname <shortname>, 2-6 characters matching [a-z][a-z0-9]{1,5}, e.g. lym fabric clone --shortname lx <weft-url> <warp-url>")
	}
	return "", false, "this weft records no repo shortname, so no strand can be named until one is set; way forward: run `lym fabric shortname <shortname>` in the hub", nil
}
