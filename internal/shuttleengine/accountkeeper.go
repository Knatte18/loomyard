// accountkeeper.go declares AccountKeeper, the optional engine capability that assigns a new worktree its account.

package shuttleengine

// AccountKeeper is the optional capability of an engine that spreads its sessions over several accounts.
// A caller reaches it through the engine registry and never names the engine.
type AccountKeeper interface {
	// PickAccount picks the account of the worktree the engine is told, once, when the worktree is made on this machine.
	// With keepEntry set, a worktree that already holds an account keeps it and is not counted again.
	// It writes only machine-local state, and a worktree the engine does not spread, such as the prime, is left as it is.
	PickAccount(keepEntry bool) error
	// SetAccount sets the told worktree's account to the listed account name.
	// It takes effect at the worktree's next launch the engine builds, and a running strand keeps its account.
	// A non-nil error refuses the change, naming the way forward.
	SetAccount(name string) error
	// ShowAccounts returns a readable report of the told worktree's resolved account, the pins and each account's usage.
	ShowAccounts() (string, error)
	// RecordStatusLine records the usage the provider's status line payload input reports for the listed account name.
	// It returns one short line for the status line to show.
	// A non-nil error refuses a name no listed account has, and nothing is recorded.
	RecordStatusLine(name string, input []byte) (string, error)
}
