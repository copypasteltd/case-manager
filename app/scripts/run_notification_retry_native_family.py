"""Re-execute only the unchanged integration family affected by durable native DROP."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import verify_backend_batches as base
from notification_retry_native_cluster import prepare

APP = base.APP
E = APP.parent / 'docs/quality/evidence/notification-retry'
B = E.parent / 'notification-retry-backend'


def main():
    target = E / 'native-family-result.json'
    raw = APP / '.local/notification-retry-backend-native-8.log'
    assert not target.exists() and not raw.exists()
    state = json.loads((B / 'batch-progress.json').read_text())
    source = base.digest()
    assert source == state['source_sha256']
    required = {tuple(v) for v in state['expected_tests'] if v[1] == 'TestPlatformIntegration'}
    assert required
    prepare()
    started = datetime.now(timezone.utc).isoformat()
    with raw.open('w',encoding='utf-8') as output:
        status = subprocess.run([sys.executable,str(APP / 'scripts/test_notification_retry_native.py'),'-json',
            '-run','^TestPlatformIntegration$','-timeout=20m'],cwd=APP,stdout=output,stderr=subprocess.STDOUT).returncode
    checks = []
    for line in raw.read_text(encoding='utf-8').splitlines():
        try:
            v = json.loads(line)
        except json.JSONDecodeError:
            continue
        if v.get('Action') in ('pass','fail','skip') and v.get('Test'):
            checks.append({k:v[k] for k in ('Action','Package','Test','Elapsed') if k in v})
    observed = {(v['Package'],v['Test']) for v in checks if '/' not in v['Test']}
    unchanged = source == base.digest()
    passed = status == 0 and unchanged and observed == required and all(v['Action']=='pass' for v in checks)
    sha = lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
    record = {'passed':passed,'started_at':started,'completed_at':datetime.now(timezone.utc).isoformat(),
        'exit_code':status,'source_sha256':source,'source_unchanged_during_tests':unchanged,'checks':checks,
        'raw_log':raw.relative_to(APP).as_posix(),'raw_log_sha256':sha(raw),
        'native_cluster_record_sha256':sha(E / 'native-cluster.json'),'verification_script_sha256':sha(Path(__file__))}
    target.write_text(json.dumps(record,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'passed':passed,'family':'TestPlatformIntegration','records':len(checks),'durability_preserved':True}))
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
