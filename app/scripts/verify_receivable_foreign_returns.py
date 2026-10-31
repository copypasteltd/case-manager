"""Verify executed focused checks, exact sources and retained frozen artifacts."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import socket
import subprocess
from receivable_foreign_returns_sources import APP, ROOT, CLIENT_SCOPES, sources, digest

E = ROOT / 'docs/quality/evidence/receivable-foreign-returns'
BASE = '16554e916d7dc8f4a19f3864828a4fe6a79b7fa1'
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
    scopes = ['backend', 'scripts/test_receivable_foreign_returns_native.py', 'scripts/contract_body_native_cluster.py']
    check('Four native HTTP/PG suites actually pass without skips, including new dual-unit return journeys',
        backend['passed'] and backend['exit_code'] == 0 and backend['top_level_suites'] == 4
        and backend['passing_records'] >= 50 and not backend['failures'] and not backend['skips']
        and any(e['name'].startswith('TestReceivableForeignReturns/') for e in backend['events']))
    check('Complete backend and native harness inputs still match actual execution bytes',
        ['app/' + n for n in sources(scopes)] == [r['file'] for r in backend['input_files']]
        and backend['source_unchanged'] and all(sha(ROOT / r['file']) == r['sha256'] for r in backend['input_files'])
        and hashlib.sha256(json.dumps(backend['input_files'], sort_keys=True, separators=(',', ':')).encode()).hexdigest() == backend['source_sha256'])
    for name in ('typecheck', 'web-tests', 'mobile-tests', 'web-build', 'weapp-build', 'douyin-build'):
        r = record(E / (name + '.json'))
        check('Current client execution and exact source binding: ' + name,
            r['passed'] and r['exit_code'] == 0 and r['source_unchanged']
            and r['source_files'] == sources(CLIENT_SCOPES)
            and r['source_sha256'] == r['source_after_sha256'] == digest(r['source_files']))
    web, mobile = read(E / 'web-tests.json'), read(E / 'mobile-tests.json')
    check('Eight new shared return cases pass in both actual client consumers',
        web['tests'] == {'numPassedTests': 444, 'numFailedTests': 0, 'numPendingTests': 0}
        and mobile['tests'] == {'pass': 529, 'fail': 0, 'skip': 0})
    native = record(E / 'native-cluster-closed.json')
    check('Owned durable native PostgreSQL closes without altering persistent development services',
        native['passed'] and not (APP / '.local/receivable-foreign-returns-native-cluster.json').exists())
    group = E.parent / 'receivable-foreign-returns-web'
    browser, manifest, business = record(group / 'result.json'), record(group / 'source-manifest.json'), record(group / 'business-proof.json')
    check('Actual authenticated Web mutation, durable readback and owned-resource cleanup pass',
        browser['passed'] and browser['source_unchanged'] and browser['build_source_unchanged']
        and browser['source_sha256'] == browser['source_after_sha256'] == manifest['sha256'] == digest(manifest['files'])
        and browser['owned_browser_closed'] and browser['owned_review_server_stopped']
        and browser['isolated_database_cleanup_passed'] and browser['native_fixture_exit_code'] == 0
        and not (APP / '.local/business-browser-fixture.json').exists() and port_closed(5189)
        and business['principal'] == '195.000000' and business['attributed_cash'] == '25.000000'
        and business['reserved_cash'] == '75.000000' and business['unallocated_cash'] == '25.000000'
        and business['durable_readback'] and not business['bank_transfer_initiated'])
    media, visual = record(E / 'media.json'), record(E / 'visual-review.json')
    check('Inspected actual Web screenshots, post-authentication MP4 and screenshot-tour GIF retain exact bytes',
        media['passed'] and len(media['artifacts']) == 5
        and all(sha(ROOT / r['file']) == r['sha256'] for r in media['artifacts'])
        and visual['passed'] and all(r['reviewed'] and sha(ROOT / r['file']) == r['sha256'] for r in visual['artifacts'])
        and {r['file'] for r in visual['artifacts']} == {r['file'] for r in media['artifacts']})
    changed = subprocess.check_output(['git', 'diff', '--name-only', BASE, '--', 'docs/quality/evidence', 'docs/media'], cwd=ROOT).decode('utf-8').splitlines()
    allowed = ('docs/quality/evidence/receivable-foreign-returns/', 'docs/quality/evidence/receivable-foreign-returns-web/',
        'docs/media/screenshots/receivable-foreign-returns-', 'docs/media/videos/receivable-foreign-returns-', 'docs/media/gifs/receivable-foreign-returns-')
    unchanged = all(n == 'docs/media/README.md' or n.startswith(allowed) for n in changed)
    migrations = subprocess.check_output(['git', 'diff', '--name-only', BASE, '--', 'app/backend/migrations'], cwd=ROOT).decode('utf-8').splitlines()
    check('Published historical proof/media and migrations1–101 remain immutable', unchanged
        and all(n == 'app/backend/migrations/0102_receivable_foreign_returns.sql' for n in migrations))
    check('Original first-freeze tag and artifact branch remain pinned',
        subprocess.check_output(['git', 'rev-parse', 'v0.1.0-rc.1^{}'], cwd=ROOT).decode().strip() == 'a95c6afbd0e76684a64dc641c6abe598ad625277'
        and subprocess.check_output(['git', 'rev-parse', 'codex/release-v0.1.0-rc.1'], cwd=ROOT).decode().strip() == '8ddb3f7a98a24910af49ede2a6c8b8c744d50973')
    check('All eight Skills remain draft and no formal model/platform/cloud acceptance is claimed',
        len(list((APP / 'skills').glob('*/manifest.json'))) == 8
        and all(read(p)['state'] == 'draft' for p in (APP / 'skills').glob('*/manifest.json')))
    retained = {'passed': unchanged, 'base_commit': BASE, 'full_backend_increment': 121,
        'runtime_increment': 121, 'android_artifact_increment': 133, 'freeze_tag': 'v0.1.0-rc.1',
        'current_android_apk_built': False, 'previous_proof_and_media_reexecuted': False}
    result = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'increment': 135, 'verification_mode': 'scoped_increment', 'checks': checks, 'records': records, 'retained': retained,
        'interaction_kind': 'receivable-foreign-returns', 'interaction_source_sha256': digest(),
        'verification_script_sha256': sha(Path(__file__)), 'fixture_only_adjustment': None, 'inspection_only_adjustment': None,
        'full_backend_suite_executed': False, 'formal_android_acceptance': False, 'current_android_apk_built': False,
        'current_development_database_schema': 100, 'schema_migration': 102,
        'skipped_checks': ['Full backend/runtime regression', 'Additional mobile browser journey', 'New Android JS/JVM/APK build',
            'Persistent development database100-to102 upgrade', 'Actual platform/cloud/model/TG/Linux/production acceptance'],
        'skip_authority': 'Explicit user instruction on2026-10-31 to preserve the main functional loop and defer selected validation for faster first release; same focused post-freeze workflow.',
        'full_goal_complete': False, 'production_ready': False}
    if save:
        (E / 'final-source-check.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    return result


if __name__ == '__main__':
    result = verify(save=True)
    print(json.dumps({'passed': result['passed'], 'checks': len(result['checks']), 'failed': [r['name'] for r in result['checks'] if not r['passed']]}))
    raise SystemExit(0 if result['passed'] else 1)
