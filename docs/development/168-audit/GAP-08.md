# GAP-08 — 工作台、站内通知与个人 TG 提醒

基线：`930cdde97b9d5d2e85d892664b0857a10a811739`；168代码阶段只读审查。没有运行测试、构建、浏览器、服务、虚拟化或外部投递，没有修改应用代码。本记录不关闭GAP-08及任何正式验收。

> “工作台包含‘待我审批、我的任务、即将到期、复制异常、服务事件、秘书待处理’；每个数字按当前人范围计算。”
> — `docs/requirements/01-product-and-journeys.md:30`
>
> “已读不等于业务已处理，已送达不等于人已阅读。”
> — `docs/requirements/08-status-notifications-and-nfr.md:23`
>
> “权限缓存命中也需要确认会话未撤销和安全版本有效。无法取得授权状态时受限读取/写入 fail closed。”
> — `docs/requirements/02-identity-and-permissions.md:55`

**Verdict:** partial · confidence: high for the narrow source divergence below; independent refutation remains required. Ordinary workbench/notice/retry/personal-TG paths are implemented on the traced scope. Full company/provider acceptance is undecidable from source.

**What this demands:** Current-person counts and actionable records must come from current domain facts and ordinary access. Marking an observed delivery read must not resolve the domain duty. Queue acceptance cannot establish storage recovery or external delivery. Human reads/writes must not continue solely from an Actor captured before its original session was revoked. TG requires explicit current personal consent/binding and current business visibility, with unknown outcomes kept separate.

---

## F-G08-01 — 原会话已撤销后，已鉴权排队的通知读取／已读仍可通过

**Where enforcement lives:**

- `app/backend/internal/httpapi/server.go:193`–229 obtains Actor through `Identity.Authenticate`; `read:276`–289 and `command:245`–273 pass that captured Actor to the domain later. Cookie Origin/CSRF and initial authentication exist. This is not a claim that a new request with an already logged-out token succeeds.
- `app/backend/internal/notification/access.go:19`–34 rejects agent runs/nonhuman channels and checks organization authorization version, user auth version and active status, holding organization/user shared locks. It never reads `sessions`, the original `SessionID`, revocation or expiry, and returns nil after those checks.
- `app/backend/internal/notification/service.go:31`–40 uses that snapshot for ordinary List/Summary/Get/Failures transactions; `MarkRead:203`–218 uses the same snapshot as the Commands authorization callback, then `:220`–244 updates the read sequence.
- `app/backend/internal/platform/commands.go:46`–68 independently checks organization/user versions before the authorization callback. It does not check session liveness; the same callback runs before idempotent replay.
- `app/backend/internal/identity/service.go:384`–399 Logout changes `sessions.revoked_at`, audits and commits, without changing organization/user authorization versions. Family revocation similarly updates sessions at `:338`.

**Concrete ordering:** Authenticate and capture a human Actor; before notification.readTx or MarkRead acquires its locks, finish Logout (or revoke/rotate that session family). Reuse the captured Actor in the queued ordinary notification operation. Organization/user versions and resource visibility can remain unchanged, so snapshot and Commands authorize; a readable notification can be returned, and a new MarkRead can update its head. No session query in those paths rejects the now revoked original session. This is a source-derived ordering, not a newly executed reproduction.

**Counter-evidence retained:** New HTTP requests are correctly rejected by `Identity.Authenticate` (`identity/service.go:284`–289); actual organization/user changes are rejected by snapshot/Commands. Workbench reads already call `RequireInteractiveSession` before/after their snapshot (`workbench/service.go:152`, `:178`); notification retry authority does so at `notification/retries.go:66`–75, and personal-TG settings/reconciliation use current interactive sessions (`telegram_portal.go:100`, `:327`). Those checks do not execute for ordinary notification snapshot.

**Boundary for a repair:** `identity/interactive.go:16`–37 is the existing session/expiry/user/version check with an original-session row lock. Apply current human checks at the public read/write boundary, preserving background recipient evaluation: `notification/jobs.go:206`–219 constructs sessionless recipient Actors and calls `view`, so imposing an interactive session directly on the shared worker `view` would break legitimate background notices.

**Searched:**

- `snapshot(` in `app/backend/internal/notification` production Go finds its definition plus only service.readTx and MarkRead authorization calls. No worker calls that human snapshot.
- `RequireInteractiveSession|SessionID|revoked_at` in notification access/service finds no original-session enforcement; the retry and personal-TG modules contain the separate checks listed above.
- `Authenticate`, `read`, `command` in HTTP server traced initial authentication before domain invocation; no enclosing DB transaction/session lock spans the two.
- `Logout|UPDATE sessions` in identity service and Commands.Do authorization/replay branches read directly; no hidden auth-version increment covers this ordering.

**How reached:** The requirement explicitly includes session revocation in current authorization; current ordinary-notification enforcement covers account/role versions but omits that axis after authentication. This is narrower than saying logout or all notification authorization is broken. Independent code/spec refutation must decide inclusion before implementation.

---

## Other paths walked and retained boundaries

