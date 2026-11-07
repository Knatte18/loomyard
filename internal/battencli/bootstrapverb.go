// bootstrapverb.go declares batten's bootstrap-verb capability constant.

package battencli

// BootstrapVerb records that batten has no bootstrap verb.
//
// The empty value is the declaration, not an unfilled placeholder.
// batten has no `lym batten start`, so the driver of `lym batten run <slug>` is the process the operator typed,
// and there is no spawn seam to branch on a seed's driver.
// The `--driver` validator in arm.go reads this constant directly.
// It does so because battencli cannot import shedcli without an import cycle.
// The value changes only once batten grows a bootstrap verb.
// At that point, `driver: llm` becomes supportable for batten with no edit to the validators that read this constant.
const BootstrapVerb = ""
