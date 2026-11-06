# GAP-11 — fixed plans, current authority and cancellation

Read-only increment168 source audit, baseline `930cdde`. No tests, builds, services, browser, model calls or application edits.

**Requirement:** “工具动作每次鉴权，撤权/停用立即阻止新调用” (`docs/requirements/07-secretary-and-channels.md:33`); “批准绑定完整 plan_hash 与授权版本” (same file:56); “权限缓存命中也需要确认会话未撤销和安全版本有效” (`docs/requirements/02-identity-and-permissions.md:55`). AI fees remain excluded.

**Inspected paths** (backend paths relative to `app/backend/internal/`):

- `agent/plan_mutations.go:20–147`, `agent/plans.go:256–323`: human approval/commit bind exact hash/version, owner/session/MFA, live source/context, template and atomic ordinary project receipt.
- `agent/execution.go:73–228`, `agent/execution_tools.go:101–113`, `agent/execution_results.go:80`: tool/final-result admission rechecks cancellation, deadline, lease, current audience, runtime/Skill and source/intent basis.
- `agent/fixed_deliveries.go:59–136,200–315`: reviewed fixed ordinary scope-transfer/result-disclosure plans, saved session, ordinary commands and immutable receipts; no unrestricted delegation.
- `agent/telegram_write_grants.go:78–293`: fixed run/input/original session, expiry, current group/topic/binding/audience; original-group trigger activates once. File/OA original-session checks: `agent/file_session.go:15–42`, `agent/oa_requests.go:336–350`.
- Consumers: `app/web/src/ConversationPlans.tsx`, `app/web/src/TelegramWriteGrant.tsx`, `app/mobile/src/pages/secretary/plans.tsx:51–70`, shared-mobile `telegram-write-grant.tsx`: explicit decisions/fixed triggers.

**F-G11-01 — candidate pending independent refutation:** `agent/conversations.go:173–191` human read snapshot checks org/user versions only. `agent/run_cancel.go:18–33` authorizes through `agent/run_reads.go:40–69` → `agent/conversations.go:113–123` → ACL without original-session lookup. `platform/commands.go:54–73` likewise checks versions. Logout (`identity/service.go:384–400`) and family revocation (`identity/account_security.go:114–140`) revoke sessions without changing those versions. An already-authenticated captured Actor can therefore reach read/cancel after revocation. This source schedule was not reproduced. Fresh HTTP rejects revoked tokens (`identity/service.go:277–294`). Repair human snapshot/cancel boundaries; sessionless worker/AI audience helpers remain distinct.

**Searches:** complete bodies above; `SessionID`, `RequireInteractiveSession`, `cancel_requested_at`, `executionSources`, plan/hash/version consumers. Failed guessed filenames were subsequently located; no absence conclusion relies on them.

**Unestablished:** every protocol/UI branch, native runtime, actual provider/TG/configuration/company acceptance. Eight Skills remain draft; formal packages remain open.