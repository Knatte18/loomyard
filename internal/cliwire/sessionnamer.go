// sessionnamer.go chooses the provider that fills reed's session-name seam.

package cliwire

import (
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
)

// SessionNamer returns the provider reed's watchdog uses to read and repair a session's own name.
// It is the one place a provider is chosen for reed, which stays provider-blind.
func SessionNamer() reedengine.SessionNamer {
	return claudeengine.New()
}
