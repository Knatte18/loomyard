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
//
// # Lifecycle
//
// Run is one server's life, and the lock in the state directory allows one server of a hub at a time.
// The holder truncates the log to open it with its pid, removes the former state file and registers itself.
// It then binds a loopback port and writes its pid, port and token as the state file, which the user alone can read.
// Live reads the lock, so a state file left by an exited server says nothing.
// One watch loop wakes the long polls when board.json changes.
// The same loop ends the server when its binary is replaced or no /api request has arrived within the idle bound.
// On exit the server takes no new connection, frees the lock and then waits out the requests in flight,
// so a write completes while a new server may already start.
// WriteLauncher writes the page that hands a browser the token in the address's fragment.
package boardserve
