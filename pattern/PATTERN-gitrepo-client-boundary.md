# PATTERN-gitrepo-client-boundary

`internal/gitrepo` splits local from remote by client: go-git and the `dotgit` geometry read own local reads, and `gitexec` owns anything remote-authenticating or working-tree-mutating.

- The parity oracle `internal/gitrepo/internal/gitoracle` imports only the standard library and `gitexec`, never `gitrepo`, enforced by its leaf test.
