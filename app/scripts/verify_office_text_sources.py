"""Verify actual final-source results; never promote fixtures to production acceptance."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
import sys
from office_text_sources import APP, digest

ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/office-text'
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))


def module(name):
    spec = importlib.util.spec_from_file_location('office_verify_' + name, APP / 'scripts' / (name + '.py'))
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def app_digest(names):
    h = hashlib.sha256()
    for n in names:
        h.update(n.encode() + b'\0' + (APP / n).read_bytes() + b'\0')
    return h.hexdigest()


def main():
    checks = []
    def check(name, value):
        checks.append({'name': name, 'passed': bool(value)})
    backend_source = module('verify_backend_batches').digest()
    bdir = E.parent / 'office-text-backend'
    backend, state, logs = [read(bdir / n) for n in ('result.json', 'batch-progress.json', 'raw-log-archive.json')]
    check('All discovered backend cases pass against the exact current Go/SQL/contract source',
        backend['passed'] and backend['source_sha256'] == state['source_sha256'] == backend_source
        and backend['all_discovered_tests_executed'] and backend['source_unchanged_during_tests']
        and all(v['Action'] == 'pass' for v in backend['checks'])
        and sorted((v['Package'], v['Test']) for v in backend['checks'] if '/' not in v['Test']) == sorted(map(tuple, state['expected_tests']))
        and all(v['status'] == 'passed' for v in state['batches']))
    check('Successful batch raw logs remain private and match every final result', logs['passed']
        and logs['source_sha256'] == backend_source and logs['records'] == len(backend['checks'])
        and all(sha(APP / v['file']) == v['sha256'] for v in logs['logs']))
    names = {v['Test'] for v in backend['checks']}
    check('Office parsing, actual pipeline, old formats, full comparisons and agent index cases executed',
        {'TestOfficeTextExtraction', 'TestOfficeTextDocuments', 'TestFileTextExtraction', 'TestFileTextSearch', 'TestFileTextComparison', 'TestAgentIndexedSearch'} <= names)
    upgrade = read(E / 'schema-upgrade.json')
    check('Actual published91 to current92 normal upgrade preserves populated formal and derived facts', upgrade['passed']
        and upgrade['before_schema'] == 91 and upgrade['after_schema'] == 92
        and upgrade['source_sha256'] == backend_source and upgrade['original_facts_unchanged'] and upgrade['old_text_evidence_unchanged']
        and upgrade['no_body_backfill'] and upgrade['selector_and_guard_ready'] and upgrade['facts']['file_text_derivations']['rows'] >= 8
        and not upgrade['counts_as_current_backend_tests'] and upgrade['verification_script_sha256'] == sha(APP / 'scripts/verify_office_text_upgrade.py'))
    clients = read(E / 'client-checks.json')
    cc = {v['name']: v for v in clients['checks']}
    check('All eight client commands completed with unchanged full inputs and matching raw logs', clients['passed']
        and clients['source_unchanged'] and clients['source_sha256'] == clients['source_after_sha256'] == app_digest(clients['source_files'])
        and len(cc) == 8 and all(v['passed'] and v['exit_code'] == 0 and sha(APP / '.local' / Path(v['raw_log']).name) == v['raw_log_sha256'] for v in cc.values()))
    web, mobile, runtime = [read(E / n) for n in ('web-build.json', 'mobile-build.json', 'runtime-build.json')]
    mobile_source = module('verify_mobile').digest()[0]
    check('Web/mobile/runtime summaries retain actual test counts and current complete sources', web['passed'] and web['source_sha256'] == app_digest(web['sources'])
        and web['tests']['numPassedTests'] == cc['web-tests']['test_count'] == 313
        and mobile['passed'] and mobile['source_sha256'] == mobile_source and mobile['tests'] == {'pass':409,'fail':0,'cancelled':0,'skipped':0}
        and runtime['status'] == 'passed' and runtime['source_sha256'] == module('verify_agent_runtime').digest()[0]
        and runtime['tests']['pass'] == cc['runtime-tests']['test_count'] == 56
        and all(v['derived_from']['sha256'] == sha(E / 'client-checks.json') for v in (web, mobile, runtime)))
    check('Both miniapp artifacts contain all20 routes', set(mobile['targets']) == {'weapp','tt'}
        and all(v['page_count'] == 20 and sha(APP / 'mobile/dist' / k / 'app.json') == v['app_json_sha256'] for k,v in mobile['targets'].items()))
    build, inspection, content, jvm = [read(E / n) for n in ('build.json','apk-inspection.json','android-feature-content.json','android-test-scope.json')]
    check('Exact current development APK passes structural and Office-content inspection without execution', build['passed']
        and build['source_unchanged_during_build'] and build['source_sha256'] == module('build_android').sources()[0]
        and build['scoped_bundle_invocation']['max_workers'] == 1 and build['scoped_bundle_invocation']['exit_code'] == 0
        and build['scoped_build_script'] == 'app/scripts/build_office_text_android.py'
        and build['scoped_build_script_sha256'] == sha(APP / 'scripts/build_office_text_android.py') and build['bundle_source_map_guard_applied']
        and all(sha(APP / v['path']) == v['sha256'] for v in build['artifacts'].values())
        and inspection['passed'] and content['passed'] and all(v['passed'] for v in content['checks'])
        and content['verification_script_sha256'] == sha(APP / 'scripts/verify_office_text_android_content.py')
        and not content['artifact_executed'] and not build['production_release'] and not build['device_executed'])
    check('JVM26 discloses actual cache status and exact result XML', jvm['passed'] and jvm['tests'] == 26
        and jvm['new_test_execution'] == (not jvm['gradle_cached']) and not jvm['device_executed']
        and sha(E / 'native-build.log') == jvm['native_log_sha256'] and len(jvm['xml_sha256']) == 4
        and all(sha(E / 'android-unit-results' / n) == v for n,v in jvm['xml_sha256'].items()))
    linux = read(E / 'linux-build.json')
    check('Two exact Linux ELF targets cross-build without claiming Linux execution', linux['passed']
        and linux['source_sha256'] == linux['source_after_sha256'] == backend_source and len(linux['artifacts']) == 2
        and all(sha(APP / v['file']) == v['sha256'] for v in linux['artifacts']) and not linux['artifact_executed'] and not linux['deployed'])
    interaction = digest()
    shots = []
    for client in ('web','mobile'):
        folder = E.parent / ('office-text-' + client)
        page, manifest, capture = [read(folder / n) for n in ('result.json','source-manifest.json','capture.json')]
        check('Actual ' + client + ' Office journey passes all five groups on unchanged sources and releases owned resources',
            page['passed'] and len(page['checks']) == 5 and page['source_unchanged']
            and page['source_sha256'] == manifest['sha256'] == app_digest(manifest['files'])
            and page['build_source_sha256'] == page['build_source_after_sha256'] == interaction
            and all(page[n] for n in ('owned_browser_closed','owned_review_server_stopped','owned_api_proxy_stopped','isolated_database_cleanup_passed'))
            and page['native_fixture_exit_code'] == 0 and not page['console_errors'] and not page['server_errors']
            and len(capture['documents']) == 2 and all(v['extractor'] == 'office-v1' for v in capture['documents'])
            and all(sha(ROOT / v['file']) == v['sha256'] for v in capture['screenshots'])
            and not capture['real_model'] and not capture['real_cloud'])
        shots.extend(capture['screenshots'])
    media, visual = [read(E / n) for n in ('media.json','visual-review.json')]
    check('Actual authenticated synthetic frames produce two exact MP4/GIF pairs', media['passed'] and len(media['recordings']) == 2
        and media['encoder_script_sha256'] == sha(APP / 'scripts/encode_office_text_media.py')
        and all(sha(ROOT / v['capture_manifest']) == v['capture_manifest_sha256'] and all(sha(ROOT / a['file']) == a['sha256'] for a in v['artifacts']) for v in media['recordings']))
    check('Seven final screenshots and decoded GIF samples have a visual review', visual['reviewed'] and len(visual['screenshots']) == len(shots) == 7
        and len(visual['gifs']) == 2 and set(visual['gif_frames_reviewed']) == {'web','mobile'}
        and all(sha(ROOT / v['file']) == v['sha256'] for v in visual['screenshots']+visual['gifs']))
    retained, archive, audit = [read(E / n) for n in ('reused-evidence.json','source-archive.json','dependency-audit.json')]
    check('All316 previous media and published evidence retain their original bytes', retained['passed']
        and retained['prior_media_count'] == 316 and retained['all_prior_evidence_git_objects_unchanged']
        and all(sha(ROOT / v['file']) == v['sha256'] for v in retained['records']))
    check('Current source archive and unchanged dependency evidence preserve exact provenance', archive['passed'] and archive['source_sha256'] == interaction
        and all(sha(APP / v['file']) == v['sha256'] for v in archive['files']) and not retained['new_dependency_audit']
        and audit['package_lock_sha256'] == sha(APP / 'package-lock.json') and not audit['production_security_gate_passed'])
    update, dev = [read(E / n) for n in ('development-update.json','development-project-resources.json')]
    check('Attested development services use exact schema92 sources and unchanged existing facts', update['passed'] and dev['passed']
        and update['previous_migration_count'] == 91 and update['migration_count'] == dev['migration_count'] == 92
        and update['legacy_and_usage_rows_preserved'] and update['backend_source_sha256'] == dev['backend_source_sha256'] == backend_source
        and all(dev['ownership'].values()) and dev['worker_sources_match'] and dev['integrations_disabled'] and dev['database']['disposable_test_databases'] == 0)
    closed = read(E / 'native-cluster-closed.json')
    ports_closed = True
    for p in (5189,5190,9229):
        with socket.socket() as probe:
            probe.settimeout(1)
            ports_closed = ports_closed and probe.connect_ex(('127.0.0.1',p)) != 0
    check('Owned native test cluster and temporary browser fixture/ports are closed', closed['passed'] and closed['owned_cluster_stopped']
        and closed['remaining_disposable_databases'] == 0 and ports_closed and not (APP / '.local/business-browser-fixture.json').exists())
    scope = read(ROOT / 'docs/delivery/04-current-scope.json')
    skills = [read(p) for p in (APP / 'skills').glob('*/manifest.json')]
    check('All23 packages, five exclusions and eight draft Skills keep the corrected AI scope', scope['counts']['active'] == 23
        and scope['counts']['excluded'] == 5 and scope['flags']['ai_secretary_required'] and scope['flags']['ai_authorization_required']
        and scope['flags']['ai_execution_isolation_required'] and not scope['flags']['ai_cost_control_required'] and len(skills) == 8 and all(v['state'] == 'draft' for v in skills))
    report = {'passed':all(v['passed'] for v in checks),'checked_at':datetime.now(timezone.utc).isoformat(),'checks':checks,
        'interaction_kind':'office_text_extraction','interaction_source_sha256':interaction,'backend_source_sha256':backend_source,
        'mobile_source_sha256':mobile_source,'verification_script_sha256':sha(Path(__file__)),
        'full_goal_complete':False,'production_ready':False,'new_browser_capture':True,'new_backend_test_execution':True}
    (E / 'final-source-check.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':report['passed'],'checks':len(checks),'failed':[v['name'] for v in checks if not v['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
