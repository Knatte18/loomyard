// doc.go carries the package godoc for twilldirective: what the directive says, who renders it and why the package derives no path.

// Package twilldirective renders the twill directive, so a writer that has settled its record reviews it through lenses, one fork per lens, before it ends.
//
// # What it says
//
// Directive renders the lens review procedure, which the writer runs once its files are written and its self-check passes: it reads its rubric, spawns one fork per lens in one message and judges the findings the forks return.
// The writer edits nothing while any fork is outstanding and judges all findings together once the last has returned, so duplicates across lenses merge and no finding cites text an earlier fix has changed.
// A host may ask for a writer-fork part through Values.WriterFork: one fork, fork0, then writes the reviewed files while the writer explores before it and reviews what it wrote after.
// The part replaces the relaunch paragraphs, which a host without a writer fork keeps; exactly one of the two is rendered, each from its own stencil.
// Exactly one judge part is rendered, each from its own stencil: the critical fixer for a host that asks for it through Values.CriticalJudge, where the writer disputes a finding on a cited ground, fixes it with its own change, or fixes it as proposed, and otherwise the judge that fixes or rejects each finding by the host's rejection rule.
// Once the last fork has returned, and before judging, the writer saves each fork's final message verbatim in a lens report of its own, so an audit reads every report from the run records.
// Each fork's boilerplate tells it to review the written files through its lens as its inherited context holds them, without reading them again, and return its findings as its final message.
// Spawning after the write, rather than before it, lets the forks start on the finished files with nothing to wait for.
// The block is host-neutral: every host-specific value (the reviewed files, the spawn point, the rejection rule, for a host without a writer fork the files a relaunched writer finishes, for a host that asks for the critical judge the input a finding's premise is weighed against and the evidence kinds a dispute cites, the severity rule, the rubric, the lenses, the steps, the self-check command, the place the review is recorded, and the path and format spec of the lens reports) arrives through Values, so a later host can render it with its own values.
//
// # Who renders it
//
// Each spawning module calls Directive once per prompt and passes the text as the value of the MarkerName marker its template carries, an optional one, so a prompt that renders no twill block keeps its bytes.
// The module builds Values from its own names and converts its lenses to this package's Lens, since the package imports no feature package.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path: the stencil is read at call time from the told stencils directory through `stencilstore.Read`, never from embedded bytes.
package twilldirective
