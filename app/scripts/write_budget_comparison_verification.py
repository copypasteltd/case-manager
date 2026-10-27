"""Write the bilingual delivery record from actual current source/artifact evidence."""
from pathlib import Path
import json

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/budget-comparison'


def read(name):
    return json.loads((E / name).read_text(encoding='utf-8'))


def main():
    backend, web, runtime, mobile = [read(name) for name in ('result.json', 'web-build.json', 'runtime-build.json', 'mobile-build.json')]
    build, inspection, unit, media, history = [read(name) for name in ('build.json', 'apk-inspection.json', 'android-test-scope.json', 'media.json', 'historical-86.json')]
    development = read('development-project-resources.json')
    assert all(item['passed'] for item in (backend, web, mobile, build, inspection, media, development))
    assert runtime['status'] == 'passed'
    names = [row['Test'] for row in backend['checks']]
    leaves = sum(not any(other.startswith(name + '/') for other in names) for name in names)
    apk = build['artifacts']['apk']
    rows = []
    for client in ('web', 'mobile'):
        browser = json.loads((E.parent / ('budget-comparison-' + client) / 'result.json').read_text(encoding='utf-8'))
        capture = json.loads((E.parent / ('budget-comparison-' + client) / 'capture.json').read_text(encoding='utf-8'))
        assert browser['passed'] and browser['owned_browser_closed'] and browser['isolated_database_cleanup_passed']
        recording = next(row for row in media['recordings'] if row['client'] == client)
        rows.append(f"| {client} | {len(browser['checks'])} flows / {len(capture['layouts'])} sizes | {recording['capture_frame_count']} frames / {recording['capture_duration_ms']} ms | `{browser['source_sha256']}` |")
    cache = '缓存复用 / cached results' if unit['gradle_cached'] else '本轮新执行 / newly executed'
    content = f'''# 增量87验证 / Increment87 verification

项目当前批准预算与规范实际费用的完整对比，包含逐项期间、项目份额、冲销、预算外及原记录分页。Web和Taro提供真实操作入口。全部本地工作使用原生工具，不调用Docker、WSL、虚拟机或模拟器；完整四端生产目标仍开放。

This functional increment adds current implemented budget comparison and canonical source drilldown across Web and Taro. All checks use native tools. No real cloud/model/Telegram operation, device acceptance or production deployment is claimed.

## 1. 当前源码验证 / Current source verification

| Layer | Actual result | Source SHA-256 |
| --- | --- | --- |
| Go / PostgreSQL | {backend['discovered_top_level_tests']} top-level / {len(names)} results / {leaves} leaves; {backend['batch_count']} batches; all72 existing migrations unchanged | `{backend['source_sha256']}` |
| Web | {web['tests']['numPassedTests']} tests and build, including6 new shared-client regressions | `{web['source_sha256']}` |
| Runtime / Skills | {runtime['tests']['pass']} tests, validation and build | `{runtime['source_sha256']}` |
| Taro | {mobile['tests']['pass']} tests, types, WeChat/Douyin compilation;17 pages each | `{mobile['source_sha256']}` |
| Android | {apk['bytes']:,} byte development APK; {len(inspection['checks'])} package checks;25 JVM tests: {cache} | `{build['source_sha256']}` |

APK SHA-256：`{apk['sha256']}`。开发包保留在忽略目录，未提交到Git。JVM任务标记、四份XML与日志已冻结；缓存不算新执行。Linux只证明当前源码交叉编译，不证明已部署或执行。

The APK remains outside Git. Frozen native task markers and XML establish the exact cache/execution scope. Linux evidence establishes cross-compilation only.

新增11组原生业务场景：无预算与明确零预算、无日期／类别的部分草稿、闭区间边界、多个项目的实际份额、原费用与冲销自己的日期、原币分列、原记录与状态筛选、签名分页及格式错误、仅报表／逐条权限与实际撤权、完整100条明细，以及实施新预算后旧依据失效。金额期望来自固定人工算例，不由应用函数反算。

Eleven native scenarios use actual HTTP business transactions, including independent approval and explicit implementation. They verify exact arithmetic, original canonical identities and live authority. The shared client checks reject cached cross-project/currency data, old bases, draft contributions and decimal coercion.

人工算例：HKD预算1300；预算内当前可读净发生1385，差额-85；预算外正费用55、冲销1000，净额-945；全期间净发生440。两笔未确认费用不计入实际支出。一笔100费用只贡献本项目60份额，不加另一项目40；另一币种不混入。页面明确这是授权范围，不称为项目全部剩余额度或可用资金。

The independent example reconciles HKD1385 inside the plan plus HKD-945 outside to HKD440 lifetime. Drafts, another project's share and another currency do not inflate incurred expenditure. No bank action is performed.

## 2. 实际页面与素材 / Actual pages and media

| Client | Actual flows | Actual captured pixels | Capture source SHA-256 |
| --- | --- | --- | --- |
{chr(10).join(rows)}

每端执行真实MFA登录、当前批准依据、逐项与未确认来源、未完整草稿、预算外冲销、真实费用分页与原记录跳转、100条外币预算和无预算查询，以及3个屏幕尺寸。录制发生在身份输入完成之后，帧中没有密码、MFA输入或能力令牌。4张PNG和2组MP4／GIF来自实际像素，不把静态拼图冒充操作录像。

Both clients exercise the authenticated native API with isolated synthetic data. Actual source pagination and navigation are captured after credentials leave the screen. Frame manifests retain original timing and hashes; encoded output hashes and FFprobe checks bind the public artifacts. Raw JPEGs remain ignored.

最终媒体目录共48张PNG、17段MP4、17个GIF。两个最终上下文及其浏览器、临时API／数据库和页面服务均已关闭。失败与被后续脚本替代的素材保留在attempts中，不能当作当前通过证据。

The catalog contains48 PNGs,17 MP4s and17 GIFs. Owned resources are closed. Failed/superseded attempts stay separate from current passing evidence.

## 3. 历史、失败与开发状态 / History, attempts and native state

增量86的完整启动来源与两端完整捕获清单从已发布提交`{history['commit']}`重建，共{len(history['preserved_evidence'])}份旧证据保持原字节。旧媒体不重新绑定本轮来源；更早84的编译缓存限制仍按原历史说明保留。

Increment86 complete startup and both capture manifests reconstruct from the published Git objects. Historical evidence bytes are preserved.

`attempts/initial-checks`明确只作早期未冻结失败的诊断摘要：命令工作目录、完整测试提案与创建回执、原冲销接口语义、读取授权的多个有效来源，以及最终证明脚本的AST预检。首次移动捕获在来源加载时点击了暂禁用的选择器；等待组件就绪后通过。下一次捕获通过5组流程，但返回缓存项目页时误等已改变的展开按钮；检查器随后复用已展开表单。失败记录、自建资源释放和被替代的来源／帧清单均保留。 后端第5批原平台测试的业务断言通过，但原夹具删除隔离数据库超过2分钟；保留失败批次状态，精确清理1个无连接测试库，保持持久化与超时不变，待其他构建／捕获结束后通过原来源校验恢复执行失败批及剩余批次。

最终来源检查首次有两项失败：新检查器误写第72项既有迁移的文件名；新文档缺少Android及“完整四端”的明确文字。保留当时的23项通过／2项失败、检查脚本与文档原字节，修正工具和说明后重跑；全部72项真实迁移一直与上次发布一致，没有更改应用或测试。

The first final-proof attempt retained23 passing and two failing checks: a mistyped existing migration filename and missing explicit client-scope wording. Its checker, report and document bytes remain archived. Only the checker and documentation were corrected; all72 actual migrations already matched the published bytes.

发布链接检查曾将原字节归档Markdown按新目录解析，报告归档中的3个旧相对链接；保留24项通过／1项失败，并仅将归档命名为`.md.source`，原字节与原目录记录不变。当前正式Markdown的链接仍完整检查。

The publication-link attempt retained24 passes and one failure: an exact archived Markdown snapshot was parsed relative to its evidence directory. It now has a `.md.source` suffix with the original path and unchanged hash recorded. All live guide links remain checked.

Initial unfrozen failures are explicitly diagnostic summaries. Archived captures preserve the picker-readiness race and the cached-page expansion-state assumption, including the passing flows before failure and cleanup. No business/MFA behavior was weakened to pass a checker. One backend batch failed only in the existing database-cleanup deadline. Its state and exact inactive-fixture cleanup are retained; the unchanged runner resumes by source hash and reruns the failed batch after concurrent builds/captures stop.

只更新归属已验证的本机API，保留PostgreSQL与通知Worker。只读探针确认72迁移、组织／权限与原预算不可变控制、零活动模型配置／启用TG／残留测试库。八项Skill保持draft，project-update保持1.6.0／16工具／14写工具。没有新建生产测试预算或费用。

Only the verified owned local API is reloaded. PostgreSQL and the notification worker are preserved. Runtime probes confirm disabled external integrations and no disposable databases. No real business records or default financial policy were introduced.

## 4. 复现与未完成验收 / Reproduction and remaining acceptance

按[操作与接口文档](../../../development/87-project-budget-comparison.md)的原生命令执行；重新捕获前先归档原证据，保持来源冻结。实际APK检查、历史重建及最终来源检查分别由`inspect_android_apk.py`、`record_budget_comparison_history.py`和`verify_budget_comparison_sources.py`执行。发布另核对实际Git暂存字节、敏感值和本地链接。

Use the documented native setup. Archive earlier reports/media before rerunning; do not rebind an old pass to changed source. APK, history, source and staged-publication checks bind the actual artifacts.

完整筛选／异步导出、公司汇率与摊销制度、移动财务写入、跨OA旅程、真实云与模型／TG、实体设备、120GB上传／2TB迁移、生产备份恢复与四端首发仍须验收。锁文件未改；原审计114项（11 critical／32 high）继续作为未关闭安全门，不声称已修复或放行。全部28项GAP保留，`full_goal_complete=false`。

Full financial/mobile/production scope remains open. The unchanged dependency lock retains the prior114 findings, including11 critical and32 high; this increment makes no new security-clearance claim.
'''
    (E / 'verification.md').write_text(content, encoding='utf-8')
    print(json.dumps({'written': True, 'backend_results': len(names), 'browser_clients': len(rows)}))


if __name__ == '__main__':
    main()
