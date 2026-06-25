{"status":"success","commit_sha":"102be8f058bd3ec4c0eba22433e78236bdd68d46","session_id":"41d22672-df8c-4c2c-bc99-6d1cf52c4bcf"}

All four cards of batch 2 (templates-home) are complete:

- Card 7: board embeds its own template via `//go:embed`, drops `configtmpl` delegation.
- Card 8: worktree — same pattern.
- Card 9: weft — same pattern.
- Card 10: `internal\configreg\configreg.go` rewritten to import the three feature packages directly; entire `internal\configtmpl\` directory deleted (4 files). Zero `configtmpl` references remain in Go source.

Verify passes for all 26 packages.

{"status":"success","commit_sha":"102be8f058bd3ec4c0eba22433e78236bdd68d46","session_id":"41d22672-df8c-4c2c-bc99-6d1cf52c4bcf"}
