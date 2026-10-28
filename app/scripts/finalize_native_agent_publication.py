"""Publish documentation only after current native execution checks have completed."""
import json
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/native-agent-runtime'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n', encoding='utf-8')


def prepend(name, text):
    path = ROOT / name
    old = path.read_text(encoding='utf-8')
    marker = text.splitlines()[0]
    if marker not in old:
        title, rest = old.split('\n', 1)
        path.write_text(title + '\n\n' + text.rstrip() + '\n\n' + rest.lstrip('\n'), encoding='utf-8')


def main():
    backend = read(E.parent / 'native-agent-backend/result.json')
    runtime = read(E / 'runtime-build.json')
    package = read(E / 'package-checks.json')
    linux = read(E / 'linux-build.json')
    reused = read(E / 'reused-evidence.json')
    assert backend['passed'] and runtime['status'] == 'passed' and runtime['tests']['pass'] == 55
    assert package['passed'] and linux['passed'] and reused['passed']
    tops, records = backend['discovered_top_level_tests'], len(backend['checks'])
    zh = '增量105：新增仓库内的 Linux 原生 Codex 执行控制器，覆盖单任务 systemd 隔离、实际内核状态取证签名、私有模型／公司工具网关、取消与 cgroup 清理，以及独立运行时发布目录。模型密钥与证明私钥留在 root 控制器中。真实服务器隔离、Codex／模型互通和八项 Skill 正式评测仍待完成；23个工作包（14 P0／9 P1）保持开放，5项移出不变。'
    en = 'Increment 105 adds a Linux native Codex broker with systemd confinement for each run, signed kernel observations, private model/company relays, cancellation with cgroup teardown and a dedicated runtime release. Provider/signing keys stay in the root broker. Actual Linux execution, genuine CLI/model interoperability and eight Skill release evaluations remain open; 23 work packages remain active, 5 excluded.'
    tests_zh = f'本轮完整后端{tops}个顶层测试／{records}条结果、运行时55项、打包4项边界检查及两个打包入口通过；两项Linux amd64交叉编译通过且未执行。Web159、移动261、四端构建及199份媒体沿用104原始范围，已核对输入与字节，不声明本轮重新录制或设备／云／模型／生产验收。'
    tests_en = f'Fresh checks: {tops} backend top-level tests/{records} result records, 55 runtime tests, 4 packaging checks, 2 packaged entry points and 2 Linux amd64 cross-builds. The 159 Web and 261 mobile checks, four-client builds and 199 media files retain their increment 104 provenance, verified against current inputs and original bytes. No new UI capture, device/cloud/model or production acceptance is claimed.'
    for name, text, links in [
        ('README.md', zh + '\n\n' + en + '\n\n' + tests_zh + '\n\n' + tests_en, '[方案与部署 / Design and deployment](docs/development/105-native-agent-runtime.md) · [验证 / Verification](docs/quality/105-native-agent-runtime-verification.md) · [全部未完成项 / All remaining work](docs/delivery/03-goal-gap-assessment.md)'),
        ('docs/README.zh-CN.md', zh + '\n\n' + tests_zh, '[完整安装和运行逻辑](development/105-native-agent-runtime.md) · [当前验证](quality/105-native-agent-runtime-verification.md) · [完整待办](delivery/03-goal-gap-assessment.md)'),
        ('docs/README.en.md', en + '\n\n' + tests_en, '[Installation and execution design](development/105-native-agent-runtime.md) · [Current verification](quality/105-native-agent-runtime-verification.md) · [Remaining work](delivery/03-goal-gap-assessment.md)')]:
        prepend(name, text + '\n\n' + links)
    replacements = {
        'docs/README.zh-CN.md': ('本轮本地业务逻辑测试：Web 159 项、移动共享逻辑 261 项、Agent runtime 52 项；构建、浏览器及生产验收以本轮验证记录为准。', '增量104历史检查：Web159、移动261、运行时52；原采集和构建范围保留，当前运行时见上方105记录。'),
        'docs/README.en.md': ('Local logic tests for this increment: Web 159, shared mobile 261 and Agent runtime 52. Build, browser and production scope are reported separately in the verification record.', 'Historical increment104 checks: Web159, shared mobile261 and runtime52. Original capture/build scope is preserved; current runtime checks are reported above for105.')}
    for name, (before, after) in replacements.items():
        path = ROOT / name
        path.write_text(path.read_text(encoding='utf-8').replace(before, after), encoding='utf-8')
    link = '[实现与部署](../development/105-native-agent-runtime.md) · [验证](../quality/105-native-agent-runtime-verification.md)'
    for name in ('docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md'):
        prepend(name, zh + '\n\n' + link)
    prepend('docs/README.md', zh + '\n\n[开发与部署](development/105-native-agent-runtime.md) · [当前验证](quality/105-native-agent-runtime-verification.md)')
    prepend('docs/media/README.md', '增量105 / Increment105：执行控制器没有界面变更；原113 PNG／43 MP4／43 GIF共199份按104及更早采集来源保留，已核对发布Git对象和SHA256。未重新录制，也未以旧API夹具证明当前Linux／模型执行。见[保留清单](../quality/evidence/native-agent-runtime/reused-evidence.json)和[本轮验证](../quality/105-native-agent-runtime-verification.md)。')
    prepend('docs/operations/01-deployment-and-operations.md', '增量105原生执行控制器：Linux root控制器与公司任务Worker分离，专用发布目录、私有模型密钥、实际内核取证和cgroup回收的安装顺序见[详细手册](../development/105-native-agent-runtime.md)。本机仅完成可移植检查和交叉编译，未启用目标服务；范围继续遵循[修订99](../delivery/04-current-scope-and-acceptance.md)。')
    prepend('docs/development/27-agent-skill-packages-and-runtime-adapter.md', '增量105更新：仓库现已提供[Linux原生控制器与独立发布目录](105-native-agent-runtime.md)，原文中缺少签发器的实现缺口已推进为目标服务器验收；真实模型／Skill发布仍开放。AI费用控制已按[修订99](../delivery/04-current-scope-and-acceptance.md)移出，技术资源限制仍保留。')
    old = ROOT / 'docs/development/27-agent-skill-packages-and-runtime-adapter.md'
    old.write_text(old.read_text(encoding='utf-8').replace('接单预留、执行中费用控制和实际结算仍须预算业务层完成。', 'AI费用预留／配额／结算不再属于当前发布要求；技术用量和执行限制按修订99及增量100处理。'), encoding='utf-8')
    prepend('docs/CHANGELOG.md', '## 增量105 · 原生Codex执行控制器 / Native Codex execution broker · 2026-10-29\n\n- 新增root所有策略／二进制与发布目录摘要、独立systemd身份／目录／网络／cgroup资源边界、Unix对端身份核验、真实内核状态签名、取消与不确定停止拒绝、重启仅回收已登记单元。\n- 模型及证明密钥留在控制器；私有网关固定模型与文本Responses路由，拒绝远程提供商工具、文件／图片输入、重定向及错误正文泄露。补齐控制器自身连接／请求／取证上限。\n- 增加独立依赖闭包打包、部署模板和完整运行手册；运行时55项测试通过，所有八项Skill仍为draft。' + f'完整后端{tops}／{records}及两项Linux交叉编译通过。\n- ' + en + '\n- [方案与部署](development/105-native-agent-runtime.md) · [验证与保留证据](quality/105-native-agent-runtime-verification.md)。原199份媒体不改采集范围；首次回归在工具输出边界补强后按源码守卫停止，原5批记录保留，最终回归重新执行。')
    agreement = ROOT / 'AGENTS.md'
    rule = '- Increment105 adds a native Linux systemd/cgroup-v2 Codex broker and a dedicated JS/Skill bundle. Provider/proof keys stay in the root broker; Windows checks and cross-builds do not establish Linux/model acceptance. Keep GAP-09/10/13 open, all eight Skills draft and AI fee controls excluded. See `docs/development/105-native-agent-runtime.md`.'
    if rule not in agreement.read_text(encoding='utf-8'):
        with agreement.open('a', encoding='utf-8') as stream:
            stream.write('\n' + rule + '\n')
    scope_path = ROOT / 'docs/delivery/04-current-scope.json'
    scope = read(scope_path)
    gap_text = ROOT / 'docs/delivery/03-goal-gap-assessment.md'
    content = gap_text.read_text(encoding='utf-8')
    for item in scope['items']:
        if item['id'] == 'GAP-09' and 'NATIVE-MODEL-RELAY-105' not in item.setdefault('completed_adaptations', []):
            item['completed_adaptations'].append('NATIVE-MODEL-RELAY-105')
        if item['id'] == 'GAP-10':
            before = item['remaining']
            item['remaining'] = '部署并验证增量105原生控制器，在目标服务器核验身份、目录／网络／工具／资源限制、真实证明、取消和重启回收；修复实际环境不兼容。'
            content = content.replace(before, item['remaining'])
            content = '\n'.join(line.replace('| 补实现＋服务器功能验收 |', '| 服务器接入＋功能验收 |') if '| GAP-10 |' in line else line for line in content.split('\n'))
            item['kind'] = '服务器接入＋功能验收'
            item['completed_adaptation'] = '增量105已提供Linux原生控制器、签发端和私有模型网关；Windows可移植检查与交叉编译不等于真实服务器隔离验收。'
    save(scope_path, scope)
    gap_text.write_text(content, encoding='utf-8')
    note = f'''# 增量105验证：Codex原生执行控制器

日期：2026-10-29（Asia/Taipei）。实现、安装、边界与目标服务器验收见[开发手册](../development/105-native-agent-runtime.md)。完整工作包仍为23项（14 P0／9 P1），5项移出；GAP-09／10／13继续开放。

| 编号 | 检查 | 实际结果与证据 |
|---|---|---|
| V-01 | 完整后端 | 最终固定源码{tops}个顶层／{records}条pass结果，所有发现测试执行、无skip。原生PG、11个串行批次；见[结果](evidence/native-agent-backend/result.json)和[原始日志归档摘要](evidence/native-agent-backend/raw-log-archive.json)。 |
| V-02 | 运行时 | 当前55项、Skill校验和编译通过；受控SDK客户端和loopback HTTP，未调用真实模型。见[运行时](evidence/native-agent-runtime/runtime-build.json)。 |
| V-03 | 打包 | 4项打包边界、真实发布目录的Skill／profile两个入口通过，685文件、八项draft，输入与输出逐字节匹配。见[打包](evidence/native-agent-runtime/package-checks.json)。 |
| V-04 | Linux | 当前platform和agent-native两个ELF64 amd64产物编译通过；未执行、未部署。见[交叉编译](evidence/native-agent-runtime/linux-build.json)。 |
| V-05 | 原客户端 | Web159、移动261、微信／抖音各20页及Android构建按104来源保留，当前输入摘要相同；26条JVM结果仍按原UP-TO-DATE范围。见[保留记录](evidence/native-agent-runtime/reused-evidence.json)。 |
| V-06 | 媒体 | 113 PNG／43 MP4／43 GIF＝199份，核对104发布Git对象与SHA256；没有新UI采集，不声明当前后端的浏览器执行。 |
| V-07 | 原生开发服务 | schema82及现有API／通知／报表Worker更新和只读核对见[更新](evidence/native-agent-runtime/development-update.json)与[检查](evidence/native-agent-runtime/development-project-resources.json)。未启动模型控制器／TG／云集成，最终临时测试库为0。 |
| V-08 | 发布安全 | 未改锁，沿用103完整依赖审计70项（6 high／57 moderate／7 low），所有剩余包记录继续开放；本轮没有新扫描声明。 |

首次回归执行中的代码复核补强了工具结果内的图片／文件输入拒绝。当前批次正常结束后，源码摘要守卫拒绝把前后版本混用；归档的5批与停止原因见[初次记录](evidence/native-agent-runtime/attempts/01-before-tool-output-hardening/result.json)。它不是最终通过结果，最终{tops}项从头重新执行，未复用中间结果。早期本地策略检查曾发现Go正则重复上限问题，已改成字符集合与独立长度检查后重跑；该诊断不作为最终证明。

内核观察测试使用显式合成快照；原生控制器在Windows拒绝启动，不存在生产“总是成功”验证器。本轮没有实际Linux单元、真实模型／CLI生成、真实云／TG或生产验收，也未做真机、模拟器、网络性能、实网120 GB、历史迁移、容灾或AI费用配额。目标服务器需要继续验证实际拒绝访问、资源限制、取消、重启回收、固定CLI HTTP兼容和八项Skill评测。

English: Final backend{tops}/{records},runtime55,4 packaging tests,2 packaged entry points and2 Linux cross-builds pass. Kernel snapshots and SDK clients are controlled fixtures. Existing UI evidence remains historical104 evidence with unchanged inputs and exact199 media. No Linux execution, real model/cloud/TG, production readiness or completed full goal is claimed.
'''
    (ROOT / 'docs/quality/105-native-agent-runtime-verification.md').write_text(note, encoding='utf-8')
    baseline = read(ROOT / 'docs/quality/current-functional-baseline.json')
    baseline.update(increment=105, evidence_group='native-agent-runtime', verification_script='verify_native_agent_sources.py',
        backend_evidence_group='native-agent-backend', backend_top_level_tests=tops, backend_result_records=records,
        runtime_evidence_group='native-agent-runtime', runtime_tests=55, web_evidence_group='multipart-cleanup',
        development_verification_script='verify_native_agent_development.py', interaction_kind='native_execution_controller_and_retained_ui', interaction_sources='native_agent_sources.py',
        retained_evidence_record='docs/quality/evidence/native-agent-runtime/reused-evidence.json', new_browser_capture=False, new_backend_test_execution=True,
        full_goal_complete=False, production_ready=False, planning_scope_verification_record='docs/quality/evidence/native-agent-runtime/scope-check.json',
        planning_publication_record='docs/quality/evidence/native-agent-runtime/repository-check.json')
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    added = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    text_types = {'.md','.py','.go','.sql','.ts','.tsx','.mjs','.cjs','.js','.json','.toml','.yaml','.yml','.xml','.css','.scss','.html','.gradle','.properties','.txt','.mod','.sum','.kt','.kts','.java','.sh','.ps1','.service','.example'}
    baseline['publication_files'] = sorted(set(baseline['publication_files'] + [v for v in changed + added if v and (ROOT / v).is_file() and Path(v).suffix.lower() in text_types]))
    save(ROOT / 'docs/quality/current-functional-baseline.json', baseline)
    print(json.dumps({'passed': True, 'increment': 105, 'backend_top_level': tops, 'backend_records': records, 'remaining': 23, 'full_goal_complete': False}))


if __name__ == '__main__':
    main()
