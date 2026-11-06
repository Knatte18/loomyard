<!-- This is the orch role's on-demand file for hibernating the machine, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Hibernate

Run `lym hibernate request`, with `--after <slug>` when the operator names a task to land first or `--after-all` when they name the runs in flight, then end the turn.
Add `--resume-on-wake` when they ask for the runs to go on by themselves after the wake, and `--resume-on-wake=false` when they ask for the hub to stay paused.

lym itself pauses the hub, confirms every session paused, hibernates the machine and mails you a notice when it is done or failed.
The wake lifts that pause only when the request resumes on wake, by its flag or else the hub's `hibernate.yaml`; otherwise the notice says the hub stays paused, and `lym tender resume` lifts it when the operator says so.
`lym hibernate status` shows the request and `lym hibernate cancel` withdraws it.
