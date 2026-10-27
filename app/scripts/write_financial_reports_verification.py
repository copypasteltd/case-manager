"""Write the bilingual financial delivery record from actual frozen local results."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/finance-reports'


def read(name):
    return json.loads((E / name).read_text(encoding='utf-8'))


def main():
    backend, web, runtime, mobile = [read(n) for n in ('result.json', 'web-build.json', 'runtime-build.json', 'mobile-build.json')]
    build, inspection, unit, media, history, development = [read(n) for n in (
        'build.json', 'apk-inspection.json', 'android-test-scope.json', 'media.json',
        'historical-87.json', 'development-project-resources.json')]
    assert all(r['passed'] for r in (backend, web, mobile, build, inspection, unit, media, development))
    assert runtime['status'] == 'passed'
    names = [r['Test'] for r in backend['checks']]
    leaves = sum(not any(other.startswith(name + '/') for other in names) for name in names)
    apk = build['artifacts']['apk']
    rows = []
    for client in ('web', 'mobile'):
        folder = E.parent / ('financial-reports-' + client)
        browser = json.loads((folder / 'result.json').read_text(encoding='utf-8'))
        capture = json.loads((folder / 'capture.json').read_text(encoding='utf-8'))
        proof = json.loads((folder / 'download-proof.json').read_text(encoding='utf-8'))
        assert browser['passed'] and browser['owned_browser_closed'] and browser['isolated_database_cleanup_passed']
        assert proof['actual_download_verified']
        recording = next(r for r in media['recordings'] if r['client'] == client)
        rows.append(f"| {client} | {len(browser['checks'])} flows / {len(capture['layouts'])} sizes; {proof['row_count']} downloaded rows | {recording['capture_frame_count']} frames / {recording['capture_duration_ms']} ms | `{browser['source_sha256']}` |")
    cache = '缓存复用 / cached results' if unit['gradle_cached'] else '本轮新执行 / newly executed'
    content = f'''# 增量88验证 / Increment88 verification

财务发生、现金、订阅预测的完整筛选、规范来源与限时异步CSV导出。对应FIN-12、GAP-03及GAP-18；[设计、操作与接口](../../../development/88-financial-report-filters-and-exports.md)。

Complete filters, canonical source pagination and expiring asynchronous CSV exports for incurred, cash and subscription-forecast reports. This record binds local source and artifacts; the full four-client production goal remains open.

## 1. 当前源码与检查 / Current sources and checks

| Layer | Actual result | Source SHA-256 |
| --- | --- | --- |
| Go / PostgreSQL | {backend['discovered_top_level_tests']} top-level / {len(names)} results / {leaves} leaves; {backend['batch_count']} batches; migration73 additive, earlier72 unchanged | `{backend['source_sha256']}` |
| Web | {web['tests']['numPassedTests']} tests and production build, including8 report-client regressions | `{web['source_sha256']}` |
| Runtime / Skills | {runtime['tests']['pass']} tests, validation and build | `{runtime['source_sha256']}` |
| Taro | {mobile['tests']['pass']} tests, types, WeChat/Douyin builds with18 pages each | `{mobile['source_sha256']}` |
| Android | {apk['bytes']:,} byte development APK; {len(inspection['checks'])} package checks; {unit['tests']} JVM tests: {cache} | `{build['source_sha256']}` |

APK SHA-256：`{apk['sha256']}`。开发包保留在忽略目录，不提交Git。四份XML和原生日志保留准确的`{unit['task']}`标记；本轮26项确实新执行，不以JVM或包检查代替设备验收。Linux只进行当前源码交叉编译，没有执行或部署。

The private development APK is not a release artifact. Four frozen XML suites and the native task marker establish actual new JVM execution. Linux evidence proves compilation only; no real device or deployment is claimed.

新增14组原生数据库业务场景：人工原币／项目分摊／冲销／草稿算例、所有筛选与签名翻页、严格查询、现金与预测独立事实、可读来源候选、幂等排队、围栏生成与安全CSV、后续事实不改旧快照、取消、三种口径导出、本人任务分页、数据库不可变约束、到期清理及授权变化。另有精确小数／公式安全测试。旧秘书财务证据查询通过完整回归。

Fourteen new native scenarios exercise the real HTTP and database workflow, including independent manual arithmetic, fixed snapshot semantics and authority changes. Separate client tests reject malformed, reordered, cross-job or corrupt chunks and clear buffered data after cancellation or failure.

人工算例中可读HKD净发生76.4、USD10分别显示，另有一笔草稿不计入；现金90、预测200各用自身规范来源。导出实际筛选项目、HKD、software、供应方及经办人，完整生成并下载27条；不能把筛选后的金额与未筛选总额混用。费用／现金的经办是原记录创建者，预测负责人来自固定期间修订。导出在worker执行时固定，排队时并未冻结。

The manual fixture preserves separate original currencies and excludes drafts. Cash and forecast use independent records. Both clients actually retrieve a complete filtered27-row CSV and verify its length and digest; enqueueing alone never counts as retrieval or a fixed snapshot.

## 2. 页面、录像与下载 / Pages, recordings and downloads

| Client | Actual checks | Actual viewport capture | Capture source SHA-256 |
| --- | --- | --- | --- |
{chr(10).join(rows)}

每端5组流程涵盖真实MFA、三种口径、筛选、来源分页／原记录、排队状态和实际文件下载，并检查3个尺寸。录像只从身份输入结束后的报表页开始。新增6张PNG、2段MP4和2个GIF；目录合计54／19／19。短录屏来自连续实际视口帧；自动化操作较快，可用播放器慢速查看。GIF保留1倍时间，编码帧数与原始采样帧数分别记录。

Six screenshots and two MP4/GIF pairs contain actual post-login browser pixels. Capture manifests retain original timestamps and hashes; output hashes and FFprobe checks bind the public media. All six screenshots and first/middle/last animation frames received visual inspection. Taro H5 checks do not establish WeChat, Douyin or Android-device acceptance. Raw frames and CSV remain ignored.

两个最终浏览器上下文、测试API／隔离数据库及5189页面服务已关闭／释放，私有业务fixture已删除。每端download-proof记录实际CSV行数、长度、摘要及筛选，未公开CSV或业务登录信息。

## 3. 历史、失败及开发环境 / History, attempts and development

已发布增量87按`{history['commit']}`重建完整启动与两端捕获来源，{len(history['preserved_evidence'])}份旧证据逐字节不变；更早84缓存限制继续按原记录保留。

Published increment87 reconstructs from its actual Git objects; {len(history['preserved_evidence'])} historical evidence artifacts remain byte-identical. Earlier cache limitations are not relabeled as newly verified work.

早期未冻结尝试仅保留诊断与私有日志摘要，不宣称可重建。后续三次失败浏览器检查分别来自重复金额文本的严格选择器、Web／移动不同翻页名称及嵌套locator；另一次通过捕获被最终脚本替代。四次均保留完整890文件启动源码ZIP、实际结果及清理状态；捕获清单额外引用的一份未改辅助脚本按已发布73d994b对象补证，完整捕获摘要一致。来源复核首次遗漏该辅助脚本的诊断也按原验证器字节保留。完整后端回归发现旧秘书证据查询仍传10参数而共享谓词已改13参数，保留当时完整后端ZIP、失败批状态和日志摘要；修复两个调用后重新执行全部9批并通过。没有放宽业务权限或MFA来通过检查。

Frozen failed and superseded attempts retain exact source archives. The full regression exposed a genuine old secretary-query compatibility bug, which was corrected before a complete fresh nine-batch pass. Superseded passes are not current release evidence. Unfrozen early failures remain diagnostic only.

只更新归属已核对的本机API到73迁移，新增归属已记录且只读取数据库环境配置的报表worker；保留PostgreSQL与通知worker。只读探针确认2张报表表、4项不可变触发器、运行身份不能更新请求或快照、所有权与当前后端来源匹配、零残留隔离测试库。没有生成正式业务测试记录。八项Skill保持draft，project-update保持1.6.0／16工具／14写工具；模型／云／TG继续关闭。

Only the recorded owned local API is reloaded, and one DB-only export worker is started. PostgreSQL and the notification worker are preserved. Native probes confirm migration73, report guards, source ownership and no disposable databases. No real model, cloud, TG or production integration is activated.

## 4. 复现与放行边界 / Reproduction and remaining acceptance

按[开发文档](../../../development/88-financial-report-filters-and-exports.md)使用原生命令。完整后端由`verify_backend_batches.py --batch-size 20 --evidence-group finance-reports`执行；两端由`run_financial_reports_browser.py`（移动加`--mobile`）串行捕获，`encode_financial_reports_media.py`转码。运行前归档已存在证据，禁止将旧通过重绑修改后源码。Android包检查、准确JVM冻结、历史重建和当前来源核对分别由`inspect_android_apk.py`、`freeze_financial_reports_evidence.py`、`record_financial_reports_history.py`、`verify_financial_reports_sources.py`执行。发布另校验实际Git暂存字节、敏感值和全部本地Markdown链接。

Use native tools only, archive superseded evidence and close every owned temporary browser/service. The source verifier recomputes source and artifact hashes; publication checks additionally inspect selected and actual staged repository bytes.

完整四端Goal继续开放：财务制度／汇率与跨OA旅程、更多移动财务操作、真实微信／抖音／Android设备文件交付、真实云与模型／TG、120GB及2TB迁移、生产隔离／安全、备份恢复与正式上线仍待验收。依赖锁文件未改，保留114项审计问题（11critical／32high）的未关闭安全门。本轮不声称已修复或放行。全部28项GAP保留，`full_goal_complete=false`。

Full business, platform, real-device, cloud/model/TG, capacity, recovery and production acceptance remain open. The unchanged dependency lock retains the prior114 findings, including11 critical and32 high; no new security-clearance claim is made.
'''
    (E / 'verification.md').write_text(content, encoding='utf-8')
    print(json.dumps({'written': True, 'backend_results': len(names), 'browser_clients': len(rows), 'native_tests': unit['tests']}))


if __name__ == '__main__':
    main()
