# GAP-10 — independent source/spec audit, increment168

Source-only review against increment167; no tests, builds, browser, services, virtualization or application edits. **No new actionable code difference confirmed in the reviewed runtime path. Target Linux acceptance remains undecidable.**

## Requirement and verdict

`docs/development/105-native-agent-runtime.md:11` preserves identity, authorization, isolation, audit and technical limits; only AI fees are excluded. Its lines66–71 require isolated identity/root, readonly release, restricted execution/network and bounded resources. Line66: “每任务 DynamicUser；专用 RootDirectory；能力集为空、NoNewPrivileges”. `docs/development/119-native-production-release.md:49` additionally requires exact candidate runtime/broker bindings.

**Reviewed code: implemented; confidence high for the traced paths.** This is not Linux/model/Skill acceptance.

| Property | Producer → actual consumer / rejection |
|---|---|
| Exact runtime, profile, identity and readonly mounts | `app/backend/internal/nativeruntime/config.go:96,191` validates pins, paths, profile/tools and deadlines; `plan.go:23` produces fixed unit/properties; `linux_host.go:233` executes fixed systemd-run arguments; `linux_server.go:37` verifies actual root-owned pins/release digest. `app/deploy/release/controller.py:354` binds candidate metadata and invokes native preflight. |
| CPU, memory/swap, tasks, tmpfs, files, time, network | `plan.go:35` generates DynamicUser, namespaces, capabilities, syscall/ExecPaths restrictions, cgroups, bounded noexec tmpfs and NOFILE. `linux_host.go:433` reads PID1/proc/cgroup/mount evidence; `observation.go:35` rejects mismatches before signed proof. |
| Private keys and controlled upstream | `linux_server.go:113,121` loads private root-owned keys; `linux_client.go:83` gives children an allowlisted environment and private relays. `gateway.go:20` rechecks peer/current run and restricts provider, payload, streams and response sizes. No provider/proof key is supplied to the child. |
| Current authorization, proof, teardown | `broker.go:64,148` tracks deadlines, observes before signing, stops whole cgroups and blocks launches after uncertain teardown. `agentworker/proof.go:52` verifies fresh exact proof; `agentworker/runner.go:19` renews authority and cancels on failure. `agent/execution.go:73` rechecks original execution dependencies; `agent/file_session.go:15` rechecks original file-intent session. `codex-adapter.ts:150` requires confinement before client creation. |

Paths walked: config → unit producer → actual systemd invocation/observation → signed proof → adapter → gateway → current-authority heartbeat → cgroup stop/recovery. Also read CLI dispatch and broker service, not templates alone.

Searches: `ValidateRun|Plan|Observe|Stop|ServeHTTP|Child`, `ReleaseDigest|SigningKey|ModelKey|WorkerUID`, `RequireInteractiveSession|executionTx`; these located the consumers above. No absent implementation is inferred from missing credentials.

Open/unread: actual target systemd/kernel enforcement, escape attempts, installation, cancellation under real model load and eight Skill evaluations (`105:113` onward); release builder internals and every non-file delegated business handler were not exhaustively audited here. Existing constraints and intentional unsupported protocols are not new defects. GAP-10 remains formally open.
