"""Preflight tracked/candidate publication bytes, local links, real media and retained source proofs."""
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import io
import json
from pathlib import Path, PurePosixPath
import re
import socket
import subprocess
from urllib.parse import unquote, urlsplit

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
EVIDENCE = ROOT / 'docs/quality/evidence/repository-showcase'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def module(filename):
    spec = importlib.util.spec_from_file_location('publication_' + filename.replace('.', '_'), APP / 'scripts' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args])


def object_bytes(names, prefix=':'):
    requests = ''.join(prefix + name + '\n' for name in names).encode('utf-8')
    raw = subprocess.run(['git', '-C', str(ROOT), 'cat-file', '--batch'], input=requests,
                         capture_output=True, check=True).stdout
    stream = io.BytesIO(raw)
    result = {}
    for name in names:
        header = stream.readline().split()
        if len(header) != 3 or header[1] != b'blob':
            raise ValueError('Expected an ordinary staged blob: ' + name)
        size = int(header[2])
        result[name] = stream.read(size)
        assert stream.read(1) == b'\n'
    return result


def archived_capture_digest(manifest, baseline):
    commit = baseline['historical_capture_commit']
    if not re.fullmatch(r'[a-f0-9]{40}', commit):
        return None
    receipt = read(ROOT / baseline['historical_capture_source_record'])
    if not (receipt['passed'] and receipt['source_commit'] == commit
            and receipt['original_capture_source_sha256'] == receipt['reconstructed_source_sha256'] == manifest['sha256']
            and receipt['original_manifest_sha256'] == sha(EVIDENCE / 'source-manifest.json')
            and receipt['preparation_script_sha256'] == sha(APP / 'scripts/record_historical_capture_sources.py')
            and not receipt['generated_cache_bytes_published']
            and [entry['name'] for entry in receipt['files']] == manifest['files']):
        return None
    committed = [entry for entry in receipt['files'] if entry['origin'] == 'historical-git-blob']
    generated = [entry for entry in receipt['files'] if entry['origin'] == 'ignored-generated-cache']
    if not (len(committed) + len(generated) == len(manifest['files']) == receipt['source_count']
            and len(committed) == receipt['git_source_count'] and len(generated) == receipt['generated_cache_count'] == 1
            and {entry['name'] for entry in generated} == {'web/tsconfig.tsbuildinfo'}):
        return None
    blobs = object_bytes(['app/' + entry['name'] for entry in committed], commit + ':')
    value = hashlib.sha256()
    for entry in receipt['files']:
        name = entry['name']
        if entry['origin'] == 'historical-git-blob':
            data = blobs['app/' + name]
            if hashlib.sha256(data).hexdigest() != entry['sha256'] or len(data) != entry['bytes']:
                return None
        elif not re.fullmatch(r'[a-f0-9]{64}', entry['sha256']) or not isinstance(entry['bytes'], int) or entry['bytes'] < 1:
            return None
        value.update(name.encode() + b'\0' + entry['sha256'].encode() + b'\0')
    return manifest['sha256'] if value.hexdigest() == receipt['per_file_hash_manifest_sha256'] else None


