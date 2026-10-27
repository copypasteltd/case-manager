# 增量89验证 / Increment89 verification

移动费用、资金、退款、准确员工结算与全额冲销，对应FIN-01—07／11—12及GAP-02／03／18。[设计与操作](../../../development/89-mobile-financial-operations.md) · [实际素材](../../../media/README.md)。完整四端生产目标保持开放。

Mobile expense and cash workflows connect the existing authorized APIs. This record identifies newly executed checks, exact-byte reuse and remaining acceptance; it does not establish production or real-device readiness.

## 1. 当前来源与复用 / Current sources and reuse

| Layer | Actual scope | Source SHA-256 |
| --- | --- | --- |
| Taro | 204 tests, types and both mini-program builds;18 pages each | `3b670e1d651789758b932dfa251b4959be950ef506103e1878f97e8e894b2859` |
| Android | 42,921,174-byte development APK;18 package inspections | `bcc31cffc1e05d77c883840945a04e03cf6f90f8ef8c710b750c850bcfd0fae1` |
| Go/PostgreSQL | Original increment88:161 top-level,1057 results,960 leaves,9 batches; all73 migrations unchanged | `55f825143e0f3aa685fa00b0fa8383c3633652a9983f91739657b629be860fac` |
| Web | Original increment88:112 tests and build | `0fb40361ab563afc54a5c2cfbec54699c6f6ca553e4576a654b88646e70921c3` |
| Runtime/Skills | Original increment88:47 tests and build | `3b966f4741d4da3fd35234f6e5843373055398bb76374e5f5d6f8b6f66a38c96` |

本轮17个新增财务用例加上原187项，共204项；新增成本权限正向断言及拒绝虚构权限的断言包括在内。后端／Web／运行时／Linux的原证据继续位于`../finance-reports/`，只在完整声明来源摘要仍相等时复用，不复制成“本轮新执行”。OpenAPI仍为284路径／352操作／619模式，无迁移、依赖或锁文件变更。

Seventeen additional financial tests extend the original187 to204. Backend, Web, runtime and Linux evidence remains at its original increment88 paths; the source verifier compares complete declared current bytes before reuse. There is no new backend execution, migration, dependency update or OpenAPI change in this increment.

APK SHA-256：`7e2bc06a0ba7e24e0dd57bec782520ef0091d6eee3dda32e6098f9bb047ed06a`。26项JVM结果本轮为**Gradle缓存复用 / cached**；任务标记为`> Task :app:testDebugUnitTest UP-TO-DATE`。四份XML和原生日志保留原字节，不称缓存为新运行。APK留在忽略目录，未上设备、未生产签名或分发，也未使用模拟器或虚拟化。

## 2. 实际页面业务 / Actual page journeys

本轮实际浏览器共9组检查、3种宽度（390／430／768）。使用真实原生Go API及隔离PostgreSQL；独立审批者与业务操作者使用各自身份。录屏只覆盖登录完成后的草稿表单，不录密码、MFA或其他秘密。

1. 实际列表入口起草费用100HKD，分摊项目60／公司40，固定供应方及一份正式文件版本，保存仍为草稿。
2. 真正的并发修订触发409并保留文字；另一次PATCH实际已提交但响应丢失，页面保持未知状态且只有一次写入，刷新后核对真实版本。
3. 本人新鲜MFA确认费用，再关联独立审批通过的准确采购修订，不新建第二项费用。
4. 真实资金100HKD：香港2月1日02:00准确为UTC1月31日18:00；登记后分配60、释放20，净结算40。
5. 独立收到退款10：登记时映射5，登记后补映射5；原付款退款10、原费用净结算30。
6. 独立批准员工垫付报销100，移动起草并确认关联费用，以原申请员工、批准修订、准确行和供应方结算60，报销余额40。
7. 资金按香港2月10日10:00全额冲销，实际反向记录为UTC02:00；报销结算恢复0／100，保留原历史。
8. 释放第一笔费用余下30后按2月5日全额冲销，真实反向记录保留原60／40分摊、币种、范围和固定凭证。
9. 展开固定修订、核对历史并检查三种宽度无页面横向溢出。

These are actual UI commands and independently checked API results. Browser setup uses synthetic parties, formal files and approval workflows, without changing component/business state to manufacture results. The committed-response interruption is deliberate transport fault injection. Exact synthetic record IDs and before/after employee settlement are retained in [business-proof.json](../mobile-finance-operations-mobile/business-proof.json).

捕获完整启动来源：`9d0b3035c0f5143d806e97601a9b3f805bda0bfde3aba6dd190aaac0b63b6255`；当前客户端及所有实际调用帮助脚本：`79c8578c8ca06a1265e0be1252bdc6ddebf19a22418e1f8ef1276a831ef1ab85`。前后摘要相等。自建浏览器上下文、临时API／隔离数据库、5189服务均已释放，私有fixture已移除。

