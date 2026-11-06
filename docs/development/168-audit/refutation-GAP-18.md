# Independent refutation — GAP-18 ordinary miniapp attachments

Read-only second-context review of `GAP-18.md` F18-01, during increment168 before implementation/tests. Verdict: **retained, narrowly scoped**. No tests, builds, browser, services, devices, virtualization, application edits or further agents were used.

## Attempted refutations

| Counterargument | Independent evidence | Result |
| --- | --- | --- |
| User chose miniapp read/approval, so all miniapp upload was removed | The actual clarification question was about the120GB file and whether miniapps must directly upload that file. `docs/01-requirements-and-scope.md:35` says “上传大文件”; `docs/06-decisions-risks-and-acceptance.md:16` calls it “大文件分工”. Detailed `docs/development/06-clients-and-pages.md:22` excludes120GB, but adjacent23 separately requires “普通附件/拍照票据” on both miniapps. | Does not refute ordinary bounded upload |
| Existing authorized attachment selection fulfills the requirement | `app/mobile/src/pages/business/controls.tsx:55–74` fetches existing files/versions; `execution-controls.tsx:11–12` chooses formal fixed-version evidence. `docs/development/47-mobile-business-drafts-and-submission.md:39` expressly says “普通附件/拍照上传仍须按原首发矩阵补齐；本页选择已有正式版本不能替代小程序本机附件上传能力。” | Does not refute local acquisition gap |
| An unadvertised adapter/upload page already exists | Registered `pages/uploads/index.tsx` imports platform view; `pages/uploads/view.tsx:4` only directs users to Web/Android. `platform/upload-entry.tsx:2` returns null; `platform/uploads.ts:5–13` has unsupported selection/preparation/transfer methods. Search across production mobile source/config for `chooseMessageFile|chooseMedia|chooseImage|uploadFile|chooseFile` returned0. Android `.rn` sources are a separate consumer. | Source path absent, not merely hidden UI |
| Ordinary server upload can already serve a later mini UI | `app/backend/internal/files/uploads.go:171–175` allows only `web`/`android` with empty RunID; both authorization194 and create308 call it. Target authority exists at177–190 but cannot bypass channel rejection. | Backend adaptation also necessary |
| Current exclusions cancelled attachments or all platform validation | `docs/delivery/04-current-scope.json:369–376` keeps platform/file adaptation and release preparation active; only entity-device verification is removed. Development47:39 still explicitly retains ordinary upload. No cancellation of ordinary local attachments was found in the current scope. | Does not refute requirement; device/network benchmarks remain excluded |

## Confirmed difference and repair boundary

F18-01 is one source gap shared by GAP-18 and GAP-19: neither miniapp can acquire and upload a new ordinary local receipt/attachment. Preserve existing file/version authorization, current human sessions, content integrity, scanning and explicit commit. Repair needs bounded platform-supported acquisition/transfer and explicit backend channel/size admission, plus affected lifecycle consumers. Existing fixed-version evidence selection remains useful.

Do **not** expand this into120GB miniapp support, unrestricted filesystem access, a guaranteed20MB provider ceiling, device validation, network performance or formal platform approval. `docs/01-requirements-and-scope.md:52` proposes20MB pending platform limits; user later removed device validation. Exact supported platform API/format/limit and real release configuration must be stated without manufacturing acceptance. A missing generic release-material generator is not part of F18-01.

Inspected source/spec paths and searches are recorded above and independently in `GAP-19.md`. This review does not establish all four-client business flows, backend security branches or current Android artifact completeness; those remain separate open work.
