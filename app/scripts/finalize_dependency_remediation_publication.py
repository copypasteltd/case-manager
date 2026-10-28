"""Update bilingual delivery entries from completed, inspectable execution records."""
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
E = ROOT / 'docs/quality/evidence/dependency-remediation'
PUBLISHED = '52899b6531d118b02770d2a40842d4e56622872c'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def save(path, data):
    existing = path.read_bytes() if path.exists() else b''
    bom = b'\xef\xbb\xbf' if existing.startswith(b'\xef\xbb\xbf') else b''
    newline = '\r\n' if b'\r\n' in existing else '\n'
    text = json.dumps(data, ensure_ascii=False, indent=2) + '\n'
    path.write_bytes(bom + text.replace('\n', newline).encode('utf-8'))


def edit(name, transform):
    path = ROOT / name
    raw = path.read_bytes()
    bom = raw.startswith(b'\xef\xbb\xbf')
    line = '\r\n' if b'\r\n' in raw else '\n'
    text = raw.decode('utf-8-sig').replace('\r\n', '\n')
    updated = transform(text)
    path.write_bytes((b'\xef\xbb\xbf' if bom else b'') + updated.replace('\n', line).encode('utf-8'))


def prepend(name, content, replacements=()):
    def transform(text):
        assert content.split('\n')[0] not in text, 'Publication entry already exists; do not duplicate it.'
        for before, after in replacements:
            assert before in text, before
            text = text.replace(before, after, 1)
        title, body = text.split('\n', 1)
        return title + '\n\n' + content.strip() + '\n\n' + body.lstrip('\n')
    edit(name, transform)


