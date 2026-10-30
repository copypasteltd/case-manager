# Repository working agreement / 仓库工作约定

## Current scope / 当前范围（用户修订99）

- Follow `docs/delivery/04-current-scope-and-acceptance.md` and `04-current-scope.json`. Keep Web, WeChat, Douyin, Android, Codex secretary, eight Skills and TG. AI execution isolation, authorization, audit and technical resource limits remain required.
- 用户明确移出真机验证、网络性能／实网120GB专项、历史数据迁移与容灾、AI费用控制。不得把移出项重新作为当前发布门槛；不因此启用模拟器。
- Preserve production PG persistence, normal schema upgrades, OSS＋R2 storage/replication, ordinary file/version recovery and company OA/finance. Only AI cost/token quota/billing controls are excluded; increment100 removes mandatory AI fee policies/reservations from admission and execution. Historical cost records are read-only; real Codex integration remains GAP-09.
- 当前23项待完成（14 P0／9 P1），5项移出。旧28项ID和原始证据保留，不把取消范围记成测试通过。范围修改需运行 `python app/scripts/verify_current_scope.py`，并继续执行相应仓库发布检查。

- Implement application code under `app/`. Keep specifications, guides, evidence, release notes and media under `docs/`; the root README is the bilingual repository entry point.
- 应用实现放在 `app/`；需求、开发、使用、验证、更新说明和演示素材放在 `docs/`。根目录 README 作为双语入口。
- Do not start or use local Docker, WSL, virtual machines or emulators. Use native tools. Close every browser context, tab and temporary server owned by a completed check; preserve the user's unrelated processes and tabs.
- 禁止调度本地 Docker、WSL、虚拟机或模拟器。浏览器检查结束即关闭所有自建且不再使用的页面／上下文／临时服务，保留用户原有进程与页面。
- Preserve credentials, databases, `.local/`, `.tools/`, signing keys, dependency installations and build caches outside Git. Demonstrations use isolated synthetic data; never record passwords, MFA secrets or real business records.
- 凭据、数据库、`.local/`、`.tools/`、签名私钥、依赖和缓存不得提交；演示使用隔离合成数据，不录入密码、MFA秘密或真实业务资料。
- The user has authorized publishing this repository to `git@github.com:copypasteltd/case-manager.git`. For subsequent completed updates, maintain the Chinese and English README guides, relevant documentation, `docs/CHANGELOG.md` and affected screenshots/recordings/GIFs; run checks appropriate to the change, commit and push the corresponding code and update notes together. Do not force-push or overwrite unrelated remote work.
- 用户已授权推送到上述仓库。后续完成更新时须同步中英文 README、相关文档、`docs/CHANGELOG.md` 及受影响的截图／录屏／GIF，完成适当验证后连同代码提交并推送；禁止强推或覆盖无关远端工作。
- Keep the full four-client platform scope. Describe source-bound local checks, reused evidence and real-device/cloud/model/production acceptance separately. A development APK, synthetic recording or passing unit test does not establish production readiness.
- 保留完整四端目标，区分本地检查、原证据复用与真实设备／云／模型／生产验收，不以开发包、合成演示或单元测试替代正式验收。

- Increment101 adds selected OA prose proposals and explicit human adoption; keep full contract-body attachments, wider OA tools and real model/Skill acceptance open under GAP-04/12/13. See `docs/development/101-oa-prose-drafting.md`.

- Increment102 adds explicit manual FX book publication and pinned report valuation across four clients. Cross-currency settlement, exchange fees/differences, amortization and full GAP-03 acceptance remain open. See `docs/development/102-finance-fx-valuation.md`.

- Increment103 repairs Taro/Metro build consumers with explicit local compatibility packages. Keep the full remaining dependency register open, bind local package sources into Android evidence and distinguish retained102 media/Go and cached JVM results. See `docs/development/103-dependency-consumer-remediation.md`.

- Increment104 adds controlled known multipart cleanup across four clients, migration82 and a separate worker. Keep formal history, completed draft objects and unknown orphan cleanup open under GAP-05; live cloud acceptance remains GAP-20. See `docs/development/104-multipart-cleanup.md`.

- Increment105 adds a native Linux systemd/cgroup-v2 Codex broker and a dedicated JS/Skill bundle. Provider/proof keys stay in the root broker; Windows checks and cross-builds do not establish Linux/model acceptance. Keep GAP-09/10/13 open, all eight Skills draft and AI fee controls excluded. See `docs/development/105-native-agent-runtime.md`.

- Increment106 adds explicit human cost-amortization plans, immutable history and an independent restatable report across four clients. Source expense/cash facts and completed exports stay fixed. Keep company policy adoption, cross-currency settlement, fees/differences and full GAP-03/18 acceptance open. See `docs/development/106-cost-amortization.md`.

- Increment 107 replaces the remaining high-severity Taro template/dev-server consumers with disclosed maintained adapters. Use Node >=22.15.0 <23. All63 remaining audit entries stay open; retained106 UI/backend/media evidence must be distinguished from fresh client/native/server checks. See `docs/development/107-dependency-toolchain.md`.

- Current applicability for all 30 original AC and 72 TC IDs is recorded in `docs/quality/04-current-acceptance-applicability.md` and its JSON manifest. Run `python app/scripts/verify_acceptance_applicability.py` after acceptance-scope changes. Classification does not execute or pass business cases; GAP-28 stays active.

- Increment108 fixes the remaining seven directly advised npm consumers with a disclosed MIT merge patch, a maintained URI parser bridge and patched Babel/esbuild/XML/send/UUID versions. Full npm audit63→0 does not sign off formal production artifacts; keep GAP-26 active for that remaining acceptance. See `docs/development/108-dependency-integrity.md`.

