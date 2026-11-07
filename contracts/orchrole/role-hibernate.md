<!-- This is the orch role's on-demand file for hibernating the machine, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Hibernate

Run `lym hibernate request`, then end the turn.
Pass `--after <slug>` when the operator names a task to land first, and `--after-all` when they name the runs in flight.
Add `--resume-on-wake` when they ask for the runs to go on by themselves after the wake.
Add `--resume-on-wake=false` when they ask for the hub to stay paused.

lym pauses the hub, confirms every session paused, and hibernates the machine.
It mails you a notice when the request is done or failed.
The wake lifts that pause only when the request resumes on wake, by its flag or else the hub's `hibernate.yaml`;
otherwise the notice says the hub stays paused, and `lym tender resume` lifts it when the operator says so.
`lym hibernate status` shows the request and `lym hibernate cancel` withdraws it.
