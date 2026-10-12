// Package haltnote is the sole renderer and parser of the Go-authored halt note loom writes into its friction directory when a run halts `blocked` or `failed`.
// Render produces the note's bytes, Parse reads them back tolerantly for a later reader, and IsNoteName tells a halt note from any other file in the directory.
// It imports the standard library only, so a reader outside the loom command layer can parse a note without importing it.
package haltnote
