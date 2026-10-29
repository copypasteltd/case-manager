"""Publish bilingual109 notes only after actual source-bound verification succeeds."""
import json
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/audit-console'


def read(p):
    return json.loads(p.read_text(encoding='utf-8'))


def save(p,v):
    p.write_text(json.dumps(v, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def prepend(name, text):
    p = ROOT / name; old = p.read_text(encoding='utf-8')
    if text.splitlines()[0] not in old:
        title, rest = old.split('\n',1)
        p.write_text(title + '\n\n' + text.rstrip() + '\n\n' + rest.lstrip('\n'), encoding='utf-8')


def main():
    backend = read(E.parent / 'audit-console-backend/result.json')
    assert backend['passed'] and read(E / 'final-source-check.json')['passed'] and read(E / 'media.json')['passed']
    web, mobile, runtime = [read(E / n) for n in ('web-build.json','mobile-build.json','runtime-build.json')]
    assert web['passed'] and mobile['passed'] and runtime['status'] == 'passed'
    tops, records = backend['discovered_top_level_tests'], len(backend['checks'])
    cached = read(E / 'android-test-scope.json')['gradle_cached']
    shots = sum(len(read(E.parent / ('audit-console-' + client) / 'capture.json')['screenshots']) for client in ('web','mobile'))
    zh = '增量109已接通四端组织级审计查询与固定CSV导出：准确筛选、签名游标、元数据详情、本人导出历史、近期MFA和下载摘要校验。原业务正文权限继续生效；导出最多10,000条／8 MiB、有效24小时。抖音端可查询和生成，CSV暂由同账号Web下载。GAP-07的模型／Skill、TG运行及存储管理仍开放；全部23项（14 P0／9 P1）和5项移出不变。AI仅移出费用控制，秘书、八项Skill、TG、授权、审计、执行隔离和技术限制保留。'
    en = 'Increment 109 adds organization audit metadata queries and sealed CSV exports across Web and the shared WeChat, Douyin and Android pages: exact filters, signed cursors, event details, owner history, recent MFA and verified file bytes. Business-content permissions remain enforced. Exports allow up to 10,000 rows / 8 MiB for 24 hours. Douyin supports query/create; use the same account on Web to download its CSV. Model/Skill, TG operations and storage administration remain open under GAP-07. All 23 packages (14 P0/9 P1) and 5 exclusions remain. Only AI cost control is excluded; secretary, eight Skills, TG, authorization, audit, isolation and technical limits are retained.'
    validation = f'当前源码核验：后端{tops}个顶层测试／{records}条结果，Web185、共享移动281、运行时55，微信／抖音各20页，Android开发APK及18＋4项静态检查、2个Linux交叉构建。Web与Taro H5各5组实际浏览器／API操作通过；新增{shots} PNG／2静音MP4／2 GIF，原211份媒体字节保留。26条JVM结果来自'+('Gradle缓存' if cached else '本轮执行')+'。Web布局修正后3项相关命令重跑，5项未受影响命令保留本轮原始执行；不声明真机、真实云／模型或生产验收。'
    validation_en = f'Current checks: {tops} backend top-level tests / {records} records, 185 Web, 281 shared mobile and 55 runtime tests; 20 pages per miniapp, a development APK with 18 + 4 static checks and 2 Linux cross-builds. Both real Web/Taro H5 journeys pass 5 check groups. New media: {shots} PNGs, 2 silent MP4s and 2 GIFs; all 211 historical media retain exact bytes. The 26 JVM records came from '+('Gradle cache' if cached else 'this execution')+'. After the Web layout correction, 3 affected commands reran; 5 unaffected commands retain their original same-increment execution. No device, live cloud/model or production acceptance is claimed.'
    links = '[设计与操作 / Design and use](docs/development/109-audit-console.md) · [验证与来源 / Verification](docs/quality/109-audit-console-verification.md) · [全部待办 / All remaining work](docs/delivery/03-goal-gap-assessment.md)'
    prepend('README.md', zh + '\n\n' + en + '\n\n' + links + '\n\n' + validation + '\n\n' + validation_en + '\n\n![审计查询与固定导出 / Audit query and sealed exports](docs/media/gifs/audit-console-web.gif)')
    for name, text in [('docs/README.zh-CN.md',zh + '\n\n' + validation), ('docs/README.en.md',en + '\n\n' + validation_en), ('docs/README.md',zh + '\n\n' + en + '\n\n' + validation + '\n\n' + validation_en)]:
        prepend(name, text + '\n\n' + links.replace('(docs/','('))
    prepend('docs/CHANGELOG.md', '## 增量109 · 审计查询与固定导出 / Audit query and sealed exports · 2026-10-29\n\n- ' + zh + '\n- ' + en + '\n- ' + validation + '\n- ' + validation_en + '\n- [设计与操作](development/109-audit-console.md) · [验证及失败尝试](quality/109-audit-console-verification.md)。')
    media = f'增量109 / Increment109：实际Web／Taro H5审计筛选、详情、固定导出历史和文件摘要核对，新增{shots} PNG、2段静音MP4、2个GIF。仅隔离合成数据；录制从认证后开始，不含密码或MFA输入。原211份媒体按原来源保留，不声明真机或生产验收。\n\n| 客户端 / Client | 详情 / Details | 固定导出 / Export | 录屏 / Recording | GIF |\n|---|---|---|---|---|\n| Web | [PNG](screenshots/audit-console-web-detail.png) | [PNG](screenshots/audit-console-web-export.png) | [MP4](videos/audit-console-web.mp4) | [GIF](gifs/audit-console-web.gif) |\n| Taro H5 | [PNG](screenshots/audit-console-mobile-detail.png) | [PNG](screenshots/audit-console-mobile-export.png) | [MP4](videos/audit-console-mobile.mp4) | [GIF](gifs/audit-console-mobile.gif) |\n\n[验证与来源 / Provenance](../quality/109-audit-console-verification.md)。'
    prepend('docs/media/README.md', media)
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'; scope = read(scope_path)
    gap = next(v for v in scope['items'] if v['id'] == 'GAP-07')
    remaining = '补Skill／模型正式管理、TG运行／披露／投递管理、存储配置及必要运行状态管理；继续完成统一后台的真实配置与业务验收。'
    old_remaining = gap['remaining']; gap['remaining'] = remaining
    gap['completed_adaptations'] = sorted(set(gap.get('completed_adaptations',[]) + ['AUDIT-CONSOLE-109']))
    gap['completed_detail'] = '增量109已实现组织级审计元数据查询、准确筛选、签名分页、固定事件引用CSV、本人导出历史、近期MFA、权限失效和分段／完整摘要核对；四端共享使用约束，抖音文件下载暂由Web承接。整个GAP-07继续active。'
    save(scope_path, scope)
    p = ROOT / 'docs/delivery/03-goal-gap-assessment.md'; text = p.read_text(encoding='utf-8')
    if old_remaining != remaining:
        assert old_remaining in text; p.write_text(text.replace(old_remaining,remaining), encoding='utf-8')
    note = '增量109：GAP-07的审计查询与固定导出已接通四端页面、真实API和原生PG约束；模型／Skill、TG运行和存储配置管理仍未完成。工作包按完整范围计数，保持23 active（14 P0／9 P1）／5 excluded，不把子功能或移出项记为整个工作包通过。见[设计](../development/109-audit-console.md)与[验证](../quality/109-audit-console-verification.md)。'
    for name in ('docs/delivery/02-implementation-ledger.md','docs/delivery/03-goal-gap-assessment.md','docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name,note)
    p = ROOT / 'docs/requirements/02-identity-and-permissions.md'
    note = '\n## 组织级审计元数据 / Organization audit metadata · Increment109\n\n明确授予的`audit.read`可查询全组织技术事件，不因此获得项目或业务正文权限；`audit.export`另行授予且需同时拥有查询权。只向需要这些操作线索的人员授权。通用响应及CSV仅含固定元数据列，不含自由原因、会话ID、payload或正文。导出仅人类交互会话可用，固定事件引用、原始筛选、创建者和授权版本；24小时或权限变化后失效。创建和下载要求近期MFA。见[实现与使用](../development/109-audit-console.md)。\n\nExplicit organization audit authority is independent of business content. Export access is separately granted, owner-bound, time-limited and checked against current sessions and authorization. Reasons and content remain outside the generic disclosure.\n'
    if 'Organization audit metadata · Increment109' not in p.read_text(encoding='utf-8'):
        p.write_text(p.read_text(encoding='utf-8').rstrip() + '\n' + note, encoding='utf-8', newline='\n')
    p = ROOT / 'AGENTS.md'
    rule = '- Increment109 adds organization audit metadata query and sealed owner-bound CSV exports across Web and shared mobile settings. Keep dedicated audit.read/audit.export, human sessions, recent MFA, metadata allowlist, immutable references and current-authority/hash checks. Model/Skill, TG operations and storage administration remain open under GAP-07. See `docs/development/109-audit-console.md`.'
    if rule not in p.read_text(encoding='utf-8'):
        p.write_text(p.read_text(encoding='utf-8').rstrip() + '\n\n' + rule + '\n', encoding='utf-8')
    proof_count = len(read(E / 'final-source-check.json')['checks'])
    note = f'''# 审计查询与固定导出验证 / Audit console verification

增量109通过当前源码检查；GAP-07仅完成审计子功能，完整23项工作包仍开放。AI仅排除费用控制。应用实现和命令见[开发说明](../development/109-audit-console.md)。

{validation}

{validation_en}

| 编号 | 核验 | 实际结果与边界 |
|---|---|---|
| AUD-109-01 | 原生后端全量 | {tops}个顶层／{records}条结果，全部发现项执行，无skip；[最终记录](evidence/audit-console-backend/result.json)与[原始日志摘要](evidence/audit-console-backend/raw-log-archive.json)。 |
| AUD-109-02 | 审计功能 | 3个原生单元测试＋1个HTTP顶层／9个子场景：准确边界、并列时间分页、游标换条件／人员、四个会话渠道、独立导出权、MFA、幂等／固定引用、摘要、10,001条整单拒绝、空CSV、DB不可变约束、审计失败回滚、到期和撤销。 |
| AUD-109-03 | 客户端 | [命令记录](evidence/audit-console/client-checks.json)：Web185／移动281／运行时55；原8项命令通过后，只修正2个Web文件，类型／Web测试／Web构建3项重新执行，未受影响的5项保留原始记录与准确输入。 |
| AUD-109-04 | 微信／抖音 | 各20页构建，保留真实AppID／域名／身份发布验收；抖音不声明原生CSV文件出口。 |
| AUD-109-05 | Android | 原生Metro＋Gradle开发包，18项APK及4项功能内容静态检查；26条JVM为{'缓存' if cached else '新执行'}，XML与任务状态保留。无真机／模拟器。 |
| AUD-109-06 | 实际浏览器 | [Web](evidence/audit-console-web/result.json)和[Taro H5](evidence/audit-console-mobile/result.json)各5组真实API操作；准确筛选、空结果、分页、详情、固定导出、本人历史与文件大小／SHA核对。所有自建上下文、服务和数据库清理完成。 |
| AUD-109-07 | 媒体 | {shots} PNG、2段原速静音MP4、2个GIF来自认证后的真实浏览器像素；原211份媒体字节未变，见[媒体记录](evidence/audit-console/media.json)和[保留来源](evidence/audit-console/reused-evidence.json)。 |
| AUD-109-08 | 开发持久环境 | 普通schema83→84升级，历史费用记录摘要保留，API及2个Worker仅替换已核对身份的自有进程；临时数据库0，云／TG／模型接入未启用，不是生产部署。 |
| AUD-109-09 | 契约与源码 | 原314路径／687模型语义逐项保持；新增后合计320路径／395操作／698模型；{proof_count}组最终来源核对通过。2个Linux amd64 ELF交叉编译，未在Linux执行。 |

HTTP四渠道检查使用隔离fixture的Web Cookie＋CSRF及微信／抖音／Android渠道会话，不是实际平台登录。组织边界通过现有单组织约束、未知组织外键和未知事件拒绝核对；不声称部署了两个真实租户。系统管理员可读明确授权的组织元数据，实际项目正文请求仍返回404。

失败与修正均保留：早期OpenAPI完整生成遗漏已有摊销扩展，源码守卫中止该不完整尝试；已把扩展加入统一生成流程，并逐项核对既有契约。新增回执测试的字面量类型错误在类型检查时被拒绝；修正后完整客户端命令通过。首轮浏览器准备请求遗漏Origin被403拒绝，修正脚本而未放宽CSRF。首次Web功能已通过，随后修正按钮对齐和导航标题并重新检查、重新采集最终媒体。

全量后端首次完整执行中，旧项目集成用例在DROP临时数据库时超过独立两分钟清理期限；初始失败及全部原始日志保留。仅清理超时的批次按同一后端摘要串行复跑，通过的其他批次保留原始执行，未放宽业务断言或更改数据库耐久设置。详见[清理复跑来源](evidence/audit-console-backend/result.json)。首次清理超时留下3个PostgreSQL已标记无效的测试库；按准确名称、创建时间、失败日志窗口、无活动连接和无效删除标志归属后，未使用FORCE完成清理，未声称读取不可连接库内的组织或迁移内容。最终零残留检查独立通过，见[清理回执](evidence/audit-console/fixture-cleanup.json)。

本轮没有重新查询依赖公告或重新执行增量108的56＋2依赖安全专项；锁文件、根manifest及workspace依赖字段保持一致，完整npm审计0项等结果保留增量108的原日期和范围。正式生产依赖与产物验收继续属于GAP-26。

No real-device validation, network performance/real120GB exercise, historical migration/disaster recovery or AI cost control is added as a gate. Live Codex/model/Skills, actual cloud/TG/miniapp identities, formal Android release and complete applicable business journeys remain in the [numbered work register](../delivery/03-goal-gap-assessment.md). Passing local checks does not establish production readiness.
'''
    (ROOT / 'docs/quality/109-audit-console-verification.md').write_text(note, encoding='utf-8')
    (E / 'verification.md').write_text('# Increment109 verification\n\nSee [complete verification and provenance](../../109-audit-console-verification.md) and [implementation/usage](../../../development/109-audit-console.md). All current-source checks are local; full goal and production acceptance remain open.\n', encoding='utf-8')
    p = ROOT / 'docs/quality/current-functional-baseline.json'; baseline = read(p)
    baseline.update(increment=109, evidence_group='audit-console', verification_script='verify_audit_console_sources.py',
        backend_evidence_group='audit-console-backend', backend_top_level_tests=tops, backend_result_records=records,
        web_evidence_group='audit-console', web_tests=185, mobile_tests=281, runtime_evidence_group='audit-console', runtime_tests=55,
        development_verification_script='verify_audit_console_development.py', interaction_kind='organization_audit_console',
        interaction_sources='audit_console_sources.py', retained_evidence_record='docs/quality/evidence/audit-console/reused-evidence.json',
        new_browser_capture=True, new_backend_test_execution=True, full_goal_complete=False, production_ready=False,
        planning_scope_verification_record='docs/quality/evidence/audit-console/scope-check.json',
        planning_publication_record='docs/quality/evidence/audit-console/repository-check.json')
    changed = subprocess.check_output(['git','diff','--name-only','-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git','ls-files','--others','--exclude-standard','-z'], cwd=ROOT).decode().split('\0')
    kinds = {'.md','.py','.go','.sql','.ts','.tsx','.mjs','.cjs','.js','.json','.toml','.yaml','.yml','.xml','.css','.scss','.html','.gradle','.properties','.mod','.sum','.kt','.kts','.java','.sh','.ps1','.service','.example'}
    baseline['publication_files'] = sorted(set(baseline['publication_files'] + [name for name in changed + added if name and (ROOT / name).is_file() and Path(name).suffix.lower() in kinds]))
    save(p, baseline)
    print(json.dumps({'passed':True,'increment':109,'backend_top_level':tops,'backend_records':records,'new_screenshots':shots,'remaining':23,'full_goal_complete':False}))


if __name__ == '__main__':
    main()
