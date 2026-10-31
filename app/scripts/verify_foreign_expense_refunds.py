"""Limited post-freeze proof: executed critical checks and separately retained evidence."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import socket
import subprocess
from foreign_expense_refunds_sources import APP, ROOT, CLIENT_SCOPES, sources, digest

E = ROOT / 'docs/quality/evidence/foreign-expense-refunds'
BASE = '1dd066a5419c73c82f81403d0d7f840b87040433'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def port_closed(port):
    with socket.socket() as s:
        s.settimeout(.2)
        return s.connect_ex(('127.0.0.1', port)) != 0


def verify(save=False):
    checks, records = [], []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    def record(file):
        records.append({'file': file.relative_to(ROOT).as_posix(), 'sha256': sha(file)})
        return read(file)

    backend = record(E / 'backend.json')
    backend_scopes = ['backend', 'scripts/test_foreign_expense_refunds_native.py', 'scripts/contract_body_native_cluster.py']
    names = ['app/' + n for n in sources(backend_scopes)]
    check('Five executed native HTTP/PG suites pass all64 selected records without skips',
        backend['passed'] and backend['source_unchanged'] and backend['exit_code'] == 0
        and backend['top_level_suites'] == 5 and backend['passing_records'] == 64
        and not backend['failures'] and not backend['skips'])
    check('Backend execution retains exact complete listed Go/SQL/API and native harness inputs',
        names == [r['file'] for r in backend['input_files']]
        and all(sha(ROOT / r['file']) == r['sha256'] for r in backend['input_files'])
        and hashlib.sha256(json.dumps(backend['input_files'], sort_keys=True, separators=(',', ':')).encode()).hexdigest() == backend['source_sha256'])
    binding = record(E / 'client-source-bindings.json')
    check('Generated reporter cache exclusion retains original execution hashes and unchanged application inputs',
        binding['passed'] and not binding['application_sources_changed'] and not binding['commands_reexecuted']
        and binding['projected_source_files'] == sources(CLIENT_SCOPES)
        and binding['projected_source_sha256'] == binding['independent_no_cache_typecheck_hash'] == digest(sources(CLIENT_SCOPES)))
    for name in ('typecheck', 'web-tests', 'mobile-tests', 'web-build', 'weapp-build', 'douyin-build'):
        r = record(E / (name + '.json'))
        bound = next(v for v in binding['records'] if v['record'] == name + '.json')
        check('Executed current client check ' + name,
            r['passed'] and r['exit_code'] == 0 and r['source_unchanged']
            and bound['record_sha256'] == sha(E / (name + '.json'))
            and bound['original_source_reconstructed_before_exclusion']
            and bound['execution_source_sha256'] == r['source_sha256'] == r['source_after_sha256']
            and bound['projected_source_sha256'] == binding['projected_source_sha256']
            and all(n == 'web/.vitest/json/output.json' for n in bound['excluded_generated_files'])
            and [n for n in r['source_files'] if n not in bound['excluded_generated_files']] == binding['projected_source_files'])
    web, mobile = read(E / 'web-tests.json'), read(E / 'mobile-tests.json')
    check('Actual436 Web and521 shared mobile cases include eight new foreign-refund cases each',
        web['tests'] == {'numPassedTests': 436, 'numFailedTests': 0, 'numPendingTests': 0}
        and mobile['tests'] == {'pass': 521, 'fail': 0, 'skip': 0})
    native = record(E / 'native-cluster-closed.json')
    check('The owned native fully durable test PostgreSQL was closed',
        native['passed'] and not (APP / '.local/foreign-expense-refunds-native-cluster.json').exists())
    group = ROOT / 'docs/quality/evidence/foreign-expense-refunds-web'
    browser, manifest = record(group / 'result.json'), record(group / 'source-manifest.json')
    check('Actual human Web flow re-reads committed three-currency inverse and releases all owned resources',
        browser['passed'] and browser['source_unchanged'] and browser['build_source_unchanged']
        and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == digest(manifest['files'])
        and browser['build_source_sha256'] == browser['build_source_after_sha256']
        and browser['owned_browser_closed'] and browser['owned_review_server_stopped']
        and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code'] == 0
        and not (APP / '.local/business-browser-fixture.json').exists() and port_closed(5189))
    media, visual = record(E / 'media.json'), record(E / 'visual-review.json')
    check('Three inspected actual screenshots, one MP4 and one labelled screenshot-tour GIF retain their bytes',
        media['passed'] and len(media['artifacts']) == 5
        and all(sha(ROOT / r['file']) == r['sha256'] for r in media['artifacts'])
        and visual['passed'] and all(r['reviewed'] and sha(ROOT / r['file']) == r['sha256'] for r in visual['artifacts'])
        and {r['file'] for r in visual['artifacts']} == {r['file'] for r in media['artifacts']})
    old_paths = subprocess.check_output(['git', 'diff', '--name-only', BASE, '--', 'docs/quality/evidence', 'docs/media'], cwd=ROOT).decode('utf-8').splitlines()
    allowed = ('docs/quality/evidence/foreign-expense-refunds/', 'docs/quality/evidence/foreign-expense-refunds-web/',
        'docs/media/screenshots/foreign-expense-refunds-', 'docs/media/videos/foreign-expense-refunds-', 'docs/media/gifs/foreign-expense-refunds-')
    unchanged = all(n == 'docs/media/README.md' or n.startswith(allowed) for n in old_paths)
    old_migrations = subprocess.check_output(['git', 'diff', '--name-only', BASE, '--', 'app/backend/migrations'], cwd=ROOT).decode('utf-8').splitlines()
    check('Published historical proof/media and migrations1–100 remain unchanged', unchanged
        and all(n == 'app/backend/migrations/0101_foreign_refund_expense_attribution.sql' for n in old_migrations))
    tag = subprocess.check_output(['git', 'rev-parse', 'v0.1.0-rc.1^{}'], cwd=ROOT).decode().strip()
    check('First freeze remains at its exact published source; this is post-freeze development', tag == 'a95c6afbd0e76684a64dc641c6abe598ad625277')
    check('Eight Skills remain draft; AI fees stay excluded and external acceptance stays open',
        len(list((APP / 'skills').glob('*/manifest.json'))) == 8
        and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')))
    retained = {'passed': unchanged, 'base_commit': BASE, 'full_backend_increment': 121,
        'runtime_increment': 121, 'android_artifact_increment': 133,
        'freeze_tag': 'v0.1.0-rc.1', 'current_android_apk_built': False,
        'previous_proof_and_media_reexecuted': False}
    result = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'increment': 134, 'verification_mode': 'scoped_increment', 'checks': checks, 'records': records, 'retained': retained,
        'interaction_kind': 'foreign-expense-refunds', 'interaction_source_sha256': digest(),
        'verification_script_sha256': sha(Path(__file__)), 'fixture_only_adjustment': None,
        'inspection_only_adjustment': {'kind': 'generated_report_exclusion_only',
            'binding_record_sha256': sha(E / 'client-source-bindings.json'),
            'application_sources_changed': False, 'commands_reexecuted': False,
            'scope': 'Original client and browser startup execution hashes remain original. Only generated Vitest output is excluded from the current client input projection. Browser action manifest still matches current backend/Web/contracts/capture sources exactly; verification-only checker changes are outside that executed action manifest.'},
        'full_backend_suite_executed': False, 'formal_android_acceptance': False,
        'current_android_apk_built': False, 'current_development_database_schema': 100,
        'skipped_checks': ['Full backend/runtime regression', 'Additional mobile browser journey',
            'New Android JS/APK/JVM build', 'Persistent development database100-to101 upgrade',
            'Real platform/cloud/model/TG/Linux/production acceptance'],
        'skip_authority': 'User explicitly requested a first frozen release with selected validation deferred on2026-10-31; retain that fast focused workflow for this post-freeze increment.',
        'full_goal_complete': False, 'production_ready': False}
    if save:
        (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    return result


if __name__ == '__main__':
    result = verify(save=True)
    print(json.dumps({'passed': result['passed'], 'checks': len(result['checks']), 'failed': [r['name'] for r in result['checks'] if not r['passed']]}))
    raise SystemExit(0 if result['passed'] else 1)
