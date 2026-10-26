// batchpause.go holds the up-front refusal begin-batch, recover-batch, fix-verify and fix-plan make under a pause, and the lookup of Master's own strand that refusal claims into the pause.

package webstercli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// envMasterStrand returns the lookup of the calling strand's name and guid: the name is read from the strand-name environment variable and the guid is resolved from it through resolve.
// An unset or empty variable is an error naming it, and a resolve error is returned as is.
func envMasterStrand(resolve func(name string) (string, error)) func() (name, guid string, err error) {
	return func() (string, string, error) {
		name := os.Getenv(agentname.StrandNameEnv)
		if name == "" {
			return "", "", fmt.Errorf("%s is not set", agentname.StrandNameEnv)
		}
		guid, err := resolve(name)
		if err != nil {
			return name, "", err
		}
		return name, guid, nil
	}
}

// refuseCoveredBatch returns the refusal message of a batch verb a pause holds, or the empty string when the verb goes on.
// It is refuseCovered for the call `<verb> <batch>`.
func (c *websterCLI) refuseCoveredBatch(verb, batch string) string {
	return c.refuseCovered(verb+" "+batch, verb+" starts no new batch", "a running batch finishes, and ")
}

// refuseCovered returns the refusal message of the `lyx webster <call>` a pause holds, or the empty string when the call goes on.
// held says what the refused call does not start, and running, empty or ending in a space, what keeps running meanwhile.
// Under a covering entry it claims Master into the pause, so `lyx tender resume` gives Master its resume line;
// a Master that cannot be claimed is refused with a way forward that needs no resume line.
// A pair with no pause geometry, an unreadable record and an entry lifted before the claim all go on.
func (c *websterCLI) refuseCovered(call, held, running string) string {
	if c.pause == (pauserecord.Geometry{}) {
		return ""
	}
	now := time.Now()
	coverage, err := pauserecord.ReadCoverage(c.pause.RecordPath, c.pause.Slug, now)
	if err != nil {
		logger.Warn("webstercli: the pause record could not be read, so the verb goes on", "call", call, "record", c.pause.RecordPath, "err", err)
		return ""
	}
	if !coverage.Covered {
		return ""
	}
	holder := "a hub-wide pause"
	if coverage.Entry.Scope != pauserecord.ScopeHub {
		holder = "the pause of " + coverage.Entry.Scope
	}
	name, guid, err := c.masterStrand()
	if err == nil {
		var covered bool
		_, covered, err = pauserecord.ClaimSession(c.pause.RecordPath, c.pause.Slug, pauserecord.Session{Strand: name, GUID: guid, Slug: c.pause.Slug}, now)
		if err == nil && !covered {
			return ""
		}
		if err == nil {
			return fmt.Sprintf("webster: %s holds this pair, so %s; way forward: end your turn now; %sonce the resume line arrives run \"lyx webster %s\" again", holder, held, running, call)
		}
	}
	logger.Warn("webstercli: Master could not be claimed into the pause", "strand", name, "err", err)
	return fmt.Sprintf("webster: %s holds this pair, so %s, and no resume line will come; way forward: end your turn now, then run \"lyx webster %s\" again once \"lyx tender status\" shows no pause", holder, held, call)
}
