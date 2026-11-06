# GAP-20 — OSS/R2 integration preparation

> “配置启用前需受控设置 quota 和初始存储纪元，确认主体区域、CORS和真实云权限。相关配置管理用例仍待完成。”
> — `docs/development/12-upload-worker-and-cloud-identities.md:L51`
>
> “身份/权限/Skill/存储/备份管理 | 完整”
> — Web capability, `docs/development/06-clients-and-pages.md:L27`; settings entry at L60. Backup/disaster recovery remains excluded by current scope.

**Verdict:** partial, high confidence for the narrow missing producer below; actual cloud acceptance is undecidable without the deferred configuration. Source observation: baseline `930cdde97b9d5d2e85d892664b0857a10a811739` plus in-progress increment168. No application edits, tests, builds, services or provider requests were made by this audit.

**What this demands:** explicit organization storage metadata must be provisionable through a controlled implementation before ordinary uploads can work. Real accounts, least-privilege cloud policies, CORS and buckets must still be supplied and accepted separately. This does not require automatic cloud-account creation, historical migration, provider failover, or a 120GB network benchmark.

## F20-01 — Initial storage metadata has consumers but no controlled producer

`storage_epochs` (`0001_identity_platform.sql:L167`), `storage_quotas` (`0016_upload_control.sql:L1`) and `storage_targets` (`0019_file_promotion.sql:L1`) are created empty. Quota SQL explicitly requires operator provisioning; runtime application grants only SELECT on quota/targets (0016:L55, 0019:L63).

The real control-plane path consumes these rows: `files/upload_queue.go:L20-L54` checks current epoch and positive quota; `files/uploads.go:L243-L250` returns `STORAGE_NOT_CONFIGURED` without an epoch, and Create L307-L388 reserves capacity and pins that epoch. `files/upload_commit.go:L110-L125` requires both enabled targets before committing. Merely filling valid process credentials cannot produce these rows.

No executable production metadata provisioner was found. `identity.Bootstrap` (`identity/bootstrap.go:L37-L163`) creates the organization, administrator and roles, without quota/epoch/target records. `cmd/platform/main.go:L55-L219` dispatches bootstrap/migration and workers; its storage paths load configuration, not metadata registration. The release controller validates fixed configuration/unit files (`deploy/release/controller.py:L207-L269`); the runbook delegates business/storage setup to module parameters (`docs/operations/03-native-release-runbook.md:L84`) and supplies no storage-registration command. Web storage operations (`files/storage_operations.go:L105`) read existing epoch/replica metadata and retry known work; they do not initialize these rows.

**Searched:** `storage_targets|storage_epochs|storage_quotas` across non-test Go/Python/SQL/shell in `app/backend`, `app/scripts`, `app/deploy`, excluding fixtures/prepare/check/verify/publication scripts, returned table definitions and SELECT consumers only. A focused repeat across identity, command and deployment roots returned no reference. Case-insensitive `(INSERT INTO|UPDATE|COPY|MERGE INTO) (storage_targets|storage_epochs)` returned no production writer. No generic configuration HTTP route was found that dispatches writes to these tables. Test fixture insertion is not a production provisioner.

**Own refutation:** manual privileged SQL could initialize the database, and the user deferred actual credentials. Those facts explain why no real installation has happened; they do not implement the explicitly pending controlled configuration use case or the Web management capability. A finite reviewed initial-provisioning path can be implemented without real keys. It must not silently change a populated primary/epoch or manufacture cloud acceptance. Independent refutation is requested from another requirement context before treating F20-01 as confirmed work.

## Existing paths walked / limits

Explicit per-purpose identities exist in `storageconfig/config.go:L24-L236`, including signer/inspector, lifecycle, verify, promote, replica, restore and independent readers. `cmd/platform/main.go:L188-L219,L230-L275` wires them into real workers/API. OSS/R2 adapters require explicit HTTPS settings and fixed-source reads (`objectstore/client.go:L23-L43,L142-L148`; `oss.go:L127-L148`; `r2.go:L136-L151`). Upload freeze reconciles all cloud parts, preserves uncertain completion and pins the exact object (`files/upload_jobs.go:L294-L405`). Verification compares source identity/hash (`upload_integrity.go:L190-L202`), scanning precedes promotion (`upload_promotion.go:L75-L113`), and promotion/replication use `objectstore.CopyFrozen` (`copy.go:L72-L175`) with bounded range transfer and whole-target SHA checking. `replica_jobs.go:L231-L402` commits verified status only after exact hash/size evidence and fresh checkpoints. Original/multipart cleanup have separate explicit location/identity loaders (`storageconfig/object_retirement.go:L12-L104`, `multipart_cleanup.go:L12-L64`). No new absent adapter/worker finding was established in these paths.

This audit did not execute those branches, exhaustively reread every lifecycle/cleanup helper or verify cloud policies/CORS/versioning. Unknown objects, retention-policy adoption and wider cleanup identity belong to GAP-05. The independently reported miniapp ordinary-upload channel gap belongs to GAP-18. Real bucket/identity/domain configuration and provider receipts remain GAP-20 acceptance; deferred performance/device/history-DR work is not reintroduced.
