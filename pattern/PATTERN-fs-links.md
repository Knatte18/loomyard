# PATTERN-fs-links

Every filesystem link lym creates goes through `internal/fslink`'s `CreateDirLink`, which owns the platform difference between link kinds.

- The scan in `cmd/lym/fslinkonly_test.go` parses every production Go file of the module, `tools/` included, and reports each `os.Symlink` or `os.Link` call, through whatever name the file binds `os` to, outside `internal/fslink`; a dot import of `os` binds no name, so the scan reports the import line itself.
- The scan's allowlist is keyed `<file>:<call>` and admits one call with a reason naming the obstacle to going through fslink.
  `tools/deploy/main.go` is admitted for `os.Symlink`: it copies a plugin-tree symlink verbatim, the link may point at a file, and fslink has no file-link primitive.
- Bound: test files are not scanned, because tests plant links as hostile or foreign fixtures that fslink's refusals would block.
- The entry claims nothing about how a link behaves on Windows: fslink's Windows path has never been executed, and `internal/fabricengine`'s package documentation records what settling it takes.
