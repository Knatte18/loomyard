// doc.go holds the package documentation for twilldirective.

// Package twilldirective renders the twill directive.
// A writer that has settled its record reviews it through lenses, one fork per lens, before it ends.
//
// # What it says
//
// Directive renders the lens review procedure.
// The writer runs it once its files are written and its self-check passes.
// The writer reads its rubric, spawns one fork per lens in one message, and judges the findings the forks return.
// It edits nothing while any fork is outstanding.
// It judges all findings together once the last has returned.
// That lets duplicates across lenses merge, and no finding cites text an earlier fix has changed.
// It waits for the forks inside `lym shuttle await-forks` rather than by ending its turn,
// so it takes every fork's result in one turn instead of waking once per fork.
// A host may ask for a writer-fork part through Values.WriterFork.
// One fork, fork0, then writes the reviewed files while the writer explores before it and reviews what it wrote after.
// The part replaces the relaunch paragraphs, which a host without a writer fork keeps;
// exactly one of the two is rendered, each from its own stencil.
// A host whose files a gate checks once the writer's turn ends sets Values.Gated.
// The writer-fork part is then the one without its self-check steps,
// and the directive's closing step ends the turn without a final self-check.
// The forks are not told the files passed one.
// Exactly one judge part is rendered, each from its own stencil.
// A host that asks for it through Values.CriticalJudge gets the critical fixer.
// There the writer disputes a finding on a cited ground, fixes it with its own change, or fixes it as proposed.
// Any other host gets the judge that fixes or rejects each finding by the host's rejection rule.
// Once the last fork has returned, and before judging, the writer saves each fork's final message verbatim.
// Each goes in a lens report of its own, and an audit then reads every report from the run records.
// Each fork's boilerplate tells it to review the written files through its lens as its inherited context holds them.
// The fork does not read them again, and it returns its findings as its final message.
// Spawning after the write lets the forks start on the finished files with nothing to wait for.
// The block is host-neutral: every host-specific value arrives through Values.
// A later host can render it with its own values, and the field docs of Values say which part reads each one.
//
// # Who renders it
//
// Each spawning module calls Directive once per prompt.
// It passes the text as the value of the optional MarkerName marker its template carries,
// so a prompt that renders no twill block keeps its bytes.
// The module builds Values from its own names and converts its lenses to this package's Lens.
// The package imports no feature package.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path.
// The stencil is read at call time from the told stencils directory through `stencilstore.Read`.
// It is never read from embedded bytes.
package twilldirective
