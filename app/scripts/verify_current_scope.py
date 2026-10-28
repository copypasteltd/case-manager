"""Verify the user's current scope, traceable exclusions and honest retained evidence."""
from collections import Counter
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/scope-revision-99'
MANIFEST = ROOT / 'docs/delivery/04-current-scope.json'
DOCUMENTS = ['AGENTS.md', 'README.md', 'docs/README.zh-CN.md', 'docs/README.en.md', 'docs/README.md',
    'docs/CHANGELOG.md', 'docs/media/README.md', 'docs/delivery/02-implementation-ledger.md',
    'docs/delivery/03-goal-gap-assessment.md', 'docs/delivery/04-current-scope-and-acceptance.md',
    'docs/delivery/04-current-scope.json', 'docs/requirements/01-product-and-journeys.md',
    'docs/quality/02-traceability-and-release.md', 'docs/quality/99-scope-revision-verification.md',
    'docs/governance/02-decisions-and-open-questions.md', 'docs/operations/01-deployment-and-operations.md',
    'docs/operations/02-backup-migration-and-recovery.md', 'docs/development/86-secretary-token-budgets.md',
    'app/scripts/verify_current_scope.py', 'app/scripts/verify_repository_publication.py']


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    scope = read(MANIFEST)
    rows = scope['items']
    active = [row for row in rows if row['status'] == 'active']
    excluded = [row for row in rows if row['status'] == 'excluded']
    check('All28 stable IDs remain unique and traceable', len(rows) == 28
        and {row['id'] for row in rows} == {f'GAP-{n:02d}' for n in range(1, 29)})
    expected = {'registered': 28, 'active': 23, 'excluded': 5, 'active_p0': 14, 'active_p1': 9, 'newly_completed': 0}
    check('Current scope counts are23 active/5 excluded and14 P0/9 P1, not new completions',
        scope['counts'] == expected and len(active) == 23 and len(excluded) == 5
        and Counter(row['priority'] for row in active) == {'P0': 14, 'P1': 9})
    check('Only14/17/21/22/27 are excluded',
        {row['id'] for row in excluded} == {'GAP-14', 'GAP-17', 'GAP-21', 'GAP-22', 'GAP-27'})
    false_flags = ['real_device_verification_required', 'network_performance_tests_required',
        'live_120gb_network_acceptance_required', 'historical_data_migration_required',
        'disaster_recovery_required', 'ai_cost_control_required']
    true_flags = ['ai_secretary_required', 'skills_required', 'telegram_agent_required',
        'ai_execution_isolation_required', 'ai_authorization_required', 'business_finance_required',
        'oss_r2_storage_required', 'production_database_persistence_required',
        'schema_upgrade_required', 'functional_upload_recovery_required']
    check('The four exclusions follow the corrected AI-cost-only instruction',
        all(scope['flags'][name] is False for name in false_flags))
    check('Secretary/Skills/TG, isolation/authorization, business finance, storage and persistence remain',
        all(scope['flags'][name] is True for name in true_flags)
        and scope['required_clients'] == ['Web', 'WeChat', 'Douyin', 'Android'])
    gap = (ROOT / 'docs/delivery/03-goal-gap-assessment.md').read_text(encoding='utf-8-sig')
    active_text = gap.split('## 2. 当前待完成项', 1)[1].split('## 3.', 1)[0]
    excluded_text = gap.split('## 3. 移出本轮的原编号', 1)[1].split('## 4.', 1)[0]
    ids = lambda text: re.findall(r'^\| (GAP-\d{2}) \|', text, re.M)
    check('The visible active/excluded tables match the complete structured remaining work',
        ids(active_text) == [row['id'] for row in active]
        and ids(excluded_text) == [row['id'] for row in excluded]
        and all(row['remaining'] in gap and row['acceptance'] in active_text for row in active))
    budget_source = APP / 'backend/internal/agent/budget.go'
    gate_present = 'AGENT_BUDGET_UNCONFIGURED' in budget_source.read_text(encoding='utf-8')
    check('Existing mandatory cost admission is honestly recorded as pending GAP-09 adaptation',
        not gate_present or 'AGENT_BUDGET_UNCONFIGURED' in next(row['remaining'] for row in active if row['id'] == 'GAP-09'))
    before = read(E / 'publication-before.json')
    check('All17 prior publication snapshots retain exact original Git bytes',
        len(before['files']) == 17 and before['published_commit'] == scope['functional_baseline_commit']
        and all(sha(ROOT / row['snapshot']) == row['sha256']
            and (ROOT / row['snapshot']).read_bytes() == subprocess.check_output(['git', 'show', before['published_commit'] + ':' + row['file']], cwd=ROOT)
            for row in before['files']))
    baseline = read(ROOT / 'docs/quality/current-functional-baseline.json')
    proof = read(ROOT / 'docs/quality/evidence' / baseline['evidence_group'] / 'final-source-check.json')
    spec = importlib.util.spec_from_file_location('current_functional_scope', APP / 'scripts' / baseline['interaction_sources'])
    source = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(source)
    check('Current functional evidence still matches exact application inputs without a new execution claim',
        proof['passed'] and source.digest() == proof['interaction_source_sha256']
        and not proof['full_goal_complete'] and baseline['increment'] >= scope['functional_baseline_increment'])
    check('Current functional and planning baselines are explicitly separated',
        baseline['planning_scope_revision'] == 99 and baseline['planning_scope_manifest'] == MANIFEST.relative_to(ROOT).as_posix()
        and baseline['planning_scope_verification_script'] == 'verify_current_scope.py'
        and not baseline['full_goal_complete'] and not baseline['production_ready'])
    media = []
    for line in subprocess.check_output(['git', 'ls-tree', '-r', before['published_commit'], '--', 'docs/media'], cwd=ROOT).decode().splitlines():
        meta, name = line.split('\t', 1)
        if Path(name).suffix.lower() in ('.png', '.mp4', '.gif'):
            media.append((name, meta.split()[2]))
    actual = subprocess.check_output(['git', 'hash-object', '--', *[name for name, blob in media]], cwd=ROOT).decode().splitlines()
    check('Original97 PNG/35 MP4/35 GIF retain their published Git objects; no new capture',
        Counter(Path(name).suffix for name, blob in media) == {'.png': 97, '.mp4': 35, '.gif': 35}
        and actual == [blob for name, blob in media])
    policy_docs = ['AGENTS.md', 'README.md', 'docs/README.zh-CN.md', 'docs/README.en.md',
        'docs/requirements/01-product-and-journeys.md', 'docs/quality/02-traceability-and-release.md',
        'docs/governance/02-decisions-and-open-questions.md', 'docs/operations/01-deployment-and-operations.md',
        'docs/operations/02-backup-migration-and-recovery.md', 'docs/development/86-secretary-token-budgets.md']
    check('Working agreement, both guides and affected requirement/quality/operations documents point to the current scope',
        all('04-current-scope' in (ROOT / name).read_text(encoding='utf-8-sig') for name in policy_docs))
    report = {'passed': all(row['passed'] for row in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'scope_revision': 99, 'counts': scope['counts'], 'checks': checks, 'scope_manifest': MANIFEST.relative_to(ROOT).as_posix(),
        'scope_manifest_sha256': sha(MANIFEST), 'verification_script_sha256': sha(Path(__file__)),
        'input_files': [{'file': name, 'sha256': sha(ROOT / name)} for name in DOCUMENTS],
        'functional_evidence_group': baseline['evidence_group'], 'functional_baseline_increment': baseline['increment'],
        'mandatory_budget_gate_still_present': gate_present, 'new_functional_test_execution': False,
        'new_browser_capture': False, 'real_device_verification': False, 'virtualization_used': False,
        'runtime_behavior_changed': False, 'full_goal_complete': False, 'production_ready': False}
    E.mkdir(parents=True, exist_ok=True)
    (E / 'scope-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'counts': report['counts'],
        'failed': [row['name'] for row in checks if not row['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
