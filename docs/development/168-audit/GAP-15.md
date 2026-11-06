# GAP-15 — independent source audit / 独立源码核对

Baseline: `930cdde97b9d5d2e85d892664b0857a10a811739`; 2026-11-06. Read-only application inspection during increment168's code-first wave. No tests, builds, browser, services or provider calls.

Requirement: “指定股东群可通过已绑定身份与秘书沟通，群披露范围受控” (`docs/requirements/01-product-and-journeys.md:20`); J-08's “提及 Bot → 身份/群校验 → 受理 → 查询/执行 → 摘要+链接” (`:43`). Detailed identity, disclosure and uncertainty rules: `docs/requirements/07-secretary-and-channels.md:62`, `:64`, `:66`, `:86`.

**Verdict:** no new confirmed actionable code divergence in the inspected chain. GAP-15 remains open for actual configuration/integration and formal acceptance.

| Path checked | Current enforcement / exact source |
|---|---|
| Webhook → immutable inbox | Secret/type/64KiB checks: `app/backend/internal/httpapi/telegram.go:127`; exact group/topic, explicit trigger, current binding/user, pause on membership/migration, transactional payload-hash dedup: `app/backend/internal/telegram/ingress.go:93`, `:223`. |
| Human identity | Original-session recent MFA, one-use proof, permanent numeric-identity ownership: `app/backend/internal/telegram/bindings.go:97`. |
| Same secretary runtime | Current scope and dispatch: `app/backend/internal/agent/telegram.go:26`, `:78`, `:236`; ordinary runtime authority: `app/backend/internal/agent/execution.go:132`. Intentional sessionless TG actor remains valid. |
| Human tools / handoff | Eight-skill handoff invariants: `app/backend/migrations/0116_telegram_human_handoffs.sql:16`; fixed-project grant, original human/session/MFA and single activation: `app/backend/internal/agent/telegram_write_grants.go:78`, `:168`, `:231`; mutation human recovery: `app/backend/internal/agent/mutations.go:536`. |
| Progress / reviewed content | Generic portal-only phases, phase dedup and pre-send checks: `app/backend/internal/telegram/delivery.go:112`, `:209`, `:228`; exact effective-run organization authority: `app/backend/migrations/0132_telegram_grant_delivery.sql:2`. Separate current-source human-reviewed fragments: `app/backend/internal/agent/telegram_disclosure.go:43`, `:97`; original target: `app/backend/internal/telegram/reviewed_delivery.go:10`. |
| Retry / uncertainty | Ambiguous provider outcomes stay unknown: `app/backend/internal/telegram/delivery_transport.go:55`; expired sending lease becomes unknown: `app/backend/internal/telegram/delivery.go:153`. Owner MFA reconciliation: `app/backend/internal/telegram/delivery_portal.go:72`; organization reviewed reconciliation: `app/backend/internal/telegram/operations.go:176`. |
| Installed native configuration | Complete Python tool catalog and PID1 private credential mapping: `app/deploy/release/catalog.py:2`, `:29`, `:51`; installed copying: `app/scripts/build_production_release.py:66`; bounded private Go consumer: `app/backend/internal/telegram/config_file.go:14`; actual dispatch/delivery entry points: `app/backend/cmd/platform/telegram_worker.go:19`, `telegram_delivery_worker.go:19`. |

Searches: `rg --files app`; targeted `rg -n` for `RequireInteractiveSession`, `telegram_effective_run_links`, `telegram_delivery_authorized`, `delivery_unknown`, `enqueueRun`, `LoadCredential`, `RELEASE_TOOLS`, `CP_TELEGRAM`, and `telegramHandoff`.

Declared bounds are preserved: two automatic read skills, fixed human-authorized project writes, remaining tasks through reviewed portal handoff; no arbitrary group-wide disclosure. Web/shared-mobile entry consumers were located, not exhaustively reviewed. Live Bot/group/provider/Linux installation, all UI branches and formal journeys remain unread/unverified here; credentials were not inspected. Existing source/evidence/freezes are not acceptance claims.
