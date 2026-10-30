"""Preserve actual passed client scope and an interrupted APK before test selection repair."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import zipfile

APP = Path(__file__).resolve().parents[1]
ROOT = APP.parent
E = ROOT / 'docs/quality/evidence/contract-body'
OUT = ROOT / 'docs/quality/evidence/contract-body-attempts/before-test-selection'
PRIVATE = APP / '.local/contract-body-attempts/before-test-selection'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    assert not OUT.exists() and not PRIVATE.exists()
    record = json.loads((E / 'client-checks.json').read_text(encoding='utf-8'))
    assert record['passed'] and record['source_unchanged']
    h = hashlib.sha256()
    for name in record['source_files']:
        h.update(name.encode() + b'\0' + (APP / name).read_bytes() + b'\0')
    assert h.hexdigest() == record['source_sha256']
    names = set(record['source_files'])
    for name in ('build_android', 'verify_agent_runtime', 'contract_body_sources'):
        spec = importlib.util.spec_from_file_location(name, APP / 'scripts' / (name + '.py'))
        value = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(value)
        names.update(value.sources()[1] if name == 'build_android' else value.sources())
    normalized = set()
    for name in names:
        path = (APP / name).resolve()
        assert path.is_relative_to(ROOT.resolve()) and path.is_file()
        normalized.add(path.relative_to(ROOT.resolve()).as_posix())
    PRIVATE.mkdir(parents=True)
    source = PRIVATE / 'source.zip'
    with zipfile.ZipFile(source, 'x', zipfile.ZIP_DEFLATED) as output:
        for name in sorted(normalized):
            output.write(ROOT / name, name)
    OUT.mkdir(parents=True)
    public, private = [], []
    for name in ('client-checks.json', 'web-build.json', 'mobile-build.json', 'runtime-build.json', 'build.json'):
        old = E / name
        if old.exists():
            saved = OUT / name
            shutil.move(old, saved)
            public.append({'file': saved.relative_to(ROOT).as_posix(), 'sha256': sha(saved)})
    logs = [APP / '.local' / Path(v['raw_log']).name for v in record['checks']]
    logs += [APP / '.local/contract_body-skill-check.log', APP / '.local/android/bundle.log']
    for original in logs:
        assert original.exists()
        saved = PRIVATE / original.name
        shutil.move(original, saved)
        private.append({'file': saved.relative_to(ROOT).as_posix(), 'sha256': sha(saved)})
    report = {'archived_at': datetime.now(timezone.utc).isoformat(),
        'passed_original_client_checks': True, 'original_mobile_tests': 390,
        'missing_mobile_tests': 9, 'reason': 'Nine new mobile tests were not yet selected by the regular script; APK bundle phase was deliberately interrupted before input correction.',
        'android_build_passed': False, 'counts_as_current_source_pass': False,
        'source_archive': source.relative_to(ROOT).as_posix(), 'source_archive_sha256': sha(source),
        'source_files': sorted(normalized), 'public_records': public, 'private_records': private,
        'archive_script_sha256': sha(Path(__file__))}
    (OUT / 'preservation.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'preserved_source_files': len(normalized), 'passed_current_scope': False}))


if __name__ == '__main__':
    main()
