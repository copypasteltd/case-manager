# GAP-06 — ordinary account lifecycle source review

Baseline: `930cdde97b9d5d2e85d892664b0857a10a811739` (increment167). This is an independent, read-only requirement review during the168 code phase. No test, build, browser, database query, service or virtualization was run; no application source was edited.

> “邀请制开户；组织内登录标识唯一；邀请随机单用且过期；员工状态 invited/active/disabled”
> — `docs/requirements/02-identity-and-permissions.md:13`, IAM-01
>
> “Web 使用安全 HttpOnly 会话；移动端使用短时访问令牌+旋转刷新；不同设备可单独撤销”
> — same document:14, IAM-02
>
> “管理员 MFA；密码采用审计认可的现代哈希参数；登录限速；找回单用令牌”
> — same document:15, IAM-03
>
> “权限缓存命中也需要确认会话未撤销和安全版本有效。无法取得授权状态时受限读取/写入 fail closed。”
> — same document:55

**Verdict:** partial · confidence: high for F-G06-01; F-G06-02 requires independent code/spec refutation. Existing ordinary journeys are present, but a reissued account cannot traverse the current disable command because its historical invitation conflicts with schema133. No formal account-policy or platform acceptance is established.

## Where enforcement lives and paths walked

| Journey | Actual source path and enforcement read |
|---|---|
| Invite → inspect → reissue → activate | `httpapi/administration.go:26–88` → `identity/administration.go:101–194`, `invitation_reissues.go:21–118`. Invite/reissue commands use `RequireInteractiveSession(...,true)` before permission/ordinary atomic receipts; latest exact invitation, pending-user/version checks,24h successor, revoked predecessor and token-free permanent receipt are present. Activation excludes consumed/revoked/expired invitations and requires `invited`, locks organization/invitation/user, replaces the placeholder password and increments user/org authorization versions. |
| Password login → MFA enrollment/verification | `httpapi/auth.go:42–100` → `identity/service.go:53–269`. Login has device allowlist, IP/key rate checks, dummy password work and generic invalid-credential response. Current active user/credential version is rechecked before issuance; MFA-required roles or an existing authenticator lead to a short-lived challenge. `crypto.go:22–46` implements Argon2id64MiB/3iterations/parallelism2, random16-byte salt and12-character minimum. Challenge attempts/expiry/current version, TOTP monotonic step, encrypted secret and hashed one-use recovery codes are enforced. |
| Protected Web/mobile request | `httpapi/server.go:193–230` → `identity/service.go:277–304`: ordinary entry checks the current session/user, disallows mixed cookie/bearer credentials and wrong channel; Web writes additionally check configured Origin/CSRF. `auth.go` login response removes Web access/refresh values from response and uses session cookies. This read does not establish deployed HTTPS/cookie configuration. |
| Refresh/logout/device revoke | `identity/service.go:306–399`; `account_security.go:44–141`. Refresh locks the existing credential, checks user/status/version/expiry/channel/CSRF, rotates and revokes its predecessor; predecessor reuse revokes the entire family. Logout revokes its exact session. Device-family revoke checks a current interactive actor and revokes that user's family. Device metadata reads also use a current interactive transaction. |
| Password change/reset/MFA recovery | `account_security.go:154–411`, `recovery.go:12–93`, `step_up.go:21–102`. Change/reset issuance/display require a current human session and applicable MFA; reset consumption rechecks current subject, issuer credential version and live reset authority, consumes/revokes exact reset records, changes the password and revokes sessions/challenges. MFA recovery consumes one stored digest and preserves the authenticator; password reset does not remove MFA. |
| Role grant/revoke/disable | `httpapi/administration.go:89–109` → `identity/roles.go:119–246`, `administration.go:199–262` → `platform/commands.go:24–151`. Organization/user versions, ordinary permission, supported scope, classification ceiling, exact revoke/disable version, last-admin checks and atomic audit/outbox exist. The two differences below remain. |
| Web and shared-mobile entry | `web/src/Login.tsx:20–64`, `PasswordRecovery.tsx:6–10`, `AccountSecurity.tsx:12–47`, `Administration.tsx:13–55`, `PasswordResetDialog.tsx:7–13`; `mobile/src/pages/login/index.tsx:11–53`, `platform/session.ts:104–170`, `pages/settings/index.tsx:15–29`, `settings/security.tsx:14–65`, `settings/members.tsx:16–93`. Both login/activation/MFA/recovery and personal security paths exist; Web has role/disable/admin-reset UI, shared mobile additionally has167 member invitation/reissue UI. |

Paths above are repository-relative below `app/backend/internal/` unless a client prefix is given. They name the actual reviewed enforcement, not executed cases.

## F-G06-01 — revoked predecessor makes later account disable roll back

**Confirmed source contradiction; runtime reproduction not run.**

