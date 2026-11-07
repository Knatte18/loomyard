// Package haltnote is the sole renderer and parser of the halt note loom writes into its friction directory.
// Loom writes the note when a run halts `blocked` or `failed`.
// Render produces the note's bytes.
// Parse reads them back tolerantly for a later reader.
// IsNoteName tells a halt note from any other file in the directory.
// It imports the standard library only.
// A reader outside the loom command layer can therefore parse a note without importing that layer.
package haltnote
