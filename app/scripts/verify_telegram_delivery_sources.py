"""Bind this functional increment's results to source and owned cleanup."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import socket
from urllib.request import urlopen

APP = Path(__file__).resolve().parents[1]
EVIDENCE = APP.parent / 'docs/quality/evidence/telegram-delivery'


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def module(name, file):
    spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / file)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


def digest(names):
    value = hashlib.sha256()
    for name in names:
        value.update(name.encode('utf-8') + b'\0' + (APP / name).read_bytes() + b'\0')
    return value.hexdigest()


def main():
    checks = []
    def check(name, passed):
        checks.append({'name': name, 'passed': bool(passed)})
    backend = module('account_backend', 'verify_backend_batches.py').digest()
    b = read(EVIDENCE / 'result.json')
    check('Every discovered backend test passes with current source', b['passed'] and b['source_sha256'] == backend and b['source_unchanged_during_tests'] and b['all_discovered_tests_executed'] and all(c['Action'] == 'pass' for c in b['checks']))
    observed = {c['Test'] for c in b['checks']}
    check('Native outbound delivery, Bot API transport, configuration, dispatch and ingress regressions were executed', {'TestTelegramDelivery','TestTelegramBotAPIOutcomes','TestTelegramDeliveryConfiguration','TestTelegramDispatch','TestTelegramIngress'}.issubset(observed))
    linux = read(EVIDENCE / 'linux-build.json')
    check('Linux compilation matches current source; artifact not executed or deployed', linux['passed'] and linux['source_sha256'] == backend and not linux['artifact_executed'] and not linux['deployed'])
    w = read(EVIDENCE / 'web-build.json')
    check('Web tests and production build match current source', w['passed'] and w['source_unchanged_during_run'] and w['source_sha256'] == digest(w['sources']))
    mobile = module('account_mobile', 'verify_mobile.py').digest()[0]
    m = read(EVIDENCE / 'mobile-build.json')
    check('Mobile tests and both mini-program builds match current source', m['passed'] and m['source_unchanged_during_run'] and m['source_sha256'] == mobile)
    for target in ('weapp','tt'):
        pages=read(APP/'mobile/dist'/target/'app.json')['pages']
        check(target+' registers exactly 14 public pages including authenticated Telegram receipts',len(pages)==14 and 'pages/telegram/index' in pages)
    runtime = module('account_runtime', 'verify_agent_runtime.py').digest()[0]
    r = read(EVIDENCE / 'runtime-build.json')
    check('Runtime regression and eight draft Skill checks match current source', r['status'] == 'passed' and r['source_sha256'] == runtime == r['source_after_sha256'])
    for group in ('telegram-delivery-web', 'telegram-delivery-mobile'):
        folder = EVIDENCE.parent / group
        result, manifest = read(folder / 'result.json'), read(folder / 'source-manifest.json')
        check(group + ' browser results match current source', result['passed'] and result['source_unchanged'] and result['source_sha256'] == digest(manifest['files']) == manifest['sha256'] and not result['console_errors'] and not result['server_errors'])
        check(group + ' browser, fixture and temporary server closed', result['owned_browser_closed'] and result['browser_contexts_closed'] > 0 and result['isolated_database_cleanup_passed'] and result['native_fixture_exit_code'] == 0 and (result.get('owned_vite_stopped') is True if group.endswith('-web') else result.get('owned_review_server_stopped') is True))
        if group.endswith('-mobile'):
            check('Mobile fixture, H5 build and browser used identical source', result['build_and_browser_sources_match'] and result['source_sha256_before_fixture_and_build'] == result['source_sha256_after_build'] == result['source_sha256'])
    development = read(EVIDENCE / 'development-telegram-delivery.json')
    check('Owned development schema, privileges, routes and fixture cleanup', development['passed'] and development['migration_count'] == 63 and development['database']['disposable_test_databases'] == 0)
    check('Development evidence matches verification script', development['verification_script_sha256'] == hashlib.sha256((APP / 'scripts/verify_telegram_delivery_development.py').read_bytes()).hexdigest())
    check('Documented and embedded OpenAPI copies match', (APP / 'backend/internal/contract/openapi.json').read_bytes() == (APP.parent / 'docs/development/openapi.json').read_bytes())
    check('Private browser fixture removed', not (APP / '.local/business-browser-fixture.json').exists())
    check('Delivery journal is append only and each revision has a deferred exact event', development['database']['journal_append_only'] and development['database']['head_no_delete'] and development['database']['delivery_guards']==3 and development['database']['deferred_event_binding']==1)
    startup = read(EVIDENCE / 'delivery-startup.json')
    check('Actual delivery command rejects disabled configuration without contacting Telegram', startup['passed'] and startup['backend_source_sha256']==backend and startup['owned_process_exited'] and all(hashlib.sha256((APP/name).read_bytes()).hexdigest()==value for name,value in startup['sources'].items()))
    check('Owned development keeps sending, profiles and all Telegram integrations disabled', development['production_integrations_disabled'] and development['all_eight_skills_draft'] and all(development['database'][key]==0 for key in ('enabled_channels','delivery_count','event_count','worker_budget_count','active_profiles')))
    owner = module('account_owned', 'verify_notifications_sources.py').owned_process
    check('Owned API and notification worker identities match recorded start time', owner('telegram-delivery-api-owner.json', 'api') and owner('telegram-delivery-worker-owner.json', 'worker-notification'))
    with socket.socket() as probe:
        probe.settimeout(1)
        check('Temporary review port released', probe.connect_ex(('127.0.0.1', 5189)) != 0)
    for url in ('http://localhost:5178/', 'http://localhost:5178/health/ready', 'http://127.0.0.1:18080/health/ready'):
        try:
            with urlopen(url, timeout=5) as response:
                passed = response.status == 200
        except OSError:
            passed = False
        check(url, passed)
    report = {'passed': all(c['passed'] for c in checks), 'checked_at': datetime.now(timezone.utc).isoformat(), 'checks': checks,
              'backend_source_sha256': backend, 'scope': 'Telegram portal-only lifecycle notifications, durable preparation, shared rate limits, unknown outcomes, MFA reconciliation and Web/Taro request review; native development only, excludes actual models, external delivery, devices, cloud storage and full platform acceptance.'}
    (EVIDENCE / 'final-source-check.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': report['passed'], 'checks': len(checks), 'failed': [c['name'] for c in checks if not c['passed']]}))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