def main():
    audit = read(E / 'dependency-audit.json'); security = read(E / 'security-checks.json')
    clients = read(E / 'client-checks.json'); android = read(E / 'build.json'); jvm = read(E / 'android-test-scope.json')
    retained = read(E / 'reused-evidence.json'); dev = read(E / 'development-project-resources.json')
    assert all(v['passed'] for v in [audit, security, clients, android, jvm, retained, dev])
    counts = audit['vulnerabilities']; total, high, moderate, low = [counts[v] for v in ['total', 'high', 'moderate', 'low']]
    cn = f'增量103 / Increment103：修复Taro模板下载、版本查询和Metro图像解析的实际调用链。完整依赖审计从80项降至{total}项，高风险13→{high}；剩余{total}项仍逐项开放。标准npm ci、31项依赖检查、2项原生JS检查及四端本地构建通过。界面源码未改，原107 PNG／41 MP4／41 GIF按原采集来源保留。'
    en = f'Current increment103 replaces legacy Taro template/update consumers and preserves Metro’s callable API over a maintained image parser. The full audit changes from80 to{total} package findings, high13→{high}; every remaining finding stays open. Standard npm ci,31 dependency checks,2 native JS checks and four-client local builds pass. Original UI media retain their historical capture scope;23 work packages remain active.'
    rootlinks = '[实现与使用 / Design and usage](docs/development/103-dependency-consumer-remediation.md) · [本轮验证 / Verification](docs/quality/103-dependency-remediation-verification.md) · [剩余安全记录 / Remaining findings](docs/quality/evidence/dependency-remediation/remaining-findings.json)'
    prepend('README.md', cn + '\n\n' + en + '\n\n' + rootlinks, [('增量102 / Increment102：', '此前增量102 / Previous increment102：')])
    links = '[开发说明](development/103-dependency-consumer-remediation.md) · [验证与剩余事项](quality/103-dependency-remediation-verification.md)'
    prepend('docs/README.zh-CN.md', cn + '\n\n' + links, [('最新增量102：', '此前增量102：')])
    prepend('docs/README.en.md', en + '\n\n[Implementation](development/103-dependency-consumer-remediation.md) · [Verification and open findings](quality/103-dependency-remediation-verification.md)', [('Latest increment102', 'Previous increment102')])
    prepend('docs/README.md', cn + '\n\n' + en + '\n\n' + links, [('当前增量102：', '此前增量102：')])
    prepend('docs/media/README.md', '增量103 / Increment103：本轮为构建依赖调用链修复，界面源码未改，未新增截图／录屏／GIF。107 PNG、41 MP4、41 GIF逐项保持原发布字节与原采集来源，见[复用清单](../quality/evidence/dependency-remediation/reused-evidence.json)；原102演示继续见下文。 / No new capture; all189 published media are retained byte-for-byte with their original provenance.')
    prepend('docs/contributing.md', '增量103使用 `app/scripts/verify_dependency_remediation_sources.py` 汇总当前检查。依赖和客户端／Android检查为本轮执行，未改Go和界面媒体保留102来源；Gradle缓存单独标记。见[开发约束](development/103-dependency-consumer-remediation.md)及[验证记录](quality/103-dependency-remediation-verification.md)。', [('增量102的当前功能证明', '此前增量102的功能证明')])
    prepend('docs/CHANGELOG.md', f'''## 增量103 · 构建依赖调用链修复 / Build consumer remediation · 2026-10-29

- 实现：本地CommonJS适配保留Taro模板下载／版本查询与Metro图像接口；实际使用image-size2.0.4、latest-version9.0.0、原生fetch／Git及维护版归档提取；移除旧got／git-clone／download链。Android摘要新增本地适配源码。
- 验证：标准npm ci、31项依赖＋2项原生JS检查、Web151／移动255／运行时52、两小程序各20页、Android构建／18项包检查通过；JVM26按实际缓存范围记录。完整审计80→{total}（high13→{high}），余下全部开放。
- 来源：原189份媒体和未改Go191／1201证明保留原字节；首次安装树／归档失败和npm解析尝试保留。23个完整工作包仍开放。
- English: Replaced legacy build consumers with disclosed source adapters and maintained parsers/metadata. Fresh clean-install, consumer, portable-client and native-artifact checks pass. Full-scope audit80→{total}, high13→{high}; unresolved findings and historical/cached evidence remain explicit. See [implementation](development/103-dependency-consumer-remediation.md) and [verification](quality/103-dependency-remediation-verification.md).''')
    edit('AGENTS.md', lambda text: text + '\n- Increment103 repairs Taro/Metro build consumers with explicit local compatibility packages. Keep the full remaining dependency register open, bind local package sources into Android evidence and distinguish retained102 media/Go and cached JVM results. See `docs/development/103-dependency-consumer-remediation.md`.\n')
    edit('docs/development/08-local-development.md', lambda text: text + '\n\n增量103维护Taro／Metro构建调用链，安装仍使用标准npm ci。实际接口、失败尝试、来源绑定及复现命令见[依赖调用链修复](103-dependency-consumer-remediation.md)与[验证记录](../quality/103-dependency-remediation-verification.md)。\n')
    pending_text = f'核对剩余{total}项审计记录（{high} high／{moderate} moderate／{low} low）的可达性，逐项修复、替换或形成有证据的处置；重做受影响回归与产物检查。'
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    row = next(v for v in scope['items'] if v['id'] == 'GAP-26')
    previous = row['remaining']; row['remaining'] = pending_text
    row['completed_adaptations'] = list(dict.fromkeys(row.get('completed_adaptations', []) + ['BUILD-CONSUMERS-103']))
    row['completed_detail'] = f'增量103完成三类实际构建调用链修复及当前锁复现／回归；完整审计80→{total}，high13→{high}。剩余记录均开放，整个工作包保持active。'
    save(ROOT / 'docs/delivery/04-current-scope.json', scope)
    edit('docs/delivery/03-goal-gap-assessment.md', lambda text: text.replace(previous, pending_text).replace('依赖审计原记录仍有80项（13high／60moderate／7low）', f'增量103依赖审计由80项降至{total}项（{high}high／{moderate}moderate／{low}low），三类调用链已修复'))
    cache = 'Gradle缓存，非本轮重新执行 / cached, not newly executed' if jvm['gradle_cached'] else '本轮执行 / newly executed'
    note = f'''# 增量103验证：构建依赖调用链修复

日期：2026-10-29（Asia/Taipei）。实现和完整操作见[开发说明](../development/103-dependency-consumer-remediation.md)。本轮推进GAP-26，整体仍为23项开放工作包（14 P0／9 P1），5项移出范围。

| 编号 | 实际检查 | 结果与来源 |
|---|---|---|
| V-01 | 干净安装 | 标准npm ci通过、锁字节不变；未使用force／legacy-peer。见[安装记录](evidence/dependency-remediation/clean-install.json)。 |
| V-02 | 实际调用 | 31项依赖检查＋2项原生JS检查通过，真实Metro／Taro、合成Git／HTTP／registry和图像死循环检查均在当前源码运行；所选依赖安装树有效。见[安全检查](evidence/dependency-remediation/security-checks.json)。 |
| V-03 | 便携客户端 | Web151、移动255、运行时52项测试；类型、Web／运行时及微信／抖音构建通过，两小程序各20页。见[命令记录](evidence/dependency-remediation/client-checks.json)。 |
| V-04 | Android | 原生Metro／Gradle构建、18项APK结构检查；源摘要同时绑定锁与本地适配源码。见[构建](evidence/dependency-remediation/build.json)。无设备／模拟器运行。 |
| V-05 | JVM | 26项结果：{cache}，原始任务状态／XML保留，见[范围](evidence/dependency-remediation/android-test-scope.json)。 |
| V-06 | 原生开发服务 | 只读核对schema81、当前任务API／Worker来源、无临时测试数据库，外部集成未启用。见[记录](evidence/dependency-remediation/development-project-resources.json)。 |
| V-07 | 保留证据 | 未改Go191顶层／1201结果及Linux交叉编译沿用102同源记录；107 PNG／41 MP4／41 GIF保持原字节和采集范围。见[复用清单](evidence/dependency-remediation/reused-evidence.json)。 |

| 完整扫描 | Critical | High | Moderate | Low | Total |
|---|---:|---:|---:|---:|---:|
| 原102锁，本轮新扫描 | 0 | 13 | 60 | 7 | 80 |
| 当前103锁 | {counts['critical']} | {high} | {moderate} | {low} | {total} |

这是npm按包汇总的记录，含传播关系，不是独立CVE数量。两次扫描均完整纳入构建／开发依赖，原始报告、锁摘要及逐项剩余{total}条记录见[审计](evidence/dependency-remediation/dependency-audit.json)和[剩余清单](evidence/dependency-remediation/remaining-findings.json)。扫描命令返回1表示仍有告警；GAP-26和安全发布门槛继续开放。

首次安装仍解析到嵌套旧图像库，31项最终检查之前的30项初次检查有3项失败；原记录见[初次尝试](evidence/dependency-remediation/attempts/01-initial-resolution/result.json)。npm dedupe的peer冲突和准确锁节点重解析步骤见[解析记录](evidence/dependency-remediation/attempts/02-npm-resolution/result.json)。最终以标准干净安装、当前安装树和当前源码通过结果为准，不改写原失败。

完整审计首次达到180秒命令期限，未取得的报告不计成功；见[超时记录](evidence/dependency-remediation/attempts/03-audit-timeout/result.json)。确认原进程已停止后，改用直接Node／npm CLI执行重试，完整返回当前报告，依赖扫描范围保持完整。

原生构建成功后，归档脚本首次因Windows JVM混合编码而读取失败；见[归档尝试](evidence/dependency-remediation/attempts/04-native-log-encoding/result.json)。修复为从原始字节读取ASCII任务标记，日志保持原字节与摘要。JVM任务确认为UP-TO-DATE，26项历史结果单独标记缓存，4份XML逐项校验摘要。

English: Standard npm ci,31 real-consumer/security tests,2 native JS tests,Web151/mobile255/runtime52 tests and four-client local builds pass. Full audit80→{total},high13→{high}; every remaining package finding stays open. Original189 media and unchanged Go191/1201 evidence remain historical; JVM cache scope is explicit. No device, emulator, network performance, cloud/model/TG or production acceptance is claimed.
'''
    (ROOT / 'docs/quality/103-dependency-remediation-verification.md').write_text(note, encoding='utf-8')
    baseline = read(ROOT / 'docs/quality/current-functional-baseline.json')
    baseline.update(increment=103, evidence_group='dependency-remediation', web_evidence_group='dependency-remediation', runtime_evidence_group='dependency-remediation',
                    verification_script='verify_dependency_remediation_sources.py', development_verification_script='verify_dependency_remediation_development.py',
                    dependency_audit_record='docs/quality/evidence/dependency-remediation/dependency-audit.json',
                    interaction_kind='dependency_consumer_remediation_and_retained_media', interaction_sources='dependency_remediation_sources.py',
                    retained_evidence_record='docs/quality/evidence/dependency-remediation/reused-evidence.json', new_browser_capture=False,
                    new_backend_test_execution=False, full_goal_complete=False, production_ready=False,
                    planning_scope_verification_record='docs/quality/evidence/dependency-remediation/scope-check.json',
                    planning_publication_record='docs/quality/evidence/dependency-remediation/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    text_types = {'.md','.py','.go','.sql','.ts','.tsx','.mjs','.cjs','.js','.json','.toml','.yaml','.yml','.xml','.css','.scss','.html','.gradle','.properties','.txt','.mod','.sum','.kt','.kts','.java','.sh','.ps1'}
    baseline['publication_files'] = sorted(set(baseline['publication_files'] + [v for v in changed + added if v and (ROOT / v).is_file() and Path(v).suffix.lower() in text_types]))
    save(ROOT / 'docs/quality/current-functional-baseline.json', baseline)
    print(json.dumps({'passed': True, 'increment': 103, 'audit': counts, 'full_goal_complete': False}))


if __name__ == '__main__':
    main()