1. `identity/invitation_reissues.go:56–63` leaves the old invitation as `revoked_at != NULL`, `revoked_by != NULL`, `consumed_at == NULL`, then inserts a successor. This is correct167 historical evidence.
2. `migrations/0133_invitation_reissues.sql:10` requires `consumed_at IS NULL OR revoked_at IS NULL` (`invitations_not_consumed_and_revoked`). Revocation and activation consumption are distinct terminal facts.
3. `identity/administration.go:250` still executes `UPDATE invitations SET consumed_at=coalesce(consumed_at,now()) WHERE org_id=$1 AND user_id=$2`, including that revoked historical predecessor.
4. The predecessor receives a non-null consumed timestamp while retaining its non-null revocation timestamp, violating schema133. `platform/commands.go:102–104` returns the mutation error and the owned transaction rolls back; the user's preceding `disabled` update/session revocation and the whole disable receipt/audit do not commit.

This affects an invited account after one or more reissues and an active account later activated with the successor: old revoked predecessors remain attached in both cases. Neither Web's selected current user version nor the existing current-session guard removes that historical row. It is an ordinary lifecycle failure, not an unavailable company-policy choice. Preserve the predecessor's original revocation and only retire still-outstanding usable invitations when disabling; add the complete reissue→(activate)→disable fixture before the unified check phase.

## F-G06-02 — role mutations retain only the captured actor MFA check

**Concrete source difference; independent spec/code refutation pending.**

`identity/roles.go:139–143` and `204–208` call `recentMFA(c.Actor)` followed by `RequireOrganization(...,"role.assign")`. `recentMFA` (`administration.go:47–52`) examines only the previously captured timestamp. Unlike Invite/reissue/Disable and the account-security writes, these callbacks do not call `RequireInteractiveSession`.

`platform/commands.go:55–74` checks locked organization/user versions and active-user state but has no session lookup. `authz/policy.go:18–31` passes no session identifier to `can_access`; the final `0011_project_viewer_policy.sql:3–45` checks active user, role/scope/classification/channel, not current session revocation. Logout (`identity/service.go:384–399`) changes only `sessions.revoked_at`, so actor user/org versions can remain equal.

An already-authenticated role request (`httpapi/server.go:245–264`) can wait before its command transaction, lose its original session to logout/device-family revoke, then still grant/revoke or replay a role receipt using the captured actor. Current-session expiry can similarly occur between entry authentication and the mutation callback. The review treats this as a narrow remaining current-session requirement difference under IAM-02 and the current-session/cache clause at identity requirements:55, **not** as failure of all logout authentication.

Counter-evidence: ordinary new HTTP requests after logout are rejected by `Authenticate`; actual role/subject/org revocation changes auth versions and is rejected by `Commands.Do`. This finding does not claim that an unauthenticated caller can create arbitrary actors or bypass entry authentication. A second reviewer should decide the queued-command/receipt window against the exact spec before turning it into a168 code task.

## Searched and counter-evidence

- `UPDATE invitations SET consumed`, `revoked_at`, `RequireInteractiveSession`, `recentMFA` across identity administration/reissue/roles/account-security: the disable UPDATE includes all historical invitations; role write callbacks have the remaining actor-only MFA calls. The133 constraint and56–63 reissue write establish the conflicting row, not a grep-only absence.
- `SessionID`, `sessions`, `auth_version`, `AuthorizationMutation` in `platform/commands.go`, `authz/policy.go`, final `can_access`, server command wrapper, logout and role write bodies: version checks exist; original-session recheck is absent on the two role callbacks. Reviewed `identity/interactive.go:16–41` as the current guarded alternative.
- Client `activate`, `mfa`, `recover`, `password/reset`, `logout`, `refresh`, settings membership and security functions were followed to the actual routes above. Mobile's lack of full role/disable/reset-administration editing is **not** reported as missing mandatory work: `docs/development/06-clients-and-pages.md:27` explicitly assigns identity/permission management to full Web and permission-dependent status viewing on the other clients.167 member invitation actions are an added ordinary capability.
- No self-service re-enable, SSO, automatic mail delivery, HR directory or break-glass policy is inferred as mandatory from the original three-state identity requirement. `docs/development/54-account-security-and-recovery.md:103–107` already discloses loss of all administrator/authenticator/recovery routes and requires a separately agreed company/operations recovery mechanism; current user configuration is deferred.

## Open / not proved

Company onboarding, trusted invitation/reset delivery, MFA recovery responsibility and production secrets/HTTPS remain unavailable/formal-acceptance matters. Real platform binding is separately scoped under GAP-19, TG under GAP-15, project-member side effects under GAP-01/11; this account review does not claim to exhaust those call chains or all shared business commands. No test success, real-device/cloud result or23-package closure is claimed.
