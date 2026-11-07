// sessionusage.go declares the provider-neutral token reading of one session.
// It also declares the optional UsageReader capability an engine implements to supply that reading.

package shuttleengine

// SessionUsage is one session's token reading with the session's forks included.
// The zero value is the unknown reading.
type SessionUsage struct {
	// Known is true when the provider could read the session; every other field is zero otherwise.
	Known bool
	// Fresh is the input, cache-creation and output tokens of the session and its forks.
	Fresh int64
	// CacheRead is the cache-read tokens of the session and its forks.
	CacheRead int64
	// Forks is the number of fork transcripts the reading includes.
	Forks int
	// ForkFresh is the part of Fresh the forks account for.
	ForkFresh int64
	// ForkCacheRead is the part of CacheRead the forks account for.
	ForkCacheRead int64
	// SplitKnown is true when the provider read the four counts apart; the four are zero otherwise.
	SplitKnown bool
	// Input is the uncached input tokens of the session and its forks.
	Input int64
	// CacheWrite is the cache-creation tokens of the session and its forks.
	CacheWrite int64
	// Output is the output tokens of the session and its forks.
	Output int64
}

// UsageReader is an optional capability an engine may implement: the provider's token reading of a session.
// The run reads it at every terminal outcome, and an engine without it leaves the usage unknown.
type UsageReader interface {
	// SessionUsage returns the token reading of session sessionID, recorded under the pane cwd workdir, forks included.
	// It never errors: a session it cannot read yields the unknown reading.
	SessionUsage(sessionID, workdir string) SessionUsage
}
