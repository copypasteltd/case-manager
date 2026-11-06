# F-G11-01 — independent refutation

Increment168 source-only review, independently following callers and guards. No tests, builds, services, browsers, virtualization or application edits.

**Verdict: survives refutation, narrowly confirmed; high confidence.** The defect concerns an already-authenticated human continuation/captured Actor after completed session revocation. It is not a fresh-token logout bypass or unrestricted worker access.

Requirement: `docs/requirements/02-identity-and-permissions.md:55`: “权限缓存命中也需要确认会话未撤销和安全版本有效。无法取得授权状态时受限读取/写入 fail closed。” The demanded property is current original human-session validity at the protected service transaction, in addition to current organizational/user/resource authority.

| Counterargument checked | Actual source / result |
|---|---|
| HTTP authentication already prevents this | `httpapi/server.go:247,278` authenticates once before passing Actor to the handler; `identity/service.go:277,284` rejects a newly presented revoked token. Correct, but does not recheck a captured Actor after authentication. The SSE route (`httpapi/agent_runs.go:115`) is one bounded batch, not an indefinitely reused identity. |
| Logout invalidates the versions checked downstream | `identity/service.go:384,393` changes session revocation only. `identity/account_security.go:114,135` revokes the selected family, without changing user/organization authorization versions. Thus unchanged-version continuation is reachable. |
| A deeper shared guard supplies session validation | `agent/conversations.go:172` checks organization/user status and versions only. `GetRun`/`RunEvents` (`run_reads.go:94,255`) and `GetPlan`/`ListPlans` (`plans.go:335,358`) use it. `run_cancel.go:21` authorizes through readableRun/require; `platform/commands.go:54` checks versions and callback before replay. `authz/policy.go:18` and `migrations/0011_project_viewer_policy.sql:3` check ACLs without session identity. No session check closes this path. |

Source-level schedule, **not an executed test**: authenticate human A; logout or family revocation commits; continuation reaches ordinary conversation/run/plan read or CancelRun with A. If current membership, versions and ACLs still match, those guards accept; cancellation can also replay an earlier receipt. Exact `identity.RequireInteractiveSession` (`interactive.go:16`) would reject A's revoked/expired original row.

Repair belongs at human snapshot/public command boundaries. Do not blanket-modify `require`, `readableRun` or `NotificationRun`: `agent/execution.go:87` reconstructs sessionless worker identity; `agent/context.go:145` checks synthetic audience identities; `agent/telegram.go:96` constructs TG identity; `notification/jobs.go:207` constructs recipient identity used by `notification/access.go:170`. These require their existing execution/channel/current-recipient authority, not an invented interactive session.

Searches: `readSnapshot|readableRun|CancelRun`, `RequireInteractiveSession|Authenticate|Logout`, `family_id|revoked_at`, `NotificationRun|core.Actor`, and all can_access definitions. Remaining concern is verification of the proposed repair after the complete code wave; actual model/platform acceptance remains separate.
