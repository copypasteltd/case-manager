# GAP-19 — Miniapp platform source audit

Independent read-only inspection during increment168. No tests, builds, browser, services, devices, virtualization or application edits were performed. Verdict: **partial**. Platform identity and ordinary read/approval consumers exist; bounded ordinary local attachment upload is absent and duplicates F18-01. Actual configuration and platform admission remain open. This is source inspection, not platform or production acceptance.

## Requirement and scope

`docs/delivery/04-current-scope.json:369–376` keeps GAP-19 active: “补真实AppID、密钥、域名及主体资料；完成平台接口／文件能力适配、源码与构建核对、提审材料和发布配置。” Its acceptance asks for inspectable configuration, fixed adapters, error handling and build artifacts, with actual review status recorded. Line376 explicitly removes entity-device verification and prohibits replacing it with local emulators.

`docs/development/06-clients-and-pages.md:13–27` requires miniapp login, project/file reading, OA submission/approval, secretary and notifications; line22 reserves120GB uploads for Web/Android, while line23 separately requires ordinary attachments/photo receipts within platform limits. `docs/01-requirements-and-scope.md:52` makes20MB a proposed test target, not an established platform ceiling. User exclusions do not remove ordinary attachments.

## Trace and verdicts

| Area | Production enforcement / consumer | Judgment |
| --- | --- | --- |
| Private platform profiles | `app/backend/internal/platform/config.go:49–58`; `internal/miniapp/client.go:30–56`; `internal/identity/miniapp.go:61–78` loads server-only AppID/Secret/key, rejects partial/invalid profiles and publishes only configured platform/AppID | Implemented source; real inputs deferred |
| Client platform identity | `app/mobile/src/platform/miniapp-login.ts:8–23`; `miniapp-context.ts:5–16` checks real runtime, reads runtime AppID, matches published profile and obtains a bounded fresh Taro.login code | Implemented; H5/Android cannot manufacture platform proof |
| Fixed server exchange | `app/backend/internal/miniapp/client.go:59–116` selects the separate official WeChat GET/Douyin POST HTTPS endpoints, ten-second timeout, no redirects,32KiB response bound and required provider identity fields | Implemented source; live protocol acceptance unverified |
| Durable single-use code claim | `app/backend/internal/identity/miniapp.go:85–107` inserts purpose/platform/AppID-bound HMAC code claim before external exchange and returns a subject HMAC | Implemented; network failure does not release a used code |
| Explicit same-user binding | `identity/miniapp.go:110–184,193–239`; `app/mobile/src/pages/settings/miniapp-bindings.tsx:23–49` requires current password, original interactive session and applicable recent MFA, rechecks after external exchange, issues a two-minute proof and consumes an exact same-user/session/auth-version proof on explicit confirmation | Implemented; no silent registration or identity merge |
| Login/MFA/revocation | `identity/miniapp.go:275–305,309–397`; `identity/service.go:186–258,285–286,306–375`; `identity/recovery.go:73`; `app/mobile/src/pages/login/index.tsx:25–33,45–52` retains current binding, active employee, MFA enrollment/verification, session linkage and refresh/revocation checks | Implemented paths inspected; not exhaustive security acceptance |
| Actual read/approval consumers | `app/mobile/src/pages/files/index.tsx:30–33,64–98`; `pages/content/index.tsx:7–15`; `platform/content.ts:97–107`; `pages/detail/index.tsx:56`; `pages/detail/mutations.tsx:27–32` connects current files/fixed versions, bounded authenticated content and exact-version ordinary approval commands with applicable MFA | Implemented source; platform execution remains unverified |
| Bounded ordinary local upload | `app/mobile/src/platform/upload-entry.tsx:2` returns null; `pages/uploads/view.tsx:4` redirects to Web/Android; `platform/uploads.ts:5–13` rejects source/transfer; `app/backend/internal/files/uploads.go:171–175,194,308` rejects WeChat/Douyin actors | **Absent; confirmed F18-01, not a second finding** |
| Release configuration/materials | `app/mobile/config/index.ts:4–12,17,22` requires explicit HTTPS origin outside local profile; `app/mobile/package.json:6–7` has separate platform builds; `docs/development/31-mobile-client-foundation.md:15` and `97-mini-program-identity.md:53–59,67` retain actualAppID, domains, privacy/subject and review work | Open / dependent on real inputs; do not call absence of supplied credentials absence of identity code |

## Searches and counter-evidence

Filename inspection covered `app/mobile/config`, miniapp platform/page files, backend identity/miniapp/httpapi adapters and release/scripts. `rg -n 'chooseMessageFile|chooseMedia|chooseImage|uploadFile|chooseFile' app/mobile/src app/mobile/config -g '!*.test.*'` returned0. `pages/business/controls.tsx:55–74` and `execution-controls.tsx:11–12` select existing formal versions; this supports read/approval but cannot acquire a new local receipt.

Non-test source searches for `project.config.json|project.tt.json|CP_MOBILE_WECHAT|CP_MOBILE_DOUYIN|miniapp.*material|miniapp.*profile`, and for domain/review-material producers, found no repository-owned miniapp project/material generator. Separate builds/local origin settings and historical proof scripts are present. No specification inspected mandates automation of platform review itself, so lack of such a generator is **not promoted to a separate mandatory application defect**. Concrete realAppID, subject, privacy declaration, valid domains and platform review remain disclosed input/preparation work.

The user's “Web/Android 上传大文件，小程序查阅和审批” answer followed the120GB-specific question. `docs/06-decisions-risks-and-acceptance.md:16` labels this “大文件分工”. It does not cancel matrix23 or the explicit remaining ordinary-upload sentence in development47:39. The independent GAP-18 refutation records the narrower repair.

Unread/unverified: actual provider credentials or dashboards, platform policy changes, built package runtime, actual device/cloud/Linux/model acceptance, full business/security regression and company review materials. No external protocol fact was newly asserted as current verified API behavior; the endpoints above describe checked source. All23 packages remain open.
