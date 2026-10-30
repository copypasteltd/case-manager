"""Publish bilingual118 guides and gap details after current-source verification."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/payment-fee'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert '增量118'.encode() not in raw[:500] and b'Increment 118' not in raw[:500], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    path.write_bytes(first + b'\n' + newline + body.strip().encode().replace(b'\n', newline) + newline + newline + rest.lstrip(b'\r\n'))


def main():
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 117
    backend = read(E.parent / 'payment_fee-backend/result.json')
    proof, web, mobile, media, jvm, upgrade = [read(E / n) for n in ('final-source-check.json', 'web-build.json', 'mobile-build.json', 'media.json', 'android-test-scope.json', 'schema-upgrade.json')]
    assert all(v['passed'] for v in (backend, proof, web, mobile, media, jvm, upgrade))
    top, records, nw, nm = backend['discovered_top_level_tests'], len(backend['checks']), web['tests']['numPassedTests'], mobile['tests']['pass']
    groups = len(read(E.parent / 'payment-fee-web/result.json')['checks'])
    cached = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮执行'
    cached_en = 'from actual Gradle cache' if jvm['gradle_cached'] else 'executed now'
    zh = '增量118已接通四端付款手续费依据：从实际资金列表选择已登记手续费付款和整条费用原分配，核对完整双币依据、准确版本与摘要，明确确认及MFA后保存。误关联通过独立撤销记录纠正；关联、撤销均不新增费用或现金，不改变原报表金额。'
    en = 'Increment 118 connects actual payment fee evidence across four clients. Select recorded fee cash and its original incurred-cost allocation, review both currency axes and exact source versions/hashes, then confirm explicitly with MFA. A separate immutable revocation corrects a mistaken association. Neither action creates expenses/cash or changes original report amounts.'
    usage = '使用：先确认真实手续费费用、登记实际资金并核销 → 原付款详情 → 查阅手续费依据 → 关联手续费依据 → 选择实际资金（或同笔付款）及原分配 → 核对凭证、填写依据、明确确认和MFA → 查看历史；错误关联可明确撤销。原付款、手续费资金及费用须同管理项目、同分类。过期资料先刷新，结果未知先核对历史。'
    usage_en = 'Usage: confirm the actual fee cost, record cash and allocate it first. Open the original payment, expand fee evidence, select actual fee cash (or the same payment) and its root allocation, review the source receipts, enter a reason and confirm with MFA. Correct mistakes through explicit revocation. All three facts must share management project and classification. Refresh changed sources; inspect history before retrying an uncertain result.'
    boundary = '当前仍23项工作包（14 P0／9 P1）、5项移出、8项draft Skill。GAP-03／18增加本轮手续费依据整理；公司财务口径、汇兑差额、异币实收退款和完整四端旅程继续开放。仅AI费用控制移出；秘书、TG、授权、审计、隔离及技术资源限制保留。'
    boundary_en = 'All 23 packages (14 P0/9 P1), five exclusions and eight draft Skills remain. This adds fee evidence within GAP-03/18; company accounting policy, exchange differences, other-currency refunds and complete client journeys stay open. Only AI fee controls are excluded; secretary, TG, authorization, audit, isolation and technical limits remain required.'
    validation = f'本轮当前源码：后端{top}顶层／{records}条结果，Web{nw}、共享移动{nm}、运行器56；八项客户端命令、两小程序各20页，Web／H5各{groups}组实际页面与API检查。新增6 PNG、2静音MP4和2截图导览GIF，保留306份旧媒体。Android开发包18＋3项静态检查，26条JVM{cached}；正常90→91升级保留18类原记录，开发服务已按准确源码重载，独立测试PG关闭。两个Linux目标仅交叉构建；没有设备、模拟器、真实云／模型／平台发布或生产验收声明。'
    validation_en = f'Current source: {top} backend top-level tests/{records} result records, Web {nw}, shared mobile {nm}, runtime 56; eight client commands, 20 routes per miniapp, and {groups} actual UI/API groups per Web/H5. New media: six PNGs, two silent MP4s and two screenshot-tour GIFs; 306 older media remain exact. The development APK passes 18+3 static checks; 26 JVM results are {cached_en}. Normal90→91 upgrade preserves18 original fact groups; attested development services reload and the owned test PG closes. Two Linux artifacts only cross-build. No device, emulator, live cloud/model, platform publication or production acceptance is claimed.'
    for name, prefix, lang in [('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')]:
        paragraphs = [zh, en, usage, usage_en, boundary, boundary_en, validation, validation_en] if lang == 'both' else [zh, usage, boundary, validation] if lang == 'cn' else [en, usage_en, boundary_en, validation_en]
        links = f'[方案与使用 / Design]({prefix}development/118-payment-fee-evidence.md) · [核验 / Verification]({prefix}quality/118-payment-fee-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining]({prefix}delivery/03-goal-gap-assessment.md)'
        prepend(name, '\n\n'.join(paragraphs + [links]))
    intro = '增量118：GAP-03／18接通四端真实手续费原费用／资金／分配的准确关联、当前双币余额、明确人工撤销、不可变审计历史及版本分页。整理依据不改费用／现金事实，不自动推导汇兑差额。整个23项工作包（14 P0／9 P1）和5项移出不变。见[设计](../development/118-payment-fee-evidence.md)与[核验](../quality/118-payment-fee-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap_path.read_bytes()
    changes = {
        'GAP-03': '继续完成公司财务口径采纳、汇兑差额确认／结转、与原付款异币的实收退款及完整四端财务旅程。增量106提供人工选用摊销，116提供双币核销，118提供真实手续费依据关联与纠错；制度采纳、更多手续费报表口径和完整验收仍开放。',
        'GAP-18': '按四端矩阵核验完整项目／OA／财务旅程、普通附件、账期跨域核对；补正式业务引用文件及超限目录树的跨范围协同、批量策略编辑器和保留执行器；核对上传／暂停／重试／凭据过期等功能。增量115跨范围计划、117合同正文采用、118手续费依据均为局部实现。',
    }
    for row in scope['items']:
        if row['id'] not in changes:
            continue
        assert row['remaining'].encode() in raw
        raw = raw.replace(row['remaining'].encode(), changes[row['id']].encode())
        row['remaining'] = changes[row['id']]
        row.setdefault('completed_adaptations', []).append('ACTUAL-PAYMENT-FEE-EVIDENCE-118')
        row['completed_detail'] += ' 增量118已实现四端真实手续费依据选择、准确费用／现金／整条双币分配快照、显式人工MFA关联与撤销、动态原余额、组织串行及完整审计回滚；关联与撤销不改变原金额，完整工作包仍开放。'
    gap_path.write_bytes(raw)
    save(scope_path, scope)
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量118 / Increment118 · 付款手续费依据 / Actual payment fees', zh, en, validation, validation_en,
        'Schema91新增两张仅追加表及四个不可变／来源约束触发器。保留旧334路径与736模型全部语义，现337路径／414操作／748模型。六组后端业务用例核对真实事实、双币余额、显式MFA、并发、分页、撤权和审计回滚；八个共享边界场景在Web与移动各执行一次。依赖未变，复用已发布117的准确锁文件零公告证据，未重新查询公告；正式产物安全仍GAP-26。初次编译、合成重名定位及只读脚本旧版本预期失败保留原记录，不计最终通过。',
        '[设计](development/118-payment-fee-evidence.md) · [核验](quality/118-payment-fee-verification.md)']))
    prepend('docs/media/README.md', '''增量118 / Increment118：实际认证后的隔离合成手续费页面；6 PNG、2静音MP4、2截图导览GIF。MP4记录已关联依据的真实页面滚动；GIF按三张实际截图展示核对、关联及撤销，每张2秒。密码、MFA输入及私有能力不入素材。旧素材保留原日期与原检查范围。

| 端 | 完整依据核对 | 已关联 | 已撤销 | 实际录屏 | 截图导览 |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/payment-fee-web-review.png) | [PNG](screenshots/payment-fee-web-linked.png) | [PNG](screenshots/payment-fee-web-revoked.png) | [MP4](videos/payment-fee-web.mp4) | [GIF](gifs/payment-fee-web.gif) |
| 共享移动 | [PNG](screenshots/payment-fee-mobile-review.png) | [PNG](screenshots/payment-fee-mobile-linked.png) | [PNG](screenshots/payment-fee-mobile-revoked.png) | [MP4](videos/payment-fee-mobile.mp4) | [GIF](gifs/payment-fee-mobile.gif) |

[来源与编码](../quality/evidence/payment-fee/media.json) · [实际核验](../quality/118-payment-fee-verification.md)。''')
    verification = f'''# 增量118核验 / Increment118 verification

{validation}

{validation_en}

| 检查 | 对应记录 |
|---|---|
| 当前源码最终{len(proof['checks'])}项 | [最终源码](evidence/payment-fee/final-source-check.json)、[完整私有源码归档摘要](evidence/payment-fee/source-archive.json) |
| 后端全部发现用例 | [240顶层／1492条结果](evidence/payment_fee-backend/result.json)、[12份原日志保留](evidence/payment_fee-backend/raw-log-archive.json) |
| 完整客户端命令及真实计数 | [八项命令](evidence/payment-fee/client-checks.json)、[Web](evidence/payment-fee/web-build.json)、[共享移动](evidence/payment-fee/mobile-build.json)、[运行器](evidence/payment-fee/runtime-build.json) |
| 实际页面与API | [Web14组](evidence/payment-fee-web/result.json)、[H5 14组](evidence/payment-fee-mobile/result.json) |
| 当前Android开发包 | [构建](evidence/payment-fee/build.json)、[18项静态检查](evidence/payment-fee/apk-inspection.json)、[3项功能内容检查](evidence/payment-fee/android-feature-content.json)、[实际JVM缓存范围](evidence/payment-fee/android-test-scope.json) |
| 媒体、原帧与人工可视核对 | [编码及原帧摘要](evidence/payment-fee/media.json)、[实际图像检查](evidence/payment-fee/visual-review.json) |
| 正常旧版升级 | [已发布117双币旅程／26条原分配／18类原记录](evidence/payment-fee/schema-upgrade.json) |
| 开发环境与关闭 | [准确服务重载](evidence/payment-fee/development-update.json)、[只读核对](evidence/payment-fee/development-project-resources.json)、[独立测试PG关闭](evidence/payment-fee/native-cluster-closed.json) |
| Linux仅交叉构建 | [两个ELF](evidence/payment-fee/linux-build.json) |
| 保留旧证据与依赖范围 | [306份旧媒体](evidence/payment-fee/reused-evidence.json)、[准确锁文件复用零公告](evidence/payment-fee/dependency-audit.json) |
| 初期与失败尝试 | [私有日志及源码摘要](evidence/payment-fee/preliminary-checks.json)、[合成重名定位失败](evidence/payment-fee-attempts/01-web-duplicate-titles/preservation.json) |

首轮编译有一个未使用测试辅助函数，修正后执行专项与全部回归；Web首次检查因多条合成付款同名而无法唯一定位，按实际新记录位置选择并重新核对最终命令准确来源；服务只读核验脚本曾预期90，修为91后复查通过。原记录及相应源码保留，未将失败改写成通过。所有原始帧使用独立目录，并核对每帧摘要；凭据、数据库、APK及私有源码归档留在Git之外。

These records establish the bounded actual-fee evidence workflow. Reused dependency evidence retains its original query date; cached JVM results retain their real status. Classification of AC/TC applicability is separate and executes no business cases. Live model/Skills, TG, cloud, formal platform artifacts, production database and full business acceptance remain open. No excluded device/network/DR/AI-budget gate is reintroduced.

[方案与使用](../development/118-payment-fee-evidence.md) · [所有未完成项](../delivery/03-goal-gap-assessment.md)
'''
    (ROOT / 'docs/quality/118-payment-fee-verification.md').write_text(verification, encoding='utf-8')
    (E / 'README.md').write_text('# 增量118证据 / Increment118 evidence\n\n准确源码、完整本地回归、真实页面与媒体、正常旧版升级、服务重载及资源关闭分别记录。初期／失败／复用／缓存证据保留实际范围。\n\n[方案](../../../development/118-payment-fee-evidence.md) · [核验](../../118-payment-fee-verification.md)。\n', encoding='utf-8')
    path = ROOT / 'AGENTS.md'
    path.write_bytes(path.read_bytes() + b'\r\n- Increment118 adds actual payment fee evidence and explicit human revocation across four clients, schema91 and immutable independent history. Preserve original cost/cash/allocation facts, exact dual-currency basis, current human authority and MFA, and one active association per root. Company policy, FX differences, other-currency refunds and full GAP-03/18 remain open; AI fee controls stay excluded. See `docs/development/118-payment-fee-evidence.md`.\r\n')
    baseline.update(increment=118, evidence_group='payment-fee', mobile_tests=nm,
        verification_script='verify_payment_fee_sources.py', backend_evidence_group='payment_fee-backend',
        backend_top_level_tests=top, backend_result_records=records, web_evidence_group='payment-fee', runtime_evidence_group='payment-fee',
        web_tests=nw, runtime_tests=56, development_verification_script='verify_payment_fee_development.py',
        interaction_kind='actual_payment_fee_evidence', interaction_sources='payment_fee_sources.py',
        retained_evidence_record='docs/quality/evidence/payment-fee/reused-evidence.json',
        dependency_audit_record='docs/quality/evidence/payment-fee/dependency-audit.json',
        planning_scope_verification_record='docs/quality/evidence/payment-fee/scope-check.json',
        planning_publication_record='docs/quality/evidence/payment-fee/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    future = ['docs/quality/evidence/payment-fee/' + n for n in ('scope-check.json', 'acceptance-applicability-check.json')]
    names = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    baseline['publication_files'] = sorted(n for n in names if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != 'docs/quality/evidence/payment-fee/repository-check.json')
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 118, 'backend_top_level': top, 'backend_records': records, 'active': 23, 'excluded': 5}))


if __name__ == '__main__':
    main()
