// Package perchengine is the deterministic gate loop over burler rounds: it
// spawns a fresh burlerengine round each iteration, reads its verdict, and
// decides APPROVED or stuck via a milestone cap ladder and an ephemeral
// progress judge — never by trusting a burler's own self-grading. This
// header is intentionally concise; later batches in this task expand it
// into the full durable design header (mirroring burlerengine's doc.go)
// once docs/modules/perch.md is deleted per the documentation lifecycle.
package perchengine