- Increment109 adds organization audit metadata query and sealed owner-bound CSV exports across Web and shared mobile settings. Keep dedicated audit.read/audit.export, human sessions, recent MFA, metadata allowlist, immutable references and current-authority/hash checks. Model/Skill, TG operations and storage administration remain open under GAP-07. See `docs/development/109-audit-console.md`.

- Increment110 adds controlled in-app notification retries across four clients. Keep explicit organization read/retry authority, human sessions, recent MFA, fixed input, immutable parent/child evidence and current-recipient checks. Dashboard aggregation, reminder configuration, actual storage failures and cross-channel delivery remain open under GAP-08. See `docs/development/110-notification-retry.md`.

- Increment111 adds explicit versioned organization contract reminder policies across four clients. Keep separate read/manage roles, recent MFA, immutable history, publication-day backfill boundary and scheduler rechecks. A published disable overrides deployment fallback without recalling existing events. Dashboard aggregation, real storage failures and cross-channel delivery remain open under GAP-08. See `docs/development/111-contract-reminder-policies.md`.

- Increment112 adds explicitly selected exact file draft recovery into a new run, immutable provenance, current dependency/session checks and fresh upload/scan/commit evidence. Keep wider formats, full-text diffs, complete OA attachments and real model/Skill acceptance open under GAP-04/12/13; all23 packages and five exclusions remain. See `docs/development/112-secretary-file-recovery.md`.

- Increment113 adds explicit exact-version full derived-text comparison across four clients, complete pinned pagination, byte/text equality separation and stale-authority rejection. Keep wider formats, original layout/PDF/OCR/complex DOCX and full business journeys open under GAP-04/18; all23 packages and five exclusions remain. See `docs/development/113-file-version-comparison.md`.

- Increment114 adds a read-only personal business workbench across four clients: exact active approvals, effective-contract expiry and generated subscription reviews, complete authorized counts and bounded samples. Preserve current human sessions, shared approval visibility, active-revision/date semantics and stale-result hiding. Wider aggregation, actual failures/cross-channel receipts and full business acceptance remain GAP-08/18; all23 packages and five exclusions remain. See `docs/development/114-business-workbench.md`.

- Increment115 adds explicit human file/folder-tree scope-transfer plans across four clients, schema88, complete read-policy impact review, recent MFA and immutable decisions. Preserve current source/target and ancestor rights, all content history, reference/hold/in-flight blockers, exact rechecks and organization authorization invalidation. Formal-reference transfers, larger trees, bulk policy and full GAP-18 acceptance remain open; this does not add an AI tool or complete GAP-11. See `docs/development/115-file-scope-transfers.md`.

- Increment116 adds reviewed actual cross-currency cost/claim settlement across four clients, schema89 and dual-unit inverses. Preserve original facts, explicit quotation evidence, human MFA and independent balances. Company policy, fees/exchange differences, other-currency refunds and full GAP-03/18 remain open. See `docs/development/116-cross-currency-settlement.md`.

- Increment117 adds exact contract-basis full-text file drafting and separate human adoption across four clients, schema90 and complete run/event-log views. Preserve original commercial facts, current authorization, recent MFA, exact UTF-8 file hashes and immutable ordinary revision receipts. Wider OA/Office/PDF/OCR attachments and real model/Skill acceptance remain GAP-04/12/18; all eight Skills stay draft. See `docs/development/117-contract-body-drafting.md`.

- Increment118 adds actual payment fee evidence and explicit human revocation across four clients, schema91 and immutable independent history. Preserve original cost/cash/allocation facts, exact dual-currency basis, current human authority and MFA, and one active association per root. Company policy, FX differences, other-currency refunds and full GAP-03/18 remain open; AI fee controls stay excluded. See `docs/development/118-payment-fee-evidence.md`.

- Increment119 adds native production packaging, strict private configuration, controlled application releases/recovery and a manual self-hosted native CI workflow. Functional code/proofs remain118; keep all316 prior media and proof bytes. Windows pointer/process doubles, cross-builds and archive inspection do not establish actual Linux/CI/HTTPS/production PG acceptance. Keep GAP-24/25/26 open. See `docs/development/119-native-production-release.md`.

- Increment120 adds bounded XLSX/PPTX extraction with immutable office-v1 derivations, exact current-head search, per-version comparisons and secretary source reads across four clients, schema92. Preserve formula/cache/hidden-content/layout limitations and all legacy text-v1 evidence. PDF/OCR, complex DOCX, Office editing/layout and full GAP-04/18 remain open. See `docs/development/120-office-text-extraction.md`.

- Increment121 adds actual foreign-currency refund drafts, reviewed unallocated original principal, cash reports and immutable full reversal across four clients, schema93. Reject foreign expense/contract attribution; keep company FX policy and full GAP-03/18 acceptance open. Preserve source-bound evidence, legacy ledger bytes and the AI fee-only exclusion. See `docs/development/121-actual-foreign-refunds.md`.

- Increment122 adds assigned incident and creator-owned current-file storage work across four-client source, plus shared mobile replica retry and exact receipts. Keep full service visibility, active directory chains, pinned head/timeline references and pending work until verification. Explicit scoped publication binds fresh selected checks separately from retained121 full regression/APK; do not relabel retained artifacts or formal acceptance. GAP-08/18, all23 packages, five exclusions and eight draft Skills remain open. See `docs/development/122-operations-workbench.md`.

- Increment123 adds bounded docx-v2 exact-part Word stories, revision/cached-field markers and four-client preview/search/comparison/secretary source reuse, schema94. Preserve legacy terminal text-v1 rows and queued algorithm semantics. PDF/OCR, Office creation/editing/layout and real business-sample/model acceptance remain open under GAP-04/12/18. See `docs/development/123-docx-text-extraction.md`.
