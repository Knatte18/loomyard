Perfect! All work is complete. The batch has been successfully implemented with all three cards committed:

**Card 1:** Rebuilt overview.md module/lib map with status markers
- Removed links to deleted module docs (board, worktree, ide, muxpoc)
- Added ✅ Implemented markers for landed modules
- Added 🚧 Design — not built marker for mux
- Updated shared-libs list and removed (planned) marker from internal/state
- Removed (see modules/*.md) comments from directory-tree
- Updated Other docs section with research/ and reference/ folder references

**Card 2:** Added Documentation lifecycle section and fixed portal method documentation
- Added new "Documentation lifecycle" section stating the delete-on-landing convention
- Added missing PortalLink(slug) to Path Invariants Layout method list
- Tagged PortalsDir, PortalLink, PortalTarget as deprecated (removed in task 006)
- Clarified that weft has no Go implementation yet; portals are still live

**Card 3:** Updated CONSTRAINTS.md with deprecation tags and convention pointer
- Tagged PortalsDir, PortalLink, PortalTarget as deprecated (removed in task 006)
- Added pointer to documentation-lifecycle convention in docs/overview.md

{"status":"success","commit_sha":"eee6264533217b69a249d02c31587ae0811497b2","session_id":"cb4758c9-a2f2-4349-8141-1a82d19dbf52"}