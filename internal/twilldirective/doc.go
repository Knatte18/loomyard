// doc.go carries the package godoc for twilldirective: what the directive says, who renders it and why the package derives no path.

// Package twilldirective renders the twill directive, so a writer that has settled its record reviews it through lenses, one fork per lens, before it ends.
//
// # What it says
//
// Directive renders the lens review procedure, which the writer runs once its files are written and its self-check passes: it reads its rubric, spawns one fork per lens in one message and judges the findings the forks return.
// The writer judges each finding as a critical fixer: it disputes the finding on a cited ground, fixes it with its own change, or fixes it as proposed.
// Each fork's boilerplate tells it to review the written files through its lens as its inherited context holds them, without reading them again, and return its findings as its final message.
// Spawning after the write, rather than before it, lets the forks start on the finished files with nothing to wait for.
// The block is host-neutral: every host-specific value (the reviewed files, the spawn point, the input a finding's premise is weighed against, the evidence kinds a dispute cites, the severity rule, the rubric, the lenses, the steps, the self-check command and the place the review is recorded) arrives through Values, so a later host can render it with its own values.
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
