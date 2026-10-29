"""Check current execution records, patched sources and retained historical bytes."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/dependency-integrity'
PUBLISHED = 'a7f323f631eb07d6d6852d3d666df8e085af02a3'


def read(path):
    return json.loads(path.read_text(encoding='utf-8-sig'))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def module(name):
    spec = importlib.util.spec_from_file_location('integrity_' + name, APP / 'scripts' / (name + '.py'))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


def main():
    checks = []
    def check(name, value):
        checks.append({'name': name, 'passed': bool(value)})

    manifest = read(APP / 'package.json'); paths = read(APP / 'package-lock.json')['packages']
    aliases = {'deepmerge': 'packages/deepmerge-safe', 'decode-uri-component': 'packages/uri-decoder-compat'}
    check('All patched aliases have disclosed identities and portable links, without old nested implementations',
          all(manifest['overrides'][name] == '$' + name and manifest['devDependencies'][name] == 'file:' + target
              and read(APP / target / 'package.json')['name'].startswith('@copypaste/') for name, target in aliases.items())
          and all(row.get('link') and row.get('resolved') == aliases[name.split('node_modules/')[-1]]
                  for name, row in paths.items() if name.split('node_modules/')[-1] in aliases))
    expected = {'@babel/core': '7.29.7', 'esbuild': '0.28.2', 'fast-xml-parser': '5.7.0', 'uuid': '11.1.1'}
    check('Every locked affected upstream consumer uses the selected fixed version',
          all(any(name.endswith('node_modules/' + key) for name in paths) and
              all(row.get('version') == value for name, row in paths.items() if name.endswith('node_modules/' + key))
              for key, value in expected.items())
          and paths['node_modules/@copypaste/uri-decoder-upstream']['version'] == '0.5.1'
          and all(row.get('version') in ('0.19.2', '1.2.1') for name, row in paths.items() if name.endswith('node_modules/send')))
    upstream = read(APP / 'packages/deepmerge-safe/UPSTREAM.json')
    patched = (APP / 'packages/deepmerge-safe/index.cjs').read_text(encoding='utf-8')
    original = patched[:patched.index('// Derived')] + patched[patched.index('var isMergeableObject'):]
    replacements = {
        'defineOwn(destination, key, cloneUnlessOtherwiseSpecified(target[key], options));': 'destination[key] = cloneUnlessOtherwiseSpecified(target[key], options);',
        'defineOwn(destination, key, getMergeFunction(key, options)(target[key], source[key], options));': 'destination[key] = getMergeFunction(key, options)(target[key], source[key], options);',
        'defineOwn(destination, key, cloneUnlessOtherwiseSpecified(source[key], options));': 'destination[key] = cloneUnlessOtherwiseSpecified(source[key], options);',
    }
    for old, new in replacements.items():
        original = original.replace(old, new)
    repro = read(E / 'deepmerge-before.json')
    check('The disclosed MIT merge patch changes exactly the three unsafe writes from the reproduced upstream entry',
          patched.count('defineOwn(destination, key,') == 4
          and all(patched.count(value) == 1 for value in replacements)
          and hashlib.sha256(original.encode()).hexdigest() == upstream['original_entry_sha256'] == repro['source_sha256']
          and repro['upstream_version'] == '4.3.1' and repro['returned_prototype_changed']
          and repro['inherited_synthetic_value'] and repro['global_prototype_unchanged']
          and 'Permission is hereby granted' in (APP / 'packages/deepmerge-safe/LICENSE').read_text(encoding='utf-8'))
    install = read(E / 'clean-install.json'); config = read(E / 'install-config.json')
    check('Standard immutable-lock npm ci completed with lifecycle scripts and no peer bypass',
          install['passed'] and install['exit_code'] == 0 and install['lock_unchanged'] and install['manifest_unchanged']
          and install['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and install['package_manifest_sha256'] == sha(APP / 'package.json')
          and install['command'] == ['npm', 'ci', '--no-audit', '--no-fund']
          and install['install_scripts_enabled'] and not install['force_or_legacy_peer_flags']
          and install['public_log_sha256'] == sha(E / 'clean-install.log')
          and config['passed'] and config['effective_values'] == {'ignore-scripts': 'false', 'force': 'false', 'legacy-peer-deps': 'false', 'omit': '', 'include': ''})
    security = read(E / 'security-checks.json')
    check('All56 real dependency/security and2 native JS cases pass on unchanged current sources',
          security['passed'] and security['source_unchanged'] and not security['focused']
          and digest(security['sources']) == security['source_sha256']
          and [v['tests']['pass'] for v in security['checks']] == [56, 2]
          and all(v['passed'] and not v['timed_out'] for v in security['checks'])
          and security['tree_valid'] and security['tree_exit_code'] == 0
          and security == read(E / 'attempts' / security['label'] / 'result.json')
          and security['tree_sha256'] == sha(E / 'attempts' / security['label'] / 'installed-tree.json'))
    attempts_ok = True
    for folder in sorted((E / 'attempts').iterdir()):
        report = read(folder / 'result.json')
        attempts_ok = attempts_ok and report['source_unchanged']
        value = hashlib.sha256()
        with zipfile.ZipFile(folder / 'source-inputs.zip') as archive:
            attempts_ok = attempts_ok and archive.namelist() == report['sources']
            for name in report['sources']:
                value.update(name.encode() + b'\0' + archive.read(name) + b'\0')
        attempts_ok = attempts_ok and value.hexdigest() == report['source_sha256']
        attempts_ok = attempts_ok and report['source_archive_sha256'] == sha(folder / 'source-inputs.zip')
        attempts_ok = attempts_ok and report['tree_sha256'] == sha(folder / 'installed-tree.json')
        attempts_ok = attempts_ok and all(v['public_log_sha256'] == sha(folder / (v['name'] + '.log')) for v in report['checks'])
    initial = read(E / 'attempts/focused01/result.json')
    check('Every security attempt keeps complete exact inputs and original logs, including the initial13/3 failure',
          attempts_ok and not initial['passed'] and not initial['tree_valid']
          and initial['checks'][0]['tests'] == {'tests': 16, 'pass': 13, 'fail': 3, 'skipped': 0, 'cancelled': 0})
    cold = read(E / 'attempts/full01/result.json')
    check('The original HTTP-deadline failure and successful retry use identical inputs and assertions',
          not cold['passed'] and cold['source_sha256'] == security['source_sha256'] and cold['tree_valid']
          and cold['checks'][0]['tests'] == {'tests': 56, 'pass': 55, 'fail': 1, 'skipped': 0, 'cancelled': 0}
          and cold['checks'][1]['tests'] == {'tests': 2, 'pass': 2, 'fail': 0, 'skipped': 0, 'cancelled': 0}
          and 'Synthetic HTTP request timed out.' in (E / 'attempts/full01/security-tests.log').read_text(encoding='utf-8'))
    resolved = read(E / 'resolution03.json'); dedupe = read(E / 'dedupe02.json')
    early = read(E / 'early-attempt-logs.json')
    check('Early command outcomes retain original result bytes and sanitized exact-source logs',
          len(early['attempts']) == 2 and all(
              v['record_not_rewritten'] and v['original_record_sha256'] == sha(E / v['original_record'])
              and v['public_log_sha256'] == sha(E / v['public_log'])
              and v['raw_log_sha256'] == read(E / v['original_record'])['raw_log_sha256'] for v in early['attempts']))
    check('Failed dedupe stays failed; normal isolated resolution produces the final portable candidate',
          not dedupe['passed'] and dedupe['exit_code'] == 1 and not dedupe['force_or_legacy_peer_flags']
          and resolved['passed'] and resolved['exit_code'] == 0 and resolved['aliases_portable_and_exact']
          and resolved['candidate_lock_sha256'] == sha(APP / 'package-lock.json')
          and resolved['previous_lock_sha256'] == dedupe['package_lock_sha256']
          and resolved['removed_stale_candidate_entries'] and not resolved['force_or_legacy_peer_flags'])
    audit = read(E / 'dependency-audit.json'); before = read(E / 'audit-before.json'); after = read(E / 'audit-after.json')
    prior = read(E / 'audit-before-source.json')
    old_lock = subprocess.check_output(['git', 'show', PUBLISHED + ':app/package-lock.json'], cwd=ROOT)
    check('Full before/after audit binds the exact published and current locks without suppressed scope',
          audit['passed'] and audit['all_dependency_scopes_included'] and not audit['advisories_suppressed']
          and prior['commit'] == PUBLISHED and prior['all_dependency_scopes']
          and prior['raw_log_sha256'] == sha(E / 'audit-before.json')
          and hashlib.sha256(old_lock).hexdigest() == prior['package_lock_sha256']
          and audit['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and audit['raw_report_sha256'] == sha(E / 'audit-after.json')
          and audit['before_vulnerabilities'] == before['metadata']['vulnerabilities']
          and audit['vulnerabilities'] == after['metadata']['vulnerabilities']
          and before['metadata']['vulnerabilities']['total'] == 63
          and not audit['production_security_gate_passed'])
    pending = read(E / 'remaining-findings.json'); dispositions = read(E / 'remediations.json')
    check('No npm advisory remains; every prior direct advisory has a disclosed tested implementation change',
          after['metadata']['vulnerabilities']['total'] == 0 and after['vulnerabilities'] == {}
          and audit['npm_audit_gate_passed'] and pending['items'] == []
          and dispositions['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and len(dispositions['items']) == dispositions['direct_packages'] == 7
          and {v['name'] for v in dispositions['items']} == {'@babel/core', 'decode-uri-component', 'deepmerge', 'esbuild', 'fast-xml-parser', 'send', 'uuid'}
          and all(v['original_advisories'] == [a for a in before['vulnerabilities'][v['name']]['via'] if isinstance(a, dict)]
                  and v['status'] == 'implemented' and not v['production_signoff'] for v in dispositions['items'])
          and {name for name, value in before['vulnerabilities'].items() if any(isinstance(v, dict) for v in value['via'])}
              == {'@babel/core', 'decode-uri-component', 'deepmerge', 'esbuild', 'fast-xml-parser', 'send', 'uuid'})
    clients = read(E / 'client-checks.json')
    check('All eight fresh client commands pass with current complete source inputs',
          clients['passed'] and clients['source_unchanged'] and len(clients['checks']) == 8
          and clients['source_sha256'] == digest(clients['source_files'])
          and all(v['passed'] and v['exit_code'] == 0 and not v['timed_out'] for v in clients['checks'])
          and {v['name']: v['test_count'] for v in clients['checks'] if v['test_count'] is not None}
              == {'web-tests': 172, 'mobile-tests': 273, 'runtime-tests': 55})
    for row in clients['checks']:
        check('Exact raw client log: ' + row['name'], sha(APP / '.local' / Path(row['raw_log']).name) == row['raw_log_sha256'])
    mobile = read(E / 'mobile-build.json'); runtime = read(E / 'runtime-build.json')
    check('Both20-page miniapps and runtime summaries match current sources and executed command records',
          mobile['passed'] and mobile['source_sha256'] == module('verify_mobile').digest()[0]
          and all(v['page_count'] == 20 for v in mobile['targets'].values())
          and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0]
          and runtime['skills_check']['passed']
          and all(read(E / name)['derived_from']['sha256'] == sha(E / 'client-checks.json') for name in ['web-build.json', 'mobile-build.json', 'runtime-build.json']))
    android = read(E / 'build.json'); apk = read(E / 'apk-inspection.json'); jvm = read(E / 'android-test-scope.json')
    check('Fresh native Android/APK records bind every local compatibility source without claiming a device or release',
          android['passed'] and android['source_unchanged_during_build']
          and android['source_sha256'] == module('build_android').sources()[0]
          and all(any(n.startswith('packages/' + name + '/') for n in android['sources']) for name in
                  ['deepmerge-safe', 'uri-decoder-compat', 'dev-server-compat', 'template-minifier'])
          and sha(APP / android['artifacts']['apk']['path']) == android['artifacts']['apk']['sha256']
          and apk['apk_sha256'] == android['artifacts']['apk']['sha256']
          and apk['passed'] and len(apk['checks']) == 18 and all(v['passed'] for v in apk['checks'])
          and not android['device_executed'] and not android['emulator_started'] and not android['production_release'])
    uri = read(E / 'native-uri-bundle.json')
    map_path = APP / '.local/android/index.android.map'
    map_data = read(map_path)
    map_sources = {name.replace('\\', '/') for name in map_data['sources']}
    check('The actual packaged Metro bundle includes both the local URI bridge and maintained upstream parser',
          uri['passed'] and not uri['device_executed']
          and uri['bundle_sha256'] == android['artifacts']['bundle']['sha256']
          and uri['source_map_sha256'] == sha(map_path)
          and uri['package_lock_sha256'] == sha(APP / 'package-lock.json')
          and uri['bridge_source_sha256'] == sha(APP / 'packages/uri-decoder-compat/index.cjs')
          and len(uri['sources']) == 2 and uri['source_count'] == len(map_data['sources'])
          and all(v['present_in_actual_source_map'] and APP.as_posix() + '/' + v['source'] in map_sources for v in uri['sources']))
    check('JVM26 scope reports actual Gradle caching and exact original XML/log bytes',
          jvm['passed'] and jvm['tests'] == 26 and jvm['new_test_execution'] == (not jvm['gradle_cached'])
          and jvm['native_log_sha256'] == sha(E / 'native-build.log') and len(jvm['xml_sha256']) == 4
          and all(sha(E / 'android-unit-results' / name) == value for name, value in jvm['xml_sha256'].items()))
    retained = read(E / 'reused-evidence.json')
    check('All211 unchanged media and7 historical execution records retain exact published bytes',
          retained['passed'] and retained['published_commit'] == PUBLISHED and sum(retained['media_counts'].values()) == 211
          and not retained['new_browser_capture'] and not retained['new_backend_test_execution']
          and len(retained['retained_records']) == 7
          and all(sha(ROOT / row['file']) == row['sha256'] for row in [*retained['media'], *retained['retained_records']])
          and not subprocess.check_output(['git', 'diff', '--name-only', PUBLISHED, '--', *retained['ui_source_paths']], cwd=ROOT).strip())
    check('All historical107 evidence stays unchanged at its published commit',
          not subprocess.check_output(['git', 'diff', '--name-only', PUBLISHED, '--', 'docs/quality/evidence/dependency-toolchain'], cwd=ROOT).strip())
    backend = read(E.parent / 'cost-amortization-backend/result.json'); linux = read(E.parent / 'cost-amortization/linux-build.json')
    backend_digest = module('verify_backend_batches').digest()
    check('Unchanged backend224/1355 and2 Linux cross-builds retain original execution scope',
          backend['passed'] and backend['source_sha256'] == backend_digest and backend['all_discovered_tests_executed']
          and backend['discovered_top_level_tests'] == 224 and len(backend['checks']) == 1355
          and linux['passed'] and linux['source_sha256'] == linux['source_after_sha256'] == backend_digest
          and not linux['artifact_executed'] and not linux['deployed'])
    dev = read(E / 'development-project-resources.json')
    check('Read-only native development check retains schema83, exact owned processes and no disposable databases',
          dev['passed'] and dev['migration_count'] == 83 and dev['backend_source_sha256'] == backend_digest
          and dev['database']['disposable_test_databases'] == 0 and dev['integrations_disabled']
          and dev['worker_sources_match'] and all(dev['ownership'].values()))
    watch = read(E / 'h5-server-check.json'); browser = read(E / 'h5-client-check.json'); h5_logs = read(E / 'h5-log-archive.json')
    check('Actual current Taro H5 and native Edge checks pass and close every owned browser/server resource',
          watch['passed'] and watch['source_unchanged'] and watch['source_sha256'] == module('dependency_integrity_sources').digest()
          and len(watch['checks']) == 5 and watch['owned_process_terminal'] and watch['owned_port_released']
          and watch['client_record_sha256'] == sha(E / 'h5-client-check.json') and browser['passed']
          and len(browser['checks']) == 3 and browser['context_closed'] and browser['browser_closed']
          and not browser['recorded_credentials'] and not browser['new_media']
          and h5_logs['passed'] and h5_logs['watch_record_sha256'] == sha(E / 'h5-server-check.json')
          and len(h5_logs['logs']) == 2 and all(v['public_log_sha256'] == sha(E / v['public_file'])
              and v['raw_log_sha256'] == watch['raw_' + v['kind'] + '_log_sha256'] for v in h5_logs['logs']))
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('Current scope retains23 active/5 excluded, eight draft Skills and AI authorization/isolation',
          scope['counts']['active'] == 23 and scope['counts']['excluded'] == 5
          and not scope['flags']['ai_cost_control_required'] and scope['flags']['ai_execution_isolation_required']
          and scope['flags']['ai_authorization_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    leftovers = [p.name for p in (APP / '.local').iterdir() if p.is_dir() and re.fullmatch(r'dependency-(?:integrity|toolchain|remediation|maintenance|security)-[a-zA-Z0-9]{6}', p.name)]
    check('Completed synthetic consumer checks removed their own temporary workspaces', leftovers == [])
    proof = {'passed': all(v['passed'] for v in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
             'interaction_kind': 'dependency_integrity_and_retained_media',
             'interaction_source_sha256': module('dependency_integrity_sources').digest(),
             'mobile_source_sha256': module('verify_mobile').digest()[0], 'backend_source_sha256': backend_digest,
             'verification_script_sha256': sha(Path(__file__)), 'historical_evidence_commit': PUBLISHED,
             'new_browser_capture': False, 'new_backend_test_execution': False, 'npm_advisories': audit['vulnerabilities'],
             'full_goal_complete': False, 'production_ready': False}
    (E / 'final-source-check.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': proof['passed'], 'checks': len(checks), 'failed': [v['name'] for v in checks if not v['passed']]}))
    return 0 if proof['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
