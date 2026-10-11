// doc.go carries the package godoc for twilldirective: what the two blocks say, who renders them and why the package derives no path.

// Package twilldirective renders the twill blocks, so a writer that has settled its record reviews it through lenses, one fork per lens, before it ends.
//
// # What it says
//
// Directive renders the lens review procedure: the writer reads its rubric, spawns one fork per lens in one message, writes and self-checks its files, writes a ready marker and judges the findings the forks return.
// Each fork's boilerplate tells it to wait for the ready marker through `lyx shuttle await`, review the written files through its lens and return its findings as its final message.
// WriteScope renders the paragraph that adds the ready marker to the writer's write scope.
// Both blocks are host-neutral: every host-specific value (the reviewed files, the marker, the rubric, the lenses, the steps, the self-check command and the place the review is recorded) arrives through Values, so a later host can render them with its own values.
//
// # Who renders it
//
// Each spawning module calls Directive and WriteScope once per prompt and passes the texts as the values of the MarkerName and WriteScopeMarkerName markers its template carries, optional ones, so a prompt that renders no twill block keeps its bytes.
// The module builds Values from its own names and converts its lenses to this package's Lens, since the package imports no feature package.
//
// # Imports
//
// The package imports only the standard library, `stencil` and `stencilstore`.
// It derives no path: the stencils are read at call time from the told stencils directory through `stencilstore.Read`, never from embedded bytes.
package twilldirective
