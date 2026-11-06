# Independent refutation — GAP-20 initial storage metadata producer

Read-only second-context review of `GAP-20.md` F20-01, during increment168 before implementation/tests. Verdict: **retained with a narrower delivery boundary**. No tests, builds, browser, services, devices, virtualization, provider requests or application edits were performed.

## Requirement and concrete difference

`docs/development/12-upload-worker-and-cloud-identities.md:51` states: “配置启用前需受控设置 quota 和初始存储纪元，确认主体区域、CORS和真实云权限。相关配置管理用例仍待完成。”

The requirement is a controlled initial provisioning use case. `docs/development/06-clients-and-pages.md:27` gives Web complete storage management, but does **not** independently require every deployment-only initialization operation to be a mutable Web form. A reviewed native deployment CLI can satisfy this initial-producer gap. Do not expand F20-01 into a new general mutable storage administration suite.

The tables are created without an organization configuration: `app/backend/migrations/0001_identity_platform.sql:167` defines storage_epochs; `0016_upload_control.sql:1–6` defines positive quota and expressly requires operator provisioning; `0019_file_promotion.sql:1–5` defines provider/bucket targets. Application grants are SELECT-only for quota (`0016:55`) and targets (`0019:63`).

`app/backend/internal/identity/bootstrap.go:32–163` initializes organization/user/classification/roles/audit, but contains no reference to the three storage tables. `app/backend/cmd/platform/main.go:143–184` wires bootstrap through the migration identity; supported dispatch at224 has no initial-storage provisioning operation. Merely constructing real store adapters in `internal/storageconfig/config.go:24–236` cannot insert missing metadata because these loaders do not accept or write a database.

## Attempted refutations

| Counterargument | Independent outcome |
| --- | --- |
| User deferred real credentials, so this is only unavailable input | Credentials are external inputs, but supplying them only configures adapters. `files/upload_queue.go:35–50` still needs latest epoch and positive quota; `files/uploads.go:243–250` rejects absent epoch; `files/upload_commit.go:110–124` rejects absent enabled primary/secondary targets. Inputs do not implement the explicitly pending DB provisioning path. |
| Migrations or bootstrap already seed suitable defaults | Three table definitions have no initial organization rows, and quota explicitly forbids invented production capacity. Bootstrap has no storage table references. No hidden seeding path found. |
| A production script or generic settings handler already writes metadata | Case-insensitive `(INSERT INTO|UPDATE|DELETE FROM|COPY).*(storage_quotas|storage_epochs|storage_targets)` over backend/deploy production Go/Python/SQL returned0. A table-name search over non-test Go returned only11 files under `internal/files`, all consumers. Release/deployment roots contain no producer. Script matches were `prepare_storage_browser_fixture.py:37` and `prepare_upload_browser_fixture.py:47` synthetic epoch INSERTs; not production initialization. |
| Manual privileged SQL could do it | Technically true, and the schema intentionally reserves this authority for deployment. No checked-in supported SQL/CLI use case was found; the explicit pending controlled setup remains unimplemented. A finite reviewed SQL/CLI producer would satisfy the difference without runtime DB grants. |
| Web full management demands cloud-secret management/primary switch | Not established by the inspected requirement. Initial metadata can be deployed privately with no client secret exposure. Historical migration, disaster recovery and automated provider switching remain excluded or separately scoped. |

## Narrow repair boundary and remaining uncertainty

Provide a controlled, organization-bound deployment operation that takes explicit positive capacity, initial primary/epoch and exact OSS/R2 targets, validates inputs and writes the initial set atomically with review/audit evidence. Repeated exact input may safely reconcile; a mismatched populated primary/epoch/target must not be silently overwritten. Keep privileged deployment identity separate from the runtime application's read-only metadata grants. This is initial provisioning, not a new failover or historical migration system.

Searches also covered `app/scripts` and deployment sources for all three table names, production command/bootstrap paths and checked-in Markdown/SQL for insertion instructions. No supported writer was found beyond synthetic fixtures. This is source evidence, not proof that any real deployment or operator has attempted configuration. Real accounts/regions, least-privilege permissions, CORS, object identities, provider receipts and formal cloud acceptance remain deferred; all23 work packages remain open.
