"""Publish bilingual117 guides only after exact-source validation and resource cleanup."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/contract-body'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, body):
    path = ROOT / name
    raw = path.read_bytes()
    assert '增量117'.encode() not in raw[:500] and b'Increment 117' not in raw[:500], name
    first, rest = raw.split(b'\n', 1)
    newline = b'\r\n' if first.endswith(b'\r') else b'\n'
    path.write_bytes(first + b'\n' + newline + body.strip().encode().replace(b'\n', newline) + newline + newline + rest.lstrip(b'\r\n'))


def main():
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    assert baseline['increment'] == 116
    backend = read(E.parent / 'contract_body-backend/result.json')
    proof, web, mobile, runtime, media, jvm, upgrade = [read(E / n) for n in ('final-source-check.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'media.json', 'android-test-scope.json', 'schema-upgrade.json')]
    assert all(v['passed'] for v in (backend, proof, web, mobile, media, jvm, upgrade)) and runtime['status'] == 'passed'
    top, records, nw, nm, nr = backend['discovered_top_level_tests'], len(backend['checks']), web['tests']['numPassedTests'], mobile['tests']['pass'], runtime['tests']['pass']
    retained = len(read(E / 'reused-evidence.json')['media'])
    groups = len(read(E.parent / 'contract-body-web/result.json')['checks'])
    cached = '来自Gradle缓存' if jvm['gradle_cached'] else '本轮执行'
    cached_en = 'from the actual Gradle cache' if jvm['gradle_cached'] else 'executed now'
    zh = '增量117已接通完整合同正文起草与人工采用：员工选择准确合同原修订及新文件位置，秘书通过现有扫描和提交流程生成完整文本／Markdown文件。任务页面显示全文、准确版本与SHA-256，员工核对后明确采用，系统保存一条正常合同新修订，只替换正文附件，保留原商业条件。'
    en = 'Increment 117 connects complete contract-body drafting to separate human adoption. A human selects an exact contract revision and new-file destination; ordinary staging, scanning and commit tools create the complete text/Markdown file. The task shows its full text, precise version and SHA-256. Explicit human review saves a normal contract revision that changes only its draft-body attachment and preserves commercial facts.'
    usage = '使用：本人私有秘书会话 → 新建或更新文件 → 选择准确新文件目录和名称 → 选择合同正文依据并加入请求 → 发送 → 任务进度阅读全文 → 勾选本人核对、填写依据 → 采用完整合同正文。会话、合同、目录须同项目同分类，正文最多48KiB。取消、撤权、原合同变化或文件哈希不符会阻止采用；结果未知先重新读取。'
    usage_en = 'Usage: open a private secretary conversation, choose the file task and exact new-file destination, bind the original contract, and send. Read the complete body in task progress, explicitly acknowledge review, enter a reason and adopt. Conversation, contract and directory must share project and classification; the complete file is limited to 48 KiB. Cancellation, revoked authority, changed facts or mismatched file hashes block adoption. Re-read an uncertain result before retrying.'
    boundary = '仍有23项工作包（14 P0／9 P1）、5项移出、8项draft Skill。GAP-04／12／18仅增加本轮完整文本合同附件流程；更多OA正文与Office／PDF／OCR、真实Codex／Skill质量、云和正式平台／生产验收继续开放。秘书没有合同采用、送审、签署或生效工具。仅AI费用控制移出，授权、审计、隔离及技术资源限制保留。'
    boundary_en = 'All 23 packages (14 P0/9 P1), five exclusions and eight draft Skills remain. This closes a bounded text-contract attachment path within GAP-04/12/18; wider OA/Office/PDF/OCR documents, real Codex/Skill quality, cloud and formal platform/production acceptance stay open. The model has no adoption, approval, signing or activation tool. Only AI fee controls are excluded; authorization, audit, isolation and technical limits remain.'
    validation = f'当前源码：后端{top}顶层／{records}条结果，Web{nw}、共享移动{nm}、运行器{nr}；八项客户端命令、微信／抖音各20页。Web与Taro H5各{groups}组真实页面／API检查。新6 PNG、2静音MP4、2截图导览GIF，{retained}份旧媒体保持原字节。当前开发APK通过18＋3静态检查，26条JVM{cached}；未运行设备或模拟器。另验证已发布116的实际合同旅程正常89→90升级，18张原事实表不变；原生开发服务按准确源码重载，最终耐久测试PG关闭。两个Linux目标仅交叉构建。'
    validation_en = f'Current source: {top} backend top-level tests/{records} result records, Web {nw}, shared mobile {nm}, runtime {nr}; eight client commands and 20 routes per miniapp. Web and Taro H5 each pass {groups} actual UI/API groups. New media: six PNGs, two silent MP4s and two screenshot-tour GIFs; {retained} older media remain byte-exact. The current development APK passes 18+3 static checks; 26 JVM results are {cached_en}. No device or emulator runs. A published116 contract journey verifies the normal89→90 upgrade with 18 original fact tables unchanged. Attested development services reload and the final durable test PG closes; two Linux targets only cross-build.'
    validation += ' 两次早期Web尝试的临时录屏帧被覆盖，未保留且不计最终证据；原记录和归档缺失说明保留，最终本轮素材完整。'
    validation_en += ' Temporary recording frames from two earlier Web attempts were overwritten and are unavailable, with no final-evidence credit. Their original records and retention-failure disclosure remain; the final current media are complete.'
    for name, prefix, lang in [('README.md', 'docs/', 'both'), ('docs/README.md', '', 'both'), ('docs/README.zh-CN.md', '', 'cn'), ('docs/README.en.md', '', 'en')]:
        paragraphs = [zh, en, usage, usage_en, boundary, boundary_en, validation, validation_en] if lang == 'both' else [zh, usage, boundary, validation] if lang == 'cn' else [en, usage_en, boundary_en, validation_en]
        links = f'[方案与使用 / Design]({prefix}development/117-contract-body-drafting.md) · [核验 / Verification]({prefix}quality/117-contract-body-verification.md) · [截图／录屏／GIF / Media]({prefix}media/README.md) · [全部未完成项 / Remaining]({prefix}delivery/03-goal-gap-assessment.md)'
        prepend(name, '\n\n'.join(paragraphs + [links]))
    intro = '增量117：GAP-04／12／18接通四端准确合同原资料绑定、完整文本正文文件、全文核对和独立人工采用。只替换正常合同修订的正文附件，保留原商业条件。更广格式、完整OA附件体系和真实模型验收继续开放；23 active（14 P0／9 P1）、5 excluded不变。见[设计](../development/117-contract-body-drafting.md)与[核验](../quality/117-contract-body-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, intro)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_path = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    raw = gap_path.read_bytes()
    changes = {
        'GAP-04': '补更多格式的读取／预览／创建／编辑与原件排版差异；用PDF、Office、OCR及复杂DOCX样本核对实际支持范围。增量113支持已提取格式全文比较，增量117支持完整文本合同正文与独立人工采用，更广格式仍开放。',
        'GAP-12': '补更多项目工具、文件格式与跨范围整理、更广OA正文附件、更多申请类型与模板写作、跨订阅完整审阅计划和人员／模板目录；评测真实模型异常判断与起草质量。增量117完成受控完整文本合同正文及人工采用，更广OA与真实模型验收仍开放。',
        'GAP-18': '按四端矩阵核验完整项目／OA／财务旅程、普通附件、账期跨域核对；补正式业务引用文件及超限目录树的跨范围协同、批量策略编辑器和保留执行器；核对上传／暂停／重试／凭据过期等功能。增量115受控人工跨范围计划、117完整文本合同正文采用均为局部实现。',
    }
    for row in scope['items']:
        if row['id'] not in changes:
            continue
        assert row['remaining'].encode() in raw
        raw = raw.replace(row['remaining'].encode(), changes[row['id']].encode())
        row['remaining'] = changes[row['id']]
        row.setdefault('completed_adaptations', []).append('CONTRACT-BODY-DRAFTING-117')
        row['completed_detail'] += ' 增量117接通准确合同原修订和新文件授权、受控完整UTF-8文本／Markdown正文、两类任务读取全文、当前权限与近期MFA人工采用、不可变收据和正常合同新修订；更广格式、完整旅程及真实模型质量仍开放。'
    gap_path.write_bytes(raw)
    save(scope_path, scope)
    prepend('docs/requirements/07-secretary-and-channels.md', '增量117当前约束：file-update 1.4.0（draft）可按准确合同依据新建完整UTF-8文本／Markdown正文，48KiB上限；采用由独立人类命令执行，秘书不得采用、送审、签署或生效。更广正文附件、Office／PDF／OCR与真实模型评测仍开放。见[详细实现](../development/117-contract-body-drafting.md)。')
    prepend('docs/CHANGELOG.md', '\n\n'.join(['## 增量117 / Increment117 · 完整合同正文与人工采用 / Complete contract bodies', zh, en, validation, validation_en,
        'Schema90新增两张不可变绑定／采用表；原333路径保留，新增一个人工采用路径，现334路径／410操作／736模型。七组API／PG覆盖完整文件、事件流全文、当前权限、MFA、原资料变化、取消、幂等和事务回滚；九个共享正文边界场景在Web与移动各执行一次，不计为十八个不同场景。首次编译、类型入口、漏选移动测试、浏览器夹具、旧版升级收集和事件流缺项均保留原记录；修正后重新运行最终适用检查。',
        '[设计](development/117-contract-body-drafting.md) · [核验](quality/117-contract-body-verification.md)']))
    prepend('docs/media/README.md', '''增量117 / Increment117：真实认证后的隔离合成合同正文页面。6 PNG、2静音MP4、2截图导览GIF。录屏包含完整正文阅读、显式勾选与真实采用回执；认证信息不入素材。GIF每张2秒，属于截图导览。

| 端 | 原资料绑定 | 全文核对 | 已采用 | 录屏 | GIF导览 |
|---|---|---|---|---|---|
| Web | [PNG](screenshots/contract-body-web-scope.png) | [PNG](screenshots/contract-body-web-review.png) | [PNG](screenshots/contract-body-web-adopted.png) | [MP4](videos/contract-body-web.mp4) | [GIF](gifs/contract-body-web.gif) |
| 共享移动 | [PNG](screenshots/contract-body-mobile-scope.png) | [PNG](screenshots/contract-body-mobile-review.png) | [PNG](screenshots/contract-body-mobile-adopted.png) | [MP4](videos/contract-body-mobile.mp4) | [GIF](gifs/contract-body-mobile.gif) |

[媒体来源](../quality/evidence/contract-body/media.json) · [核验](../quality/117-contract-body-verification.md)。旧媒体保留实际捕获日期与原范围。''')
    verification = f'''# 增量117核验 / Increment117 verification

[业务方案与接口](../development/117-contract-body-drafting.md) · [全部未完成项](../delivery/03-goal-gap-assessment.md)。

| 编号 | 检查 | 实际结果与边界 |
|---|---|---|
| CB-V01 | 完整后端 | {top}顶层／{records}条结果，12个串行批次，所有发现用例执行，无跳过。嵌套结果不等于不同业务场景数。 |
| CB-V02 | 新API／PG | 七组覆盖准确完整文件、人类采用与幂等、未核对／错误哈希／近期MFA／原子回滚、原合同变化、执行前依据失效、未绑定历史来源、当前读取权限和停止运行；同时核对任务及事件流的全文、采用历史和撤权拒绝。 |
| CB-V03 | 客户端 | Web{nw}、共享移动{nm}、运行器{nr}；九个共同边界场景在Web与移动重复执行，不算十八个不同场景；运行器新增独立模型工具边界检查。 |
| CB-V04 | 四端构建 | 八项类型／测试／构建命令，微信／抖音各20页；当前开发APK18＋3静态检查，JVM26{cached}，四份XML保留。未运行设备、模拟器或平台审核。 |
| CB-V05 | 实际页面 | Web、Taro H5各{groups}组，实际API／PG，不替换响应：选择准确依据、提交、等待／取消、完整文件全文、独立显式采用、原商业事实保留、重放、拒绝重复采用及重新读取历史。 |
| CB-V06 | 正常升级 | 已发布116的真实合同修订旅程先在89执行，再以当前migrator升90；18张原合同／引用／财务／历史AI表保持原字节摘要，新表为空且四项约束就绪；不算当前后端测试。 |
| CB-V07 | 媒体 | 6 PNG、2真实静音录屏及2截图导览GIF，认证前后分开；{retained}份旧媒体原字节保留。 |
| CB-V08 | 开发与关闭 | 仅有归属证明的API和两项worker按当前源码重载，原生开发PG89→90，合同／现金／历史资料不变。最终耐久测试PG关闭且0临时库；早期中止集群已关闭，1个中止合成库离线私有保留，不冒充已清空。 |
| CB-V09 | 接口与Linux | 原路径不变，现334路径／410操作／736模型；两个Linux ELF只交叉构建，不是目标内核／真实模型验收。 |
| CB-V10 | 依赖与范围 | 当前完整依赖扫描0已知公告；只显式声明原已解析@noble/hashes2.4.0，解析包未变化。正式产物安全仍GAP-26；23 active／5 excluded、八项draft Skill和完整目标未完成状态保留。 |

实际页面最初发现事件流漏返正文视图，已修正并补API覆盖，最终全部后端按修正后源码重新运行。此前的已完成批次、中止批次和通过的旧源码升级／Linux构建保持原摘要，不借用为最终通过。原客户端通过记录因移动漏选九项而保留旧390计数，补测试入口后新执行为399。编译、类型、浏览器夹具和升级收集失败也保留原日志，不替代最终验收。

两次早期通过的Web尝试没有复制临时录屏清单和帧，后续捕获已覆盖原字节，无法恢复或重新核验。原源码、截图、结果和日志保留，原归档引用不改写；[归档失败原检查](evidence/contract-body-attempts/recording-retention-v1/original-failed-source-check.json)与[明确缺失说明](evidence/contract-body-attempts/recording-retention-v1/disclosure.json)保持公开。这两份旧录屏不计保存成功，也不计最终证据。最终本轮Web／移动录屏、清单和媒体独立核验；后续归档器已改为复制原清单及全部哈希核对后的帧。

| 证据 | 入口 |
|---|---|
| 准确源码 | [最终检查](evidence/contract-body/final-source-check.json)、[私有完整源码归档摘要](evidence/contract-body/source-archive.json) |
| 后端 | [全量](evidence/contract_body-backend/result.json)、[批次](evidence/contract_body-backend/batch-progress.json)、[原日志](evidence/contract_body-backend/raw-log-archive.json) |
| 客户端与页面 | [八项命令](evidence/contract-body/client-checks.json)、[Web](evidence/contract-body-web/result.json)、[Taro H5](evidence/contract-body-mobile/result.json) |
| Android | [构建](evidence/contract-body/build.json)、[18项检查](evidence/contract-body/apk-inspection.json)、[正文内容](evidence/contract-body/android-feature-content.json)、[JVM范围](evidence/contract-body/android-test-scope.json) |
| 升级与开发 | [实际89→90](evidence/contract-body/schema-upgrade.json)、[服务重载](evidence/contract-body/development-update.json)、[只读检查](evidence/contract-body/development-project-resources.json) |
| 媒体与依赖 | [捕获与编码](evidence/contract-body/media.json)、[目视核对](evidence/contract-body/visual-review.json)、[旧媒体](evidence/contract-body/reused-evidence.json)、[当前扫描](evidence/contract-body/dependency-audit.json) |
| Linux与资源 | [交叉构建](evidence/contract-body/linux-build.json)、[最终PG关闭](evidence/contract-body/native-cluster-closed.json) |
| 初期尝试 | [日志摘要](evidence/contract-body/preliminary-checks.json)、[原类型失败](evidence/contract-body-attempts/client-types-v1.json)、[移动入口修正前](evidence/contract-body-attempts/before-test-selection/preservation.json) |
| 浏览器与旧版升级 | [夹具参数](evidence/contract-body-attempts/01-fixture-flags/preservation.json)、[事件流缺项](evidence/contract-body-attempts/02-event-log-contract-body/preservation.json)、[升级收集](evidence/contract-body-attempts/upgrade-v1/preservation.json)、[修正前后端](evidence/contract-body-attempts/before-event-log-fix/preservation.json) |

These checks establish the bounded complete-text drafting and explicit human adoption workflow. Prior, interrupted and cached evidence retains its actual execution scope. Two earlier Web recordings were lost through temporary-path reuse; the original failed retention check is disclosed and these recordings receive no preservation or final-pass credit. Final current media are independently verified. Live model/Skills, cloud, formal platform artifacts and production acceptance remain open.
'''
    (ROOT / 'docs/quality/117-contract-body-verification.md').write_text(verification, encoding='utf-8')
    (E / 'README.md').write_text('# 增量117证据 / Increment117 evidence\n\n准确源码、完整本地回归、真实页面与像素、旧版合同升级、开发服务和资源关闭分别记录。失败、中止及旧源码通过结果保持原范围；所有凭据、APK、原始帧和私有源码归档留在Git之外。\n\n[方案](../../../development/117-contract-body-drafting.md) · [核验](../../117-contract-body-verification.md)。\n', encoding='utf-8')
    path = ROOT / 'AGENTS.md'
    raw = path.read_bytes()
    raw += b'\r\n- Increment117 adds exact contract-basis full-text file drafting and separate human adoption across four clients, schema90 and complete run/event-log views. Preserve original commercial facts, current authorization, recent MFA, exact UTF-8 file hashes and immutable ordinary revision receipts. Wider OA/Office/PDF/OCR attachments and real model/Skill acceptance remain GAP-04/12/18; all eight Skills stay draft. See `docs/development/117-contract-body-drafting.md`.\r\n'
    path.write_bytes(raw)
    baseline.update(increment=117, evidence_group='contract-body', mobile_tests=nm,
        verification_script='verify_contract_body_sources.py', backend_evidence_group='contract_body-backend',
        backend_top_level_tests=top, backend_result_records=records, web_evidence_group='contract-body', runtime_evidence_group='contract-body',
        web_tests=nw, runtime_tests=nr, development_verification_script='verify_contract_body_development.py',
        interaction_kind='human_contract_body_adoption', interaction_sources='contract_body_sources.py',
        retained_evidence_record='docs/quality/evidence/contract-body/reused-evidence.json',
        dependency_audit_record='docs/quality/evidence/contract-body/dependency-audit.json',
        planning_scope_verification_record='docs/quality/evidence/contract-body/scope-check.json',
        planning_publication_record='docs/quality/evidence/contract-body/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    future = ['docs/quality/evidence/contract-body/' + n for n in ('final-source-check.json', 'scope-check.json', 'acceptance-applicability-check.json')]
    names = set(baseline['publication_files'] + future + [n for n in changed + added if n and (ROOT / n).is_file()])
    baseline['publication_files'] = sorted(n for n in names if Path(n).suffix.lower() not in {'.png', '.gif', '.mp4', '.webm', '.log'} and n != 'docs/quality/evidence/contract-body/repository-check.json')
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 117, 'backend_top_level': top, 'backend_records': records, 'active': 23, 'excluded': 5}))


if __name__ == '__main__':
    main()