## 3. 实际媒体 / Actual media

| Recording | Actual sampled frames | Capture duration | GIF speed |
| --- | --- | --- | --- |
| mobile-finance-cost-draft | 27 | 5514 ms | 1.000× |
| mobile-finance-payment-draft | 35 | 7122 ms | 1.000× |

新增4PNG、2MP4、2GIF，完整目录58／21／21。MP4采用原始帧时间；GIF按公开倍率播放，补帧后的编码帧数不冒称原始采样数。四张截图及两份动画首／中／末帧均经实际像素检查，摘要绑定在[visual-review.json](visual-review.json)和[media.json](media.json)。原始JPEG、账号、数据库和APK留在忽略目录。

Four actual post-login screenshots and two recording/GIF pairs use synthetic data. Taro H5 pixels establish shared-page behavior only; they do not establish WeChat, Douyin or Android device acceptance.

## 4. 保留尝试与历史 / Retained attempts and history

完整冻结的失败尝试共9份，不将其中部分成功断言改标为整轮成功：

- `browser-02`：The checker assumed user-list entries expose login names; actual actor identities must come from each authenticated /me response. The unchanged application and the complete failed capture source are retained.
- `browser-03`：The existing complete business fixture already contains some of the required roles; seed only missing bindings instead of attempting duplicate grants. No application permissions are relaxed.
- `browser-04`：Actual role grants expose cost.record; mobile list and draft guard incorrectly checked cost.create, so no cost creation button was shown.
- `browser-05`：After three real business checks passed, direct hash navigation reused an earlier Taro list page with stale query parameters; the verification must use visible workspace navigation rather than query-only hash changes.
- `browser-06`：The browser checker treated a null page.goBack response as missing history, although SPA fragment navigation legitimately has no network response.
- `browser-07`：Repeated generic browser back navigation traversed beyond the SPA document; the checker now opens the unparameterized workspace route and uses visible list/record links.
- `browser-08`：Visible workspace navigation reached the payment form correctly; the synthetic payment draft omitted its required actual payment method, so the real backend correctly refused registration.
- `browser-09`：Four actual financial checks passed; the refund draft also needed its explicit real payment-method label before registration. The server correctly rejected the incomplete synthetic refund.
- `startup-01`：The runner still required the report-only fixture marker after selecting the existing complete business fixture; native test exited0 and all owned resources were released before correction.

`browser-03`另保留被严格冲销回执守卫替代的202项构建；`browser-04/superseded-builds`保留修正`cost.record`前的204项构建、旧APK检查、26项缓存XML及任务日志。对应源码在该尝试完整ZIP中，最终通过仅绑定当前来源。

Published increment88 commit `e87656f822b44795305dd3d8a4ca4f2042cdb8a5` reconstructs complete startup and both client capture manifests. All80 historical evidence artifacts retain exact original bytes. Earlier cache-provenance limitations remain in their original records. The initial incorrectly named mobile CLI invocation produced usage output only and did not run or publish a validation result.

## 5. 开发环境与未完成验收 / Development and remaining acceptance

只读探针确认73迁移、归属API／通知worker／DB-only报表worker、零隔离测试库、四个报表不可变约束、8项draft Skill及外部云／模型／TG关闭。本轮无需重启未变的API或PostgreSQL。数据库和对象存储原件的生产恢复方案仍需独立验收。

继续推进完整四端旅程、微信／抖音实际平台与Android实体设备、原生下载／上传、120GB／2TB、双云备份恢复、真实Codex／TG与跨境链路。依赖锁文件未改，原审计114项（critical11／high32）的放行门仍开放；不新增汇率／摊销／跨币制度。全部28项GAP和完整生产Goal保持开放。

## 6. 复核 / Reproduction

先归档已有捕获和构建，不覆盖历史结果；按本机原生环境与私有配置运行：

```powershell
python app/scripts/verify_mobile.py mobile-finance-operations
$env:CP_MOBILE_BUILD_PROFILE='local'
$env:CP_MOBILE_API_ORIGIN='http://127.0.0.1:18080'
python app/scripts/build_android.py --google-mirror --evidence-group mobile-finance-operations
python app/scripts/inspect_android_apk.py --evidence-group mobile-finance-operations
python app/scripts/freeze_mobile_finance_evidence.py
python app/scripts/run_mobile_finance_browser.py --mobile
python app/scripts/encode_mobile_finance_media.py
python app/scripts/verify_mobile_finance_development.py
python app/scripts/verify_mobile_finance_sources.py
```

原始来源、实际执行、复用范围、失败尝试及归属资源清理均可逐项核对。[final-source-check.json](final-source-check.json)报告最终来源复核；生产Goal不以本地检查代替。
