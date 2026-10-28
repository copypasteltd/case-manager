"""Inspect the current packaged JS feature bytes without a device or emulator."""
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import zipfile

APP = Path(__file__).resolve().parents[1]
E = APP.parent / 'docs/quality/evidence/cost-free-execution'


def main():
    build = json.loads((E / 'build.json').read_text())
    assert build['passed']
    apk = APP / build['artifacts']['apk']['path']
    assert hashlib.sha256(apk.read_bytes()).hexdigest() == build['artifacts']['apk']['sha256']
    with zipfile.ZipFile(apk) as package:
        bundle = package.read('assets/index.android.bundle')
    text = bundle.decode('utf-8').lower()

    def present(value):
        return value in text or json.dumps(value, ensure_ascii=True)[1:-1].lower() in text

    checks = [
        {'name': 'Packaged JS exactly matches the fresh current-source Metro artifact',
         'passed': hashlib.sha256(bundle).hexdigest() == build['artifacts']['bundle']['sha256']},
        {'name': 'Current read-only secretary archive title is packaged', 'passed': present('秘书用量历史')},
        {'name': 'Retired monthly fee edit forms are absent from packaged JS',
         'passed': not any(present(value) for value in ('配置秘书月度Token额度', '配置月度额度', '确认核对'))},
    ]
    result = {'passed': all(r['passed'] for r in checks), 'checked_at': datetime.now(timezone.utc).isoformat(),
        'checks': checks, 'apk_sha256': build['artifacts']['apk']['sha256'],
        'bundle_sha256': hashlib.sha256(bundle).hexdigest(), 'verification_script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'device_executed': False, 'emulator_started': False, 'runtime_rendering_claimed': False}
    (E / 'android-feature-inspection.json').write_text(json.dumps(result, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'passed': result['passed'], 'checks': len(checks), 'device_executed': False}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
