// Package burlermarker derives the machine-local ready marker of a burler round from the round's review path.
//
// The marker is the file Go writes once a review is on disk and the fixer's `lyx burler await-review` waits on.
// Path is the one derivation of it: both shedadapters.BurlerProducer and `lyx burler run` call it,
// so any process re-derives the same marker and no caller builds its own.
//
// The rule: the review path is resolved against root when relative and must lie inside root.
// Its path relative to root drops a leading lyxdirs.LyxDirName segment,
// and the marker is that path joined under base's lyxdirs.DotLyxDirName directory, with the suffix ".ready" on the file name.
// In hub geometry root and base are both the anchor, so the marker mirrors the review's durable subpath under the ephemeral directory.
// In standalone geometry root is the reviewed target and base the told state directory.
//
// The package lives outside internal/burlerengine because the engine is told the result and derives no path.
package burlermarker
