# GAP-18 — Four-client capability source audit

Read-only inspection during increment168, before the proposed miniapp fix. No tests, builds, browser, services, devices or virtualization were used. Verdict: **partial; one additional mandatory source gap confirmed**. This is not formal acceptance.

## Requirement and confirmed difference

`docs/development/06-clients-and-pages.md:13–27` defines capabilities rather than desktop layout parity. Line23 requires ordinary attachments/photo receipts on both miniapps within platform limits; line22 separately reserves120GB for Web/Android. `docs/01-requirements-and-scope.md:52` proposes20MB without freezing a platform limit. `docs/development/47-mobile-business-drafts-and-submission.md:39` explicitly says selecting existing versions does not replace miniapp local uploads.

**F18-01: WeChat/Douyin cannot upload ordinary attachments/photos.**

1. Registered upload page: `app/mobile/src/app.config.ts:2` → `pages/uploads/index.tsx` → platform view. `pages/uploads/view.tsx:4` redirects all uploads to Web/Android; `platform/upload-entry.tsx:2` returns null; `platform/uploads.ts:5–13` rejects source selection/transmission.
2. `pages/business/editor.tsx:75–78,145,161` only selects existing references; `pages/business/controls.tsx:55–63` queries files/versions. No local acquisition path.
3. `app/backend/internal/files/uploads.go:171–175,194,308` rejects every human upload channel except Web/Android. UI changes alone cannot fix this.

Required repair: bounded ordinary miniapp selection/photo and transfer adapters, explicit server channel/size admission, existing source-integrity/scan/commit authority, and downstream channel-sensitive lifecycle handling. Actual platform configuration/acceptance remains deferred; do not infer a verified20MB ceiling.

## Consumers traced / counter-evidence

Mobile paths below are relative to `app/mobile/src/`.

| Capability | Mounted consumer and actual request/action |
| --- | --- |
| Auth/workbench | `platform/page.ts:20–35`; `pages/home/index.tsx:16` |
| Project/task | `platform/page.ts:49–56`; `pages/project/index.tsx:26–42`; `project/task-forms.tsx:37,101`; `detail/index.tsx:51` |
| OA | `pages/business/index.tsx:24–65`; `editor.tsx:46–61`; `request-actions.tsx:21,65,112`; `detail/index.tsx:56` |
| Finance | `platform/page.ts:41`; `pages/business/index.tsx:53`; `reports/index.tsx:55–77` |
| Files/knowledge | `pages/files/index.tsx:69–98`; `content/index.tsx:32–51`; `knowledge/index.tsx:27,75,129` |
| Secretary/status/notifications | `pages/secretary/index.tsx:151–225`; `secretary/plans.tsx:56`; `status/index.tsx:33–37`; `notifications/index.tsx:40–43` |
| Web/Android | Web route mounts: `app/web/src/App.tsx:158`; native transfer consumer: `pages/uploads/view.rn.tsx:23–78` |

Search: `rg -n 'chooseMessageFile|chooseMedia|chooseImage|uploadFile|chooseFile' app/mobile/src app/mobile/config -g '!*.test.*'` returned0. Documentation search retained the explicit unfinished ordinary-upload requirement. Historic development11:44 describes the implemented restriction, not removal of later47:39.

Mobile complex IAM configuration is intentionally status-only (matrix27); Office editing is platform-limited. Known168 taskboard/timeline and contract-node tasklinks are already being implemented and are not duplicate findings. Other backend authorization branches, full cross-client journeys, current Android artifacts, real configurations and company samples were not independently accepted here.
