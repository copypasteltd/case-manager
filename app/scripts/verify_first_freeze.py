"""Reduced first-freeze checks authorized by the user; skipped cases never pass."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import socket
import subprocess
import verify_scoped_increment as scoped

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/contract-refunds'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def digest():
    h = hashlib.sha256(scoped.digest(config=scoped.plan('docs/quality/evidence/contract-refunds/verification-plan.json')).encode())
    for name in ('verify_first_freeze.py', 'verify_repository_publication.py'):
        h.update(name.encode() + b'\0' + (APP / 'scripts' / name).read_bytes())
    return h.hexdigest()


def verify(save=False):
    checks, records = [], []
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    def record(path):
        records.append({'file': path.relative_to(ROOT).as_posix(), 'sha256': sha(path)})
        return read(path)
    before_dir = E / 'attempts/05-development-trigger-expectation'
    old = read(before_dir / 'verification-plan.json.before')
    check('The original business commands, selection and deadlines are unchanged', old == scoped.plan('docs/quality/evidence/contract-refunds/verification-plan.json'))
    inspector = (before_dir / 'verify_project_business_development.py.before').read_bytes()
    nl = b'\r\n' if b'\r\n' in inspector else b'\n'
    anchor = b'        query = query.replace("\'schema99\',(SELECT count(*)=99", "\'schema100\',(SELECT count(*)=100")' + nl
    addition = b'        query = query.replace("\'refund_quote_guards\',(SELECT count(*)=3 FROM pg_trigger WHERE tgname IN (\'payment_refund_quote\',\'payment_foreign_refund_no_attribution\',\'contract_foreign_refund_no_attribution\') AND tgenabled=\'O\')", "\'refund_quote_guards\',(SELECT count(*)=2 FROM pg_trigger WHERE tgname IN (\'payment_refund_quote\',\'payment_foreign_refund_no_attribution\') AND tgenabled=\'O\') AND NOT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=\'contract_finance_events\'::regclass AND tgname=\'contract_foreign_refund_no_attribution\'),\'contract_principal_guard\',EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=\'contract_finance_events\'::regclass AND p.proname=\'prepare_contract_finance_event\' AND t.tgenabled=\'O\' AND NOT t.tgisinternal)")\n'.replace(b'\n', nl)
    check('Only the disclosed schema100 metadata expectation changed in the inspector', inspector.count(anchor) == 1 and (APP / 'scripts/verify_project_business_development.py').read_bytes() == inspector.replace(anchor, anchor + addition))
    checker_path = E / 'attempts/06-mobile-history-selector/check_contract_refunds.mjs.before'
    checker = checker_path.read_bytes()
    needle = ".filter({ has: root().getByText('归属实际退回款 · 记录 5', { exact: true }) })".encode()
    replacement = ".filter({ hasText: '归属实际退回款 · 记录 5' })".encode()
    check('Only the disclosed relative mobile history selector changed', checker.count(needle) == 1 and (APP / 'scripts/check_contract_refunds.mjs').read_bytes() == checker.replace(needle, replacement))
    original = scoped.digest(config=old, substitutions={'scripts/verify_project_business_development.py': inspector, 'scripts/check_contract_refunds.mjs': checker})
    ids = ('backend', 'web-tests', 'mobile-tests', 'typecheck', 'web-build', 'weapp-build', 'douyin-build', 'native-js-tests')
    for name in ids:
        value = record(E / (name + '.json'))
        expected = next(c for c in old['commands'] if c['id'] == name)
        check('Executed critical check ' + name, value['passed'] and value['exit_code'] == 0 and value['source_unchanged'] and value['source_sha256'] == value['source_after_sha256'] == original and value['command'] == expected['command'])
    backend = read(E / 'backend.json')
    check('Five actual native HTTP/PG suites cover contract cash, refund caps, review, release and reversal', len(backend['tests']['top_level_tests']) == 5 and backend['tests']['pass'] == 74 and backend['tests']['fail'] == backend['tests']['skip'] == 0)
    check('Complete Web/mobile units retain their actual428/513 passing counts', read(E / 'web-tests.json')['tests']['numPassedTests'] == 428 and read(E / 'mobile-tests.json')['tests']['pass'] == 513)
    update, dev = record(E / 'development-update.json'), record(E / 'development-project-resources.json')
    check('Normal99-to100 upgrade preserves original facts and owned native services', update['passed'] and update['legacy_and_usage_rows_preserved'] and dev['passed'] and dev['migration_count'] == 100 and dev['integrations_disabled'] and all(dev['ownership'].values()))
    web_dir = E / 'attempts/06-mobile-history-selector/contract-refunds-web'
    browser, manifest = record(web_dir / 'result.json'), record(web_dir / 'source-manifest.json')
    browser_digest = scoped.digest(names=manifest['files'], config=old, substitutions={'scripts/check_contract_refunds.mjs': checker})
    check('Retained actual Web release flow binds unchanged application source and closed resources', browser['passed'] and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == browser_digest and browser['owned_browser_closed'] and browser['owned_review_server_stopped'] and browser['isolated_database_cleanup_passed'])
    media = record(E / 'media.json')
    visual = record(E / 'visual-review.json')
    check('Published actual media retains inspected bytes', media['passed'] and all(sha(ROOT / v['file']) == v['sha256'] for v in media['artifacts']) and visual['passed'] and all(v['reviewed'] and sha(ROOT / v['file']) == v['sha256'] for v in visual['artifacts']) and {v['file'] for v in visual['artifacts']} == {v['file'] for v in media['artifacts']})
    check('Owned browser fixture and review ports are released', not (APP / '.local/business-browser-fixture.json').exists() and all(_port_closed(p) for p in (5189, 5190)))
    previous = scoped.retained(old)
    check('Previously published proof and media remain byte-identical', previous['passed'])
    check('Eight Skills stay draft; production and external acceptance are open', len(list((APP / 'skills').glob('*/manifest.json'))) == 8 and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')))
    adjustment = {'kind':'exact_check_only_corrections', 'execution_source_sha256': original,
        'application_build_and_test_inputs_unchanged':True,
        'scope':'Original results retain original hashes. Only the schema100 inspection expectation and a mobile relative selector changed; corrected metadata was rerun. The failed mobile UI check is retained and its rerun is deferred by the user freeze instruction.'}
    result = {'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),
        'verification_mode':'frozen_release','increment':133,'checks':checks,'records':records,'retained':previous,
        'interaction_kind':'contract-refunds','interaction_source_sha256':digest(),
        'verification_script_sha256':sha(Path(__file__)), 'fixture_only_adjustment':None,'inspection_only_adjustment':adjustment,
        'skipped_checks':['Expanded browser rerun after check-only selector correction','Full backend/runtime regression rerun','Additional native/JVM regression beyond completed build outcomes','Real platform/cloud/model/TG/production acceptance'],
        'skip_authority':'User explicitly requested a first frozen release with selected validation deferred on2026-10-31.',
        'full_backend_suite_executed':False,'formal_android_acceptance':False,'full_goal_complete':False,'production_ready':False}
    if save:
        (E / 'final-source-check.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    return result


def _port_closed(port):
    with socket.socket() as s:
        s.settimeout(.2)
        return s.connect_ex(('127.0.0.1',port)) != 0


if __name__ == '__main__':
    result = verify(save=True)
    print(json.dumps({'passed':result['passed'],'checks':len(result['checks']),'failed':[v['name'] for v in result['checks'] if not v['passed']],'skipped':result['skipped_checks']}))
    raise SystemExit(0 if result['passed'] else 1)
