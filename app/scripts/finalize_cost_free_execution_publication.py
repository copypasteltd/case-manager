"""Write the reviewable current publication manifest from completed real proofs."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/cost-free-execution'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write_document(path, text):
    newline = '\r\n' if path.exists() and b'\r\n' in path.read_bytes() else '\n'
    path.write_bytes(text.replace('\n', newline).encode('utf-8'))


def main():
    backend = read(E.parent / 'cost-free-execution-backend/result.json')
    proof = read(E / 'final-source-check.json')
    build = read(E / 'build.json')
    jvm = read(E / 'android-test-scope.json')
    assert backend['passed'] and proof['passed'] and build['passed'] and jvm['passed']
    assert backend['discovered_top_level_tests'] == 189 and len(backend['checks']) == 1181
    assert not jvm['new_test_execution'] and jvm['gradle_cached'] and jvm['tests'] == 26
    apk, bundle = build['artifacts']['apk'], build['artifacts']['bundle']
    old = read(E.parent / 'dependency-maintenance/build.json')
    assert apk['sha256'] != old['artifacts']['apk']['sha256'] and bundle['sha256'] != old['artifacts']['bundle']['sha256']
    summary = {'passed': True, 'checked_at': datetime.now(timezone.utc).isoformat(), 'source_sha256': build['source_sha256'],
        'new_metro_execution': True, 'new_apk_payload': True, 'apk': apk, 'bundle': bundle,
        'apk_inspections': 18, 'feature_payload_checks': 3, 'native_jvm_tests': 26,
        'jvm_new_execution': False, 'jvm_gradle_cached': True,
        'scope_record': 'android-test-scope.json', 'device_executed': False, 'production_signed_release': False,
        'previous_published_artifact_record': 'docs/quality/evidence/dependency-maintenance/build.json',
        'finalizer_sha256': sha(Path(__file__))}
    save(E / 'artifact-summary.json', summary)
    commit = '27a0d30aaa1ac80ce17c6703fce2eb7fb5e5848e'
    media = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', commit, '--', 'docs/media'], cwd=ROOT).decode().splitlines():
        meta, name = line.split('\t', 1)
        if Path(name).suffix in ('.png', '.mp4', '.gif'):
            media.append({'file': name, 'git_blob': meta.split()[2], 'sha256': sha(ROOT / name)})
    retained = ['docs/quality/evidence/dependency-maintenance/dependency-audit.json',
        'docs/quality/evidence/dependency-maintenance/audit-after.json',
        'docs/quality/evidence/miniapp-identity/android-test-scope.json']
    save(E / 'reused-evidence.json', {'published_commit': commit,
        'media_scope': 'All167 old canonical media retain original bytes/source scope; two new captures per medium are recorded separately.',
        'media': media, 'records': [{'file': n, 'sha256': sha(ROOT / n)} for n in retained],
        'audit_scope': 'Retained increment98 complete audit bound to unchanged lock; no current advisory-feed refresh claimed.',
        'jvm_scope': '26 cached results; original97 execution provenance remains. Current XML hashes are in android-test-scope.json.',
        'backend_scope': 'Every current-source case executed in this increment; first failed cleanup attempt retained and two failed top-level cases rerun serially.'})
    save(E / 'attempts/development-outcome-note.json', {'original_json_preserved': False,
        'scope': 'Outcome note from the initial read-only check output, not a reconstructed original artifact.',
        'initial_observed_exit_code': 1, 'initial_observed_stdout': [
            {'passed': True, 'migration_count': 79, 'probe_count': 10, 'active_runtime_profiles': 0},
            {'passed': False, 'migration_count': 79, 'disposable_databases': 2, 'integrations_disabled': True}],
        'resolution': 'Two exact invalid, inactive fixture databases matched failed DROP statements in the owned cluster log and were removed; current recheck reports0.',
        'cleanup_record': 'docs/quality/evidence/cost-free-execution/fixture-cleanup.json',
        'current_inspection_record': 'docs/quality/evidence/cost-free-execution/development-project-resources.json'})
    doc = f'''# 增量100 · 秘书执行验证 / Secretary execution verification

日期：2026-10-28。实现说明见[开发文档](../development/100-secretary-execution-without-cost-quota.md)，当前范围见[修订99](../delivery/04-current-scope-and-acceptance.md)。本次关闭GAP-09的费用前置适配子项，完整GAP-09与其他22个工作包仍开放；5项移出不算完成。

## 1. 当前源码检查

| 编号 | 项目 | 实际范围／结果 |
| --- | --- | --- |
| V-01 | 原生后端 | 189个顶层测试、1181条结果；全部当前Go／SQL／内嵌契约源码摘要一致 |
| V-02 | 新业务回归 | 无策略／旧0额度／旧超额、真实读工具与结果、幂等、并发、取消、技术用量、旧接口停写和历史授权等12个场景通过 |
| V-03 | Web | 135项测试、类型和构建通过 |
| V-04 | 共享移动端 | 248项测试、类型通过；微信／抖音本地构建各20页 |
| V-05 | Agent运行时 | 51项测试、构建、8项Skill清单检查通过；Skill仍draft，未调用真实模型 |
| V-06 | Android | 新Metro产物与新开发APK；18项包结构检查＋3项本功能载荷检查通过；未运行真机／模拟器 |
| V-07 | JVM | 26项结果为Gradle UP-TO-DATE复用，**本次不是26项新执行**；XML原字节和原97来源保留 |
| V-08 | Linux | 当前后端linux/amd64交叉编译通过；未执行或部署 |
| V-09 | 浏览器 | Web和Taro H5各4项实际流程断言；登录、只读账本、真实筛选、390／768／1440宽度及返回秘书 |
| V-10 | 开发环境 | 原生PG结构78→79；原历史表为空，升级前后数量／摘要一致；应用角色权限、10条探针、进程身份及0个临时库通过 |

当前开发APK为{apk['bytes']:,}字节，SHA-256：`{apk['sha256']}`。实际包内JS与新Metro摘要一致，包含只读账本标题，未包含旧月度费用编辑表单。缓存范围见[Android记录](evidence/cost-free-execution/artifact-summary.json)。这不建立Android真实运行或正式签名发布结论。

## 2. 失败、复跑和清理

完整首轮10个原生批次中，`TestCommentAttachmentPreservation`与`TestDownloadPermitExpiryAndClassificationChanges`的业务断言未报告失败，但2分钟隔离库清理期限到期，因此首轮整体标记失败。保留[首轮结果](evidence/cost-free-execution-backend/attempt01/result.json)与[原批次记录](evidence/cost-free-execution-backend/attempt01/batch-progress.json)，未增加清理期限或关闭数据库持久性。所有其他当前源码通过结果保留；这两项随后单独串行复跑并通过，见[最终原生结果](evidence/cost-free-execution-backend/result.json)。

最初环境检查仍发现2个残留测试库。只处理名称符合严格测试前缀、无连接、PostgreSQL invalid标志为真，且在本仓库PG日志中有准确失败DROP语句的两库；未触及日常业务库、其他数据库或重启PG。清理后重新检查0个临时库，见[受限清理](evidence/cost-free-execution/fixture-cleanup.json)。初次开发检查JSON被后续检查替换，保留的是明确标注的[输出结果说明](evidence/cost-free-execution/attempts/development-outcome-note.json)，不把它称为原JSON。

定向检查最初遇到原生PG未启动、工具参数写错、单公司测试准备与404隐藏资源语义不符；修正后12场景通过。失败日志哈希与原因保留在[定向尝试](evidence/cost-free-execution/attempts/focused-attempts.json)，完整结果以当前源码的全量后端记录为准。

首次浏览器检查因错误调用`page.innerText()`失败，修正后功能检查通过；视觉复核又发现列表刷新时就截图。两次原记录和素材保留在[浏览器尝试](evidence/cost-free-execution/attempts/)，最终采集新增准确记录可见的等待条件，随后两端重新检查、截图和录屏。原始帧留在忽略目录，公开逐帧摘要与编码记录，不修改原失败证明。

首次汇总自检还发现客户端Web构建与临时Web审阅服务的日志路径同名，原Web构建日志已被覆盖、不可恢复。原检查记录及失败汇总保留在[日志冲突记录](evidence/cost-free-execution/attempts/client-log-collision/note.json)；客户端改用独立日志命名空间后，八条命令全部重新执行，当前摘要匹配新的原始日志。应用源码未因该修正改变；未把原日志缺失记录计作当前成功证明。

原生执行用例使用隔离合成资料及测试控制器／许可，验证实际服务、事务、工具和结果落库流程；没有真实模型调用或OS隔离环境认证。历史数据保留在合成用例中验证；日常开发库旧费用表本来为空，空表升级摘要一致不等同于存量业务数据迁移验收。

## 3. 媒体与原证据

| 客户端 | 截图 | 原速录屏 | GIF |
| --- | --- | --- | --- |
| Web | [PNG](../media/screenshots/secretary-cost-free-web.png) | [MP4](../media/videos/secretary-cost-free-web.mp4) | [GIF](../media/gifs/secretary-cost-free-web.gif) |
| 共享Taro H5 | [PNG](../media/screenshots/secretary-cost-free-mobile.png) | [MP4](../media/videos/secretary-cost-free-mobile.mp4) | [GIF](../media/gifs/secretary-cost-free-mobile.gif) |

新增2张PNG、2段MP4、2段GIF，合成数据、真实浏览器像素、无密码／MFA画面，全部自建浏览器上下文、页面和5189临时服务均关闭。原97 PNG／35 MP4／35 GIF保留Git对象和文件哈希；当前规范媒体总数99 PNG／37 MP4／37 GIF。旧审计按原来源复用，仍80项（13high／60moderate／7low），没有刷新审计源或宣称安全门关闭，见[复用清单](evidence/cost-free-execution/reused-evidence.json)。

## 4. 可复核命令与边界

```powershell
$env:PYTHONUTF8='1'
python app/scripts/verify_cost_free_execution_backend.py
# 仅完整首轮失败时运行；严格保留原证明，仅复跑失败顶层用例：
python app/scripts/retry_cost_free_execution_backend.py
python app/scripts/verify_cost_free_execution_clients.py
python app/scripts/verify_cost_free_execution_development.py
python app/scripts/verify_cost_free_execution_sources.py
python app/scripts/verify_current_scope.py
python app/scripts/verify_repository_publication.py
```

已有证据目录时先归档已有尝试；不得覆盖原结果来伪装重跑。Android与浏览器命令、来源摘要和原日志哈希见各[证据记录](evidence/cost-free-execution/)。原生开发API18080、PG14660及日常通知／报表工作进程保留，外部存储、模型、TG和小程序平台凭据未启用。

真机、网络性能／实网120GB、历史资料迁移／容灾及AI费用控制已移出当前范围。真实Codex、执行隔离、Skill发布／评价、TG、OSS＋R2、平台接入、正式Android发布、持久PG和完整业务验收仍需完成。未使用Docker、WSL、虚拟机或模拟器。

English: All189 current-source backend cases pass with1181 result records; the first cleanup failures and the two serial retries remain visible. Web135/mobile248/runtime51 checks pass. Android has a fresh bundle/APK,18 structural and3 feature-content checks;26 JVM results are cached, with no device-runtime claim. Both actual Web/H5 archive flows and new captures pass with owned resources closed. The complete goal and production acceptance remain open.
'''
    write_document(ROOT / 'docs/quality/100-secretary-execution-verification.md', doc)
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path)
    baseline.update(increment=100, evidence_group='cost-free-execution', verification_script='verify_cost_free_execution_sources.py',
        backend_evidence_group='cost-free-execution-backend', backend_top_level_tests=189, backend_result_records=1181,
        web_evidence_group='cost-free-execution', runtime_evidence_group='cost-free-execution',
        development_verification_script='verify_cost_free_execution_development.py',
        interaction_kind='cost_independent_execution_and_archive', interaction_sources='cost_free_execution_sources.py',
        retained_evidence_record='docs/quality/evidence/cost-free-execution/reused-evidence.json',
        new_browser_capture=True, new_backend_test_execution=True,
        planning_scope_verification_record='docs/quality/evidence/cost-free-execution/scope-check.json',
        planning_publication_record='docs/quality/evidence/cost-free-execution/repository-check.json')
    # This is only the whitespace scope. Raw logs/snapshots/XML/media stay fully
    # selected, privacy-checked and hash-bound without rewriting original bytes.
    changed = subprocess.check_output(['git', 'diff', '--name-only', '-z'], cwd=ROOT).decode().split('\0')
    changed += subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    formats = {'.py', '.go', '.ts', '.tsx', '.mjs', '.json', '.sql', '.md', '.css', '.yaml', '.yml', '.cjs', '.service', '.toml'}
    baseline['publication_files'] = list(dict.fromkeys([*baseline['publication_files'], *[
        n for n in changed if n and (ROOT / n).is_file() and Path(n).suffix in formats]]))
    save(baseline_path, baseline)
    print(json.dumps({'passed': True, 'increment': 100, 'backend': [189, 1181], 'new_apk_payload': True,
        'jvm_new_execution': False, 'whole_gap_packages_closed': 0}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
