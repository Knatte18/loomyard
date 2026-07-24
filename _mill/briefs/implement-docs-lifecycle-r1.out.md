Both card commits (`docs(gitrepo): fold design rationale into package doc, delete manifest draft` for Card 10, and `docs: move gitrepo to Done and add it to the module map` for Card 11) appear in the range since batch start. 2 of 2 cards committed — all cards complete.

Summary of work done (batch `04-docs-lifecycle`, both cards committed):

- `/home/knatte/Code/loomyard/wts/gitrepo/internal/gitrepo/doc.go` — expanded the package doc comment to carry the durable design rationale (relationship to `gitexec`, the `Repo` API, self-correcting snapshot pattern, `SHAExists` history-rewrite safety, scope boundaries, push surface, snapshot remote model).
- `/home/knatte/Code/loomyard/wts/gitrepo/manifest/designs/gitrepo.md` — deleted (module landed).
- `/home/knatte/Code/loomyard/wts/gitrepo/manifest/designs/fabric.md`, `/home/knatte/Code/loomyard/wts/gitrepo/manifest/designs/semantic-index.md` — retargeted links from the deleted draft to `internal/gitrepo`.
- `/home/knatte/Code/loomyard/wts/gitrepo/manifest/roadmap.md` — moved `gitrepo` from Planned to Done.
- `/home/knatte/Code/loomyard/wts/gitrepo/docs/overview.md` — added `internal/gitrepo/` to the file-tree and shared-infrastructure sentence.

Verify (`go build ./internal/gitrepo/`) passed.

{"status":"success","commit_sha":"b216c6f3","session_id":"9242c4c5-6f00-45a2-a7e2-044607c16f9f"}