def private_values():
    values = set()

    def collect(obj):
        if isinstance(obj, dict):
            for key, value in obj.items():
                if isinstance(value, str) and re.search(r'password|secret|token|recovery|totp|mfa|cursor_key', key, re.I) and len(value) >= 12:
                    values.add(value.encode('utf-8'))
                if isinstance(value, (dict, list)):
                    collect(value)
        elif isinstance(obj, list):
            for value in obj:
                collect(value)

    for name in ('environment.json', 'development-accounts.json'):
        path = APP / '.local' / name
        if path.exists():
            config = read(path)
            collect(config)
            if name == 'environment.json':
                for key in ('CP_DATABASE_URL', 'CP_MIGRATION_DATABASE_URL'):
                    password = unquote(urlsplit(config.get(key, '')).password or '')
                    if len(password) >= 12:
                        values.add(password.encode('utf-8'))
    return values


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--staged', action='store_true', help='Check actual index bytes; do not rewrite the saved report.')
    args = parser.parse_args()
    names = sorted(set(n.decode('utf-8') for n in git('ls-files', '-z', *([] if args.staged else ['--cached', '--others', '--exclude-standard'])).split(b'\0') if n))
    staged_data = object_bytes(names) if args.staged else None
    baseline_path = ROOT / 'docs/quality/current-functional-baseline.json'
    baseline = read(baseline_path) if baseline_path.exists() else None
    known_private = private_values()
    private_matches = []
    checks, findings = [], []

    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})

    check('Publication contains code, documentation and bilingual entry points', all(n in names for n in
          ['README.md', 'AGENTS.md', '.gitignore', '.gitattributes', 'docs/README.zh-CN.md', 'docs/README.en.md',
           'docs/media/README.md', 'docs/CHANGELOG.md', 'docs/contributing.md', 'app/package-lock.json']))
    forbidden_parts = {'.local', '.tools', 'node_modules', '.gradle', '.taro', '.swc', '__pycache__', '.pytest_cache', 'coverage'}
    sensitive = [
        ('private-key', re.compile(rb'-----BEGIN (?:RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----')),
        ('jwt', re.compile(rb'eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{15,}')),
        ('github-token', re.compile(rb'(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{50,})')),
        ('provider-token', re.compile(rb'\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{32,}\b')),
        ('telegram-token', re.compile(rb'\b[0-9]{8,12}:AA[A-Za-z0-9_-]{30,40}\b')),
        ('aws-access-id', re.compile(rb'\b(?:AKIA|ASIA)[A-Z0-9]{16}\b')),
    ]
    total, large, forbidden, binary_count, changed_index = 0, [], [], 0, []
    markdown = {}
    for name in names:
        path = ROOT / name
        parts = PurePosixPath(name).parts
        if any(p in forbidden_parts for p in parts) or any(p in ('dist', 'build') for p in parts if parts[0] == 'app'):
            forbidden.append(name)
        if path.name.startswith('.env') and path.name != '.env.example' or path.suffix.lower() in ('.apk', '.aab', '.jks', '.keystore', '.exe', '.pyc'):
            forbidden.append(name)
        data = staged_data[name] if args.staged else path.read_bytes()
        if args.staged and path.read_bytes() != data:
            changed_index.append(name)
        total += len(data)
        if any(value in data for value in known_private):
            private_matches.append(name)
        if len(data) >= 100 * 1024 * 1024:
            large.append(name)
        if b'\0' in data[:8192]:
            binary_count += 1
            continue
        for kind, pattern in sensitive:
            for match in pattern.finditer(data):
                findings.append({'file': name, 'line': data[:match.start()].count(b'\n') + 1, 'kind': kind})
        if path.suffix == '.md':
            markdown[name] = data.decode('utf-8-sig')
    check('No local tools, credentials, caches, databases or release binaries selected', not forbidden)
    check('All selected Git blobs are smaller than 100 MiB', not large)
    check('High-confidence private key/token patterns absent from selected text', not findings)
    check('Known private generated development values absent from all selected bytes', not private_matches)
    if args.staged:
        check('Index bytes match reviewed working files, including source line endings', not changed_index)
    publication_files = [
        '.gitattributes', '.gitignore', 'AGENTS.md', 'README.md', 'docs/README.md', 'docs/README.zh-CN.md',
        'docs/README.en.md', 'docs/CHANGELOG.md', 'docs/contributing.md', 'docs/media/README.md',
        'docs/development/77-repository-publication-and-bilingual-readmes.md',
        'docs/delivery/02-implementation-ledger.md', 'docs/delivery/03-goal-gap-assessment.md',
        'docs/quality/evidence/repository-showcase/verification.md',
        *['app/scripts/' + n for n in ('repository_showcase_sources.py', 'repository_recording.mjs',
          'run_repository_showcase.py', 'capture_repository_showcase.mjs', 'encode_repository_showcase.py',
          'verify_repository_publication_development.py', 'verify_repository_publication.py')],
    ]
    if baseline:
        publication_files += [
            'docs/development/78-mobile-task-editing-and-dependencies.md',
            'docs/quality/evidence/mobile-tasks/verification.md',
            'app/mobile/src/pages/project/task-forms.tsx',
            'app/mobile/src/platform/task-input.ts', 'app/mobile/src/platform/task-input.test.ts',
            *['app/scripts/' + n for n in ('mobile_tasks_sources.py', 'run_mobile_tasks_browser.py',
              'check_mobile_tasks.mjs', 'encode_mobile_tasks_media.py',
              'verify_mobile_tasks_development.py', 'verify_mobile_tasks_sources.py',
              'record_historical_capture_sources.py')],
        ]
    publication_files += baseline.get('publication_files', []) if baseline else []
    publication_files = list(dict.fromkeys(publication_files))
    whitespace = []
    for name in publication_files:
        data = staged_data[name] if args.staged else (ROOT / name).read_bytes()
        lines = data.splitlines()
        if any(line.endswith((b' ', b'\t')) for line in lines) or data.endswith((b'\n\n', b'\r\n\r\n')):
            whitespace.append(name)
    check('Publication code and guides have no trailing whitespace or extra EOF blank lines', not whitespace)
    selected = set(names)
    broken = []
    for name, content in markdown.items():
        content = re.sub(r'^\s*(`{3,}|~{3,}).*?^\s*\1\s*$', '', content, flags=re.M | re.S)
        for match in re.finditer(r'!?\[[^\]\n]*\]\((<[^>\n]+>|[^)\s]+)(?:\s+"[^"\n]*")?\)', content):
            target = match.group(1).strip('<>')
            if target.startswith(('#', 'http:', 'https:', 'mailto:', 'app:', 'codex:', 'skill:', 'data:')):
                continue
            target = unquote(target.split('#', 1)[0].split('?', 1)[0])
            if not target:
                continue
            resolved = (ROOT / PurePosixPath(name).parent / target).resolve()
            try:
                relative = resolved.relative_to(ROOT.resolve()).as_posix()
            except ValueError:
                relative = None
            valid = relative in selected if relative else False
            if relative and (resolved.is_dir() or target.endswith('/')):
                valid = any(n.startswith(relative.rstrip('/') + '/') for n in selected)
                if relative == '.':
                    valid = True
            if not valid:
                broken.append({'file': name, 'target': target})
    check('All parsed local Markdown links resolve to selected repository files/directories', not broken)
    guides = [markdown.get('docs/README.zh-CN.md', ''), markdown.get('docs/README.en.md', '')]
    terms = ('120 GB', '2 TB', 'PostgreSQL', 'OSS', 'R2', 'Codex', 'TG', 'Android',
             str(baseline['mobile_tests'] if baseline else 114), 'MFA',
             'run_backend.py', 'dev_database.py', 'build:weapp', 'build:tt', 'build_android.py', 'CHANGELOG.md')
    check('Both full language guides cover architecture, workflows, setup, clients, gates and updates',
          all(all(term in guide for term in terms) for guide in guides))
    capture = read(EVIDENCE / 'result.json')
    current = module('repository_showcase_sources.py')
    capture_manifest = read(EVIDENCE / 'source-manifest.json')
    capture_digest = archived_capture_digest(capture_manifest, baseline) if baseline else current.digest()
    check('Initial capture matches historical Git sources and its explicitly recorded ignored-cache supplement' if baseline else 'Capture source manifest matches complete current application and capture scripts',
          capture['passed'] and capture['source_unchanged'] and capture['source_sha256'] == capture_manifest['sha256'] == capture_digest
          and (bool(baseline) or capture_manifest['files'] == current.sources()))
    check('Capture fixture, owned servers and isolated native database cleaned up',
          capture['native_fixture_exit_code'] == 0 and capture['isolated_database_cleanup_passed']
          and capture['owned_servers_closed'] and capture['private_fixture_removed']
          and not (APP / '.local/business-browser-fixture.json').exists())
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Owned temporary browser port5189 released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    screenshots = []
    for phase in ('web', 'mobile'):
        report = read(EVIDENCE / f'{phase}.json')
        check(f'{phase} actual UI/API flow passed and browser/context closed', report['passed'] and not report['errors']
              and not report['cleanup_errors'] and report['owned_browser_closed'] and report['contexts_closed'] == 1
              and report['credentials_excluded'] and not report['real_model'] and not report['real_cloud'] and not report['real_device'])
        screenshots.extend(report['screenshots'])
    historical = read(EVIDENCE / 'historical-media.json')
    screenshots.append(historical)
    check('All fifteen screenshots and historical source match recorded hashes', len(screenshots) == 15
          and all(sha(ROOT / shot['file']) == shot['sha256'] for shot in screenshots)
          and sha(ROOT / historical['source']) == historical['source_sha256'])
    media = read(EVIDENCE / 'media.json')
    check('Three MP4/GIF pairs match verified encoder/capture and output digests', media['passed']
          and media['encoder_sha256'] == sha(APP / 'scripts/encode_repository_showcase.py')
          and media['capture_result_sha256'] == sha(EVIDENCE / 'result.json') and len(media['recordings']) == 3
          and all(sha(ROOT / item[typ]['file']) == item[typ]['sha256']
                  and (ROOT / item[typ]['file']).stat().st_size == item[typ]['bytes'] < 15 * 1024 * 1024
                  for item in media['recordings'] for typ in ('mp4', 'gif')))
    visual = read(EVIDENCE / 'visual-review.json')
    check('Media manual review binds the exact inspected screenshots and GIFs', visual['reviewed']
          and len(visual['screenshots']) == 15 and all(sha(ROOT / entry['file']) == entry['sha256']
              for entry in visual['screenshots'] + visual['gifs']))
    previous = EVIDENCE.parent / 'deliverable-creation'
    backend_group = baseline.get('backend_evidence_group', 'deliverable-creation') if baseline else 'deliverable-creation'
    backend = read(EVIDENCE.parent / backend_group / 'result.json')
    check('Full backend proof matches complete current Go/SQL/contract source and the declared test scope', backend['passed']
          and backend['source_sha256'] == module('verify_backend_batches.py').digest()
          and backend['source_unchanged_during_tests'] and backend['all_discovered_tests_executed']
          and backend['discovered_top_level_tests'] == (baseline.get('backend_top_level_tests', 146) if baseline else 146)
          and len(backend['checks']) == (baseline.get('backend_result_records', 919) if baseline else 919)
          and all(item['Action'] == 'pass' for item in backend['checks']))
    runtime_group = baseline.get('runtime_evidence_group', 'deliverable-creation') if baseline else 'deliverable-creation'
    runtime = read(EVIDENCE.parent / runtime_group / 'runtime-build.json')
    check('Runtime proof matches complete current source and declared test scope', runtime['status'] == 'passed'
          and runtime['source_sha256'] == module('verify_agent_runtime.py').digest()[0] and runtime['tests']['pass'] == (baseline.get('runtime_tests', 39) if baseline else 39))
    web_group = baseline.get('web_evidence_group', 'deliverable-creation') if baseline else 'deliverable-creation'
    web = read(EVIDENCE.parent / web_group / 'web-build.json')
    check('Web proof matches current listed sources and declared test scope', web['passed']
          and web['source_sha256'] == digest(web['sources']) and web['tests']['numPassedTests'] == (baseline.get('web_tests', 81) if baseline else 81))
    functional = EVIDENCE.parent / (baseline['evidence_group'] if baseline else 'mobile-project-resources')
    mobile = read(functional / 'mobile-build.json')
    check('Current mobile proof matches complete source and recorded passing test count', mobile['passed']
          and mobile['source_sha256'] == module('verify_mobile.py').digest()[0]
          and mobile['tests']['pass'] == (baseline['mobile_tests'] if baseline else 114)
          and mobile['tests']['fail'] == mobile['tests']['skipped'] == mobile['tests']['cancelled'] == 0)
    android = read(functional / 'build.json')
    check('Current Android build binds complete native and portable sources' if baseline
          else 'Retained Android build binds unchanged native and portable sources; no new execution claimed',
          android['passed'] and android['source_sha256'] == digest(android['sources']))
    development = functional if baseline else EVIDENCE
    check('Current read-only development inspection passed, with no test databases/external activation',
          (bool(baseline) or read(development / 'development-check.json')['passed']) and read(development / 'development-runtime.json')['passed']
          and read(development / 'development-project-resources.json')['passed']
          and read(development / 'development-project-resources.json')['database']['disposable_test_databases'] == 0)
    if baseline:
        proof = read(functional / 'final-source-check.json')
        check('Latest functional evidence binds current sources, browser flows, media and verification script',
              proof['passed'] and proof['mobile_source_sha256'] == module('verify_mobile.py').digest()[0]
              and (proof['interaction_source_sha256'] == module(baseline['interaction_sources']).digest() and proof['interaction_kind'] == baseline['interaction_kind'] if baseline.get('interaction_kind') else proof['browser_source_sha256'] == module(baseline['browser_sources']).digest())
              and proof['verification_script_sha256'] == sha(APP / 'scripts' / baseline['verification_script'])
              and all(item['passed'] for item in proof['checks']) and not proof['full_goal_complete'])
    skills = [read(p) for p in sorted((APP / 'skills').glob('*/manifest.json'))]
    if baseline and baseline.get('planning_scope_verification_record'):
        scope = read(ROOT / baseline['planning_scope_manifest'])
        scoped = read(ROOT / baseline['planning_scope_verification_record'])
        check('Current user-approved scope and exclusions match source-bound planning evidence',
              scoped['passed'] and scoped['scope_revision'] == baseline['planning_scope_revision'] == scope['revision']
              and scoped['counts'] == scope['counts']
              and scoped['scope_manifest_sha256'] == sha(ROOT / baseline['planning_scope_manifest'])
              and scoped['verification_script_sha256'] == sha(APP / 'scripts' / baseline['planning_scope_verification_script'])
              and all(sha(ROOT / row['file']) == row['sha256'] for row in scoped['input_files'])
              and all(row['passed'] for row in scoped['checks']) and not scoped['full_goal_complete'])
    audit = read(ROOT / baseline['dependency_audit_record']) if baseline and baseline.get('dependency_audit_record') else read(EVIDENCE.parent / 'android-native/dependency-audit.json')
    check('All eight Skills remain draft and source-bound dependency security gate remains open',
          len(skills) == 8 and all(s['state'] == 'draft' for s in skills)
          and sha(APP / 'package-lock.json') == audit['package_lock_sha256'] and not audit['production_security_gate_passed'])
    check('Gap register retains all twenty-eight stable IDs',
          set(re.findall(r'\| (GAP-\d{2}) \|', markdown.get('docs/delivery/03-goal-gap-assessment.md', '')))
          == {f'GAP-{n:02}' for n in range(1, 29)})
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
              'scope': 'Actual staged index bytes' if args.staged else 'Tracked and non-ignored candidate working files',
              'file_count': len(names), 'total_bytes': total, 'binary_files': binary_count, 'markdown_files': len(markdown),
              'checks': checks, 'sensitive_findings': findings, 'forbidden_paths': sorted(set(forbidden)),
              'known_private_value_count': len(known_private), 'known_private_value_matches': private_matches,
              'publication_whitespace_findings': whitespace,
              'oversize_files': large, 'index_worktree_differences': changed_index, 'broken_local_links': broken,
              'secret_scan_scope': 'High-confidence text patterns, not exhaustive secret detection or pixel OCR.',
              'functional_evidence': 'Current source-bound backend/Web/runtime/mobile/browser/native artifacts and declared retained foundation proofs; see the baseline and final proof.' if baseline else 'Existing evidence reused by matching sources; not newly executed.',
              'current_functional_increment': baseline['increment'] if baseline else 76,
              'current_scope_revision': baseline.get('planning_scope_revision') if baseline else None,
              'verification_script_sha256': sha(Path(__file__)), 'production_ready': False}
    if not args.staged:
        target = ROOT / baseline['planning_publication_record'] if baseline and baseline.get('planning_publication_record') else (functional if baseline else EVIDENCE) / 'repository-check.json'
        target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'files': len(names), 'MiB': round(total / 1024 / 1024, 2),
                      'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']],
                      'sensitive_findings': findings, 'forbidden_paths': sorted(set(forbidden)), 'broken_local_links': broken[:30],
                      'broken_local_link_count': len(broken), 'staged': args.staged}, ensure_ascii=False))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
