// Package boardserve is the board's loopback HTTP server.
// It serves the viewer page, the board's view projection, its live changes and the page's writes.
//
// # Routes
//
// GET / serves the viewer page with a Content-Security-Policy that allows only its own inline script and style.
// GET /api/board answers the view projection with the board revision.
// With `after=<revision>` equal to the current revision it waits for another revision.
// That wait answers "unchanged" at its bound or when the server drains.
// GET /file/<name> serves a design or decision file the projection lists, as plain text.
// POST /api/write runs one operation: create, edit, release, promote, draft or undraft.
// It answers the entry as the write left it.
// A stale revision answers a conflict with the fresh entry, and any other refusal answers with its message.
//
// # Checks
//
// Every request's Host must be `127.0.0.1:<port>`, which stops DNS rebinding.
// An /api request needs the server's token as a bearer token, and the file route needs it as the `t` query parameter.
// A write needs Content-Type application/json, an Origin that is the server's own when present, and a bounded body.
// No response carries a CORS header, and every response forbids caching and content sniffing.
//
// # Writes
//
// The board is written only through boardengine's exported Board methods.
// Each write expects the revision of the entry the page read, which the board checks under its write lock.
// Release, draft and undraft also judge the entry's kind and status in a view of that revision.
package boardserve
