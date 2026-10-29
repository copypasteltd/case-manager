"""Check applicability against the original IDs and current user scope.

This verifies document consistency. It never executes or passes business cases.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / 'docs/quality/04-current-acceptance-applicability.json'
DOCUMENT = MANIFEST.with_suffix('.md')
SCOPE = ROOT / 'docs/delivery/04-current-scope.json'
CATEGORY_FLAGS = {
    'real_device': 'real_device_verification_required',
    'network_performance': 'network_performance_tests_required',
    'live_120gb': 'live_120gb_network_acceptance_required',
    'historical_migration': 'historical_data_migration_required',
    'disaster_recovery': 'disaster_recovery_required',
    'ai_cost': 'ai_cost_control_required',
}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def original_rows(path):
    result = {}
    for line in path.read_text(encoding='utf-8').splitlines():
        if re.match(r'^\| (?:AC-\d+|TC-[A-Z]+-\d+) \|', line):
            cells = [cell.strip() for cell in line.split('|')[1:-1]]
            assert cells[0] not in result, 'Duplicate original case ID.'
            result[cells[0]] = {'columns': cells, 'row_sha256': hashlib.sha256(line.encode()).hexdigest()}
    return result


def scope_digest(scope):
    relevant = {key: scope[key] for key in ('revision', 'flags', 'required_clients')}
    return hashlib.sha256(json.dumps(relevant, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def render(data):
    labels = {'required': '保留', 'partial': '部分保留', 'excluded': '范围移出'}
    lines = [
        '# 当前验收适用性 / Current acceptance applicability', '',
        '[当前范围](../delivery/04-current-scope-and-acceptance.md) · [原始AC](../06-decisions-risks-and-acceptance.md) · [原始TC](01-test-strategy-and-cases.md) · [机器清单](04-current-acceptance-applicability.json)', '',
        '按用户修订99逐项分类全部30项AC和72项TC，原编号、原文及其摘要保留。此表只界定当前要求，没有将任何业务用例标记为通过；保留或部分保留项均待完整验收，范围移出项不计为测试通过。', '',
        'Only AI cost control is excluded: model pricing, monthly token or money quotas, fee reservations and provider invoice reconciliation. The secretary, eight Skills, TG, authorization, audit, isolation and technical context/output, concurrency, runtime and temporary-storage limits remain required. Company OA, subscriptions, project budgets and finance remain required.', '',
        '不要求实体设备、模拟器、网络性能／实网120GB专项、2TB历史迁移及容灾。保留四端功能、正式签名／平台发布配置、真实云／模型／TG接入、目标Linux隔离、生产PG持久化和结构升级、OSS＋R2普通复制及文件版本恢复。两会话并发不要求两台实体设备。', '',
        '| 类型 | 保留 | 部分保留 | 范围移出 | 合计 |',
        '|---|---:|---:|---:|---:|',
    ]
    for kind in ('AC', 'TC'):
        counts = data['counts'][kind]
        lines.append('| ' + kind + ' | ' + ' | '.join(str(counts[key]) for key in ('required', 'partial', 'excluded', 'total')) + ' |')
    lines += ['', '逐项证据状态仍记录在各增量验证、发布记录和GAP-28中；本表不替代这些结果。原始历史材料中的真机、容灾或费用表述，以本表及修订99为当前适用边界。', '']
    for kind in ('AC', 'TC'):
        lines += ['## ' + kind, '', '| 编号 | 当前范围 | 原场景 | 当前保留要求 | 移出部分 |', '|---|---|---|---|---|']
        for item in data['items']:
            if item['kind'] != kind:
                continue
            cells = item['original_columns']
            scenario = cells[2] if kind == 'AC' else cells[1]
            values = [item['id'], labels[item['applicability']], scenario, item['current_requirement'], item['removed_requirement'] or '—']
            lines.append('| ' + ' | '.join(value.replace('|', '／').replace('\n', ' ') for value in values) + ' |')
        lines.append('')
    lines += ['核对命令（仓库根目录）：python app/scripts/verify_acceptance_applicability.py。该命令只检查原编号／原文、范围标志、分类、数量和文档一致性。', '']
    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--render', action='store_true', help='Regenerate the Markdown from the reviewed manifest.')
    args = parser.parse_args()
    data = json.loads(MANIFEST.read_text(encoding='utf-8'))
    scope = json.loads(SCOPE.read_text(encoding='utf-8'))
    if args.render:
        DOCUMENT.write_text(render(data), encoding='utf-8')
    checks = []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    check('Current authority and four-client scope agree', data['scope_revision'] == scope['revision'] == 99
          and data['scope_flags_sha256'] == scope_digest(scope)
          and scope['required_clients'] == ['Web', 'WeChat', 'Douyin', 'Android'])
    original = {}
    for source in data['original_sources']:
        path = ROOT / source['path']
        rows = original_rows(path)
        check('Original source and ID count: ' + source['kind'], sha(path) == source['sha256'] and len(rows) == source['row_count'])
        assert not original.keys() & rows.keys()
        original.update(rows)
    items = data['items']
    check('Every original ID appears exactly once', len(items) == len(original) == 102
          and len({item['id'] for item in items}) == len(items)
          and {item['id'] for item in items} == set(original))
    check('Original wording and row hashes remain exact', all(item['original_columns'] == original[item['id']]['columns']
          and item['original_row_sha256'] == original[item['id']]['row_sha256'] for item in items))
    check('Classification never invents a passed case', all(item['applicability'] in ('required', 'partial', 'excluded')
          and item['acceptance_state'] == ('scope_excluded' if item['applicability'] == 'excluded' else 'acceptance_pending')
          for item in items) and data['business_cases_passed_by_this_document'] == 0
          and not data['full_goal_complete'] and not data['production_ready'])
    check('Removed categories are explicitly excluded user requirements', all(
          category in CATEGORY_FLAGS and scope['flags'][CATEGORY_FLAGS[category]] is False
          for item in items for category in item['removed_categories']))
    check('Partial and excluded requirements have an explicit boundary', all(
          bool(item['removed_categories']) == (item['applicability'] != 'required')
          and bool(item['removed_requirement']) == (item['applicability'] != 'required')
          and bool(item['current_requirement']) for item in items))
    check('AI authorization, isolation and company finance remain mandatory', all(scope['flags'][key] for key in (
          'ai_secretary_required', 'skills_required', 'telegram_agent_required', 'ai_execution_isolation_required',
          'ai_authorization_required', 'business_finance_required', 'oss_r2_storage_required',
          'production_database_persistence_required', 'schema_upgrade_required', 'functional_upload_recovery_required')))
    computed = {kind: {status: sum(item['kind'] == kind and item['applicability'] == status for item in items)
                for status in ('required', 'partial', 'excluded')} for kind in ('AC', 'TC')}
    for counts in computed.values():
        counts['total'] = sum(counts.values())
    check('Counts derive from every reviewed item', computed == data['counts'])
    check('Markdown exactly matches the reviewed manifest', DOCUMENT.read_text(encoding='utf-8') == render(data))
    check('Classification does not close work packages', scope['counts']['active'] == 23
          and scope['counts']['active_p0'] == 14 and scope['counts']['active_p1'] == 9
          and next(item for item in scope['items'] if item['id'] == 'GAP-28')['status'] == 'active')
    record = {'passed': all(item['passed'] for item in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'checks': checks, 'manifest_sha256': sha(MANIFEST), 'document_sha256': sha(DOCUMENT),
              'authority_scope_sha256': sha(SCOPE), 'verification_script_sha256': sha(Path(__file__)),
              'counts': data['counts'], 'business_cases_executed': 0, 'business_cases_passed': 0,
              'scope': 'Source-bound applicability consistency only; no business or production acceptance.'}
    target = ROOT / 'docs/quality/evidence/dependency-toolchain/acceptance-applicability-check.json'
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': record['passed'], 'checks': len(checks), 'counts': data['counts'],
                      'business_cases_passed': 0, 'failed': [row['name'] for row in checks if not row['passed']]}))
    return 0 if record['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
