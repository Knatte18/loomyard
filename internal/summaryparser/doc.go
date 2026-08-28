// Package summaryparser is the sole declarer of the final-summary artifact's filename and the sole
// parser of its format.
// It takes told paths and declares no directory of its own -- callers resolve the containing
// directory themselves and pass it in.
// It imports the standard library only, so a second last-content producer can satisfy the same
// read contract without depending on any producer.
package summaryparser