- **Workbench:** HTTP `workbench.go:11`–37 → `workbench/service.go:128`–185 uses a repeatable-read snapshot, current human session, complete authorized counts plus bounded five-item samples from `queries:51`–125. Active contract revision/date and subscription review state are read from domain facts; assigned incidents, current-version file replicas and current signed resource sources are independent of notification read markers. `task_inventory.go:4`–18 adds ordinary assigned/overdue/participating/unassigned queues without granting access from participation. `followups.go:49`–58 separately covers personal request revision and independent deliverable submission/review; these are not approvals inferred from UI counts.
- **Web/shared mobile:** `BusinessWorkbench.tsx:12`–24 keys current user/authorization and hides cached business summary on error or refresh; Web `App.tsx:168`–171 mounts business/followup/task sections. Shared `home/index.tsx:16`–30 reads the same APIs and offers notifications with unread/attention counts. `workbench/inventory.go:37` supplies ordinary drill-through with a current-session/result-basis-bound cursor; more than5000 visible items returns explicit WORK_INVENTORY_LIMIT rather than an invented complete list (`:112`–141). The notification UI uses fixed resource routing, exact delivery sequence for mark-read (`Notifications.tsx:20`–24), and explicit unread versus attention filters. A separate secretary-only dashboard aggregate is not assumed mandatory here: original secretary results feed the ordinary notification attention path (`jobs.go:183`–188; `access.go:167`–178), and the detailed NTF-04 scope explicitly uses the notification entry/count. No new generic dashboard is invented.
- **Notification delivery:** `jobs.go:193`–231 re-evaluates current recipient and domain visibility for each allowed ordinary client. `:233`–270 serializes the theme, suppresses old/duplicate events and appends exact immutable event/recipient/channel/template/sequence receipts. `:274`–341 checks the worker lease/fence and records/completes notification work in its own transaction. Schema52 immutable delivery/result triggers (`0052_in_app_notifications.sql:35`, `:47`) preserve prior evidence. Delivery failure never calls back to roll back the already committed source operation.
- **Read does not resolve business:** `service.go:220`–244 only changes notification read sequence and emits its own audit/outbox event. `access.go:48`–178 derives actionable state from current tasks/approvals/contracts/periods/incidents/resources/runs. `file_replicas.go:25`–60 pins replica/version/hash/time and keeps queued/copying/verifying/retry_wait actionable; only exact verified proof removes duty, without deleting the original failure receipt.
- **Human retry:** `retries.go:66`–75 requires separately granted organization read/retry and recent original-session MFA. `:173`–210 locks/rechecks exact current revision, accepts only retryable dead fixed-input jobs and creates an immutable child task/receipt rather than overwriting history. The actual worker again uses current recipients.
- **Personal TG:** `telegram_portal.go:98`, `:146`, `:319` provide current-human settings, explicit immutable consent and recent-MFA reconciliation. `telegram_worker.go:23`–39 checks current structural consent/binding then ordinary domain eligibility; `:74`–84 selects only latest unread deliveries after the current policy within24h; `:132`–173 never automatically resends an expired sending lease and suppresses stale eligibility. The outbound text is a fixed generic prompt (`:174`); prepare records the exact attempt before I/O, and only provider-confirmed message IDs establish sent. `telegram_portal.go:335`–366 separately handles human confirm/retry/abandon without changing source read/business state. Schema113 checks current user/binding/consent/24h and append-only policy/event evidence (`0113_notification_telegram.sql:19`–33, `:64`–78).

**Open / not proven:** Real TG/Bot/group/channel configuration, independent external delivery, real cloud replica failures/recovery, company routing/reminder policy adoption and full formal four-client business acceptance remain open. This scoped source review does not exhaustively re-audit every source-domain permission helper, every scheduler clock race, transport implementation, resource monitoring rule, or all existing fixture assertions. No test, runtime, actual external acceptance or zero-bug claim is made.

## Reviewed source hashes

- `app/backend/internal/notification/access.go` — `c2767cb4a11124638a4769116d7763c916982b5e62cccd64a0fbf5e26f646003`
- `app/backend/internal/notification/service.go` — `22e80bdd2a4ba47c9d2ad621b0ef2eb4823932bc959589e1c7c5506f30d3d716`
- `app/backend/internal/notification/jobs.go` — `33f1a33c9d0d4965e98efbc8f70646109542e829f261645ccefa9be941c7da80`
- `app/backend/internal/notification/retries.go` — `575b4775cb0616d735275217ec0bab6b90e81b83e4164ccc237bb844183917e9`
- `app/backend/internal/notification/telegram_worker.go` — `3b3704b245fe5e65c92aa5cf73b3ee80afb44083526c79f6b3d6661b79ae162e`
- `app/backend/internal/notification/telegram_portal.go` — `c3f4fdefaa47352ebb155570c80d26b90805780b29850a2af6651b6904921897`
- `app/backend/internal/workbench/service.go` — `53643f1e48502ede8372c6f4f1c028a3552b67ebc9ba71a44d7c24372a0f0a3d`
- `app/backend/internal/identity/service.go` — `4b4f6465ddb07bbf322de5e0ce3d29890b47fa9d63ad750d9a7748a6e4885b68`
- `app/backend/internal/platform/commands.go` — `98a998e52606346aea4908693e9780c3b02d23f01587f70148319a61675eb424`
- `app/backend/internal/httpapi/server.go` — `7acbe10c50f743d940f8237a265ea6db1c9ca69bc8b6196dd665d3fa659a3b52`
