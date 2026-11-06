# Independent refutation — GAP-23 human status reads

Read-only second-context review of the proposed original-session gap, during increment168 and before tests/implementation. Verdict: **retained, narrowed to human read entry points after actor capture**. No tests, builds, browser, services, devices, virtualization or application changes were performed.

## Requirement and decisive trace

`docs/requirements/02-identity-and-permissions.md:55` requires: “权限缓存命中也需要确认会话未撤销和安全版本有效。无法取得授权状态时受限读取/写入 fail closed。”

`app/backend/internal/httpapi/server.go:276–289` authenticates at278 and then passes a captured actor to the reader at283. It does not hold the original session row through the business read. New HTTP requests using a token already revoked before authentication remain rejected; the candidate is **not** a blanket ability to use revoked tokens on a fresh request.

`app/backend/internal/servicestatus/service.go:133–138` gets metadata through `s.DB` and calls `present`; List140–195 similarly queries `s.DB`, filters by `can_access` and calls `present`. `present:79–131` checks service.read, then reads sources and maintenance projections, but never reads the original session. Thus a previously captured actor can retain read access after logout/family revocation when user/roles are otherwise unchanged.

`app/backend/internal/authz/policy.go:18–28` passes organization/user/resource/channel/AI state to `can_access`, **not SessionID**. Its current definition in `app/backend/migrations/0011_project_viewer_policy.sql:3–43` checks active user, classification policies, role bindings, membership and resource grants; it does not check sessions. Existing current-role enforcement therefore does not refute the original-session gap.

## Attempted refutations

| Candidate counter-evidence | Independent outcome |
| --- | --- |
| HTTP authentication already checks logout | It protects authentication time. The reader runs later with the captured actor and no shared original-session lock: server.go278→283. Narrow gap remains when revocation wins before the business read. |
| Service history has authorizeRead | `servicestatus/history.go:12–17` only gets metadata and calls service.read. Observations33–44, ObservationProof75–77, MaintenanceWindows93–95 and SourceRevisions128–130 inherit it and use `s.DB`. No original-session check. |
| Incident reads check interactive capability | `incidents.go:60–69` authorizes through `incidentAccess(...false,false)`; that helper29–58 checks interactive session only when write=true. `incidentCapability:74–87` does check an interactive session, but GetIncident68 and Incidents270 convert the error into `CanUpdate=false`; they still return content. IncidentTimeline274–285 inherits GetIncident then queries the pool. |
| All status reads already use the resource reader | `resource_store.go:95–108` does call `identity.BeginInteractiveRead` and passes its transaction to current service authorization. Its resource callers are protected; ordinary service/history/incident readers above do not call it. This narrows the finding and gives the existing enforcement pattern. |
| Add the check to every shared helper | Unsafe scope expansion: `servicestatus/agent_summary.go:14–40` is called by `agent/service_summary.go:81` with an AI actor and a supplied transaction/allow callback. `servicestatus/notification_access.go:9–14` reuses incidentAccess(false) from `notification/access.go:159` for recipient access, including non-interactive processing. These helpers intentionally support sessionless authority. |

## Repair boundary and unverified areas

Wrap **human** Get/List and the corresponding history/maintenance/source/incident entry points in a short original-session-authorized read transaction; use that transaction for the complete read. `identity/read.go:12–28` and `interactive.go:16–37` already implement organization/user/session locking, current auth versions, active employee, exact channel, expiry and revocation. No global guard belongs in `present`, `incidentAccess(false)`, AgentSummary or NotificationIncident.

Search `RequireInteractive|BeginInteractive|currentActor|readSnapshot|SessionID|Authenticate` over production `internal/servicestatus/*.go` found only write checks, incident capability and the resourceRead boundary. No hidden ordinary-read session enforcement was found. This is a source-confirmed stale-actor read difference; no race fixture, runtime exploit, full authorization audit or real monitoring integration was executed here. Preserve all intentional worker/AI/notification authority paths and keep GAP-23 formal acceptance open.
